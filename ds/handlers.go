package ds

import (
	"log"
	"net/http"

	"github.com/starfederation/datastar-go/datastar"
)

// smokeMessage is the body of the one patch /ds/_smoke/events ships on connect.
// Constant so the wire test can rely on the message landing verbatim and so the
// browser smoke (V1) has something obviously-from-the-server to look at.
const smokeMessage = "patched by datastar"

// SmokeHandler renders the M2.5a smoke page (PRD F16). One templ render to the
// response writer — the page is static; the patch arrives over SSE on load via
// the data-on-load attribute baked into SmokePage.
func SmokeHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		// Adapter B's HTML shell is origin-rendered (PRD §5 "edge topology"
		// trade) — Cloudflare won't cache this, so no-cache keeps the entry
		// document honest while still letting the Datastar bundle edge-cache.
		w.Header().Set("Cache-Control", "no-cache")
		if err := SmokePage().Render(r.Context(), w); err != nil {
			http.Error(w, "render smoke page", http.StatusInternalServerError)
		}
	})
}

// SmokeEventsHandler is the wire side of the M2.5a smoke (PRD F16). On connect
// it ships one element-patch frame whose body is a templ-rendered #smoke-target,
// then sits on the stream until the client disconnects. No subscriber, no
// pub/sub bridge yet — that's M2.5b. The point here is to prove that the
// pinned Datastar SDK + templ versions produce a wire frame the browser will
// actually apply (verified live in V1 / browser smoke).
//
// Adapter B's reconnect contract (PRD F13) is "server renders the current
// authoritative fragment on connect" — no ring buffer, no Last-Event-ID
// replay. The one-frame-on-connect shape here is the seed of that contract.
func SmokeEventsHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Datastar.NewSSE sets text/event-stream + Cache-Control: no-cache,
		// but does not know about Cloudflare's response buffering. Set the
		// PRD F2 / D3 hardening header before NewSSE flushes — the D3 Cache
		// Rule applies to /events suffix, so this is belt-and-braces.
		w.Header().Set("X-Accel-Buffering", "no")

		sse := datastar.NewSSE(w, r)
		if err := sse.PatchElementTempl(SmokeFragment(smokeMessage)); err != nil {
			log.Printf("ds smoke patch: %v", err)
			return
		}

		// Park the connection until the client disconnects (or server drains).
		// SSE is long-lived; under the eventual FE2 contract this is where a
		// pub/sub subscriber loop would live — for the M2.5a smoke, idle is
		// enough. Keepalive cadence is a M2.5b concern (F13 reuses F2's 25s).
		<-r.Context().Done()
	})
}
