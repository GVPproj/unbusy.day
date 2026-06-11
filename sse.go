package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/grahamvanpelt/unbusy.day/cards"
	"github.com/grahamvanpelt/unbusy.day/pubsub"
)

// keepaliveInterval is the idle heartbeat cadence: 25s `:keepalive` comments
// defeat intermediary (browser/NAT/Cloudflare) idle closes.
const keepaliveInterval = 25 * time.Second

// eventsHandler is the JSON adapter's live read path: a hardened SSE stream off
// the cards pub/sub. On connect it flushes any Last-Event-ID replay (or an
// overflow sentinel), then streams live mutations as `id: <txid>` + JSON data,
// with a keepalive. One TCP connection per client; runs until the request
// context is cancelled (client disconnect / server drain).
func eventsHandler(b *pubsub.Broker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Type", "text/event-stream")
		h.Set("Cache-Control", "no-cache")
		h.Set("Connection", "keep-alive")
		// Defeat proxy buffering (nginx/Cloudflare) so events are delivered one
		// at a time, not coalesced.
		h.Set("X-Accel-Buffering", "no")

		rc := http.NewResponseController(w)
		// Disable the per-connection write deadline: an SSE stream is long-lived
		// and must not be killed by a server write timeout.
		_ = rc.SetWriteDeadline(time.Time{})

		sub := b.Subscribe(r.Header.Get("Last-Event-ID"))
		defer sub.Close()

		// Backlog first: either the unrecoverable-gap sentinel or the replay.
		if sub.Overflow {
			writeOverflow(w)
		} else {
			for _, e := range sub.Replay {
				if err := writeEvent(w, e); err != nil {
					return
				}
			}
		}
		if err := rc.Flush(); err != nil {
			return
		}

		ticker := time.NewTicker(keepaliveInterval)
		defer ticker.Stop()

		for {
			select {
			case <-r.Context().Done():
				return
			case e := <-sub.Events:
				if err := writeEvent(w, e); err != nil {
					return
				}
				if err := rc.Flush(); err != nil {
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
	}
}

// writeEvent renders one mutation as an SSE frame. The `id:` is the pg txid
// (a decimal string — never a JS Number), which the browser echoes back as
// Last-Event-ID on reconnect. `data:` is the full ordered card list.
func writeEvent(w io.Writer, e cards.Event) error {
	data, err := json.Marshal(e.Cards)
	if err != nil {
		log.Printf("sse marshal: %v", err)
		return err
	}
	_, err = fmt.Fprintf(w, "id: %s\ndata: %s\n\n", e.Txid, data)
	return err
}

// writeOverflow emits the overflow sentinel: the client's Last-Event-ID
// predated the replay ring, so it must drop optimistic state and refetch the
// authoritative order. No `id:` — it isn't a txid frame.
func writeOverflow(w io.Writer) {
	_, _ = io.WriteString(w, "event: overflow\ndata: {\"reason\":\"last-event-id outside replay window; refetch\"}\n\n")
}
