-- +goose Up
-- Allowlist test2 (the user table is the allowlist, ADR 0001). ON CONFLICT
-- because dev DBs were seeded by hand before this migration landed.
INSERT INTO "user" (id, email) VALUES
  ('u_test2', 'test2@example.com')
ON CONFLICT (email) DO NOTHING;
