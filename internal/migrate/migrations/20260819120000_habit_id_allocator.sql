-- +goose Up
CREATE TABLE habit_id_allocator (
  singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
  last_id INTEGER NOT NULL CHECK (typeof(last_id) = 'integer' AND last_id >= 0)
);
INSERT INTO habit_id_allocator (singleton, last_id)
SELECT 1, COALESCE(MAX(id), 0) FROM habit;
