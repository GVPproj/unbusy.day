// Login-flow adapter tests: a fake AuthService stands in for *auth.Service
// (the DB is the system boundary); templ rendering is real, pinning the
// observable wire behavior of the OTP flow.
package frontend

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GVPproj/unbusy.day/auth"
)

type fakeAuth struct {
	verifyErr  error
	sessionErr error

	gotEmail string
	gotCode  string
}

func (f *fakeAuth) RequestCode(_ context.Context, email string) error {
	f.gotEmail = email
	return nil
}

func (f *fakeAuth) VerifyCode(_ context.Context, email, code string) (*auth.Session, error) {
	f.gotEmail, f.gotCode = email, code
	if f.verifyErr != nil {
		return nil, f.verifyErr
	}
	return &auth.Session{Token: "tok-1", UserID: testOwner, ExpiresAt: time.Now().Add(time.Hour)}, nil
}

func (f *fakeAuth) Logout(_ context.Context, token string) error { return nil }

func (f *fakeAuth) UserForSession(_ context.Context, token string) (string, error) {
	if f.sessionErr != nil {
		return "", f.sessionErr
	}
	return testOwner, nil
}

type fakeSeeder struct{ gotOwner string }

func (f *fakeSeeder) Seed(_ context.Context, owner string) error {
	f.gotOwner = owner
	return nil
}

// Requesting a code always patches the same code-entry form onto #login-form
// — identical for known and unknown emails, so responses can't enumerate.
func TestRequestCodePatchesCodeForm(t *testing.T) {
	a := &fakeAuth{}
	req := httptest.NewRequest(http.MethodPost, "/login/code",
		strings.NewReader(`{"email":"x@example.test","code":""}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	RequestCodeHandler(a).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d", rec.Code)
	}
	if a.gotEmail != "x@example.test" {
		t.Errorf("RequestCode called with %q", a.gotEmail)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "datastar-patch-elements") || !strings.Contains(body, `id="login-form"`) {
		t.Errorf("want #login-form element patch; body:\n%s", body)
	}
}

// A good code seeds the user's starter cards, sets the session cookie
// (HttpOnly, SameSite=Lax), and redirects to the board.
func TestVerifyCodeSetsCookieSeedsAndRedirects(t *testing.T) {
	a := &fakeAuth{}
	seeder := &fakeSeeder{}
	req := httptest.NewRequest(http.MethodPost, "/login/verify",
		strings.NewReader(`{"email":"x@example.test","code":"123456"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	VerifyCodeHandler(a, seeder, false).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d", rec.Code)
	}
	if seeder.gotOwner != testOwner {
		t.Errorf("Seed called with %q, want %q", seeder.gotOwner, testOwner)
	}

	var cookie *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == SessionCookie {
			cookie = c
		}
	}
	if cookie == nil {
		t.Fatalf("no %s cookie set", SessionCookie)
	}
	if cookie.Value != "tok-1" || !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode {
		t.Errorf("cookie misconfigured: %+v", cookie)
	}

	if body := rec.Body.String(); !strings.Contains(body, "/") || !strings.Contains(body, "datastar") {
		t.Errorf("want a Datastar redirect to /; body:\n%s", body)
	}
}

// A bad code re-patches the code form with an error at 200 — same
// hypermedia-truth contract as a rejected reorder. No cookie.
func TestVerifyCodeRejectionRepatchesForm(t *testing.T) {
	a := &fakeAuth{verifyErr: auth.ErrInvalidCode}
	req := httptest.NewRequest(http.MethodPost, "/login/verify",
		strings.NewReader(`{"email":"x@example.test","code":"000000"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	VerifyCodeHandler(a, &fakeSeeder{}, false).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d", rec.Code)
	}
	if len(rec.Result().Cookies()) != 0 {
		t.Errorf("rejected verify must not set a cookie")
	}
	if body := rec.Body.String(); !strings.Contains(body, `id="login-form"`) {
		t.Errorf("want #login-form re-patch; body:\n%s", body)
	}
}

// Unauthenticated page loads bounce to /login; the SSE and mutation
// endpoints get a bare 401 (a redirect would feed HTML to EventSource/@post).
func TestRequireSessionGate(t *testing.T) {
	a := &fakeAuth{sessionErr: auth.ErrNoSession}
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("next handler must not run unauthenticated")
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	RequireSession(a, next).ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/login" {
		t.Errorf("page: want 303 → /login, got %d → %q", rec.Code, rec.Header().Get("Location"))
	}

	req = httptest.NewRequest(http.MethodPost, "/cards/layout", nil)
	rec = httptest.NewRecorder()
	RequireSession(a, next).ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("mutation: want 401, got %d", rec.Code)
	}

	// SSE gets a bare 401 too — a 303 would feed the login HTML to EventSource.
	req = httptest.NewRequest(http.MethodGet, "/events", nil)
	rec = httptest.NewRecorder()
	RequireSession(a, next).ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("SSE: want 401, got %d", rec.Code)
	}
}

// A valid cookie passes through with the owner stashed in the context.
func TestRequireSessionPassesOwner(t *testing.T) {
	a := &fakeAuth{}
	var got string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = ownerFrom(r.Context())
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookie, Value: "tok-1"})
	rec := httptest.NewRecorder()
	RequireSession(a, next).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d", rec.Code)
	}
	if got != testOwner {
		t.Errorf("owner in context = %q, want %q", got, testOwner)
	}
}
