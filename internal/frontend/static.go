package frontend

import (
	"embed"
	"net/http"
	"os"
)

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
	if os.Getenv("TEMPL_DEV_MODE") != "" {
		// DirFS is rooted at internal/frontend so the "static/" path prefix
		// matches the embedded layout; go run's CWD is the repo root.
		return http.FileServerFS(os.DirFS("internal/frontend"))
	}
	return http.FileServerFS(staticFS)
}
