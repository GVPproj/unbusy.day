//go:build !embedassets

package main

import "net/http"

// spaHandler is a no-op outside release builds. In dev the SPA is served by
// the Vite dev server (`task dev:fe`); the embedded build compiles in only
// with `-tags embedassets` (see Dockerfile). This stub keeps `go run`/`go
// test` building without a frontend/dist directory.
func spaHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w,
			"SPA not embedded in this build — run the Vite dev server (task dev:fe) or build with -tags embedassets",
			http.StatusNotFound)
	})
}
