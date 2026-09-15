-- +goose Up
ALTER TABLE habit ADD COLUMN sort_order INTEGER NOT NULL DEFAULT 0;

WITH ranked AS (
  SELECT id, ROW_NUMBER() OVER (PARTITION BY owner_id ORDER BY id) - 1 AS sort_order
  FROM habit
)
UPDATE habit
SET sort_order = (SELECT ranked.sort_order FROM ranked WHERE ranked.id = habit.id);

CREATE INDEX habit_owner_sort_order_idx ON habit (owner_id, sort_order);
