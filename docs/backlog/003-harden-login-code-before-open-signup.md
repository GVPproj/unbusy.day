# 003 — Harden `/login/code` before enabling open signup

Status: complete (operational alerting remains optional follow-up)
Date: 2026-06-15
Completed: 2026-06-30

## Threat

Open signup lets an anonymous caller request an OTP for any address. A
per-address throttle alone does not bound an attacker who spreads requests over
many victims: the endpoint can become an email-bombing relay, damage sender
reputation through bounces and complaints, and amplify cost. Re-requesting a
code must also not reset the verification-attempt budget indefinitely.

The launch gate was therefore broader than ordinary OTP correctness: bound the
anonymous send path globally and by source, prove human presence, reject clearly
undeliverable addresses, create no User before email control is proven, retain
attempt pressure across re-issues, and stop mailing addresses SES has already
flagged.

## Implemented defenses

- `internal/web/ratelimit.go` wraps `POST /login/code` with per-IP and
  process-wide token buckets. `Fly-Client-IP` is trusted only behind the secure
  production proxy configuration.
- Cloudflare Turnstile gates code requests in production. A live mailer without
  `TURNSTILE_SECRET` is rejected at boot unless the explicit insecure override
  is set.
- `auth.RequestCode` performs syntax and MX validation and silently skips
  suppressed or undeliverable addresses.
- `OTP_SEND_CEILING` adds a rolling process-wide outbound-mail circuit breaker.
  A live mailer without the ceiling is rejected at boot unless explicitly
  overridden.
- Login Codes are keyed by email and may exist without a User. `VerifyCode`
  creates the User only after a correct code proves control of the address.
- Failed-attempt counts carry across code re-issues inside a recovery window,
  preventing a request from resetting the five-attempt budget.
- SES bounce/complaint feedback maintains the suppression list (ADR 0009).

All request outcomes remain non-committal to the caller, apart from the
source-based HTTP rate limit, so the defenses do not create an account or
suppression enumeration signal.

## Residual operations work

Tripping the global send ceiling currently writes a high-signal log entry; it
does not page or notify an operator. Add log-based alerting if OTP volume makes
manual log monitoring inadequate. This is operational hardening, not an
open-signup launch blocker.

## Related

- ADR 0001 — passwordless email OTP and deferred User creation.
- ADR 0009 — SES bounce/complaint suppression.
- `internal/auth/auth.go`, `internal/auth/validate.go`,
  `internal/auth/ceiling.go`, `internal/auth/presence.go`.
- `internal/web/ratelimit.go`, `cmd/unbusy/auth.go`, `cmd/unbusy/router.go`.
