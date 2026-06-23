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

// staticFS embeds frontend assets (drag.js, the Tailwind output.css, etc.) so
// the binary stays self-contained — same pattern as the embedded migrations.
//
//go:embed static
var staticFS embed.FS

// StaticHandler serves /static/* from the embedded assets.
//
// In a templ watch session (TEMPL_DEV_MODE set) it serves from disk instead, so
// `tailwindcss --watch` rewriting output.css is live on the next browser reload.
// Embedded files are frozen at compile time, and templ's text-only fast path
// skips the Go rebuild — so without this a brand-new utility class never lands
// until something forces a recompile.
func StaticHandler() http.Handler {
	var fs http.Handler
	if os.Getenv("TEMPL_DEV_MODE") != "" {
		// DirFS is rooted at internal/frontend so the "static/" path prefix
		// matches the embedded layout; go run's CWD is the repo root.
		fs = http.FileServerFS(os.DirFS("internal/frontend"))
	} else {
		fs = http.FileServerFS(staticFS)
	}
	// Embedded files have no reliable validators and the URLs are unversioned,
	// so force revalidation each deploy — otherwise an edge/CDN (Cloudflare)
	// keeps serving a stale drag.js past a deploy.
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		fs.ServeHTTP(w, r)
	})
}
