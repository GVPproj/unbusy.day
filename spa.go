//go:build embedassets

package main

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// distFS holds the Vite build. Compiled in only for release builds via
// `-tags embedassets` (see Dockerfile) so dev `go run`/`go test` don't require
// a dist directory — that path uses spa_stub.go instead.
//
//go:embed all:frontend/dist
var distFS embed.FS

// spaHandler serves the embedded SPA. Content-hashed /assets/* are immutable
// and cached for a year; every other path falls back to index.html with
// no-cache so Cloudflare revalidates the entry point and client-side routing
// resolves unknown paths.
func spaHandler() http.Handler {
	dist, err := fs.Sub(distFS, "frontend/dist")
	if err != nil {
		panic(err)
	}
	fileServer := http.FileServer(http.FS(dist))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")

		// Serve a real file when one exists (and isn't a directory); otherwise
		// fall through to the index.html SPA shell.
		if p != "" {
			if info, statErr := fs.Stat(dist, p); statErr == nil && !info.IsDir() {
				if strings.HasPrefix(r.URL.Path, "/assets/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				}
				fileServer.ServeHTTP(w, r)
				return
			}
		}

		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFileFS(w, r, dist, "index.html")
	})
}
