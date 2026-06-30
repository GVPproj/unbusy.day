package frontend

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakePresence is a PresenceVerifier under test control: ok decides the verdict,
// and it records whether Verify ran.
type fakePresence struct {
	ok     bool
	err    error
	gotTok string
	calls  int
}

func (f *fakePresence) Verify(_ context.Context, token, _ string) (bool, error) {
	f.calls++
	f.gotTok = token
	return f.ok, f.err
}

// A passing human-presence check lets the request reach RequestCode and patches
// the code-entry form as usual.
func TestRequestCodePassingPresenceReachesRequestCode(t *testing.T) {
	a := &fakeAuth{}
	pv := &fakePresence{ok: true}
	req := httptest.NewRequest(http.MethodPost, "/login/code",
		strings.NewReader(`{"email":"x@example.test","code":"","turnstileToken":"tok-abc"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	RequestCodeHandler(a, pv).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d", rec.Code)
	}
	if pv.calls != 1 || pv.gotTok != "tok-abc" {
		t.Errorf("Verify calls=%d tok=%q, want 1 / tok-abc", pv.calls, pv.gotTok)
	}
	if a.gotEmail != "x@example.test" {
		t.Errorf("RequestCode not reached: gotEmail=%q", a.gotEmail)
	}
	if !strings.Contains(rec.Body.String(), `id="login-form"`) {
		t.Errorf("want #login-form patch; body:\n%s", rec.Body.String())
	}
}

// A failing presence check returns the same non-committal patched form and
// never calls RequestCode — a script bypassing the widget gains nothing, and
// the response can't be told apart from a real send (no enumeration).
func TestRequestCodeFailingPresenceSkipsRequestCode(t *testing.T) {
	a := &fakeAuth{}
	pv := &fakePresence{ok: false}
	req := httptest.NewRequest(http.MethodPost, "/login/code",
		strings.NewReader(`{"email":"x@example.test","code":"","turnstileToken":"bad"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	RequestCodeHandler(a, pv).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d", rec.Code)
	}
	if a.gotEmail != "" {
		t.Errorf("failing presence must not reach RequestCode; gotEmail=%q", a.gotEmail)
	}
	if !strings.Contains(rec.Body.String(), `id="login-form"`) {
		t.Errorf("want identical #login-form patch; body:\n%s", rec.Body.String())
	}
}

// With no Turnstile secret configured (dev), the verifier no-ops: Verify passes
// so local login needs no Cloudflare account — mirrors LogMailer.
func TestNewPresenceVerifierNoSecretIsPermissive(t *testing.T) {
	pv := NewPresenceVerifier("")
	ok, err := pv.Verify(context.Background(), "", "127.0.0.1")
	if err != nil || !ok {
		t.Fatalf("dev no-op verifier: want (true, nil), got (%v, %v)", ok, err)
	}
}
