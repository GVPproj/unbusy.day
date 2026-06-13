package frontend

import (
	"context"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/GVPproj/unbusy.day/internal/auth"
	"github.com/GVPproj/unbusy.day/internal/frontend/components"
	"github.com/GVPproj/unbusy.day/internal/frontend/routes"
	"github.com/starfederation/datastar-go/datastar"
)

// AuthService is the login flow's view of the auth service; *auth.Service
// satisfies it.
type AuthService interface {
	RequestCode(ctx context.Context, email string) error
	VerifyCode(ctx context.Context, email, code string) (*auth.Session, error)
	Logout(ctx context.Context, token string) error
}

// Seeder gives a fresh user their starter blocks on first login (ADR 0003);
// *block.Service satisfies it.
type Seeder interface {
	Seed(ctx context.Context, owner string) error
}

// LoginPageHandler renders the email step of the OTP flow.
func LoginPageHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		if err := routes.LoginPage().Render(r.Context(), w); err != nil {
			http.Error(w, "render login page", http.StatusInternalServerError)
		}
	})
}

// loginSignals is the Datastar signals body the login forms @post.
type loginSignals struct {
	Email string `json:"email"`
	Code  string `json:"code"`
}

// RequestCodeHandler issues a login code. The response is identical whether
// the email exists, was throttled, or got a code — no account enumeration —
// so it always patches the same code-entry form.
func RequestCodeHandler(a AuthService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var sig loginSignals
		if err := datastar.ReadSignals(r, &sig); err != nil {
			http.Error(w, "invalid signals body", http.StatusBadRequest)
			return
		}
		if err := a.RequestCode(r.Context(), sig.Email); err != nil {
			log.Printf("ds request code: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		sse := datastar.NewSSE(w, r)
		if err := sse.PatchElementTempl(components.LoginCodeForm("")); err != nil {
			log.Printf("ds request code patch: %v", err)
		}
	})
}

// VerifyCodeHandler redeems the code: on success it seeds the new user's
// starter blocks, sets the session cookie, and redirects to the board. A bad
// code re-patches the form with a generic error (200 + hypermedia truth).
func VerifyCodeHandler(a AuthService, seeder Seeder, secureCookies bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var sig loginSignals
		if err := datastar.ReadSignals(r, &sig); err != nil {
			http.Error(w, "invalid signals body", http.StatusBadRequest)
			return
		}

		sess, err := a.VerifyCode(r.Context(), sig.Email, sig.Code)
		if errors.Is(err, auth.ErrInvalidCode) {
			sse := datastar.NewSSE(w, r)
			if err := sse.PatchElementTempl(components.LoginCodeForm("That code didn't work — check it or request a new one.")); err != nil {
				log.Printf("ds verify patch: %v", err)
			}
			return
		}
		if err != nil {
			log.Printf("ds verify code: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		if err := seeder.Seed(r.Context(), sess.UserID); err != nil {
			log.Printf("ds seed blocks: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		// The cookie must land before NewSSE writes the response headers.
		http.SetCookie(w, sessionCookie(sess.Token, sess.ExpiresAt, secureCookies))
		sse := datastar.NewSSE(w, r)
		if err := sse.Redirect("/"); err != nil {
			log.Printf("ds login redirect: %v", err)
		}
	})
}

// LogoutHandler revokes the session row (immediate, server-side; ADR 0002),
// expires the cookie, and redirects to the login page.
func LogoutHandler(a AuthService, secureCookies bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if c, err := r.Cookie(SessionCookie); err == nil {
			if err := a.Logout(r.Context(), c.Value); err != nil {
				log.Printf("ds logout: %v", err)
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
		}
		http.SetCookie(w, sessionCookie("", time.Unix(0, 0), secureCookies))
		sse := datastar.NewSSE(w, r)
		if err := sse.Redirect("/login"); err != nil {
			log.Printf("ds logout redirect: %v", err)
		}
	})
}
