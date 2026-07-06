package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

var ErrInvalidReason = errors.New("suppression reason must be bounce or complaint")

// Suppress records an address RequestCode must never mail again. Idempotent;
// fed by the SES → SNS feedback webhook.
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
