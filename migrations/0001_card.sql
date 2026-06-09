-- PRD F10: card table with DEFERRABLE unique on position so the bulk
-- reorder UPDATE (F1) doesn't trip the constraint on intermediate per-row
-- states. Idempotent: safe to re-run via `task migrate`.

CREATE TABLE IF NOT EXISTS card (
  id       TEXT    PRIMARY KEY,
  label    TEXT    NOT NULL,
  position INTEGER NOT NULL,
  CONSTRAINT card_position_unique UNIQUE (position) DEFERRABLE INITIALLY DEFERRED
);

INSERT INTO card (id, label, position) VALUES
  ('a', 'Alpha',   0),
  ('b', 'Bravo',   1),
  ('c', 'Charlie', 2)
ON CONFLICT (id) DO NOTHING;
