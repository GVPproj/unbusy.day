package frontend

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/GVPproj/unbusy.day/internal/jot"
	"github.com/starfederation/datastar-go/datastar"
)

// JotService is the frontend's view of the Jotpad service; *jot.Service
// satisfies it.
type JotService interface {
	Get(ctx context.Context, owner string) (jot.Pad, error)
	Set(ctx context.Context, owner, text string, base int64) (jot.Pad, error)
}

// The leading underscores keep the jot out of every *other* endpoint's POST
// (Datastar's default exclude is /(^_|\._).*/), so up to 100KB never rides
// along on a drag. Fail-safe: a future endpoint cannot leak it.
type jotSignals struct {
	Text    string `json:"_jot"`
	Version int64  `json:"_jotVersion"`
}

// jotResponse is the write path's answer: the committed version, and the
// committed text only when it differs from what the client sent (a merge or a
// rejection happened) — the client then snaps its editor to it, visibly.
type jotResponse struct {
	Version int64   `json:"version"`
	Text    *string `json:"text,omitempty"`
}

// JotHandler stores the Jotpad via compare-and-swap on _jotVersion (stale
// writes are three-way merged by the service) and answers plain JSON, not an
// element patch — morphing an editor mid-keystroke is the thing to avoid; the
// client applies any returned text itself. An over-cap write keeps its
// log-and-drop behavior but still returns the authoritative state, so the
// client stops believing an over-cap doc is saved.
func JotHandler(svc JotService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var sig jotSignals
		if err := datastar.ReadSignals(r, &sig); err != nil {
			http.Error(w, "invalid signals body", http.StatusBadRequest)
			return
		}

		pad, err := svc.Set(r.Context(), ownerFrom(r.Context()), sig.Text, sig.Version)
		switch {
		case errors.Is(err, jot.ErrTooLong):
			log.Printf("200 rejection jot: %v (%d chars)", err, len(sig.Text))
		case err != nil:
			log.Printf("ds jot: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		resp := jotResponse{Version: pad.Version}
		if pad.Text != sig.Text {
			resp.Text = &pad.Text
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			log.Printf("ds jot respond: %v", err)
		}
	})
}

// jotSignalPatch is the shape both the SSE snapshot and live jot events patch:
// underscore-prefixed so Datastar never posts the pad back on other endpoints,
// applied by the client via data-on-signal-patch — never an element patch.
func jotSignalPatch(p jot.Pad) map[string]any {
	return map[string]any{"_jotv": p.Version, "_jott": p.Text}
}
