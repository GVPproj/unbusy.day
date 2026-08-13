package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GVPproj/unbusy.day/internal/auth"
)

// fakeResolver stands in for *auth.Service on the SessionResolver seam.
type fakeResolver struct {
	owner string
	err   error
}

func (f *fakeResolver) UserForSession(_ context.Context, token string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return f.owner, nil
}

// Unauthenticated page loads bounce to /login; SSE and mutation endpoints get
// a bare 401 — a redirect would feed HTML to EventSource/@post.
func TestRequireSessionGate(t *testing.T) {
	a := &fakeResolver{err: auth.ErrNoSession}
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("next handler must not run unauthenticated")
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	RequireSession(a, next).ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/login" {
		t.Errorf("page: want 303 → /login, got %d → %q", rec.Code, rec.Header().Get("Location"))
	}

	req = httptest.NewRequest(http.MethodPost, "/blocks/layout", nil)
	rec = httptest.NewRecorder()
	RequireSession(a, next).ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("mutation: want 401, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/events", nil)
	rec = httptest.NewRecorder()
	RequireSession(a, next).ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("SSE: want 401, got %d", rec.Code)
	}
}

func TestRequireSessionPassesOwner(t *testing.T) {
	a := &fakeResolver{owner: "owner-1"}
	var got string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = OwnerFrom(r.Context())
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookie, Value: "tok-1"})
	rec := httptest.NewRecorder()
	RequireSession(a, next).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d", rec.Code)
	}
	if got != "owner-1" {
		t.Errorf("owner in context = %q, want %q", got, "owner-1")
	}
}
