-- +goose Up
-- Add the Block Type tag: a flat three-way deep/shallow/break, default shallow.
-- Additive column following the established span CHECK pattern; existing rows
-- backfill to 'shallow'. The CHECK is a belt-and-suspenders backstop to the
-- service's ErrInvalidBlockType guard.
ALTER TABLE block ADD COLUMN type TEXT NOT NULL DEFAULT 'shallow' CHECK (type IN ('deep', 'shallow', 'break'));
