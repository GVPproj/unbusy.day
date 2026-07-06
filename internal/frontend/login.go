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

// Seeder gives a fresh user their starter blocks; *block.Service satisfies it.
type Seeder interface {
	Seed(ctx context.Context, owner string) error
}

// LoginPageHandler renders the email step of the OTP flow; an empty
// turnstileSiteKey (dev) renders no presence widget.
func LoginPageHandler(turnstileSiteKey string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		if err := routes.LoginPage(turnstileSiteKey).Render(r.Context(), w); err != nil {
			http.Error(w, "render login page", http.StatusInternalServerError)
		}
	})
}

type loginSignals struct {
	Email          string `json:"email"`
	Code           string `json:"code"`
	TurnstileToken string `json:"turnstileToken"`
}

// RequestCodeHandler issues a login code. The response is identical whether
// the email exists, was throttled, or got a code — no account enumeration.
func RequestCodeHandler(a AuthService, pv PresenceVerifier) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var sig loginSignals
		if err := datastar.ReadSignals(r, &sig); err != nil {
			http.Error(w, "invalid signals body", http.StatusBadRequest)
			return
		}

		// A failed or errored presence check returns the same non-committal
		// patched form and never issues a code.
		if ok, err := pv.Verify(r.Context(), sig.TurnstileToken, clientIP(r, true)); err != nil || !ok {
			if err != nil {
				log.Printf("ds presence verify: %v", err)
			}
			patchLoginCodeForm(w, r)
			return
		}

		if err := a.RequestCode(r.Context(), sig.Email); err != nil {
			log.Printf("ds request code: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		patchLoginCodeForm(w, r)
	})
}

// patchLoginCodeForm writes the single non-committal code-entry patch every
// RequestCode outcome shares (no enumeration).
func patchLoginCodeForm(w http.ResponseWriter, r *http.Request) {
	sse := datastar.NewSSE(w, r)
	if err := sse.PatchElementTempl(components.LoginCodeForm("")); err != nil {
		log.Printf("ds request code patch: %v", err)
	}
}

// VerifyCodeHandler redeems the code: on success it seeds starter blocks, sets
// the session cookie, and redirects to the board.
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

// LogoutHandler revokes the session row, expires the cookie, and redirects to
// the login page.
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
