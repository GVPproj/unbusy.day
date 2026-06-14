// Package auth is passwordless email-OTP authentication over DB-backed
// sessions (ADRs 0001/0002): 10-min single-use 6-digit codes, one active code
// per user, 5 verify attempts, ~60s request throttle, codes stored hashed.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"math/big"
	"strings"
	"time"
)

// SessionTTL is the absolute (non-sliding) session lifetime.
const SessionTTL = 30 * 24 * time.Hour

const (
	codeTTL         = 10 * time.Minute
	maxAttempts     = 5
	requestThrottle = 60 * time.Second
)

// ErrInvalidCode covers every verify failure — unknown email, wrong code,
// expired, too many attempts — one error so responses can't enumerate.
var ErrInvalidCode = errors.New("invalid or expired code")

// ErrNoSession signals a missing, expired, or revoked session token.
var ErrNoSession = errors.New("no valid session")

// Session is one login: the opaque token the cookie carries plus its owner.
type Session struct {
	Token     string
	UserID    string
	ExpiresAt time.Time
}

type Service struct {
	db     *sql.DB
	mailer Mailer
}

func NewService(db *sql.DB, mailer Mailer) *Service {
	return &Service{db: db, mailer: mailer}
}

// formatTime renders a timestamp as UTC RFC3339 TEXT — fixed-width and
// string-sortable, so `expires_at`/`created_at` comparisons work as plain
// string `<`/`>` in SQLite.
func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

// RequestCode issues a login code for email and mails it. Unknown emails and
// throttled requests return nil identically — no account enumeration.
func (s *Service) RequestCode(ctx context.Context, email string) error {
	email = normalizeEmail(email)

	var userID string
	err := s.db.QueryRowContext(ctx, `SELECT id FROM "user" WHERE email = ?`, email).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		log.Printf("auth: uninvited user %s, skipping OTP", email)
		return nil // identical no-op for unknown emails
	}
	if err != nil {
		return err
	}

	code, err := newCode()
	if err != nil {
		return err
	}

	// Interval/now() math moves into Go: concrete timestamps passed as params.
	now := time.Now()
	nowStr := formatTime(now)
	expiresAt := formatTime(now.Add(codeTTL))
	throttleCutoff := formatTime(now.Add(-requestThrottle))

	// One active code per user; the WHERE guard is the ~60s request throttle.
	// Zero rows written means throttled — same nil response, no email.
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO login_code (user_id, code_hash, attempts, expires_at, created_at)
		VALUES (?, ?, 0, ?, ?)
		ON CONFLICT (user_id) DO UPDATE
		SET code_hash = excluded.code_hash, attempts = 0,
		    expires_at = excluded.expires_at, created_at = excluded.created_at
		WHERE login_code.created_at < ?`,
		userID, hashCode(code), expiresAt, nowStr, throttleCutoff)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		return nil // throttled
	}
	return s.mailer.SendCode(ctx, email, code)
}

// VerifyCode redeems a code for a new session. Any failure is ErrInvalidCode.
func (s *Service) VerifyCode(ctx context.Context, email, code string) (*Session, error) {
	email = normalizeEmail(email)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// The immediate write lock (_txlock=immediate) serialises concurrent verifies
	// so the attempt counter races no one — no row-level FOR UPDATE needed.
	var userID, codeHash string
	var attempts, expired int
	err = tx.QueryRowContext(ctx, `
		SELECT u.id, lc.code_hash, lc.attempts, lc.expires_at < ?
		FROM "user" u JOIN login_code lc ON lc.user_id = u.id
		WHERE u.email = ?`, formatTime(time.Now()), email).Scan(&userID, &codeHash, &attempts, &expired)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrInvalidCode
	}
	if err != nil {
		return nil, err
	}
	if expired != 0 || attempts >= maxAttempts {
		return nil, ErrInvalidCode
	}

	if subtle.ConstantTimeCompare([]byte(hashCode(strings.TrimSpace(code))), []byte(codeHash)) != 1 {
		if _, err := tx.ExecContext(ctx, `UPDATE login_code SET attempts = attempts + 1 WHERE user_id = ?`, userID); err != nil {
			return nil, err
		}
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return nil, ErrInvalidCode
	}

	// Single use: the code dies in the same tx that births the session.
	if _, err := tx.ExecContext(ctx, `DELETE FROM login_code WHERE user_id = ?`, userID); err != nil {
		return nil, err
	}

	token, err := newToken()
	if err != nil {
		return nil, err
	}
	// pgx auto-decoded RETURNING expires_at; with TEXT storage we parse it back.
	var expiresStr string
	err = tx.QueryRowContext(ctx, `
		INSERT INTO session (token, user_id, expires_at)
		VALUES (?, ?, ?)
		RETURNING expires_at`, token, userID, formatTime(time.Now().Add(SessionTTL))).Scan(&expiresStr)
	if err != nil {
		return nil, err
	}
	expiresAt, err := time.Parse(time.RFC3339, expiresStr)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &Session{Token: token, UserID: userID, ExpiresAt: expiresAt}, nil
}

// UserForSession resolves a cookie token to its user id — one indexed PK
// SELECT per request, no write (absolute expiry, ADR 0002).
func (s *Service) UserForSession(ctx context.Context, token string) (string, error) {
	var userID string
	err := s.db.QueryRowContext(ctx,
		`SELECT user_id FROM session WHERE token = ? AND expires_at > ?`, token, formatTime(time.Now())).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNoSession
	}
	if err != nil {
		return "", err
	}
	return userID, nil
}

// Logout revokes the session — immediate and authoritative (ADR 0002).
func (s *Service) Logout(ctx context.Context, token string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM session WHERE token = ?`, token)
	return err
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// newCode returns a 6-digit numeric OTP. Security rests on expiry + attempt
// limits, not entropy (ADR 0001).
func newCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

// newToken returns the opaque high-entropy session token (256 bits, hex).
func newToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func hashCode(code string) string {
	sum := sha256.Sum256([]byte(code))
	return hex.EncodeToString(sum[:])
}
