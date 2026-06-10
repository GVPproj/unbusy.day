// Package ds is Adapter B (PRD §5): the Datastar + templ route tree mounted
// alongside Adapter A (/api/*). M2.5a is the pin-and-verify spike whose only
// product is the /ds/_smoke page + /ds/_smoke/events stream that prove the
// Datastar 1.0+ element-patch wiring works end-to-end.
package ds

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestSmokeHandlerRendersTargetAndSSEReference is the M2.5a tracer bullet for
// the page side of the smoke. The handler must render an HTML document with:
//   - a stable #smoke-target element (Datastar's default outer-morph mode
//     patches by id; without a matching id the SDK is a silent no-op — F16);
//   - a reference to /ds/_smoke/events somewhere on the page so the browser
//     opens the SSE stream on load.
//
// The test does NOT pin the exact Datastar attribute syntax (e.g. data-on-load
// vs data-signals-…) — that's V1's job to verify in a real browser; locking it
// here would couple us to RC-era naming the PRD §10 explicitly distrusts.
func TestSmokeHandlerRendersTargetAndSSEReference(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/ds/_smoke", nil)
	rec := httptest.NewRecorder()

	SmokeHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("content-type: want text/html prefix, got %q", ct)
	}

	body := rec.Body.String()
	if !strings.Contains(body, `id="smoke-target"`) {
		t.Errorf("body missing #smoke-target morph anchor; body:\n%s", body)
	}
	if !strings.Contains(body, "/ds/_smoke/events") {
		t.Errorf("body missing /ds/_smoke/events SSE reference; body:\n%s", body)
	}
}

// TestSmokeEventsEmitsDatastarPatchElementsFrame is the wire-side tracer
// bullet (PRD F16, criterion 2.5a). On connect the handler must ship one
// element-patch frame whose body is a templ-rendered #smoke-target — that's
// all the browser smoke needs to confirm Datastar's runtime applies the patch.
//
// Asserts at a coarse grain on purpose. The SDK owns the exact data-line
// layout and could re-tune it across minor versions; this test pins only the
// non-negotiables called out in the PRD:
//   - SSE content type (the SDK is supposed to set it via NewSSE);
//   - the verified 1.0+ event name "datastar-patch-elements" (renamed from the
//     RC-era "datastar-merge-fragments" — see ds/NOTES.md F16);
//   - the templ fragment's #smoke-target id ends up on the wire so the
//     default outer-morph mode finds its anchor.
//
// Streams over a real server via the shared openEvents/readFrame helpers
// (adapter_test.go) — a ResponseRecorder can't be read while a streaming
// handler writes (data race caught under -race in M2.5b).
func TestSmokeEventsEmitsDatastarPatchElementsFrame(t *testing.T) {
	resp, br := openEvents(t, SmokeEventsHandler())

	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Errorf("content-type: want text/event-stream prefix, got %q", ct)
	}

	frame := readFrame(t, br)
	if !strings.Contains(frame, "datastar-patch-elements") {
		t.Errorf("missing verified 1.0+ event name 'datastar-patch-elements' on the wire; frame:\n%s", frame)
	}
	if !strings.Contains(frame, `id="smoke-target"`) {
		t.Errorf("patched fragment must carry #smoke-target so outer-morph lands; frame:\n%s", frame)
	}
}
