// Login-flow adapter tests: a fake AuthService stands in for *auth.Service;
// templ rendering is real.
package frontend

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GVPproj/unbusy.day/internal/auth"
	"github.com/GVPproj/unbusy.day/internal/web"
)

type fakeAuth struct {
	verifyErr error

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

// The patched code form is identical for known and unknown emails, so
// responses can't enumerate.
func TestRequestCodePatchesCodeForm(t *testing.T) {
	a := &fakeAuth{}
	req := httptest.NewRequest(http.MethodPost, "/login/code",
		strings.NewReader(`{"email":"x@example.test","code":""}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	RequestCodeHandler(a, &fakePresence{ok: true}).ServeHTTP(rec, req)

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

func TestVerifyCodeSetsCookieAndRedirects(t *testing.T) {
	a := &fakeAuth{}
	req := httptest.NewRequest(http.MethodPost, "/login/verify",
		strings.NewReader(`{"email":"x@example.test","code":"123456"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	VerifyCodeHandler(a, false).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d", rec.Code)
	}

	var cookie *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == web.SessionCookie {
			cookie = c
		}
	}
	if cookie == nil {
		t.Fatalf("no %s cookie set", web.SessionCookie)
	}
	if cookie.Value != "tok-1" || !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode {
		t.Errorf("cookie misconfigured: %+v", cookie)
	}

	if body := rec.Body.String(); !strings.Contains(body, "/") || !strings.Contains(body, "datastar") {
		t.Errorf("want a Datastar redirect to /; body:\n%s", body)
	}
}

func TestVerifyCodeRejectionRepatchesForm(t *testing.T) {
	a := &fakeAuth{verifyErr: auth.ErrInvalidCode}
	req := httptest.NewRequest(http.MethodPost, "/login/verify",
		strings.NewReader(`{"email":"x@example.test","code":"000000"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	VerifyCodeHandler(a, false).ServeHTTP(rec, req)

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
