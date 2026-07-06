// Integration tests over an ephemeral SQLite database.
package auth_test

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GVPproj/unbusy.day/internal/auth"
	"github.com/GVPproj/unbusy.day/internal/migrate"
	_ "modernc.org/sqlite"
)

type captureMailer struct{ codes []string }

func (m *captureMailer) SendCode(_ context.Context, _, code string) error {
	m.codes = append(m.codes, code)
	return nil
}

// passResolver makes every domain look deliverable, so MX validation never
// reaches live DNS in tests.
type passResolver struct{}

func (passResolver) LookupMX(context.Context, string) ([]*net.MX, error) {
	return []*net.MX{{Host: "mx.example.test.", Pref: 10}}, nil
}

func newSvc(db *sql.DB, mailer auth.Mailer) *auth.Service {
	return auth.NewService(db, mailer, auth.WithResolver(passResolver{}))
}

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

// Happy path: request → mail → verify → session resolves → logout revokes.
// Also pins single use: a redeemed code never verifies twice.
func TestRequestVerifyLogout(t *testing.T) {
	db := newDB(t)
	mailer := &captureMailer{}
	svc := newSvc(db, mailer)
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

// Open signup: an unknown, deliverable email gets a code mailed, but no user
// row is created until the code is verified.
func TestRequestCodeUnknownEmailIssuesPendingCode(t *testing.T) {
	db := newDB(t)
	mailer := &captureMailer{}
	svc := newSvc(db, mailer)
	email := "nobody@example.test"

	if err := svc.RequestCode(context.Background(), email); err != nil {
		t.Fatalf("want nil for unknown email, got %v", err)
	}
	if len(mailer.codes) != 1 {
		t.Fatalf("want 1 mailed code for an unknown email, got %d", len(mailer.codes))
	}
	if userCount(t, db, email) != 0 {
		t.Fatalf("RequestCode must not create a user row; got %d", userCount(t, db, email))
	}
}

// A second request inside the ~60s throttle window sends nothing (still nil).
func TestRequestCodeThrottled(t *testing.T) {
	db := newDB(t)
	mailer := &captureMailer{}
	svc := newSvc(db, mailer)
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

// Past the throttle window a fresh code issues and verifies; the superseded
// code is dead (one active code per user).
func TestRequestCodeThrottleReleases(t *testing.T) {
	db := newDB(t)
	mailer := &captureMailer{}
	svc := newSvc(db, mailer)
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

// backdateCode shifts a login_code timestamp into the past — there is no clock
// injection. Timestamps are RFC3339 TEXT, so the shift is read-parse-rewrite.
func backdateCode(t *testing.T, db *sql.DB, email, column string, by time.Duration) {
	t.Helper()
	shiftColumn(t, db,
		`SELECT `+column+` FROM login_code WHERE user_id = (SELECT id FROM "user" WHERE email = ?)`,
		`UPDATE login_code SET `+column+` = ? WHERE user_id = (SELECT id FROM "user" WHERE email = ?)`,
		email, by)
}

// shiftColumn reads a TEXT RFC3339 timestamp via sel, subtracts by, and writes
// it back via upd.
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

func TestUserForSessionExpired(t *testing.T) {
	db := newDB(t)
	mailer := &captureMailer{}
	svc := newSvc(db, mailer)
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

func TestVerifyCodeExpired(t *testing.T) {
	db := newDB(t)
	mailer := &captureMailer{}
	svc := newSvc(db, mailer)
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

// Email and code survive the case/whitespace mess users paste from a mail client.
func TestVerifyCodeNormalizesInput(t *testing.T) {
	db := newDB(t)
	mailer := &captureMailer{}
	svc := newSvc(db, mailer)
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

type fakeResolver struct {
	mx  []*net.MX
	err error
}

func (f fakeResolver) LookupMX(context.Context, string) ([]*net.MX, error) {
	return f.mx, f.err
}

// No MX record: nothing mailed, response stays the non-committal nil (no enumeration).
func TestRequestCodeRejectsNoMX(t *testing.T) {
	db := newDB(t)
	mailer := &captureMailer{}
	svc := auth.NewService(db, mailer, auth.WithResolver(fakeResolver{mx: nil}))
	email := newUser(t, db)

	if err := svc.RequestCode(context.Background(), email); err != nil {
		t.Fatalf("RequestCode: %v", err)
	}
	if len(mailer.codes) != 0 {
		t.Fatalf("mailed a code to a no-MX domain; want 0, got %d", len(mailer.codes))
	}
}

func TestRequestCodeRejectsBadSyntax(t *testing.T) {
	db := newDB(t)
	mailer := &captureMailer{}
	// Resolver would pass if reached — proves syntax rejection comes first.
	svc := auth.NewService(db, mailer, auth.WithResolver(passResolver{}))
	if _, err := db.ExecContext(context.Background(),
		`INSERT INTO "user" (id, email) VALUES (?, ?)`, "bad", "not-an-email"); err != nil {
		t.Fatalf("insert: %v", err)
	}

	if err := svc.RequestCode(context.Background(), "not-an-email"); err != nil {
		t.Fatalf("RequestCode: %v", err)
	}
	if len(mailer.codes) != 0 {
		t.Fatalf("mailed a code to a malformed address; want 0, got %d", len(mailer.codes))
	}
}

// A transient DNS error fails open — a resolver hiccup can't lock a real user out.
func TestRequestCodeFailsOpenOnTransientDNSError(t *testing.T) {
	db := newDB(t)
	mailer := &captureMailer{}
	svc := auth.NewService(db, mailer,
		auth.WithResolver(fakeResolver{err: &net.DNSError{Err: "server misbehaving", IsTemporary: true}}))
	email := newUser(t, db)

	if err := svc.RequestCode(context.Background(), email); err != nil {
		t.Fatalf("RequestCode: %v", err)
	}
	if len(mailer.codes) != 1 {
		t.Fatalf("transient DNS error should fail open; want 1 code, got %d", len(mailer.codes))
	}
}

// Re-requesting a code must NOT reset the attempt budget — an attacker can't
// farm a fresh 5 guesses by asking for a new code.
func TestRequestCodeCarriesAttemptsForward(t *testing.T) {
	db := newDB(t)
	mailer := &captureMailer{}
	svc := newSvc(db, mailer)
	ctx := context.Background()
	email := newUser(t, db)

	if err := svc.RequestCode(ctx, email); err != nil {
		t.Fatalf("request: %v", err)
	}
	// Burn 3 of the 5 guesses against the first code.
	for i := range 3 {
		if _, err := svc.VerifyCode(ctx, email, "wrong!"); !errors.Is(err, auth.ErrInvalidCode) {
			t.Fatalf("attempt %d: want ErrInvalidCode, got %v", i, err)
		}
	}

	// Release the throttle and request a fresh code.
	backdateCode(t, db, email, "created_at", 61*time.Second)
	if err := svc.RequestCode(ctx, email); err != nil {
		t.Fatalf("second request: %v", err)
	}
	if len(mailer.codes) != 2 {
		t.Fatalf("want 2 mailed codes, got %d", len(mailer.codes))
	}
	fresh := mailer.codes[1]

	// Only 2 guesses remain on the carried-forward budget.
	for i := range 2 {
		if _, err := svc.VerifyCode(ctx, email, "wrong!"); !errors.Is(err, auth.ErrInvalidCode) {
			t.Fatalf("post-reissue attempt %d: want ErrInvalidCode, got %v", i, err)
		}
	}
	// Budget exhausted: even the fresh, correct code is dead.
	if _, err := svc.VerifyCode(ctx, email, fresh); !errors.Is(err, auth.ErrInvalidCode) {
		t.Fatalf("re-request must not reset attempts; fresh code should fail, got %v", err)
	}
}

// Carry-forward decays: an exhausted budget recovers after the recovery
// window — it must not be permanent.
func TestRequestCodeAttemptsDecayAfterRecoveryWindow(t *testing.T) {
	db := newDB(t)
	mailer := &captureMailer{}
	svc := newSvc(db, mailer)
	ctx := context.Background()
	email := newUser(t, db)

	if err := svc.RequestCode(ctx, email); err != nil {
		t.Fatalf("request: %v", err)
	}
	// Exhaust the entire budget.
	for i := range 5 {
		if _, err := svc.VerifyCode(ctx, email, "wrong!"); !errors.Is(err, auth.ErrInvalidCode) {
			t.Fatalf("attempt %d: want ErrInvalidCode, got %v", i, err)
		}
	}

	// Both clocks move: created_at clears the throttle, attempts_since trips the decay.
	backdateCode(t, db, email, "created_at", 11*time.Minute)
	backdateCode(t, db, email, "attempts_since", 11*time.Minute)
	if err := svc.RequestCode(ctx, email); err != nil {
		t.Fatalf("recovery request: %v", err)
	}
	if _, err := svc.VerifyCode(ctx, email, mailer.codes[1]); err != nil {
		t.Fatalf("after recovery window, fresh code must verify; got %v", err)
	}
}

// A re-issue rewrites created_at (the throttle clock) but must NOT reset
// attempts_since — decay keys on when the budget began, or a retrying user
// stays locked indefinitely.
func TestRequestCodeReissueDoesNotResetRecoveryClock(t *testing.T) {
	db := newDB(t)
	mailer := &captureMailer{}
	svc := newSvc(db, mailer)
	ctx := context.Background()
	email := newUser(t, db)

	if err := svc.RequestCode(ctx, email); err != nil {
		t.Fatalf("request: %v", err)
	}
	// Exhaust the budget.
	for i := range 5 {
		if _, err := svc.VerifyCode(ctx, email, "wrong!"); !errors.Is(err, auth.ErrInvalidCode) {
			t.Fatalf("attempt %d: want ErrInvalidCode, got %v", i, err)
		}
	}

	// 5 min pass, then re-issue: attempts_since must stay 5 min old, not reset.
	backdateCode(t, db, email, "created_at", 5*time.Minute)
	backdateCode(t, db, email, "attempts_since", 5*time.Minute)
	if err := svc.RequestCode(ctx, email); err != nil {
		t.Fatalf("re-issue: %v", err)
	}
	// Still locked: the fresh code is dead on the carried-forward budget.
	if _, err := svc.VerifyCode(ctx, email, mailer.codes[len(mailer.codes)-1]); !errors.Is(err, auth.ErrInvalidCode) {
		t.Fatalf("re-issue must not reset the budget; fresh code should fail, got %v", err)
	}

	// 6 more min (budget ~11 min old, past recoveryWindow). Had the re-issue
	// reset attempts_since, the budget would read ~6 min old and stay locked.
	backdateCode(t, db, email, "created_at", 61*time.Second)
	backdateCode(t, db, email, "attempts_since", 6*time.Minute)
	if err := svc.RequestCode(ctx, email); err != nil {
		t.Fatalf("recovery re-issue: %v", err)
	}
	if _, err := svc.VerifyCode(ctx, email, mailer.codes[len(mailer.codes)-1]); err != nil {
		t.Fatalf("budget must recover once attempts_since passes the window; got %v", err)
	}
}

// Carry-forward must not lock out an honest user: one mistype plus a resend
// still verifies within the remaining budget.
func TestRequestCodeResendStillVerifiesWithinBudget(t *testing.T) {
	db := newDB(t)
	mailer := &captureMailer{}
	svc := newSvc(db, mailer)
	ctx := context.Background()
	email := newUser(t, db)

	if err := svc.RequestCode(ctx, email); err != nil {
		t.Fatalf("request: %v", err)
	}
	if _, err := svc.VerifyCode(ctx, email, "wrong!"); !errors.Is(err, auth.ErrInvalidCode) {
		t.Fatalf("mistype: want ErrInvalidCode, got %v", err)
	}

	backdateCode(t, db, email, "created_at", 61*time.Second)
	if err := svc.RequestCode(ctx, email); err != nil {
		t.Fatalf("resend: %v", err)
	}
	if _, err := svc.VerifyCode(ctx, email, mailer.codes[1]); err != nil {
		t.Fatalf("honest resend within budget must verify, got %v", err)
	}
}

// The global send ceiling is a source-independent backstop; tripping it still
// returns the non-committal nil (no enumeration).
func TestRequestCodeSendCeilingTrips(t *testing.T) {
	db := newDB(t)
	mailer := &captureMailer{}
	svc := auth.NewService(db, mailer,
		auth.WithResolver(passResolver{}),
		auth.WithSendCeiling(3, time.Minute))
	ctx := context.Background()

	// 4 distinct users → 4 sends attempted, but the ceiling is 3.
	for i := range 4 {
		email := newUser(t, db)
		if err := svc.RequestCode(ctx, email); err != nil {
			t.Fatalf("request %d: %v", i, err)
		}
	}
	if len(mailer.codes) != 3 {
		t.Fatalf("send ceiling: want 3 mailed (4th tripped), got %d", len(mailer.codes))
	}
}

func TestRequestCodeNoSendCeilingIsPermissive(t *testing.T) {
	db := newDB(t)
	mailer := &captureMailer{}
	svc := newSvc(db, mailer) // no WithSendCeiling
	ctx := context.Background()

	for range 5 {
		email := newUser(t, db)
		if err := svc.RequestCode(ctx, email); err != nil {
			t.Fatalf("request: %v", err)
		}
	}
	if len(mailer.codes) != 5 {
		t.Fatalf("no ceiling should send all; want 5, got %d", len(mailer.codes))
	}
}

func userCount(t *testing.T, db *sql.DB, email string) int {
	t.Helper()
	var n int
	if err := db.QueryRowContext(context.Background(),
		`SELECT count(*) FROM "user" WHERE email = ?`, email).Scan(&n); err != nil {
		t.Fatalf("count users: %v", err)
	}
	return n
}

// makePendingCode requests a code for an unregistered email, yielding a
// login_code keyed by email with NULL user_id and no user row.
func makePendingCode(t *testing.T, svc *auth.Service, mailer *captureMailer, email string) string {
	t.Helper()
	if err := svc.RequestCode(context.Background(), email); err != nil {
		t.Fatalf("request: %v", err)
	}
	return mailer.codes[len(mailer.codes)-1]
}

// VerifyCode is the account-creation point: a correct code for an email with
// no user row mints the user and returns a working session.
func TestVerifyCodeCreatesAccountForPendingEmail(t *testing.T) {
	db := newDB(t)
	mailer := &captureMailer{}
	svc := newSvc(db, mailer)
	ctx := context.Background()
	email := "pending-new@example.test"

	code := makePendingCode(t, svc, mailer, email)
	if userCount(t, db, email) != 0 {
		t.Fatalf("precondition: pending email must have no user row")
	}

	sess, err := svc.VerifyCode(ctx, email, code)
	if err != nil {
		t.Fatalf("verify pending code: %v", err)
	}
	if sess.UserID == "" {
		t.Fatalf("no user id on session: %+v", sess)
	}
	if userCount(t, db, email) != 1 {
		t.Fatalf("VerifyCode must create exactly one user; got %d", userCount(t, db, email))
	}
	owner, err := svc.UserForSession(ctx, sess.Token)
	if err != nil || owner != sess.UserID {
		t.Fatalf("resolve new session: got (%q, %v), want (%q, nil)", owner, err, sess.UserID)
	}
}

func TestVerifyCodeWrongCodeCreatesNoAccount(t *testing.T) {
	db := newDB(t)
	mailer := &captureMailer{}
	svc := newSvc(db, mailer)
	ctx := context.Background()
	email := "pending-wrong@example.test"

	_ = makePendingCode(t, svc, mailer, email)

	if _, err := svc.VerifyCode(ctx, email, "000000"); !errors.Is(err, auth.ErrInvalidCode) {
		t.Fatalf("wrong code: want ErrInvalidCode, got %v", err)
	}
	if userCount(t, db, email) != 0 {
		t.Fatalf("wrong code must create no user; got %d", userCount(t, db, email))
	}
}

// After 5 failed attempts even the right code is dead.
func TestVerifyCodeAttemptLimit(t *testing.T) {
	db := newDB(t)
	mailer := &captureMailer{}
	svc := newSvc(db, mailer)
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
