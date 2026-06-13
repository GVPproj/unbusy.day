package auth

import (
	"context"
	"log"
)

// Mailer is the email seam (ADR 0001): production swaps in a real provider
// (Resend / Postmark / SES) without touching this package's logic.
type Mailer interface {
	SendCode(ctx context.Context, email, code string) error
}

// LogMailer is the dev implementation: the code lands on stdout, so no
// external email service is required to run the app.
type LogMailer struct{}

func (LogMailer) SendCode(_ context.Context, email, code string) error {
	log.Printf("auth: login code for %s: %s", email, code)
	return nil
}
