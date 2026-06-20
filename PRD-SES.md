# PRD — Production Email via Amazon SES (SMTP)

## Problem

OTP login (ADR 0001) currently uses `LogMailer`, which prints the code to
stdout. That's fine for dev but means **there is no real email delivery in
production** — a deployed user can't actually log in. We need a production
`Mailer` implementation that sends OTP codes reliably and cheaply for a small
(but growing) user base.

## Goals

- A production `auth.Mailer` that delivers OTP codes to real inboxes.
- Zero new Go dependencies (honor CLAUDE.md "prefer stdlib / minimal deps").
- Good deliverability for transactional mail (codes don't land in spam).
- Dev keeps using `LogMailer` — no external service required to run locally.
- Cost stays negligible as the user base grows.

## Non-goals

- Marketing / bulk email, templates, unsubscribe flows.
- Bounce/complaint event ingestion (SES SNS webhooks) — deferred.
- Multi-provider abstraction beyond the existing `Mailer` seam.
- HTML email design — plain-text code is sufficient for OTP.

## Decision: SES over SMTP (not the API)

Amazon SES, reached via its **SMTP endpoint** using stdlib `net/smtp`.

Why SES: near-zero cost (~$0.10 / 1,000 emails, no monthly floor), excellent
deliverability, scales linearly far past where this product would need it.

Why SMTP rather than the SES v2 HTTPS API: the API requires AWS **SigV4**
request signing per call. Hand-rolling SigV4 is ~100 lines of fiddly crypto
with opaque failure modes; doing it cleanly pulls in `aws-sdk-go-v2` (a heavy,
multi-module dependency) for a single POST. SMTP needs neither — SES SMTP
credentials replace request signing, and `net/smtp` is already in the stdlib.

Trade-offs accepted: `net/smtp` is frozen (fine — SES SMTP is a stable target)
and ignores `context` for cancellation/deadline (acceptable for OTP; can be
upgraded to `smtp.NewClient` over a dialed conn later if a deadline is needed).

Alternatives considered: **Resend** (simpler, but a $20/mo floor past the 3k/mo
free tier and an API-key/HTTP dep), **Postmark** (great deliverability, tiny
free tier), raw Gmail SMTP (rate-limited, spam-prone — rejected).

## Setup (one-time, ops)

1. **Verify `unbusy.day` in SES** (chosen region, e.g. `us-east-1`): add the
   3 DKIM CNAME records to DNS. Also add SPF (`v=spf1 include:amazonses.com ~all`)
   and a baseline DMARC record for deliverability.
2. **Request production access** (leave the SES sandbox): short console form,
   use case = transactional OTP. Approval typically < 24h.
3. **Generate SES SMTP credentials** (distinct username/password, derived from
   an IAM principal scoped to `ses:SendRawEmail`).
4. Store credentials as env vars / Fly secrets (see Config).

## Implementation

New production `Mailer` alongside `LogMailer` in `internal/auth/mailer.go`
(or a sibling `mailer_ses.go` if the file grows):

```go
type SESMailer struct {
    Host, Port, User, Pass, From string
}

func (m SESMailer) SendCode(_ context.Context, email, code string) error {
    auth := smtp.PlainAuth("", m.User, m.Pass, m.Host)
    msg := []byte(/* From/To/Subject + text/plain body with the code */)
    return smtp.SendMail(m.Host+":"+m.Port, auth, m.From, []string{email}, msg)
}
```

Wiring in `cmd/unbusy/main.go`: select the mailer at boot — if SES env vars are
present use `SESMailer`, otherwise fall back to `LogMailer` (keeps `task dev`
and tests on stdout, no service required).

Codes are numeric, so header/body interpolation carries no injection risk; if
the message format ever grows, build it with `net/mail`/`textproto` instead of
`fmt.Sprintf`.

## Config (env vars)

| Var | Example | Notes |
|---|---|---|
| `SES_SMTP_HOST` | `email-smtp.us-east-1.amazonaws.com` | region-specific |
| `SES_SMTP_PORT` | `587` | STARTTLS |
| `SES_SMTP_USER` | (SES SMTP username) | Fly secret |
| `SES_SMTP_PASS` | (SES SMTP password) | Fly secret |
| `MAIL_FROM` | `login@unbusy.day` | on the verified domain |

Absence of these → `LogMailer` (dev default). Document in `.env.example`.

## Testing

- Unit: existing `captureMailer` continues to cover auth logic — `SESMailer`
  itself is thin glue, not unit-tested against a live server.
- Manual: with sandbox creds, verify a recipient address and confirm a real
  code arrives; check it passes SPF/DKIM/DMARC (Gmail "show original").
- Keep the dev/test path on `LogMailer` so CI needs no secrets.

## Rollout

1. Land `SESMailer` + main.go selection + `.env.example` (no behavior change
   until env vars are set — dev/CI stay on `LogMailer`).
2. Complete SES setup + production access out of band.
3. Set Fly secrets; deploy. First real login through the deployed app is the
   acceptance test.

## Open questions

- Region choice (latency vs. where the Fly app runs).
- Whether to add a short `context`-aware send timeout now or defer.
- Bounce/complaint monitoring: rely on SES console metrics initially; revisit
  SNS event ingestion if volume grows (candidate for `docs/backlog/`).
