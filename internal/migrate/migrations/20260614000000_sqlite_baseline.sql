-- +goose Up
-- Fresh SQLite baseline (Postgres → SQLite migration). The historical
-- 0001–rename_card_to_block Postgres files are collapsed here: a new SQLite
-- database starts clean, so there is no idempotency scaffolding to carry over.
-- Timestamps are TEXT (RFC3339, string-sortable); SMALLINT → INTEGER.
-- The Postgres btree_gist EXCLUDE overlap backstop has no SQLite equivalent;
-- ValidateLayout in block/ is the sole overlap guard (see ADR).

-- Users are the allowlist (ADR 0001); day bounds are 30-minute slot indexes
-- from 00:00, end exclusive (default 9:00–17:00, i.e. slots 18–34).
CREATE TABLE "user" (
  id         TEXT    PRIMARY KEY,
  email      TEXT    NOT NULL UNIQUE,
  created_at TEXT    NOT NULL DEFAULT (datetime('now')),
  day_start  INTEGER NOT NULL DEFAULT 18,
  day_end    INTEGER NOT NULL DEFAULT 34
);

-- Seed the dev allowlist: a code is only issued for an email that has a row.
INSERT INTO "user" (id, email) VALUES
  ('u_test',  'test@example.com'),
  ('u_test2', 'test2@example.com');

-- Per-user time blocks (ADRs 0003/0005). `position` is a clock slot index;
-- `span` is the block's height in slots. No position-uniqueness constraint:
-- overlap is guarded by ValidateLayout (the gist EXCLUDE backstop is dropped).
CREATE TABLE block (
  id         TEXT    PRIMARY KEY,
  label      TEXT    NOT NULL,
  position   INTEGER NOT NULL,
  span       INTEGER NOT NULL DEFAULT 1 CHECK (span >= 1),
  created_at TEXT    NOT NULL DEFAULT (datetime('now')),
  owner_id   TEXT    REFERENCES "user"(id) ON DELETE CASCADE
);

-- One active code per user (PK = user_id); the code itself is stored hashed.
CREATE TABLE login_code (
  user_id    TEXT    PRIMARY KEY REFERENCES "user"(id) ON DELETE CASCADE,
  code_hash  TEXT    NOT NULL,
  attempts   INTEGER NOT NULL DEFAULT 0,
  expires_at TEXT    NOT NULL,
  created_at TEXT    NOT NULL DEFAULT (datetime('now'))
);

-- DB-backed sessions (ADR 0002): opaque token PK, absolute expiry.
CREATE TABLE session (
  token      TEXT    PRIMARY KEY,
  user_id    TEXT    NOT NULL REFERENCES "user"(id) ON DELETE CASCADE,
  expires_at TEXT    NOT NULL,
  created_at TEXT    NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX session_user_id_idx ON session (user_id);
