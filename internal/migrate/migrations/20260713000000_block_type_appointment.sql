-- +goose Up
-- Widen the block.type CHECK to admit 'appointment' (the 4th Block Type).
-- SQLite can't alter a CHECK in place, so rebuild the table verbatim with the
-- expanded IN list. Nothing references block by FK, so no foreign_keys toggle.
CREATE TABLE block_new (
  id         TEXT    PRIMARY KEY,
  label      TEXT    NOT NULL,
  position   INTEGER NOT NULL,
  span       INTEGER NOT NULL DEFAULT 1 CHECK (span >= 1),
  created_at TEXT    NOT NULL DEFAULT (datetime('now')),
  owner_id   TEXT    REFERENCES "user"(id) ON DELETE CASCADE,
  type       TEXT    NOT NULL DEFAULT 'shallow'
                     CHECK (type IN ('deep', 'shallow', 'break', 'appointment'))
);
INSERT INTO block_new (id, label, position, span, created_at, owner_id, type)
  SELECT id, label, position, span, created_at, owner_id, type FROM block;
DROP TABLE block;
ALTER TABLE block_new RENAME TO block;
