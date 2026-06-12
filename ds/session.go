package ds

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/grahamvanpelt/unbusy.day/auth"
)

// SessionCookie carries the opaque session token (ADR 0002).
const SessionCookie = "session"

// SessionResolver is the middleware's view of the auth service;
// *auth.Service satisfies it.
type SessionResolver interface {
	UserForSession(ctx context.Context, token string) (string, error)
}

type ctxKey int

const ownerKey ctxKey = 0

// ownerFrom returns the authenticated user id RequireSession stashed.
func ownerFrom(ctx context.Context) string {
	owner, _ := ctx.Value(ownerKey).(string)
	return owner
}

// withOwner returns ctx carrying the authenticated user id. Exposed to tests;
// production code only enters owners via RequireSession.
func withOwner(ctx context.Context, owner string) context.Context {
	return context.WithValue(ctx, ownerKey, owner)
}

// RequireSession resolves the session cookie to a user id and stashes it in
// the request context. Unauthenticated page loads bounce to /login; the SSE
// and mutation endpoints get a bare 401 (a redirect would feed HTML to
// EventSource and @post).
func RequireSession(resolver SessionResolver, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(SessionCookie)
		if err == nil {
			owner, rerr := resolver.UserForSession(r.Context(), c.Value)
			if rerr == nil {
				next.ServeHTTP(w, r.WithContext(withOwner(r.Context(), owner)))
				return
			}
			if !errors.Is(rerr, auth.ErrNoSession) {
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
		}
		if r.Method == http.MethodGet && r.URL.Path == "/" {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	})
}

// sessionCookie builds the auth cookie per ADR 0002: HttpOnly, SameSite=Lax
// (baseline CSRF defense for the POSTs), Secure in production only.
func sessionCookie(token string, expires time.Time, secure bool) *http.Cookie {
	return &http.Cookie{
		Name:     SessionCookie,
		Value:    token,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
	}
}
