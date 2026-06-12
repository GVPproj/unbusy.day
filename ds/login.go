package ds

import (
	"log"
	"net/http"

	"github.com/grahamvanpelt/unbusy.day/ds/components"
	"github.com/starfederation/datastar-go/datastar"
)

// LoginPageHandler renders the placeholder login view.
func LoginPageHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		if err := components.LoginPage().Render(r.Context(), w); err != nil {
			http.Error(w, "render login page", http.StatusInternalServerError)
		}
	})
}

// LoginActionHandler "logs in" by redirecting to the main view over SSE.
// No session yet — this is just the navigation seam real auth will fill in.
func LoginActionHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sse := datastar.NewSSE(w, r)
		if err := sse.Redirect("/"); err != nil {
			log.Printf("ds login redirect: %v", err)
		}
	})
}
