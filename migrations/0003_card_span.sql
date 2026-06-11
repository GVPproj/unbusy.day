-- Additive + idempotent: safe to re-run via `task migrate`. `span` is a card's
-- height in stretch slots — the resize gesture (grip drag) used to be
-- frontend-only state; this persists it so heights survive reload and reach
-- other tabs over the SSE stream.
--
-- Default 1 keeps existing rows at the baseline height, and the running binary's
-- explicit column lists (`SELECT id, label, position`) are untouched, so the
-- migration can land before the code that reads the column (expand-then-deploy).
-- SMALLINT is ample: spans are bounded by the column's card count. CHECK keeps a
-- card from collapsing below one slot regardless of which adapter writes it.

ALTER TABLE card ADD COLUMN IF NOT EXISTS span SMALLINT NOT NULL DEFAULT 1;

DO $$ BEGIN
  ALTER TABLE card ADD CONSTRAINT card_span_positive CHECK (span >= 1);
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;
