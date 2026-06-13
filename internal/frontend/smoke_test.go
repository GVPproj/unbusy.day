package frontend

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The smoke page must render an HTML document with a stable #smoke-target
// (Datastar's default outer-morph patches by id; without a match the SDK is a
// silent no-op) and a reference to /_smoke/events so the browser opens the
// stream on load. It deliberately does not pin the attribute syntax — that's
// verified in a real browser, and locking it here would couple us to RC-era
// naming.
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

// On connect the handler must ship one element-patch frame whose body is a
// templ-rendered #smoke-target. Asserts coarsely on purpose — the SDK owns the
// exact data-line layout — pinning only: the SSE content type, the verified
// 1.0+ event name "datastar-patch-elements" (renamed from the RC-era
// "datastar-merge-fragments"), and that the #smoke-target id reaches the wire
// so outer-morph finds its anchor. Streams over a real server because a
// ResponseRecorder can't be read while a streaming handler writes.
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
