-- +goose Up
-- Day Plan (backlog 002, ADR 0005): per-user day bounds as 30-minute slot
-- indexes from 00:00 (end exclusive); default 9:00–17:00. `position` changes
-- meaning from list rank to clock slot.

ALTER TABLE "user" ADD COLUMN day_start SMALLINT NOT NULL DEFAULT 18;
ALTER TABLE "user" ADD COLUMN day_end SMALLINT NOT NULL DEFAULT 34;

-- Repack existing cards from each owner's day start by current rank,
-- accounting for spans, so list order becomes contiguous slot placement.
-- +goose StatementBegin
WITH packed AS (
  SELECT c.id,
         u.day_start + COALESCE(SUM(c2.span), 0) AS slot
  FROM card c
  JOIN "user" u ON u.id = c.owner_id
  LEFT JOIN card c2 ON c2.owner_id = c.owner_id AND c2.position < c.position
  GROUP BY c.id, u.day_start
)
UPDATE card SET position = packed.slot FROM packed WHERE card.id = packed.id;
-- +goose StatementEnd

-- The list-rank unique gives way to a range-overlap backstop: even if app
-- validation regresses, two cards can never share a slot. DEFERRABLE so the
-- bulk layout UPDATE's intermediate row states may transiently overlap.
ALTER TABLE card DROP CONSTRAINT card_owner_position_unique;
CREATE EXTENSION IF NOT EXISTS btree_gist;
ALTER TABLE card ADD CONSTRAINT card_owner_slots_excl
  EXCLUDE USING gist (owner_id WITH =, int4range(position, position + span) WITH &&)
  DEFERRABLE INITIALLY DEFERRED;
