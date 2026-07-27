package frontend

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The Jotpad rides an underscore-prefixed signal so it can never ride along on
// another endpoint's POST (see JOTPAD-SPEC). Two behaviours are load-bearing
// and unverifiable from the Go side alone; the canary asserts the wire halves
// here and the browser half at /_smoke. Keep them: a Datastar bump that breaks
// either turns a silent Jotpad breakage into a visible canary failure.
func TestSmokeEventsPatchesUnderscorePrefixedSignal(t *testing.T) {
	_, br := openEvents(t, SmokeEventsHandler())

	readFrame(t, br) // the element patch
	frame := readFrame(t, br)

	if !strings.Contains(frame, "datastar-patch-signals") {
		t.Errorf("missing event name 'datastar-patch-signals' on the wire; frame:\n%s", frame)
	}
	if !strings.Contains(frame, smokeSignalName) {
		t.Errorf("signal patch must carry %s; frame:\n%s", smokeSignalName, frame)
	}
	if !strings.Contains(frame, smokeSignalValue) {
		t.Errorf("signal patch must carry the canary value; frame:\n%s", frame)
	}
}

// The echo half: a client that transmitted the underscore signal round-trips
// its value back into #smoke-echo. Asserting the read side here means a
// browser mismatch pins the blame on the client filter, not the handler.
func TestSmokeEchoEchoesUnderscoreSignal(t *testing.T) {
	body := strings.NewReader(`{"` + smokeSignalName + `":"` + smokeSignalValue + `"}`)
	req := httptest.NewRequest(http.MethodPost, "/_smoke/echo", body)
	rec := httptest.NewRecorder()

	SmokeEchoHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d", rec.Code)
	}
	frame := rec.Body.String()
	if !strings.Contains(frame, `id="smoke-echo"`) {
		t.Errorf("patched fragment must carry #smoke-echo so outer-morph lands; frame:\n%s", frame)
	}
	if !strings.Contains(frame, smokeSignalValue) {
		t.Errorf("echo must repeat the received signal value; frame:\n%s", frame)
	}
}

// The page must ship the exact filterSignals shape the Jotpad's write path
// uses — an include for the underscore signal plus the empty-string exclude
// that overrides Datastar's default underscore stripping.
func TestSmokeHandlerRendersUnderscoreSignalCanary(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/_smoke", nil)
	rec := httptest.NewRecorder()

	SmokeHandler().ServeHTTP(rec, req)

	body := rec.Body.String()
	for _, want := range []string{
		`id="smoke-echo"`,
		`id="smoke-signal"`,
		"/_smoke/echo",
		`include: /^` + smokeSignalName + `$/`,
		`exclude: /^$/`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q; body:\n%s", want, body)
		}
	}
}

// Datastar's outer-morph patches by id; without a #smoke-target match the SDK
// is a silent no-op. Attribute syntax is deliberately unpinned (browser-verified).
func TestSmokeHandlerRendersTargetAndSSEReference(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/_smoke", nil)
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
	if !strings.Contains(body, "/_smoke/events") {
		t.Errorf("body missing /_smoke/events SSE reference; body:\n%s", body)
	}
}

// Asserts coarsely — the SDK owns the data-line layout — pinning only the SSE
// content type, the 1.0+ event name "datastar-patch-elements" (renamed from
// the RC-era "datastar-merge-fragments"), and the #smoke-target anchor.
func TestSmokeEventsEmitsDatastarPatchElementsFrame(t *testing.T) {
	resp, br := openEvents(t, SmokeEventsHandler())

	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Errorf("content-type: want text/event-stream prefix, got %q", ct)
	}

	frame := readFrame(t, br)
	if !strings.Contains(frame, "datastar-patch-elements") {
		t.Errorf("missing event name 'datastar-patch-elements' on the wire; frame:\n%s", frame)
	}
	if !strings.Contains(frame, `id="smoke-target"`) {
		t.Errorf("patched fragment must carry #smoke-target so outer-morph lands; frame:\n%s", frame)
	}
}
