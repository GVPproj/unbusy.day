package frontend

import (
	"io"
	"log"
	"net/http"
	"time"

	"github.com/GVPproj/unbusy.day/internal/frontend/components"
	"github.com/GVPproj/unbusy.day/internal/jot"
	"github.com/GVPproj/unbusy.day/internal/pubsub"
	"github.com/GVPproj/unbusy.day/internal/web"
	"github.com/starfederation/datastar-go/datastar"
)

// keepaliveInterval is the SSE heartbeat cadence, defeating intermediary idle
// closes. A var so tests can shrink it.
var keepaliveInterval = 25 * time.Second

// EventsHandler is the live SSE read path. The first frame is the full current
// column plus the jot snapshot, so a (re)connecting client is made whole by one
// render. Jot state rides as signal patches, never element patches — the client
// applies them to the editor itself (re-rendering under the typist is the thing
// to avoid).
func EventsHandler(svc BlockService, jots JotService, broker *pubsub.Broker) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
