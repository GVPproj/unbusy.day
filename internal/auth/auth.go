// Package auth is passwordless email-OTP authentication over DB-backed
// sessions (ADRs 0001/0002): 10-min single-use 6-digit codes, one active code
// per email, 5 verify attempts, ~60s request throttle, codes stored hashed.
// Codes are keyed by email so one can exist before its user row; a verified
// code for an unregistered email mints the account (item 3).
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
	"net"
	"strings"
	"time"
)

// SessionTTL is the absolute (non-sliding) session lifetime.
const SessionTTL = 30 * 24 * time.Hour

const (
	codeTTL         = 10 * time.Minute
	maxAttempts     = 5
	requestThrottle = 60 * time.Second
	// recoveryWindow decays the carried-forward attempt budget: a re-issue once
	// the budget (attempts_since) is older than this resets attempts, so an
	// honest user who exhausts their guesses recovers after a cooldown (story 14).
	recoveryWindow = 10 * time.Minute
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
	db       *sql.DB
	mailer   Mailer
	resolver Resolver
	ceiling  *sendCeiling // nil = no global send ceiling (dev/unset env)
}

// Option configures a Service at construction (mirrors the env-driven seams).
type Option func(*Service)

// WithResolver overrides the DNS resolver used for MX validation — tests inject
// a fake so address validation never touches live DNS.
func WithResolver(r Resolver) Option {
	return func(s *Service) { s.resolver = r }
}

// WithSendCeiling caps total outbound OTP mail at max sends per rolling window
// across all sources (the global circuit breaker, item 4). Unset → no ceiling
// (dev-safe permissive mode, mirrors LogMailer).
func WithSendCeiling(max int, window time.Duration) Option {
	return func(s *Service) { s.ceiling = newSendCeiling(max, window) }
}

func NewService(db *sql.DB, mailer Mailer, opts ...Option) *Service {
	s := &Service{db: db, mailer: mailer, resolver: net.DefaultResolver}
	for _, o := range opts {
		o(s)
	}
	return s
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

	// Open signup: an unknown email is no longer a no-op. We still look up an
	// existing user so the code carries their user_id; an unregistered email
	// gets a code keyed by email with a NULL user_id, and VerifyCode mints the
	// account on a correct code (item 3). The defensive layers below — rate
	// limit (upstream), suppression, MX validation, send ceiling — replace the
	// allowlist as the email-bombing blast-radius bound.
	var userID sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT id FROM "user" WHERE email = ?`, email).Scan(&userID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	// Never mail a hard-bounced or complaining address — protects SES reputation.
	// Same silent no-op as unknown/throttled, so no enumeration signal leaks.
	if suppressed, err := s.IsSuppressed(ctx, email); err != nil {
		return err
	} else if suppressed {
		log.Printf("auth: suppressed address %s, skipping OTP", email)
		return nil
	}

	// Shed bad-syntax / no-MX addresses before spending send quota. Same silent
	// no-op as unknown/throttled, so validation leaks no enumeration signal.
	if !s.deliverable(ctx, email) {
		log.Printf("auth: undeliverable address %s, skipping OTP", email)
		return nil
	}

	code, err := newCode()
	if err != nil {
		return err
	}

	now := time.Now()
	nowStr := formatTime(now)
	expiresAt := formatTime(now.Add(codeTTL))
	throttleCutoff := formatTime(now.Add(-requestThrottle))
	recoveryCutoff := formatTime(now.Add(-recoveryWindow))

	// One active code per email (item 3: codes are keyed by email so a code can
	// exist before its user row). The WHERE guard is the ~60s request throttle;
	// zero rows written means throttled — same nil response, no email. attempts
	// carries forward on re-issue (no fresh 5-guess budget for an attacker). The
	// decay keys on attempts_since — when the budget began — not created_at, so a
	// re-issue (which rewrites created_at as the throttle clock) can't push the
	// recovery deadline out: the window elapses even under a storm of re-requests,
	// and an honest user recovers from a lockout once attempts_since ages out.
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO login_code (email, user_id, code_hash, attempts, attempts_since, expires_at, created_at)
		VALUES (?, ?, ?, 0, ?, ?, ?)
		ON CONFLICT (email) DO UPDATE
		SET code_hash = excluded.code_hash, user_id = excluded.user_id,
		    attempts = CASE WHEN login_code.attempts_since < ? THEN 0 ELSE login_code.attempts END,
		    attempts_since = CASE WHEN login_code.attempts_since < ? THEN excluded.attempts_since ELSE login_code.attempts_since END,
		    expires_at = excluded.expires_at, created_at = excluded.created_at
		WHERE login_code.created_at < ?`,
		email, userID, hashCode(code), nowStr, expiresAt, nowStr, recoveryCutoff, recoveryCutoff, throttleCutoff)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		return nil // throttled
	}

	// Global send ceiling: a tripped breaker stops sending (and alerts) rather
	// than torching domain reputation. Same silent nil as throttled/unknown so
	// no enumeration signal leaks (story 10). Checked here, post-throttle, so
	// only real sends count toward the ceiling.
	if s.ceiling != nil && !s.ceiling.allow(now) {
		log.Printf("auth: send ceiling tripped — OTP to %s skipped (circuit breaker)", email)
		return nil
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
	defer func() { _ = tx.Rollback() }()

	// The immediate write lock (_txlock=immediate) serialises concurrent verifies
	// so the attempt counter races no one — no row-level FOR UPDATE needed. The
	// code is keyed by email; user_id is NULL for a not-yet-registered email
	// (item 3) and is filled in once we mint the account below.
	var userID sql.NullString
	var codeHash string
	var attempts, expired int
	err = tx.QueryRowContext(ctx, `
		SELECT lc.user_id, lc.code_hash, lc.attempts, lc.expires_at < ?
		FROM login_code lc
		WHERE lc.email = ?`, formatTime(time.Now()), email).Scan(&userID, &codeHash, &attempts, &expired)
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
		if _, err := tx.ExecContext(ctx, `UPDATE login_code SET attempts = attempts + 1 WHERE email = ?`, email); err != nil {
			return nil, err
		}
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return nil, ErrInvalidCode
	}

	// Account-creation point (item 3): a correct code for an email with no user
	// row mints the account here, in the same tx that births the session. The
	// frontend's idempotent Seeder runs post-verify and seeds starter blocks.
	ownerID := userID.String
	if !userID.Valid {
		ownerID, err = newUserID()
		if err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO "user" (id, email) VALUES (?, ?)`, ownerID, email); err != nil {
			return nil, err
		}
	}

	// Single use: the code dies in the same tx that births the session.
	if _, err := tx.ExecContext(ctx, `DELETE FROM login_code WHERE email = ?`, email); err != nil {
		return nil, err
	}

	token, err := newToken()
	if err != nil {
		return nil, err
	}

	var expiresStr string
	err = tx.QueryRowContext(ctx, `
		INSERT INTO session (token, user_id, expires_at)
		VALUES (?, ?, ?)
		RETURNING expires_at`, token, ownerID, formatTime(time.Now().Add(SessionTTL))).Scan(&expiresStr)
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
	return &Session{Token: token, UserID: ownerID, ExpiresAt: expiresAt}, nil
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

// newUserID returns an opaque user id minted at account creation (item 3).
// The `u_` prefix mirrors the seeded allowlist ids; 128 bits is ample.
func newUserID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "u_" + hex.EncodeToString(b), nil
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
