package auth

import (
	"context"
	"log"
	"net"
	"net/smtp"
	"strings"
	"time"
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

// SMTPMailer sends codes over SMTP — the production path (AWS SES SMTP, but
// any STARTTLS host works). Stdlib net/smtp keeps the dependency at zero.
type SMTPMailer struct {
	addr string // host:port
	auth smtp.Auth
	from string // From header / envelope sender; must be a verified address
}

// NewSMTPMailer wires credentials into a Mailer. host is the SMTP endpoint
// (e.g. email-smtp.us-west-2.amazonaws.com); SES SMTP creds go in user/pass.
func NewSMTPMailer(host, port, user, pass, from string) *SMTPMailer {
	return &SMTPMailer{
		addr: net.JoinHostPort(host, port),
		auth: smtp.PlainAuth("", user, pass, host),
		from: from,
	}
}

// SendCode mails one OTP. ctx is unused: stdlib SendMail has no context hook,
// and SES is fast — the dial-level timeout is the server's default.
func (m *SMTPMailer) SendCode(_ context.Context, email, code string) error {
	return smtp.SendMail(m.addr, m.auth, m.from, []string{email}, m.message(email, code))
}

func (m *SMTPMailer) message(to, code string) []byte {
	var b strings.Builder
	b.WriteString("From: unbusy.day <" + m.from + ">\r\n")
	b.WriteString("To: " + to + "\r\n")
	b.WriteString("Subject: Your unbusy.day login code\r\n")
	b.WriteString("Date: " + time.Now().Format(time.RFC1123Z) + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	b.WriteString("\r\n")
	b.WriteString("Your login code is " + code + "\r\n\r\n")
	b.WriteString("It expires in 10 minutes. If you didn't request it, ignore this email.\r\n")
	return []byte(b.String())
}
