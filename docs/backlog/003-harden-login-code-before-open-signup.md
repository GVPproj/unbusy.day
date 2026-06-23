# 003 — Harden `/login/code` before enabling open signup

Status: backlog (partially addressed — see Progress)
Date: 2026-06-15

## Progress

- **2026-06-23 — bounce/complaint suppression landed** (item 4, monitoring half;
  ADR 0009). SES feedback arrives over SNS at `POST /webhooks/ses`, and
  permanently-bounced / complaining addresses go on a `suppression` table that
  `RequestCode` now consults — a suppressed address is silently skipped, so we
  no longer keep mailing addresses SES has flagged. **Still unbuilt:** the
  per-IP/global rate limit (1), human-presence check (2), deferring user-row
  creation (3), the global send ceiling + circuit breaker (rest of 4), syntactic
  + MX validation (5), and the brute-force attempt-carry (6). The core open-relay
  risk remains until the rate limit + presence check land.

## Problem

Today the `user` table doubles as an allowlist: `RequestCode`
(`internal/auth/auth.go`) only issues+sends a code if the email already exists,
and unknown emails are a silent no-op. That single check is the only thing
bounding email-bombing blast radius to people already in the system.

Production will **not** have an allowlist (open signup). Removing the
`user`-row gate makes `RequestCode` willing to email an OTP to **any address an
anonymous attacker types into the form** — turning the login endpoint into an
open email relay. This is a bigger risk than OTP brute force, and the two need
to be addressed in the same change that drops the allowlist.

## Threat: email bombing / open relay

1. **Spam/harassment cannon (primary).** An attacker scripts `POST /login/code`
   with victim addresses they don't own; the server emails an OTP to each. The
   ~60s throttle is **per-email** (`WHERE login_code.created_at < ?`,
   `auth.go:94`) — there is **no aggregate or per-source cap**. Across N
   addresses the outbound rate is N/min, unbounded. Classic "subscription
   bombing": our form gets recruited to bury a third party's inbox.
2. **Sender-reputation collapse (expensive consequence).** Blasting unverified
   addresses drives bounces + spam complaints; SES/Postmark/Resend suspend
   sending past their thresholds, and the domain can land on blocklists.
   Legitimate login mail then stops being delivered — harder to reverse than any
   brute force.
3. **Cost.** Per-email pricing × attacker-controlled volume = billing-
   amplification DoS.

Removing the allowlist also removes the natural gate on the send path entirely,
so the rate limit + human-presence check below become the *only* thing standing
there.

## Secondary: OTP brute force across re-requested codes

Lower priority but related. Each `RequestCode` resets `attempts` to 0 and
installs a fresh code, so an attacker targeting a known email can burn 5 guesses
(`maxAttempts`), re-request, and repeat — ~5 guesses/60s ≈ 7,200/day against the
1M 6-digit space (~50% hit in ~2 weeks of sustained attack). There is no global
cap on total verify attempts or codes issued per user over a long window, and no
IP-level rate limit at the HTTP layer.

## Path forward when resumed

In rough priority:

1. **Per-IP / global rate limits on `/login/code`.** The per-email throttle does
   nothing against the spread-across-many-addresses pattern; cap the aggregate
   and per-source send rate. This is the missing layer.
2. **Human-presence check on the request form** — CAPTCHA (e.g. Cloudflare
   Turnstile) or proof-of-work. Standard defense against automated
   signup/bombing.
3. **Defer `user`-row creation until after `VerifyCode` succeeds**, so an
   unverified request can't pollute the user table with addresses the requester
   doesn't control. Keep row creation gated on proven email control.
4. **Global send ceiling + bounce/complaint monitoring** — a circuit breaker
   that trips a cap (with alerting) instead of torching domain reputation.
5. **Syntactic + MX validation** before sending, to shed obviously-bogus
   addresses.
6. **(Brute force) Don't reset `attempts` on re-request within a code's life** —
   carry attempts forward or cap total attempts per user per rolling window.
   Optionally widen the code to 8 digits (ADR 0001 rests security on
   expiry+attempt limits, not entropy — revisit there).

## Related

- ADR 0001 / 0002 — passwordless email-OTP auth and DB-backed sessions.
- ADR 0009 — SES bounce/complaint suppression (the monitoring half, now built).
- `internal/auth/auth.go` (`RequestCode`, `VerifyCode`),
  `internal/frontend/login.go` (handlers — currently no throttle middleware).
