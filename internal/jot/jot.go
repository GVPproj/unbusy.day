// Package jot is the transport-agnostic Jotpad service: one perpetual
// free-text scratchpad per user, owner-scoped like everything else (ADR 0003).
// It deliberately shares nothing with block — a Jotpad has no overlap, no push
// cascade, and no validation beyond a length cap.
package jot

import (
	"context"
	"database/sql"
	"errors"
	"unicode/utf8"
)

// MaxLen caps a Jotpad at 100,000 characters. Enforced here and by maxlength
// on the textarea; a browser cannot exceed it, only a crafted client can.
const MaxLen = 100_000

var ErrTooLong = errors.New("jot exceeds the length cap")

type Service struct {
	db *sql.DB
}

func NewService(db *sql.DB) *Service {
	return &Service{db: db}
}

// Get returns the owner's Jotpad. A Session cannot exist without a User row, so
// the row always exists and an empty scratchpad is the column default — there
// is no "not created yet" case to branch on.
func (s *Service) Get(ctx context.Context, owner string) (string, error) {
	var text string
	err := s.db.QueryRowContext(ctx,
		`SELECT jot FROM "user" WHERE id = ?`, owner).Scan(&text)
	return text, err
}

// Set replaces the owner's Jotpad with exactly the characters given — no
// trimming, no normalisation. Over-cap writes are rejected whole, so the stored
// text is never half-replaced.
func (s *Service) Set(ctx context.Context, owner, text string) error {
	if utf8.RuneCountInString(text) > MaxLen {
		return ErrTooLong
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE "user" SET jot = ? WHERE id = ?`, text, owner)
	return err
}
