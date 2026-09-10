-- +goose Up
CREATE TABLE habit_checkin (
  habit_id INTEGER NOT NULL REFERENCES habit(id) ON DELETE CASCADE,
  date TEXT NOT NULL,
  PRIMARY KEY (habit_id, date)
);
