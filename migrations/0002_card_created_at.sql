-- Additive + idempotent: safe to re-run via `task migrate`, and the running
-- binary's explicit column lists (`SELECT id, label, position`) are untouched,
-- so it can be applied before the code knows about the column. Wire payloads
-- are built from those explicit lists, not SELECT *, so a migration alone
-- never changes the wire shape — exposing a new field needs a code change +
-- deploy. Expand-then-deploy ordering is therefore safe by construction.

ALTER TABLE card ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT now();
