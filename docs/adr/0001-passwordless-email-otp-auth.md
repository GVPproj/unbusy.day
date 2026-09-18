# Passwordless authentication via email one-time codes

Status: accepted

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
- Codes are uniformly sampled eight-digit strings, including leading zeros
  (UNB-70). Entropy complements **expiry + attempt limits**: 10-min single-use
  codes, one active code per email, 5 verify attempts carried across re-issues
  within the 10-min attempt recovery window, ~60s request throttle, stored hashed.
  We retained 10-min recovery rather than 15: eight digits provide the main
  guessing-risk reduction without increasing legitimate-user lockout by 50%.
  The ticket's pessimistic annual guessing estimate falls from 0.262% to 0.175%
  with 15-min recovery; this is not measured production risk.
- Signup is open. A deliverable email can receive a code before it has a User;
  the User row is created only when that code is verified successfully. The old
  `user`-table allowlist was retired after the send path gained layered defenses:
  suppression, syntax/MX validation, Turnstile, per-IP/global rate limiting, and
  a global send ceiling.
