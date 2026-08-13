package auth

import (
	"context"
	"testing"
)

// No Turnstile secret (dev) means a permissive no-op verifier — mirrors LogMailer.
func TestNewPresenceVerifierNoSecretIsPermissive(t *testing.T) {
	pv := NewPresenceVerifier("")
	ok, err := pv.Verify(context.Background(), "", "127.0.0.1")
	if err != nil || !ok {
		t.Fatalf("dev no-op verifier: want (true, nil), got (%v, %v)", ok, err)
	}
}
