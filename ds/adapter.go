// Package ds is the Datastar + templ adapter: the /ds/* route tree mounted
// alongside the JSON adapter (/api/*), driving the same cards service and
// pub/sub. /ds/_smoke is a wiring canary for the pinned Datastar SDK + templ
// versions.
package ds

import (
	"context"
	"errors"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/grahamvanpelt/unbusy.day/cards"
	"github.com/grahamvanpelt/unbusy.day/ds/components"
	"github.com/grahamvanpelt/unbusy.day/pubsub"
	"github.com/starfederation/datastar-go/datastar"
)

// keepaliveInterval mirrors the JSON adapter's hardening: 25s `:keepalive`
// comments defeat intermediary idle closes. A var so tests can shrink it.
var keepaliveInterval = 25 * time.Second

// CardService is the Datastar adapter's view of the core cards service;
// *cards.Service satisfies it. The seam keeps the adapter testable without
// Postgres.
type CardService interface {
	List(ctx context.Context) ([]cards.Card, error)
	Reorder(ctx context.Context, order []string) (*cards.ReorderResult, error)
}

// PageHandler serves the column page, server-rendered from the core service's
// authoritative order. Origin-rendered on every hit — no-cache keeps the entry
// document honest while the Datastar/Motion bundles edge-cache on their CDNs.
func PageHandler(svc CardService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cs, err := svc.List(r.Context())
		if err != nil {
			log.Printf("ds page list: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		if err := components.CardsPage(cs).Render(r.Context(), w); err != nil {
			http.Error(w, "render page", http.StatusInternalServerError)
		}
	})
}

// reorderSignals is the Datastar signals body @post ships: the $order signal
// set from the data-id order dragInit commits on drop.
type reorderSignals struct {
	Order []string `json:"order"`
}

// ReorderHandler is the Datastar adapter's mutation endpoint. Same core
// mutation as the JSON adapter; only serialization differs — the response is
// an SSE element-patch of the committed column rather than {cards, txid} JSON,
// so the dragging client settles on the server's order in the same round-trip.
func ReorderHandler(svc CardService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var sig reorderSignals
		if err := datastar.ReadSignals(r, &sig); err != nil {
			http.Error(w, "invalid signals body", http.StatusBadRequest)
			return
		}

		res, err := svc.Reorder(r.Context(), sig.Order)
		var cs []cards.Card
		switch {
		case errors.Is(err, cards.ErrNotPermutation):
			// Rollback: patch back the authoritative column so the rejected
			// drop visibly snaps back. 200 + hypermedia truth, not 4xx —
			// Datastar applying patches on error statuses is unverified, and
			// there is no client rollback code to signal anyway.
			cs, err = svc.List(r.Context())
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

		sse := datastar.NewSSE(w, r)
		if err := sse.PatchElementTempl(components.CardColumn(cs)); err != nil {
			log.Printf("ds reorder patch: %v", err)
		}
	})
}

// EventsHandler is the Datastar adapter's live read path: same cards topic as
// /api/events, but each event renders as a templ element-patch instead of
// JSON. Reconnect differs by design: the first frame is the current
// authoritative column, so a (re)connecting client is made whole by one
// render — no ring buffer, no Last-Event-ID.
func EventsHandler(svc CardService, broker *pubsub.Broker) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// NewSSE sets text/event-stream + no-cache; the buffering header is ours.
		w.Header().Set("X-Accel-Buffering", "no")

		rc := http.NewResponseController(w)
		// SSE is long-lived: no per-connection write deadline.
		_ = rc.SetWriteDeadline(time.Time{})

		// Subscribe before the snapshot so a mutation committed in between is
		// waiting on the channel rather than lost. Frames are full-state
		// renders, so the worst interleaving is one redundant patch — never a
		// silently missed order.
		sub := broker.Subscribe("")
		defer sub.Close()

		sse := datastar.NewSSE(w, r)

		cs, err := svc.List(r.Context())
		if err != nil {
			log.Printf("ds events list: %v", err)
			return
		}
		if err := sse.PatchElementTempl(components.CardColumn(cs)); err != nil {
			return
		}

		ticker := time.NewTicker(keepaliveInterval)
		defer ticker.Stop()
		for {
			select {
			case <-r.Context().Done():
				return
			case e := <-sub.Events:
				if err := sse.PatchElementTempl(components.CardColumn(e.Cards)); err != nil {
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
