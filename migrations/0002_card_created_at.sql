-- M3c schema drill (PRD criterion 8): add a column live and observe client
-- behavior. Additive + idempotent: safe to re-run via `task migrate`, and the
-- running binary's explicit column lists (`SELECT id, label, position`) are
-- untouched, so it can be applied before the code knows about it.
--
-- Drill finding (criterion 8): neither FE observes the new column — API and
-- SSE payloads are built from explicit column lists, not SELECT *, so a
-- migration alone never changes the wire shape. Clients need no reconnect or
-- refetch; exposing a new field requires a code change + deploy (Card struct,
-- the two SELECTs, FE rendering), at which point clients pick it up on their
-- next refetch/SSE frame. Expand-then-deploy ordering (column first, code
-- after) is therefore safe by construction.

ALTER TABLE card ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT now();
