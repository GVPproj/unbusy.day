package frontend

import (
	"embed"
	"mime"
	"net/http"
	"os"
)

// Go's mime table lacks .webmanifest; without this the file server sniffs the
// JSON as text/plain and browsers reject it as a manifest.
func init() {
	_ = mime.AddExtensionType(".webmanifest", "application/manifest+json")
}

//go:embed static
var staticFS embed.FS

// Asset returns an embedded static file's bytes (e.g. "static/icon-192.png").
func Asset(name string) ([]byte, error) {
	return staticFS.ReadFile(name)
}

// StaticHandler serves /static/* from the embedded assets. In a templ watch
// session (TEMPL_DEV_MODE) it serves from disk instead, so app.css/JS edits
// land live without a Go rebuild.
func StaticHandler() http.Handler {
	var fs http.Handler
	if os.Getenv("TEMPL_DEV_MODE") != "" {
		// Rooted at internal/frontend so the "static/" prefix matches the
		// embedded layout; go run's CWD is the repo root.
		fs = http.FileServerFS(os.DirFS("internal/frontend"))
	} else {
		fs = http.FileServerFS(staticFS)
	}
	// Unversioned URLs with no validators: force revalidation each deploy or an
	// edge/CDN keeps serving a stale drag.js.
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		fs.ServeHTTP(w, r)
	})
}

// ServiceWorkerHandler serves sw.js from the site root: a service worker's
// control scope is capped to its own URL path, and scope "/" is what makes iOS
// treat the app as an installed PWA. See static/sw.js.
func ServiceWorkerHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		if os.Getenv("TEMPL_DEV_MODE") != "" {
			http.ServeFile(w, r, "internal/frontend/static/sw.js")
			return
		}
		data, err := staticFS.ReadFile("static/sw.js")
		if err != nil {
			http.Error(w, "service worker unavailable", http.StatusInternalServerError)
			return
		}
		_, _ = w.Write(data)
	})
}
