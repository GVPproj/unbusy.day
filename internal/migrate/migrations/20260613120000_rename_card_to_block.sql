-- +goose Up
-- Domain rename: the `card` table becomes `block` (a day-plan time block). The
-- binary's SQL now targets `block`; the primary key and constraints are renamed
-- to match so the schema reads cleanly.
--
-- NOT additive: this breaks expand-then-deploy — the prior binary's SQL targets
-- `card` and will error the moment this lands. The app runs a single always-on
-- machine, so the rollout window where old code meets the new table is a brief
-- blip, not a multi-instance hazard. Forward-only: fix mistakes with a new
-- migration, never an edit.

ALTER TABLE card RENAME TO block;
ALTER INDEX card_pkey RENAME TO block_pkey;
ALTER TABLE block RENAME CONSTRAINT card_owner_slots_excl TO block_owner_slots_excl;
ALTER TABLE block RENAME CONSTRAINT card_span_positive TO block_span_positive;
