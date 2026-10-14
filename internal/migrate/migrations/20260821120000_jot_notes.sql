-- +goose Up
-- Fixed perpetual notes, each with its own version and single merge base.
-- Legacy columns/tables remain for additive schema compatibility.
CREATE TABLE jot_note (
    owner TEXT NOT NULL REFERENCES "user" (id) ON DELETE CASCADE,
    note TEXT NOT NULL CHECK (note IN ('daily', 'weekly', 'quarterly')),
    text TEXT NOT NULL DEFAULT '',
    version INTEGER NOT NULL DEFAULT 0,
    base_text TEXT,
    base_version INTEGER,
    PRIMARY KEY (owner, note)
);

INSERT INTO jot_note (owner, note, text, version, base_text, base_version)
SELECT u.id, 'daily', u.jot, u.jot_version, b.text, b.version
FROM "user" AS u LEFT JOIN jot_base AS b ON b.owner = u.id;

INSERT INTO jot_note (owner, note)
SELECT id, 'weekly' FROM "user"
UNION ALL
SELECT id, 'quarterly' FROM "user";
