package frontend

import (
	"log"
	"net/http"

	"github.com/starfederation/datastar-go/datastar"
)

const smokeMessage = "patched by datastar"

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

		<-r.Context().Done()
	})
}
