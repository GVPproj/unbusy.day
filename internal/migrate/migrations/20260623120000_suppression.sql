-- +goose Up
-- Bounce/complaint suppression list, fed by SES → SNS feedback notifications.
-- An address that hard-bounces or files a complaint lands here and is never
-- mailed again, protecting the SES sending reputation. Forward-only: lift a
-- suppression with a manual DELETE if an address recovers.
CREATE TABLE suppression (
    email      TEXT PRIMARY KEY,
    reason     TEXT NOT NULL CHECK (reason IN ('bounce', 'complaint')),
    detail     TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL
);
