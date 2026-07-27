package frontend

import (
	"log"
	"net/http"

	"github.com/starfederation/datastar-go/datastar"
)

const smokeMessage = "patched by datastar"

// The Jotpad's underscore-signal canary. Underscore-prefixed signals are the
// one class Datastar strips from backend requests by default, so the Jotpad
// depends on two behaviours that nothing else here exercises: the server can
// patch one, and an explicit filterSignals pair can still transmit it.
const (
	smokeSignalName  = "_smokesig"
	smokeSignalValue = "underscore signal stored"
)

type smokeSignals struct {
	Sig string `json:"_smokesig"`
}

// SmokeHandler renders the static smoke page; the patch arrives over SSE on load.
func SmokeHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		if err := SmokePage().Render(r.Context(), w); err != nil {
			http.Error(w, "render smoke page", http.StatusInternalServerError)
		}
	})
}

// SmokeEventsHandler ships one element-patch frame on connect, proving the
// pinned Datastar SDK + templ versions produce a frame the browser applies.
func SmokeEventsHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Accel-Buffering", "no")

		sse := datastar.NewSSE(w, r)
		if err := sse.PatchElementTempl(SmokeFragment(smokeMessage)); err != nil {
			log.Printf("ds smoke patch: %v", err)
			return
		}
		if err := sse.MarshalAndPatchSignals(smokeSignals{Sig: smokeSignalValue}); err != nil {
			log.Printf("ds smoke signal patch: %v", err)
			return
		}

		<-r.Context().Done()
	})
}

// SmokeEchoHandler completes the underscore-signal canary: it patches back
// whatever $_smokesig the request carried, so a page showing the stream's own
// value proves the client both stored and transmitted it.
func SmokeEchoHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var sig smokeSignals
		if err := datastar.ReadSignals(r, &sig); err != nil {
			http.Error(w, "invalid signals body", http.StatusBadRequest)
			return
		}

		sse := datastar.NewSSE(w, r)
		if err := sse.PatchElementTempl(SmokeEchoFragment(sig.Sig)); err != nil {
			log.Printf("ds smoke echo: %v", err)
		}
	})
}
