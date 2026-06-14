// Integration tests over an ephemeral SQLite database (the DB is the system
// boundary). No external container needed; the file dies with the temp dir.
package auth_test

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GVPproj/unbusy.day/internal/auth"
	"github.com/GVPproj/unbusy.day/internal/migrate"
	_ "modernc.org/sqlite"
)

// captureMailer records sent codes — the dev seam under test control.
type captureMailer struct{ codes []string }

func (m *captureMailer) SendCode(_ context.Context, _, code string) error {
	m.codes = append(m.codes, code)
	return nil
}

// newDB returns a handle to an ephemeral SQLite database with the schema
// migrated in.
func newDB(t *testing.T) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "auth_test.db")
	dsn := "file:" + path + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_txlock=immediate"
	if err := migrate.Run(context.Background(), dsn); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// newUser inserts a throwaway allowlisted user and returns its email.
func newUser(t *testing.T, db *sql.DB) string {
	t.Helper()
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		t.Fatalf("rand: %v", err)
	}
	id := "test-" + hex.EncodeToString(b)
	email := id + "@example.test"
	if _, err := db.ExecContext(context.Background(),
		`INSERT INTO "user" (id, email) VALUES (?, ?)`, id, email); err != nil {
		t.Fatalf("insert test user: %v", err)
	}
	return email
}

// The full happy path: request → mail → verify → session resolves → logout
// revokes. Also pins single use: a redeemed code never verifies twice.
func TestRequestVerifyLogout(t *testing.T) {
	db := newDB(t)
	mailer := &captureMailer{}
	svc := auth.NewService(db, mailer)
	ctx := context.Background()
	email := newUser(t, db)

	if err := svc.RequestCode(ctx, email); err != nil {
		t.Fatalf("request: %v", err)
	}
	if len(mailer.codes) != 1 {
		t.Fatalf("want 1 mailed code, got %d", len(mailer.codes))
	}
	code := mailer.codes[0]

	if _, err := svc.VerifyCode(ctx, email, "000000"+code); !errors.Is(err, auth.ErrInvalidCode) {
		t.Fatalf("wrong code: want ErrInvalidCode, got %v", err)
	}

	sess, err := svc.VerifyCode(ctx, email, code)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if sess.Token == "" || sess.UserID == "" {
		t.Fatalf("incomplete session: %+v", sess)
	}

	// Single use: the redeemed code is gone.
	if _, err := svc.VerifyCode(ctx, email, code); !errors.Is(err, auth.ErrInvalidCode) {
		t.Fatalf("reuse: want ErrInvalidCode, got %v", err)
	}

	owner, err := svc.UserForSession(ctx, sess.Token)
	if err != nil || owner != sess.UserID {
		t.Fatalf("resolve session: got (%q, %v), want (%q, nil)", owner, err, sess.UserID)
	}

	if err := svc.Logout(ctx, sess.Token); err != nil {
		t.Fatalf("logout: %v", err)
	}
	if _, err := svc.UserForSession(ctx, sess.Token); !errors.Is(err, auth.ErrNoSession) {
		t.Fatalf("after logout: want ErrNoSession, got %v", err)
	}
}

// Unknown emails get an identical no-op: nil error, no mail (no enumeration).
func TestRequestCodeUnknownEmailIsSilentNoOp(t *testing.T) {
	db := newDB(t)
	mailer := &captureMailer{}
	svc := auth.NewService(db, mailer)

	if err := svc.RequestCode(context.Background(), "nobody@example.test"); err != nil {
		t.Fatalf("want nil for unknown email, got %v", err)
	}
	if len(mailer.codes) != 0 {
		t.Fatalf("mailed a code for an unknown email")
	}
}

// A second request inside the ~60s throttle window sends nothing (still nil).
func TestRequestCodeThrottled(t *testing.T) {
	db := newDB(t)
	mailer := &captureMailer{}
	svc := auth.NewService(db, mailer)
	ctx := context.Background()
	email := newUser(t, db)

	if err := svc.RequestCode(ctx, email); err != nil {
		t.Fatalf("request: %v", err)
	}
	if err := svc.RequestCode(ctx, email); err != nil {
		t.Fatalf("throttled request: %v", err)
	}
	if len(mailer.codes) != 1 {
		t.Fatalf("want 1 mailed code (second throttled), got %d", len(mailer.codes))
	}

	// The throttled request must not clobber the outstanding code.
	if _, err := svc.VerifyCode(ctx, email, mailer.codes[0]); err != nil {
		t.Fatalf("original code after throttled request: %v", err)
	}
}

// Once the ~60s window passes, a new request issues a fresh code that
// verifies; the superseded code is dead (one active code per user).
func TestRequestCodeThrottleReleases(t *testing.T) {
	db := newDB(t)
	mailer := &captureMailer{}
	svc := auth.NewService(db, mailer)
	ctx := context.Background()
	email := newUser(t, db)

	if err := svc.RequestCode(ctx, email); err != nil {
		t.Fatalf("request: %v", err)
	}
	backdateCode(t, db, email, "created_at", 61*time.Second)

	if err := svc.RequestCode(ctx, email); err != nil {
		t.Fatalf("second request: %v", err)
	}
	if len(mailer.codes) != 2 {
		t.Fatalf("want 2 mailed codes after window passed, got %d", len(mailer.codes))
	}
	if _, err := svc.VerifyCode(ctx, email, mailer.codes[0]); !errors.Is(err, auth.ErrInvalidCode) {
		t.Fatalf("superseded code: want ErrInvalidCode, got %v", err)
	}
	if _, err := svc.VerifyCode(ctx, email, mailer.codes[1]); err != nil {
		t.Fatalf("fresh code: %v", err)
	}
}

// backdateCode shifts one of the user's login_code timestamps into the past —
// the only way to test time-dependent paths without clock injection. Timestamps
// are RFC3339 TEXT, so the shift is read-parse-rewrite in Go.
func backdateCode(t *testing.T, db *sql.DB, email, column string, by time.Duration) {
	t.Helper()
	shiftColumn(t, db,
		`SELECT `+column+` FROM login_code WHERE user_id = (SELECT id FROM "user" WHERE email = ?)`,
		`UPDATE login_code SET `+column+` = ? WHERE user_id = (SELECT id FROM "user" WHERE email = ?)`,
		email, by)
}

// shiftColumn reads a TEXT RFC3339 timestamp via sel, subtracts by, and writes
// it back via upd. Both queries bind email as their final/only WHERE param.
func shiftColumn(t *testing.T, db *sql.DB, sel, upd, email string, by time.Duration) {
	t.Helper()
	var cur string
	if err := db.QueryRowContext(context.Background(), sel, email).Scan(&cur); err != nil {
		t.Fatalf("read timestamp: %v", err)
	}
	ts, err := time.Parse(time.RFC3339, cur)
	if err != nil {
		t.Fatalf("parse timestamp %q: %v", cur, err)
	}
	shifted := ts.Add(-by).UTC().Format(time.RFC3339)
	res, err := db.ExecContext(context.Background(), upd, shifted, email)
	if err != nil {
		t.Fatalf("shift timestamp: %v", err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		t.Fatalf("shift timestamp: rows=%d", n)
	}
}

// An expired session no longer resolves (absolute 30-day expiry, ADR 0002).
func TestUserForSessionExpired(t *testing.T) {
	db := newDB(t)
	mailer := &captureMailer{}
	svc := auth.NewService(db, mailer)
	ctx := context.Background()
	email := newUser(t, db)

	if err := svc.RequestCode(ctx, email); err != nil {
		t.Fatalf("request: %v", err)
	}
	sess, err := svc.VerifyCode(ctx, email, mailer.codes[0])
	if err != nil {
		t.Fatalf("verify: %v", err)
	}

	shiftColumn(t, db,
		`SELECT expires_at FROM session WHERE token = ?`,
		`UPDATE session SET expires_at = ? WHERE token = ?`,
		sess.Token, 31*24*time.Hour)

	if _, err := svc.UserForSession(ctx, sess.Token); !errors.Is(err, auth.ErrNoSession) {
		t.Fatalf("expired session: want ErrNoSession, got %v", err)
	}
}

// An expired code never verifies, even when it's the right code.
func TestVerifyCodeExpired(t *testing.T) {
	db := newDB(t)
	mailer := &captureMailer{}
	svc := auth.NewService(db, mailer)
	ctx := context.Background()
	email := newUser(t, db)

	if err := svc.RequestCode(ctx, email); err != nil {
		t.Fatalf("request: %v", err)
	}
	backdateCode(t, db, email, "expires_at", 11*time.Minute)

	if _, err := svc.VerifyCode(ctx, email, mailer.codes[0]); !errors.Is(err, auth.ErrInvalidCode) {
		t.Fatalf("expired code: want ErrInvalidCode, got %v", err)
	}
}

// Email is matched case/whitespace-insensitively and the code survives
// stray whitespace — what users actually paste from a mail client.
func TestVerifyCodeNormalizesInput(t *testing.T) {
	db := newDB(t)
	mailer := &captureMailer{}
	svc := auth.NewService(db, mailer)
	ctx := context.Background()
	email := newUser(t, db)

	if err := svc.RequestCode(ctx, "  "+strings.ToUpper(email)+" "); err != nil {
		t.Fatalf("request with messy email: %v", err)
	}
	if len(mailer.codes) != 1 {
		t.Fatalf("want 1 mailed code, got %d", len(mailer.codes))
	}

	if _, err := svc.VerifyCode(ctx, " "+strings.ToUpper(email)+"  ", " "+mailer.codes[0]+" "); err != nil {
		t.Fatalf("verify with messy email+code: %v", err)
	}
}

// After 5 failed attempts even the right code is dead.
func TestVerifyCodeAttemptLimit(t *testing.T) {
	db := newDB(t)
	mailer := &captureMailer{}
	svc := auth.NewService(db, mailer)
	ctx := context.Background()
	email := newUser(t, db)

	if err := svc.RequestCode(ctx, email); err != nil {
		t.Fatalf("request: %v", err)
	}
	code := mailer.codes[0]

	for i := range 5 {
		if _, err := svc.VerifyCode(ctx, email, "wrong!"); !errors.Is(err, auth.ErrInvalidCode) {
			t.Fatalf("attempt %d: want ErrInvalidCode, got %v", i, err)
		}
	}
	if _, err := svc.VerifyCode(ctx, email, code); !errors.Is(err, auth.ErrInvalidCode) {
		t.Fatalf("after attempt limit, right code must fail; got %v", err)
	}
}
