package frontend

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// PresenceVerifier is the human-presence seam (Cloudflare Turnstile): it
// confirms a request came from a real browser solving the widget, before any
// OTP is issued. Mirrors how Mailer abstracts SMTP — a fakeable boundary so the
// flow is tested without a live Cloudflare dependency.
type PresenceVerifier interface {
	Verify(ctx context.Context, token, remoteIP string) (bool, error)
}

// noopVerifier always passes — the dev default when no Turnstile secret is set,
// so `task dev` needs no Cloudflare account (mirrors LogMailer).
type noopVerifier struct{}

func (noopVerifier) Verify(context.Context, string, string) (bool, error) { return true, nil }

// turnstileVerifier checks a widget token against Cloudflare's siteverify
// endpoint with the site secret. Plain net/http, no SDK (deps rule).
type turnstileVerifier struct {
	secret string
	client *http.Client
}

const siteverifyURL = "https://challenges.cloudflare.com/turnstile/v0/siteverify"

// NewPresenceVerifier picks the Turnstile verifier when a secret is set, else
// the dev no-op (mirrors newMailer's LogMailer fallback).
func NewPresenceVerifier(secret string) PresenceVerifier {
	if secret == "" {
		return noopVerifier{}
	}
	return &turnstileVerifier{secret: secret, client: &http.Client{Timeout: 10 * time.Second}}
}

// Verify posts the token to siteverify and reports Cloudflare's success verdict.
// A transport/decode error is returned to the caller (treated as a failed check
// — fail closed, since a bypassed widget must gain nothing).
func (v *turnstileVerifier) Verify(ctx context.Context, token, remoteIP string) (bool, error) {
	form := url.Values{"secret": {v.secret}, "response": {token}}
	if remoteIP != "" {
		form.Set("remoteip", remoteIP)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, siteverifyURL,
		strings.NewReader(form.Encode()))
	if err != nil {
		return false, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := v.client.Do(req)
	if err != nil {
		return false, err
	}
	defer func() { _ = resp.Body.Close() }()
	var out struct {
		Success bool `json:"success"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return false, err
	}
	return out.Success, nil
}
