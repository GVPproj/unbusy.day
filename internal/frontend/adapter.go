// Package ds is the Datastar + templ frontend: the server-rendered route tree
// (page, events stream, reorder) driving the blocks service and pub/sub.
// /_smoke is a wiring canary for the pinned Datastar SDK + templ versions.
package frontend

import (
	"context"
	"errors"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/GVPproj/unbusy.day/internal/block"
	"github.com/GVPproj/unbusy.day/internal/frontend/components"
	"github.com/GVPproj/unbusy.day/internal/frontend/routes"
	"github.com/GVPproj/unbusy.day/internal/pubsub"
	"github.com/starfederation/datastar-go/datastar"
)

// keepaliveInterval is the SSE idle-heartbeat cadence: 25s `:keepalive`
// comments defeat intermediary idle closes. A var so tests can shrink it.
var keepaliveInterval = 25 * time.Second

// BlockService is the frontend's view of the core blocks service;
// *block.Service satisfies it. The seam keeps the handlers testable without
// Postgres.
type BlockService interface {
	List(ctx context.Context, owner string) ([]block.Block, error)
	Bounds(ctx context.Context, owner string) (block.Bounds, error)
	SetLayout(ctx context.Context, owner string, layout []block.Placement) (*block.LayoutResult, error)
	SetBounds(ctx context.Context, owner string, start, end int) error
}

// snapshot reads the owner's authoritative column and day bounds — the pair
// every render needs.
func snapshot(ctx context.Context, svc BlockService, owner string) ([]block.Block, block.Bounds, error) {
	bs, err := svc.List(ctx, owner)
	if err != nil {
		return nil, block.Bounds{}, err
	}
	b, err := svc.Bounds(ctx, owner)
	return bs, b, err
}

// PageHandler serves the column page, server-rendered from the core service's
// authoritative order. Origin-rendered on every hit — no-cache keeps the entry
// document honest while the Datastar/Motion bundles edge-cache on their CDNs.
func PageHandler(svc BlockService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bs, b, err := snapshot(r.Context(), svc, ownerFrom(r.Context()))
		if err != nil {
			log.Printf("ds page list: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		if err := routes.BlocksPage(bs, b).Render(r.Context(), w); err != nil {
			http.Error(w, "render page", http.StatusInternalServerError)
		}
	})
}

// patchColumn opens an SSE response and patches the authoritative column onto
// #block-list — the shared tail of the mutation handlers, where the committed
// (or rolled-back) truth is re-asserted over the client's optimistic gesture.
func patchColumn(w http.ResponseWriter, r *http.Request, bs []block.Block, b block.Bounds) {
	sse := datastar.NewSSE(w, r)
	if err := sse.PatchElementTempl(components.BlockColumn(bs, b)); err != nil {
		log.Printf("ds patch column: %v", err)
	}
}

// layoutSignals is the Datastar signals body the drag/resize gestures @post:
// the full proposed layout after the client-computed push (ADR 0005).
type layoutSignals struct {
	Layout []block.Placement `json:"layout"`
}

// LayoutHandler is the one mutation endpoint for the Day Plan: it submits the
// whole client-computed layout to the core and responds with an SSE
// element-patch of the committed column.
func LayoutHandler(svc BlockService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var sig layoutSignals
		if err := datastar.ReadSignals(r, &sig); err != nil {
			http.Error(w, "invalid signals body", http.StatusBadRequest)
			return
		}

		owner := ownerFrom(r.Context())
		res, err := svc.SetLayout(r.Context(), owner, sig.Layout)
		var bs []block.Block
		switch {
		case errors.Is(err, block.ErrNotSameBlocks), errors.Is(err, block.ErrOutOfBounds),
			errors.Is(err, block.ErrOverlap), errors.Is(err, block.ErrInvalidSpan):
			// Rollback at 200: patch back the authoritative column so the
			// rejected gesture visibly snaps back (house convention).
			bs, err = svc.List(r.Context(), owner)
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
			bs = res.Blocks
		}

		b, err := svc.Bounds(r.Context(), owner)
		if err != nil {
			log.Printf("ds layout bounds: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		patchColumn(w, r, bs, b)
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
func BoundsHandler(svc BlockService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var sig boundsSignals
		if err := datastar.ReadSignals(r, &sig); err != nil {
			http.Error(w, "invalid signals body", http.StatusBadRequest)
			return
		}

		owner := ownerFrom(r.Context())
		err := svc.SetBounds(r.Context(), owner, sig.Start, sig.End)
		switch {
		case errors.Is(err, block.ErrInvalidBounds), errors.Is(err, block.ErrBoundsOccupied):
			// Rejection at 200: the snapshot below re-renders the current
			// extent, so the plan is re-shown unchanged (house convention).
		case err != nil:
			log.Printf("ds bounds: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		bs, b, err := snapshot(r.Context(), svc, owner)
		if err != nil {
			log.Printf("ds bounds snapshot: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		patchColumn(w, r, bs, b)
	})
}

// EventsHandler is the live read path: the blocks pub/sub rendered as templ
// element-patches. The first frame is the current authoritative column, so a
// (re)connecting client is made whole by one render — no ring buffer, no
// Last-Event-ID.
func EventsHandler(svc BlockService, broker *pubsub.Broker) http.Handler {
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

		bs, b, err := snapshot(r.Context(), svc, owner)
		if err != nil {
			log.Printf("ds events list: %v", err)
			return
		}
		if err := sse.PatchElementTempl(components.BlockColumn(bs, b)); err != nil {
			return
		}

		ticker := time.NewTicker(keepaliveInterval)
		defer ticker.Stop()
		for {
			select {
			case <-r.Context().Done():
				return
			case e := <-sub.Events:
				if err := sse.PatchElementTempl(components.BlockColumn(e.Blocks, e.Bounds)); err != nil {
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
