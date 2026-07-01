package auth

import (
	"context"
	"encoding/base64"
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
	addr    string // host:port
	auth    smtp.Auth
	from    string // From header / envelope sender; must be a verified address
	logoPNG []byte // optional; when set, embedded via cid: instead of a text wordmark
}

// NewSMTPMailer wires credentials into a Mailer. host is the SMTP endpoint
// (e.g. email-smtp.us-west-2.amazonaws.com); SES SMTP creds go in user/pass.
// logoPNG (nil = none) is embedded inline (cid:) as the email header logo.
func NewSMTPMailer(host, port, user, pass, from string, logoPNG []byte) *SMTPMailer {
	return &SMTPMailer{
		addr:    net.JoinHostPort(host, port),
		auth:    smtp.PlainAuth("", user, pass, host),
		from:    from,
		logoPNG: logoPNG,
	}
}

// SendCode mails one OTP. ctx is unused: stdlib SendMail has no context hook,
// and SES is fast — the dial-level timeout is the server's default.
func (m *SMTPMailer) SendCode(_ context.Context, email, code string) error {
	return smtp.SendMail(m.addr, m.auth, m.from, []string{email}, m.message(email, code))
}

// accent is the Solarized-blue brand color, hardcoded because email clients
// have no access to the app's CSS custom-property theme tokens.
const accent = "#268bd2"

// Fixed multipart delimiters. Static is fine: neither appears in the OTP
// (digits), the boilerplate copy, or base64 output (no "-" in that alphabet).
const (
	altBoundary = "ubd-otp-alt-1f8c" // wraps the text/plain + text/html parts
	relBoundary = "ubd-otp-rel-1f8c" // wraps the alternative + the inline logo
	logoCID     = "ubd-logo"         // Content-ID the HTML <img> references
)

func (m *SMTPMailer) message(to, code string) []byte {
	var b strings.Builder
	b.WriteString("From: unbusy.day <" + m.from + ">\r\n")
	b.WriteString("To: " + to + "\r\n")
	b.WriteString("Subject: Your unbusy.day login code\r\n")
	b.WriteString("Date: " + time.Now().Format(time.RFC1123Z) + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")

	hasLogo := len(m.logoPNG) > 0
	if hasLogo {
		// multipart/related wraps the alternative body + the cid: logo image.
		b.WriteString("Content-Type: multipart/related; boundary=\"")
		b.WriteString(relBoundary)
		b.WriteString("\"\r\n\r\n")
		b.WriteString("--")
		b.WriteString(relBoundary)
		b.WriteString("\r\n")
	}

	// The alternative container (its own header line either starts the message
	// or nests inside related).
	b.WriteString("Content-Type: multipart/alternative; boundary=\"")
	b.WriteString(altBoundary)
	b.WriteString("\"\r\n\r\n")

	// Plain-text part: fallback for non-HTML clients and better deliverability.
	b.WriteString("--")
	b.WriteString(altBoundary)
	b.WriteString("\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n")
	b.WriteString("Your login code is " + code + "\r\n\r\n")
	b.WriteString("It expires in 10 minutes. If you didn't request it, ignore this email.\r\n\r\n")

	// HTML part: light branding — styles are inline only (email clients drop
	// <style>/external CSS), colors hardcoded to neutral grays + the accent.
	b.WriteString("--")
	b.WriteString(altBoundary)
	b.WriteString("\r\nContent-Type: text/html; charset=UTF-8\r\n\r\n")
	b.WriteString(htmlBody(code, hasLogo))
	b.WriteString("\r\n--")
	b.WriteString(altBoundary)
	b.WriteString("--\r\n")

	if hasLogo {
		// The inline image part, referenced by <img src="cid:ubd-logo">.
		b.WriteString("\r\n--")
		b.WriteString(relBoundary)
		b.WriteString("\r\nContent-Type: image/png\r\n")
		b.WriteString("Content-Transfer-Encoding: base64\r\n")
		b.WriteString("Content-ID: <" + logoCID + ">\r\n")
		b.WriteString("Content-Disposition: inline; filename=\"logo.png\"\r\n\r\n")
		b.WriteString(base64Wrap(m.logoPNG))
		b.WriteString("\r\n--")
		b.WriteString(relBoundary)
		b.WriteString("--\r\n")
	}
	return []byte(b.String())
}

// base64Wrap encodes bytes and folds them into RFC 2045 76-char lines.
func base64Wrap(data []byte) string {
	enc := base64.StdEncoding.EncodeToString(data)
	var b strings.Builder
	for len(enc) > 76 {
		b.WriteString(enc[:76])
		b.WriteString("\r\n")
		enc = enc[76:]
	}
	b.WriteString(enc)
	return b.String()
}

// htmlBody renders the centered, inline-styled OTP email. The header is the
// embedded PNG logo when present (cid:), else an accent-colored text wordmark
// (no SVG — Gmail strips it).
func htmlBody(code string, hasLogo bool) string {
	header := `<div style="font-size:22px;font-weight:700;letter-spacing:-0.02em;color:` + accent + `;">unbusy.day</div>`
	if hasLogo {
		header = `<img src="cid:` + logoCID + `" width="72" height="72" alt="unbusy.day" style="display:inline-block;border:0;border-radius:16px;">`
	}
	// Uniform 24px vertical rhythm via margin-top on each block (flex/gap is
	// unsupported in Outlook). The code box is wrapped so its margin applies.
	return `<!DOCTYPE html>
<html>
<body style="margin:0;padding:0;background:#f5f5f4;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Helvetica,Arial,sans-serif;">
  <div style="max-width:420px;margin:0 auto;padding:40px 24px;text-align:center;">
    ` + header + `
    <p style="margin:24px 0 0;font-size:15px;color:#57534e;">Your login code is</p>
    <div style="margin:24px 0 0;"><span style="display:inline-block;padding:16px 28px;border:1px solid #e7e5e4;border-radius:10px;background:#ffffff;font-size:28px;font-weight:700;letter-spacing:0.28em;color:#1c1917;">` + code + `</span></div>
    <p style="margin:24px 0 0;font-size:13px;color:#78716c;">It expires in 10 minutes.<br>If you didn't request it, you can ignore this email.</p>
    <p style="margin:24px 0 0;font-size:12px;color:` + accent + `;font-weight:600;">unbusy.day</p>
  </div>
</body>
</html>`
}
