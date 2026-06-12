-- +goose Up
-- Additive + idempotent: safe to re-run via `task migrate`. `span` is a card's
-- height in stretch slots. Default 1 keeps existing rows at baseline, so the
-- migration can land before the reading code (expand-then-deploy). CHECK keeps
-- a card from collapsing below one slot.

ALTER TABLE card ADD COLUMN IF NOT EXISTS span SMALLINT NOT NULL DEFAULT 1;

-- +goose StatementBegin
DO $$ BEGIN
  ALTER TABLE card ADD CONSTRAINT card_span_positive CHECK (span >= 1);
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;
-- +goose StatementEnd
