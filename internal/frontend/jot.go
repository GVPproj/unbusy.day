package frontend

import (
	"context"
	"errors"
	"log"
	"net/http"

	"github.com/GVPproj/unbusy.day/internal/jot"
	"github.com/starfederation/datastar-go/datastar"
)

// JotService is the frontend's view of the Jotpad service; *jot.Service
// satisfies it.
type JotService interface {
	Get(ctx context.Context, owner string) (string, error)
	Set(ctx context.Context, owner, text string) error
}

// The leading underscore keeps the jot out of every *other* endpoint's POST
// (Datastar's default exclude is /(^_|\._).*/), so up to 100KB never rides
// along on a drag. Fail-safe: a future endpoint cannot leak it.
type jotSignals struct {
	Text string `json:"_jot"`
}

// JotHandler stores the Jotpad and answers 204 with no patch — the DOM is
// already right, and patching a textarea mid-keystroke is the thing to avoid.
// An over-cap write is logged and dropped, the one deliberate exception to the
// house "reject → re-render the authoritative state" rule: snapping prose back
// would destroy text the user typed.
func JotHandler(svc JotService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var sig jotSignals
		if err := datastar.ReadSignals(r, &sig); err != nil {
			http.Error(w, "invalid signals body", http.StatusBadRequest)
			return
		}

		err := svc.Set(r.Context(), ownerFrom(r.Context()), sig.Text)
		switch {
		case errors.Is(err, jot.ErrTooLong):
			log.Printf("204 rejection jot: %v (%d chars)", err, len(sig.Text))
		case err != nil:
			log.Printf("ds jot: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	})
}
