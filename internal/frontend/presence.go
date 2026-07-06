package frontend

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// PresenceVerifier is the human-presence seam (Cloudflare Turnstile).
type PresenceVerifier interface {
	Verify(ctx context.Context, token, remoteIP string) (bool, error)
}

// noopVerifier always passes — the dev default when no Turnstile secret is set.
type noopVerifier struct{}

func (noopVerifier) Verify(context.Context, string, string) (bool, error) { return true, nil }

type turnstileVerifier struct {
	secret string
	client *http.Client
}

const siteverifyURL = "https://challenges.cloudflare.com/turnstile/v0/siteverify"

// NewPresenceVerifier picks the Turnstile verifier when a secret is set, else
// the dev no-op.
func NewPresenceVerifier(secret string) PresenceVerifier {
	if secret == "" {
		return noopVerifier{}
	}
	return &turnstileVerifier{secret: secret, client: &http.Client{Timeout: 10 * time.Second}}
}

// Verify posts the token to siteverify. Transport/decode errors fail closed —
// a bypassed widget must gain nothing.
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
