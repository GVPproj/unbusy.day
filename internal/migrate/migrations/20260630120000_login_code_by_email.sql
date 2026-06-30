-- +goose Up
-- Re-key login_code by email so a code can exist before its user row (item 3,
-- deferred account creation). user_id becomes a nullable FK, set on verify when
-- the account is minted. SQLite can't drop a PK in place, so rebuild the table.
CREATE TABLE login_code_new (
  email      TEXT    PRIMARY KEY,
  user_id    TEXT    REFERENCES "user"(id) ON DELETE CASCADE,
  code_hash  TEXT    NOT NULL,
  attempts   INTEGER NOT NULL DEFAULT 0,
  expires_at TEXT    NOT NULL,
  created_at TEXT    NOT NULL DEFAULT (datetime('now'))
);

INSERT INTO login_code_new (email, user_id, code_hash, attempts, expires_at, created_at)
  SELECT u.email, lc.user_id, lc.code_hash, lc.attempts, lc.expires_at, lc.created_at
  FROM login_code lc JOIN "user" u ON u.id = lc.user_id;

DROP TABLE login_code;
ALTER TABLE login_code_new RENAME TO login_code;
CREATE INDEX login_code_user_id_idx ON login_code (user_id);
