# Server-side sessions in Postgres, not stateless cookies

A login is represented by a row in a `session` table keyed by an opaque,
high-entropy `crypto/rand` token carried in an HttpOnly cookie. We did **not**
use a stateless signed/encrypted cookie (JWT-style) that carries identity on the
client. Postgres is already the source of truth and we run a single always-on
machine, so the usual reasons to go stateless (many instances, no shared store)
don't apply — and we need real revocation.

## Considered Options

- **Stateless signed cookie** — rejected: cannot be revoked before expiry, so
  "Log Out" would only clear the local cookie while a captured copy stays valid.
- **Encrypted (sealed) cookie** — rejected: same non-revocation tradeoff, plus a
  new dependency.

## Consequences

- Logout is a `DELETE` — immediate, authoritative, server-side. Enables "log out
  everywhere" and killing a suspected-stolen session later.
- One indexed primary-key `SELECT` per request to resolve the session. Negligible
  on one machine already doing DB round-trips.
- 30-day **absolute** expiry (no sliding), so the read path takes no write.
- Cookie: HttpOnly, SameSite=Lax (baseline CSRF defense for the `/blocks/*`
  POSTs), Secure in production only (off on `http://localhost`).
