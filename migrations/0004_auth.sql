-- +goose Up
-- Auth + per-user tenancy (ADRs 0001–0003). Additive + idempotent: safe to
-- re-run via `task migrate`, and safe to land before the code that reads it —
-- the running binary's explicit column lists never read owner_id, and it
-- never inserts cards, so deleting the ownerless seed rows is expand-safe.

CREATE TABLE IF NOT EXISTS "user" (
  id         TEXT        PRIMARY KEY,
  email      TEXT        NOT NULL UNIQUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Allowlist = the user table (ADR 0001): a code is only issued for an email
-- that already has a row. Seed the dev allowlist.
INSERT INTO "user" (id, email) VALUES
  ('u_test', 'test@example.com')
ON CONFLICT (email) DO NOTHING;

-- One active code per user (PK = user_id); the code itself is stored hashed.
CREATE TABLE IF NOT EXISTS login_code (
  user_id    TEXT        PRIMARY KEY REFERENCES "user"(id) ON DELETE CASCADE,
  code_hash  TEXT        NOT NULL,
  attempts   INTEGER     NOT NULL DEFAULT 0,
  expires_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- DB-backed sessions (ADR 0002): opaque token PK, absolute expiry.
CREATE TABLE IF NOT EXISTS session (
  token      TEXT        PRIMARY KEY,
  user_id    TEXT        NOT NULL REFERENCES "user"(id) ON DELETE CASCADE,
  expires_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS session_user_id_idx ON session (user_id);

-- Per-user boards (ADR 0003): cards gain an owner, uniqueness becomes
-- per-owner, and the old ownerless global seed rows go away.
ALTER TABLE card ADD COLUMN IF NOT EXISTS owner_id TEXT REFERENCES "user"(id) ON DELETE CASCADE;

DELETE FROM card WHERE owner_id IS NULL;

ALTER TABLE card DROP CONSTRAINT IF EXISTS card_position_unique;
-- +goose StatementBegin
DO $$ BEGIN
  ALTER TABLE card ADD CONSTRAINT card_owner_position_unique
    UNIQUE (owner_id, position) DEFERRABLE INITIALLY DEFERRED;
EXCEPTION WHEN duplicate_object OR duplicate_table THEN NULL;
END $$;
-- +goose StatementEnd
