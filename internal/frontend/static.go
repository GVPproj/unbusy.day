package frontend

import (
	"embed"
	"net/http"
)

// staticFS embeds frontend assets (drag.js etc.) so the binary stays
// self-contained — same pattern as the embedded migrations.
//
//go:embed static
var staticFS embed.FS

// StaticHandler serves /static/* from the embedded assets.
func StaticHandler() http.Handler {
	return http.FileServerFS(staticFS)
}
