-- +goose Up
-- Jotpad sync: a version counter for compare-and-swap writes, and a one-row
-- shadow of the *previous* revision per user — the base text a stale client's
-- write is three-way merged against. Not history: each write overwrites it.
ALTER TABLE "user" ADD COLUMN jot_version INTEGER NOT NULL DEFAULT 0;

CREATE TABLE jot_base (
    owner TEXT PRIMARY KEY REFERENCES "user" (id) ON DELETE CASCADE,
    version INTEGER NOT NULL,
    text TEXT NOT NULL
);
