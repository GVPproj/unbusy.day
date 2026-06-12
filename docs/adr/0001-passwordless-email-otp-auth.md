# Passwordless authentication via email one-time codes

We authenticate Users with a short-lived numeric code emailed to their address
(OTP) rather than with passwords. Email is the sole identity; there is no
password to hash, reset, or leak. This was the stated long-term destination, and
choosing it now avoids building a password system we'd later throw away (and
password reset needs email anyway).

## Considered Options

- **Passwords now, OTP later** — rejected: a second, throwaway auth system, plus
  bcrypt and a registration UI, plus email needed eventually for reset.
- **Magic link** — rejected for now: same email seam, but worse on mobile/shared
  devices than a typed code.

## Consequences

- We need an email seam. It's the `Mailer` interface in `auth/`; the dev
  implementation logs the code to stdout, so **no external email service is
  required to run the app**. Production swaps in a real provider (Resend /
  Postmark / SES) without touching `auth/`.
- Security of a 6-digit code rests on **expiry + attempt limits**, not entropy:
  10-min single-use codes, one active code per user, 5 verify attempts, ~60s
  request throttle, stored hashed.
- **Allowlist = the `user` table.** A code is issued only for an email that
  already has a User row; unknown emails get an identical no-op response (no
  account enumeration). At public launch this flips to auto-provision: requesting
  a code upserts the User, and the allowlist behavior disappears.
