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

// keepaliveInterval is the SSE heartbeat cadence, defeating intermediary idle
// closes. A var so tests can shrink it.
var keepaliveInterval = 25 * time.Second

// BlockService is the frontend's view of the core service; *block.Service
// satisfies it.
type BlockService interface {
	List(ctx context.Context, owner string) ([]block.Block, error)
	Bounds(ctx context.Context, owner string) (block.Bounds, error)
	SetLayout(ctx context.Context, owner string, layout []block.Placement) (*block.LayoutResult, error)
	SetBounds(ctx context.Context, owner string, start, end int) error
	Create(ctx context.Context, owner, label string, slot int, typ block.BlockType) (*block.CreateResult, error)
	Delete(ctx context.Context, owner, id string) (*block.DeleteResult, error)
	Clear(ctx context.Context, owner string) (*block.ClearResult, error)
	Rename(ctx context.Context, owner, id, label string) (*block.RenameResult, error)
}

// snapshot reads the owner's authoritative column and day bounds.
func snapshot(ctx context.Context, svc BlockService, owner string) ([]block.Block, block.Bounds, error) {
	bs, err := svc.List(ctx, owner)
	if err != nil {
		return nil, block.Bounds{}, err
	}
	b, err := svc.Bounds(ctx, owner)
	return bs, b, err
}

// PageHandler serves the column page, server-rendered on every hit (no-cache).
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

// patchColumn patches the authoritative column onto #block-list — the shared
// tail of every mutation handler.
func patchColumn(w http.ResponseWriter, r *http.Request, bs []block.Block, b block.Bounds) {
	sse := datastar.NewSSE(w, r)
	if err := sse.PatchElementTempl(components.BlockColumn(bs, b)); err != nil {
		log.Printf("ds patch column: %v", err)
	}
	patchEnvelope(sse, bs)
}

// patchEnvelope re-patches the occupied-envelope signals so the bounds modal's
// disabled options track the live layout. Best-effort.
func patchEnvelope(sse *datastar.ServerSentEventGenerator, bs []block.Block) {
	if err := sse.MarshalAndPatchSignals(block.OccupiedEnvelope(bs)); err != nil {
		log.Printf("ds patch envelope: %v", err)
	}
}

type layoutSignals struct {
	Layout []block.Placement `json:"layout"`
}

// LayoutHandler submits the whole client-computed layout (ADR 0005) and
// responds with an element-patch of the committed column.
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
			// Domain rejection: 200 + re-render of the authoritative column, so
			// the rejected optimistic gesture visibly snaps back (house convention).
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

type boundsSignals struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

// BoundsHandler edits the owner's day extent and patches the column at the
// committed bounds.
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
			log.Printf("200 rejection bounds: %v", err)
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

type createSignals struct {
	Slot  int    `json:"addslot"`
	Label string `json:"addlabel"`
	Type  string `json:"addtype"`
}

// CreateHandler inserts a new block at the modal's slot and patches the
// committed column.
func CreateHandler(svc BlockService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var sig createSignals
		if err := datastar.ReadSignals(r, &sig); err != nil {
			http.Error(w, "invalid signals body", http.StatusBadRequest)
			return
		}

		owner := ownerFrom(r.Context())
		_, err := svc.Create(r.Context(), owner, sig.Label, sig.Slot, block.BlockType(sig.Type))
		switch {
		case errors.Is(err, block.ErrEmptyLabel), errors.Is(err, block.ErrOutOfBounds),
			errors.Is(err, block.ErrOverlap), errors.Is(err, block.ErrInvalidBlockType):
			log.Printf("200 rejection create: %v", err)
		case err != nil:
			log.Printf("ds create: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		bs, b, err := snapshot(r.Context(), svc, owner)
		if err != nil {
			log.Printf("ds create snapshot: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		patchColumn(w, r, bs, b)
	})
}

type deleteSignals struct {
	ID string `json:"deleteid"`
}

// DeleteHandler removes the clicked block and patches the committed column.
func DeleteHandler(svc BlockService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var sig deleteSignals
		if err := datastar.ReadSignals(r, &sig); err != nil {
			http.Error(w, "invalid signals body", http.StatusBadRequest)
			return
		}

		owner := ownerFrom(r.Context())
		_, err := svc.Delete(r.Context(), owner, sig.ID)
		switch {
		case errors.Is(err, block.ErrBlockNotFound):
			log.Printf("200 rejection delete: %v", err)
		case err != nil:
			log.Printf("ds delete: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		bs, b, err := snapshot(r.Context(), svc, owner)
		if err != nil {
			log.Printf("ds delete snapshot: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		patchColumn(w, r, bs, b)
	})
}

// ClearHandler removes all the owner's blocks; bounds are untouched.
func ClearHandler(svc BlockService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner := ownerFrom(r.Context())
		if _, err := svc.Clear(r.Context(), owner); err != nil {
			log.Printf("ds clear: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		bs, b, err := snapshot(r.Context(), svc, owner)
		if err != nil {
			log.Printf("ds clear snapshot: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		patchColumn(w, r, bs, b)
	})
}

type renameSignals struct {
	ID    string `json:"renameid"`
	Label string `json:"renamelabel"`
}

// RenameHandler updates the edited block's label and patches the committed column.
func RenameHandler(svc BlockService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var sig renameSignals
		if err := datastar.ReadSignals(r, &sig); err != nil {
			http.Error(w, "invalid signals body", http.StatusBadRequest)
			return
		}

		owner := ownerFrom(r.Context())
		_, err := svc.Rename(r.Context(), owner, sig.ID, sig.Label)
		switch {
		case errors.Is(err, block.ErrEmptyLabel), errors.Is(err, block.ErrBlockNotFound):
			log.Printf("200 rejection rename: %v", err)
		case err != nil:
			log.Printf("ds rename: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		bs, b, err := snapshot(r.Context(), svc, owner)
		if err != nil {
			log.Printf("ds rename snapshot: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		patchColumn(w, r, bs, b)
	})
}

// EventsHandler is the live SSE read path. The first frame is the full current
// column, so a (re)connecting client is made whole by one render.
func EventsHandler(svc BlockService, broker *pubsub.Broker) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Accel-Buffering", "no")

		rc := http.NewResponseController(w)
		// SSE is long-lived: no per-connection write deadline.
		_ = rc.SetWriteDeadline(time.Time{})

		owner := ownerFrom(r.Context())

		// Subscribe before the snapshot so a mutation committed in between is
		// waiting on the channel rather than lost; the worst interleaving is
		// one redundant full-state patch.
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
		patchEnvelope(sse, bs)

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
				patchEnvelope(sse, e.Blocks)
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
