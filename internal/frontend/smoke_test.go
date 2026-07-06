package frontend

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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
