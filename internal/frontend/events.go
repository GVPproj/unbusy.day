package frontend

import (
	"io"
	"log"
	"net/http"
	"time"

	"github.com/GVPproj/unbusy.day/internal/frontend/components"
	"github.com/GVPproj/unbusy.day/internal/habit"
	"github.com/GVPproj/unbusy.day/internal/jot"
	"github.com/GVPproj/unbusy.day/internal/pubsub"
	"github.com/GVPproj/unbusy.day/internal/web"
	"github.com/starfederation/datastar-go/datastar"
)

// keepaliveInterval is the SSE heartbeat cadence, defeating intermediary idle
// closes. A var so tests can shrink it.
var keepaliveInterval = 25 * time.Second

// EventsHandler reconnects with full plan, Jotpad and habit snapshots.
// Jotpad state rides as signals; element patches never touch its editor.
func EventsHandler(svc BlockService, jots JotService, broker *pubsub.Broker, habits HabitService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var sig habitSignals
		if err := datastar.ReadSignals(r, &sig); err != nil {
			http.Error(w, "invalid signals", http.StatusBadRequest)
			return
		}
		month, err := habit.Calendar(sig.Timezone, time.Now())
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("X-Accel-Buffering", "no")

		rc := http.NewResponseController(w)
		// SSE is long-lived: no per-connection write deadline.
		_ = rc.SetWriteDeadline(time.Time{})

		owner := web.OwnerFrom(r.Context())

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
		pad, err := jots.Get(r.Context(), owner)
		if err != nil {
			log.Printf("ds events jot: %v", err)
			return
		}
		if err := sse.MarshalAndPatchSignals(jotSignalPatch(pad)); err != nil {
			return
		}

		if err := patchHabits(sse, r, habits, sig.Timezone); err != nil {
			log.Printf("events habits: %v", err)
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
				patchEnvelope(sse, e.Blocks)
			case je := <-sub.Jots:
				if err := sse.MarshalAndPatchSignals(jotSignalPatch(jot.Pad{Text: je.Text, Version: je.Version})); err != nil {
					return
				}
			case <-sub.Habits:
				if err := patchHabits(sse, r, habits, sig.Timezone); err != nil {
					log.Printf("events habits: %v", err)
					return
				}
			case <-ticker.C:
				// An open tab follows local midnight too, without touching form drafts.
				current, err := habit.Calendar(sig.Timezone, time.Now())
				if err != nil {
					return
				}
				if current.Today != month.Today {
					if err := patchHabits(sse, r, habits, sig.Timezone); err != nil {
						log.Printf("events habits: %v", err)
						return
					}
					month = current
				}
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
