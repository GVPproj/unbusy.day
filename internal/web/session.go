package web

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/GVPproj/unbusy.day/internal/auth"
)

const SessionCookie = "session"

// SessionResolver is the middleware's view of the auth service; *auth.Service
// satisfies it.
type SessionResolver interface {
	UserForSession(ctx context.Context, token string) (string, error)
}

type ctxKey int

const ownerKey ctxKey = 0

// OwnerFrom returns the authenticated user id RequireSession stashed.
func OwnerFrom(ctx context.Context) string {
	owner, _ := ctx.Value(ownerKey).(string)
	return owner
}

// WithOwner stashes the owner id; exported so handler tests can stand in for
// RequireSession.
func WithOwner(ctx context.Context, owner string) context.Context {
	return context.WithValue(ctx, ownerKey, owner)
}

// RequireSession stashes the session cookie's user id in the request context.
// Unauthenticated page loads bounce to /login; SSE and mutation endpoints get
// a bare 401 (a redirect would feed HTML to EventSource and @post).
func RequireSession(resolver SessionResolver, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(SessionCookie)
		if err == nil {
			owner, rerr := resolver.UserForSession(r.Context(), c.Value)
			if rerr == nil {
				next.ServeHTTP(w, r.WithContext(WithOwner(r.Context(), owner)))
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

// NewSessionCookie builds the auth cookie (ADR 0002); SameSite=Lax is the
// baseline CSRF defense for the POSTs.
func NewSessionCookie(token string, expires time.Time, secure bool) *http.Cookie {
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
