-- +goose Up
-- The Jotpad: one perpetual free-text scratchpad per user. A column on "user",
-- not a jot table, because a Session cannot exist without a User row — so the
-- row always exists and the service needs no upsert or "not created yet" branch.
-- Same shape as day_start/day_end: per-user singleton state on "user".
ALTER TABLE "user" ADD COLUMN jot TEXT NOT NULL DEFAULT '';
