-- +goose Up
CREATE TABLE habit (
  id INTEGER PRIMARY KEY,
  owner_id TEXT NOT NULL REFERENCES "user"(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  name_key TEXT NOT NULL,
  start_date TEXT NOT NULL,
  UNIQUE (owner_id, name_key)
);
CREATE INDEX habit_owner_id_idx ON habit (owner_id, id);
