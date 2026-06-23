package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ErrInvalidReason guards the suppression reason against the table CHECK.
var ErrInvalidReason = errors.New("suppression reason must be bounce or complaint")

// Suppress records an address as undeliverable (hard bounce) or complaining so
// RequestCode never mails it again. Idempotent: a repeat upserts the latest
// reason/detail. Fed by the SES → SNS feedback webhook.
func (s *Service) Suppress(ctx context.Context, email, reason, detail string) error {
	if reason != "bounce" && reason != "complaint" {
		return fmt.Errorf("%w: %q", ErrInvalidReason, reason)
	}
	email = normalizeEmail(email)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO suppression (email, reason, detail, created_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT (email) DO UPDATE
		SET reason = excluded.reason, detail = excluded.detail, created_at = excluded.created_at`,
		email, reason, detail, formatTime(time.Now()))
	return err
}

// IsSuppressed reports whether email is on the suppression list.
func (s *Service) IsSuppressed(ctx context.Context, email string) (bool, error) {
	email = normalizeEmail(email)
	var one int
	err := s.db.QueryRowContext(ctx, `SELECT 1 FROM suppression WHERE email = ?`, email).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}
