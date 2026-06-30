-- +goose Up
-- Decouple the brute-force recovery clock from created_at. created_at doubles as
-- the ~60s throttle clock and is rewritten on every code re-issue, so the old
-- attempts-decay (keyed on created_at) never elapsed while requests kept coming:
-- a user retrying after a lockout — or an attacker pinning a known email — stayed
-- locked indefinitely. attempts_since marks when the current attempt budget began;
-- it is preserved across re-issues and reset only when attempts reset, so the
-- recovery window genuinely elapses regardless of re-requests.
-- Default '' is a fail-safe sentinel: it sorts before any RFC3339 stamp, so a
-- stray row reads as "past the window" (attempts reset) rather than locked. The
-- app always writes attempts_since explicitly; the backfill covers in-flight rows.
ALTER TABLE login_code ADD COLUMN attempts_since TEXT NOT NULL DEFAULT '';
UPDATE login_code SET attempts_since = created_at;
