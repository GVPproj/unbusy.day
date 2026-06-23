-- +goose Up
-- Add me@gvp.fyi to the allowlist (ADR 0001) so a login code is issued for it.
INSERT INTO "user" (id, email) VALUES
  ('u_gvp', 'me@gvp.fyi');
