// Package ds is the Datastar + templ frontend: the server-rendered route tree
// (page, events stream, reorder) driving the cards service and pub/sub.
// /_smoke is a wiring canary for the pinned Datastar SDK + templ versions.
package frontend

import (
	"context"
	"errors"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/GVPproj/unbusy.day/cards"
	"github.com/GVPproj/unbusy.day/frontend/components"
	"github.com/GVPproj/unbusy.day/frontend/routes"
	"github.com/GVPproj/unbusy.day/pubsub"
	"github.com/starfederation/datastar-go/datastar"
)

// keepaliveInterval is the SSE idle-heartbeat cadence: 25s `:keepalive`
// comments defeat intermediary idle closes. A var so tests can shrink it.
var keepaliveInterval = 25 * time.Second

// CardService is the frontend's view of the core cards service;
// *cards.Service satisfies it. The seam keeps the handlers testable without
// Postgres.
type CardService interface {
	List(ctx context.Context, owner string) ([]cards.Card, error)
	Bounds(ctx context.Context, owner string) (cards.Bounds, error)
	SetLayout(ctx context.Context, owner string, layout []cards.Placement) (*cards.LayoutResult, error)
	SetBounds(ctx context.Context, owner string, start, end int) error
	Reorder(ctx context.Context, owner string, order []string) (*cards.ReorderResult, error)
	Resize(ctx context.Context, owner, id string, span int) (*cards.ResizeResult, error)
}

// snapshot reads the owner's authoritative column and day bounds — the pair
// every render needs.
func snapshot(ctx context.Context, svc CardService, owner string) ([]cards.Card, cards.Bounds, error) {
	cs, err := svc.List(ctx, owner)
	if err != nil {
		return nil, cards.Bounds{}, err
	}
	b, err := svc.Bounds(ctx, owner)
	return cs, b, err
}

// PageHandler serves the column page, server-rendered from the core service's
// authoritative order. Origin-rendered on every hit — no-cache keeps the entry
// document honest while the Datastar/Motion bundles edge-cache on their CDNs.
func PageHandler(svc CardService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cs, b, err := snapshot(r.Context(), svc, ownerFrom(r.Context()))
		if err != nil {
			log.Printf("ds page list: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		if err := routes.CardsPage(cs, b).Render(r.Context(), w); err != nil {
			http.Error(w, "render page", http.StatusInternalServerError)
		}
	})
}

// layoutSignals is the Datastar signals body the drag/resize gestures @post:
// the full proposed layout after the client-computed push (ADR 0005).
type layoutSignals struct {
	Layout []cards.Placement `json:"layout"`
}

// LayoutHandler is the one mutation endpoint for the Day Plan: it submits the
// whole client-computed layout to the core and responds with an SSE
// element-patch of the committed column.
func LayoutHandler(svc CardService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var sig layoutSignals
		if err := datastar.ReadSignals(r, &sig); err != nil {
			http.Error(w, "invalid signals body", http.StatusBadRequest)
			return
		}

		owner := ownerFrom(r.Context())
		res, err := svc.SetLayout(r.Context(), owner, sig.Layout)
		var cs []cards.Card
		switch {
		case errors.Is(err, cards.ErrNotSameCards), errors.Is(err, cards.ErrOutOfBounds),
			errors.Is(err, cards.ErrOverlap), errors.Is(err, cards.ErrInvalidSpan):
			// Rollback at 200: patch back the authoritative column so the
			// rejected gesture visibly snaps back (house convention).
			cs, err = svc.List(r.Context(), owner)
			if err != nil {
				log.Printf("ds layout rollback list: %v", err)
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
		case err != nil:
			log.Printf("ds layout: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		default:
			cs = res.Cards
		}

		b, err := svc.Bounds(r.Context(), owner)
		if err != nil {
			log.Printf("ds layout bounds: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		sse := datastar.NewSSE(w, r)
		if err := sse.PatchElementTempl(components.CardColumn(cs, b)); err != nil {
			log.Printf("ds layout patch: %v", err)
		}
	})
}

// boundsSignals is the Datastar signals body the bounds-settings UI @posts:
// the day's new extent as slot indexes from 00:00.
type boundsSignals struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

// BoundsHandler edits the owner's day extent and responds with an
// element-patch of the column rendered at the (possibly unchanged) committed
// bounds — success resizes the grid, rejection re-asserts the current plan.
func BoundsHandler(svc CardService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var sig boundsSignals
		if err := datastar.ReadSignals(r, &sig); err != nil {
			http.Error(w, "invalid signals body", http.StatusBadRequest)
			return
		}

		owner := ownerFrom(r.Context())
		err := svc.SetBounds(r.Context(), owner, sig.Start, sig.End)
		switch {
		case errors.Is(err, cards.ErrInvalidBounds), errors.Is(err, cards.ErrBoundsOccupied):
			// Rejection at 200: the snapshot below re-renders the current
			// extent, so the plan is re-shown unchanged (house convention).
		case err != nil:
			log.Printf("ds bounds: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		cs, b, err := snapshot(r.Context(), svc, owner)
		if err != nil {
			log.Printf("ds bounds snapshot: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		sse := datastar.NewSSE(w, r)
		if err := sse.PatchElementTempl(components.CardColumn(cs, b)); err != nil {
			log.Printf("ds bounds patch: %v", err)
		}
	})
}

// reorderSignals is the Datastar signals body @post ships: the $order signal
// set from the data-id order DragInit commits on drop.
type reorderSignals struct {
	Order []string `json:"order"`
}

// ReorderHandler is the mutation endpoint. The response is an SSE
// element-patch of the committed column, so the dragging client settles on
// the server's order in the same round-trip.
func ReorderHandler(svc CardService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var sig reorderSignals
		if err := datastar.ReadSignals(r, &sig); err != nil {
			http.Error(w, "invalid signals body", http.StatusBadRequest)
			return
		}

		owner := ownerFrom(r.Context())
		res, err := svc.Reorder(r.Context(), owner, sig.Order)
		var cs []cards.Card
		switch {
		case errors.Is(err, cards.ErrNotPermutation):
			// Rollback: patch back the authoritative column so the rejected
			// drop visibly snaps back. 200 + hypermedia truth, not 4xx —
			// Datastar applying patches on error statuses is unverified, and
			// there is no client rollback code to signal anyway.
			cs, err = svc.List(r.Context(), owner)
			if err != nil {
				log.Printf("ds reorder rollback list: %v", err)
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
		case err != nil:
			log.Printf("ds reorder: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		default:
			cs = res.Cards
		}

		b, err := svc.Bounds(r.Context(), owner)
		if err != nil {
			log.Printf("ds reorder bounds: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		sse := datastar.NewSSE(w, r)
		if err := sse.PatchElementTempl(components.CardColumn(cs, b)); err != nil {
			log.Printf("ds reorder patch: %v", err)
		}
	})
}

// resizeSignals is the Datastar signals body the grip-resize gesture @posts:
// the card's id and its new span in slots.
type resizeSignals struct {
	ID   string `json:"id"`
	Span int    `json:"span"`
}

// ResizeHandler persists a card's height and, like ReorderHandler, responds
// with an SSE element-patch of the committed column.
func ResizeHandler(svc CardService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var sig resizeSignals
		if err := datastar.ReadSignals(r, &sig); err != nil {
			http.Error(w, "invalid signals body", http.StatusBadRequest)
			return
		}

		owner := ownerFrom(r.Context())
		res, err := svc.Resize(r.Context(), owner, sig.ID, sig.Span)
		var cs []cards.Card
		switch {
		case errors.Is(err, cards.ErrInvalidSpan):
			// Rollback at 200: patch back the authoritative column so the
			// over-shrunk card snaps back. Same path as a rejected reorder.
			cs, err = svc.List(r.Context(), owner)
			if err != nil {
				log.Printf("ds resize rollback list: %v", err)
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
		case err != nil:
			log.Printf("ds resize: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		default:
			cs = res.Cards
		}

		b, err := svc.Bounds(r.Context(), owner)
		if err != nil {
			log.Printf("ds resize bounds: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		sse := datastar.NewSSE(w, r)
		if err := sse.PatchElementTempl(components.CardColumn(cs, b)); err != nil {
			log.Printf("ds resize patch: %v", err)
		}
	})
}

// EventsHandler is the live read path: the cards pub/sub rendered as templ
// element-patches. The first frame is the current authoritative column, so a
// (re)connecting client is made whole by one render — no ring buffer, no
// Last-Event-ID.
func EventsHandler(svc CardService, broker *pubsub.Broker) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// NewSSE sets text/event-stream + no-cache; the buffering header is ours.
		w.Header().Set("X-Accel-Buffering", "no")

		rc := http.NewResponseController(w)
		// SSE is long-lived: no per-connection write deadline.
		_ = rc.SetWriteDeadline(time.Time{})

		owner := ownerFrom(r.Context())

		// Subscribe before the snapshot so a mutation committed in between is
		// waiting on the channel rather than lost. Frames are full-state
		// renders, so the worst interleaving is one redundant patch — never a
		// silently missed order. The subscription is owner-keyed: only this
		// user's mutations wake this connection.
		sub := broker.Subscribe(owner)
		defer sub.Close()

		sse := datastar.NewSSE(w, r)

		cs, b, err := snapshot(r.Context(), svc, owner)
		if err != nil {
			log.Printf("ds events list: %v", err)
			return
		}
		if err := sse.PatchElementTempl(components.CardColumn(cs, b)); err != nil {
			return
		}

		ticker := time.NewTicker(keepaliveInterval)
		defer ticker.Stop()
		for {
			select {
			case <-r.Context().Done():
				return
			case e := <-sub.Events:
				if err := sse.PatchElementTempl(components.CardColumn(e.Cards, e.Bounds)); err != nil {
					return
				}
			case <-ticker.C:
				if _, err := io.WriteString(w, ":keepalive\n\n"); err != nil {
					return
				}
				if err := rc.Flush(); err != nil {
					return
				}
			}
		}
	})
}
