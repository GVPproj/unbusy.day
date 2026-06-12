// Integration tests over real Postgres (the DB is the system boundary).
// Skipped without DATABASE_URL, like the cards tests.
package auth_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/grahamvanpelt/unbusy.day/auth"
	"github.com/jackc/pgx/v5/pgxpool"
)

// captureMailer records sent codes — the dev seam under test control.
type captureMailer struct{ codes []string }

func (m *captureMailer) SendCode(_ context.Context, _, code string) error {
	m.codes = append(m.codes, code)
	return nil
}

func newPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL not set — start `task up` and `task migrate`")
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// newUser inserts a throwaway allowlisted user and returns its email.
func newUser(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		t.Fatalf("rand: %v", err)
	}
	id := "test-" + hex.EncodeToString(b)
	email := id + "@example.test"
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO "user" (id, email) VALUES ($1, $2)`, id, email); err != nil {
		t.Fatalf("insert test user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM "user" WHERE id = $1`, id)
	})
	return email
}

// The full happy path: request → mail → verify → session resolves → logout
// revokes. Also pins single use: a redeemed code never verifies twice.
func TestRequestVerifyLogout(t *testing.T) {
	pool := newPool(t)
	mailer := &captureMailer{}
	svc := auth.NewService(pool, mailer)
	ctx := context.Background()
	email := newUser(t, pool)

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
	pool := newPool(t)
	mailer := &captureMailer{}
	svc := auth.NewService(pool, mailer)

	if err := svc.RequestCode(context.Background(), "nobody@example.test"); err != nil {
		t.Fatalf("want nil for unknown email, got %v", err)
	}
	if len(mailer.codes) != 0 {
		t.Fatalf("mailed a code for an unknown email")
	}
}

// A second request inside the ~60s throttle window sends nothing (still nil).
func TestRequestCodeThrottled(t *testing.T) {
	pool := newPool(t)
	mailer := &captureMailer{}
	svc := auth.NewService(pool, mailer)
	ctx := context.Background()
	email := newUser(t, pool)

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
	pool := newPool(t)
	mailer := &captureMailer{}
	svc := auth.NewService(pool, mailer)
	ctx := context.Background()
	email := newUser(t, pool)

	if err := svc.RequestCode(ctx, email); err != nil {
		t.Fatalf("request: %v", err)
	}
	backdateCode(t, pool, email, "created_at", "61 seconds")

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

// backdateCode shifts the user's login_code timestamps into the past — the
// only way to test time-dependent paths without clock injection.
func backdateCode(t *testing.T, pool *pgxpool.Pool, email, column string, by string) {
	t.Helper()
	tag, err := pool.Exec(context.Background(), `
		UPDATE login_code SET `+column+` = `+column+` - $2::interval
		WHERE user_id = (SELECT id FROM "user" WHERE email = $1)`, email, by)
	if err != nil || tag.RowsAffected() != 1 {
		t.Fatalf("backdate %s: rows=%d err=%v", column, tag.RowsAffected(), err)
	}
}

// An expired session no longer resolves (absolute 30-day expiry, ADR 0002).
func TestUserForSessionExpired(t *testing.T) {
	pool := newPool(t)
	mailer := &captureMailer{}
	svc := auth.NewService(pool, mailer)
	ctx := context.Background()
	email := newUser(t, pool)

	if err := svc.RequestCode(ctx, email); err != nil {
		t.Fatalf("request: %v", err)
	}
	sess, err := svc.VerifyCode(ctx, email, mailer.codes[0])
	if err != nil {
		t.Fatalf("verify: %v", err)
	}

	tag, err := pool.Exec(ctx, `
		UPDATE session SET expires_at = expires_at - '31 days'::interval
		WHERE token = $1`, sess.Token)
	if err != nil || tag.RowsAffected() != 1 {
		t.Fatalf("backdate session: rows=%d err=%v", tag.RowsAffected(), err)
	}

	if _, err := svc.UserForSession(ctx, sess.Token); !errors.Is(err, auth.ErrNoSession) {
		t.Fatalf("expired session: want ErrNoSession, got %v", err)
	}
}

// An expired code never verifies, even when it's the right code.
func TestVerifyCodeExpired(t *testing.T) {
	pool := newPool(t)
	mailer := &captureMailer{}
	svc := auth.NewService(pool, mailer)
	ctx := context.Background()
	email := newUser(t, pool)

	if err := svc.RequestCode(ctx, email); err != nil {
		t.Fatalf("request: %v", err)
	}
	backdateCode(t, pool, email, "expires_at", "11 minutes")

	if _, err := svc.VerifyCode(ctx, email, mailer.codes[0]); !errors.Is(err, auth.ErrInvalidCode) {
		t.Fatalf("expired code: want ErrInvalidCode, got %v", err)
	}
}

// Email is matched case/whitespace-insensitively and the code survives
// stray whitespace — what users actually paste from a mail client.
func TestVerifyCodeNormalizesInput(t *testing.T) {
	pool := newPool(t)
	mailer := &captureMailer{}
	svc := auth.NewService(pool, mailer)
	ctx := context.Background()
	email := newUser(t, pool)

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
	pool := newPool(t)
	mailer := &captureMailer{}
	svc := auth.NewService(pool, mailer)
	ctx := context.Background()
	email := newUser(t, pool)

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
