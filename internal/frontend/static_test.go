package frontend

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestPlannerShipsNoMotionRuntime(t *testing.T) {
	err := fs.WalkDir(staticFS, "static/js", func(name string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(name, ".js") {
			return err
		}
		data, err := staticFS.ReadFile(name)
		if err != nil {
			return err
		}
		text := strings.ToLower(string(data))
		for _, dependency := range []string{"motion@", "framer-motion", "motion-dom"} {
			if strings.Contains(text, dependency) {
				t.Errorf("%s references removed runtime %q", name, dependency)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestStaticHandlerServesVendoredESModules(t *testing.T) {
	modules, err := fs.Glob(staticFS, codeMirrorVendorRoot+"/modules/*.mjs")
	if err != nil || len(modules) == 0 {
		t.Fatalf("find vendored modules: %v", err)
	}
	rec := httptest.NewRecorder()
	StaticHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/"+modules[0], nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("module: want 200, got %d", rec.Code)
	}
	if contentType := rec.Header().Get("Content-Type"); !strings.Contains(contentType, "javascript") {
		t.Errorf("content type: want JavaScript, got %q", contentType)
	}
}
