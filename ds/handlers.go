package ds

import (
	"log"
	"net/http"

	"github.com/starfederation/datastar-go/datastar"
)

// smokeMessage is the body of the one patch /ds/_smoke/events ships on connect.
// Constant so the wire test can rely on the message landing verbatim.
const smokeMessage = "patched by datastar"

// SmokeHandler renders the static smoke page; the patch arrives over SSE on
// load via the data-init attribute baked into SmokePage.
func SmokeHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		// Origin-rendered shell — no-cache keeps the entry document honest
		// while the Datastar bundle still edge-caches on its CDN URL.
		w.Header().Set("Cache-Control", "no-cache")
		if err := SmokePage().Render(r.Context(), w); err != nil {
			http.Error(w, "render smoke page", http.StatusInternalServerError)
		}
	})
}

// SmokeEventsHandler is the wire side of the smoke: on connect it ships one
// element-patch frame whose body is a templ-rendered #smoke-target, then sits
// on the stream until the client disconnects. Proves the pinned Datastar SDK +
// templ versions produce a wire frame the browser actually applies.
func SmokeEventsHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// NewSSE sets text/event-stream + no-cache but doesn't know about
		// proxy buffering; set the hardening header before NewSSE flushes.
		w.Header().Set("X-Accel-Buffering", "no")

		sse := datastar.NewSSE(w, r)
		if err := sse.PatchElementTempl(SmokeFragment(smokeMessage)); err != nil {
			log.Printf("ds smoke patch: %v", err)
			return
		}

		// Park the connection until the client disconnects (or server drains).
		// The real adapter runs a subscriber loop here; the smoke just idles.
		<-r.Context().Done()
	})
}
