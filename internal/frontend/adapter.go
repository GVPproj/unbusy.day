package frontend

import (
	"context"
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
	SetLayout(ctx context.Context, owner string, layout []block.Placement) (*block.Snapshot, error)
	SetBounds(ctx context.Context, owner string, start, end block.Slot) (*block.Snapshot, error)
	Create(ctx context.Context, owner, label string, slot block.Slot, typ block.BlockType) (*block.Snapshot, error)
	Delete(ctx context.Context, owner, id string) (*block.Snapshot, error)
	Clear(ctx context.Context, owner string) (*block.Snapshot, error)
	Rename(ctx context.Context, owner, id, label string) (*block.Snapshot, error)
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
// The page load is the Jotpad's only read: SSE carries blocks, and nothing
// re-renders a textarea the user may be typing into.
func PageHandler(svc BlockService, jots JotService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner := ownerFrom(r.Context())
		bs, b, err := snapshot(r.Context(), svc, owner)
		if err != nil {
			log.Printf("ds page list: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		jotText, err := jots.Get(r.Context(), owner)
		if err != nil {
			log.Printf("ds page jot: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		if err := routes.BlocksPage(bs, b, jotText).Render(r.Context(), w); err != nil {
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

// mutate is the one mutation handler: read signals, pull the owner, call
// apply, classify the error with block.IsRejection, and patch the committed
// column. A rejection re-reads the authoritative state and still responds 200,
// so the rejected optimistic change visibly snaps back (house convention);
// anything else is a 500. The success path needs no re-read — apply's
// Snapshot already carries the committed blocks and bounds.
func mutate[S any](
	svc BlockService,
	apply func(ctx context.Context, owner string, sig S) (*block.Snapshot, error),
) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var sig S
		if err := datastar.ReadSignals(r, &sig); err != nil {
			http.Error(w, "invalid signals body", http.StatusBadRequest)
			return
		}

		owner := ownerFrom(r.Context())
		snap, err := apply(r.Context(), owner, sig)
		switch {
		case block.IsRejection(err):
			log.Printf("200 rejection %s: %v", r.URL.Path, err)
			bs, b, rerr := snapshot(r.Context(), svc, owner)
			if rerr != nil {
				log.Printf("ds rejection re-read %s: %v", r.URL.Path, rerr)
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			snap = &block.Snapshot{Blocks: bs, Bounds: b}
		case err != nil:
			log.Printf("ds mutate %s: %v", r.URL.Path, err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		patchColumn(w, r, snap.Blocks, snap.Bounds)
	})
}

type layoutSignals struct {
	Layout []block.Placement `json:"layout"`
}

// LayoutHandler submits the whole client-computed layout (ADR 0005).
func LayoutHandler(svc BlockService) http.Handler {
	return mutate(svc, func(ctx context.Context, owner string, sig layoutSignals) (*block.Snapshot, error) {
		return svc.SetLayout(ctx, owner, sig.Layout)
	})
}

type boundsSignals struct {
	Start block.Slot `json:"start"`
	End   block.Slot `json:"end"`
}

// BoundsHandler edits the owner's day extent.
func BoundsHandler(svc BlockService) http.Handler {
	return mutate(svc, func(ctx context.Context, owner string, sig boundsSignals) (*block.Snapshot, error) {
		return svc.SetBounds(ctx, owner, sig.Start, sig.End)
	})
}

type createSignals struct {
	Slot  block.Slot `json:"addslot"`
	Label string     `json:"addlabel"`
	Type  string     `json:"addtype"`
}

// CreateHandler inserts a new block at the modal's slot.
func CreateHandler(svc BlockService) http.Handler {
	return mutate(svc, func(ctx context.Context, owner string, sig createSignals) (*block.Snapshot, error) {
		return svc.Create(ctx, owner, sig.Label, sig.Slot, block.BlockType(sig.Type))
	})
}

type deleteSignals struct {
	ID string `json:"deleteid"`
}

// DeleteHandler removes the clicked block.
func DeleteHandler(svc BlockService) http.Handler {
	return mutate(svc, func(ctx context.Context, owner string, sig deleteSignals) (*block.Snapshot, error) {
		return svc.Delete(ctx, owner, sig.ID)
	})
}

// ClearHandler removes all the owner's blocks; bounds are untouched. struct{}
// signals: Datastar always posts a signals body, so the decode succeeds.
func ClearHandler(svc BlockService) http.Handler {
	return mutate(svc, func(ctx context.Context, owner string, _ struct{}) (*block.Snapshot, error) {
		return svc.Clear(ctx, owner)
	})
}

type renameSignals struct {
	ID    string `json:"renameid"`
	Label string `json:"renamelabel"`
}

// RenameHandler updates the edited block's label.
func RenameHandler(svc BlockService) http.Handler {
	return mutate(svc, func(ctx context.Context, owner string, sig renameSignals) (*block.Snapshot, error) {
		return svc.Rename(ctx, owner, sig.ID, sig.Label)
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
