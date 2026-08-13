package frontend

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// JS tests live beside their modules under static/js but must never be served.
func TestStaticHandlerHidesJSTests(t *testing.T) {
	h := StaticHandler()

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/static/js/blocks/push.js", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("module: want 200, got %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/static/js/blocks/push.test.js", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("test file: want 404, got %d", rec.Code)
	}
}
