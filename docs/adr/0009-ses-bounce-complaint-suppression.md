# SES bounce/complaint suppression via an SNS feedback webhook

Production sends OTP mail through Amazon SES (over SMTP — the generic
`SMTPMailer`, ADR 0001). SES delivers bounce and complaint feedback over SNS; we
consume it at `POST /webhooks/ses`, record each permanently-bounced or
complaining address in a `suppression` table, and `RequestCode` then refuses to
mail any suppressed address. The point is **sender-reputation protection**: SES
suspends accounts whose bounce/complaint rates cross a threshold, so the cheapest
durable defense is to stop mailing addresses SES has already told us are bad.

## Considered Options

- **Ignore feedback, rely on SES's own account-level suppression list** —
  rejected: it's account-wide and opaque to us, we can't see or reason about who
  we've stopped mailing, and bounce/complaint *rates* still climb because SES
  counts the attempt before suppressing. Owning the list lets us short-circuit
  the send.
- **Poll SES for feedback** — rejected: no first-class polling API; SNS push is
  the supported path and arrives in near-real-time.
- **Trust the webhook body without verifying** — rejected: the endpoint is
  unauthenticated and public, so an attacker could POST a forged "complaint" to
  suppress an arbitrary address (a denial-of-login). Signature verification is
  load-bearing, not optional.

## Consequences

- **The endpoint is mounted only when `SES_SNS_TOPIC_ARN` is set** (`main.go`),
  so dev and any non-SES deploy never expose it. An empty expected ARN rejects
  everything — misconfiguration fails closed.
- **Every message is verified before it does anything**
  (`internal/frontend/seswebhook.go`): the `TopicArn` must equal our configured
  ARN, the SNS RSA signature is checked against the signing cert
  (`SignatureVersion` 1 → SHA1, 2 → SHA256), and the cert/confirm URLs are
  constrained to `*.amazonaws.com` over HTTPS so a forged message can't point us
  at an attacker-controlled cert.
- **Subscriptions auto-confirm**: a `SubscriptionConfirmation` is honored by
  fetching the (already signature-verified) `SubscribeURL`, so wiring SNS → our
  endpoint needs no manual step.
- **Only Permanent bounces and complaints suppress.** Transient bounces are
  ignored — they may deliver on retry. Suppression is keyed by lowercased email
  in the `suppression` table (`reason` ∈ {`bounce`,`complaint`}), upserted so
  repeat feedback is idempotent.
- **`RequestCode` skips suppressed addresses silently** — same no-op,
  no-enumeration response as an unknown email (ADR 0001), so the suppression
  state doesn't leak.
- **Suppression is forward-only and manual to lift**: there is no un-suppress UI;
  an address comes off the list only by a manual `DELETE`. Acceptable while the
  `user` table is still an allowlist (ADR 0001) and volume is tiny.
- This implements the **bounce/complaint-monitoring** half of backlog 003's
  path-forward; the open-relay / rate-limit defenses there remain unbuilt.

## Related

- ADR 0001 — passwordless email-OTP auth (the `Mailer` seam, allowlist,
  no-enumeration responses).
- docs/backlog/003 — harden `/login/code` before open signup (the broader
  email-bombing threat this partially addresses).
- `internal/frontend/seswebhook.go`, `internal/auth/suppression.go`,
  migration `internal/migrate/migrations/20260623120000_suppression.sql`.
