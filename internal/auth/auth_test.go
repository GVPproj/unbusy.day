// Integration tests over an ephemeral SQLite database (the DB is the system
// boundary). No external container needed; the file dies with the temp dir.
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

// captureMailer records sent codes — the dev seam under test control.
type captureMailer struct{ codes []string }

func (m *captureMailer) SendCode(_ context.Context, _, code string) error {
	m.codes = append(m.codes, code)
	return nil
}

// passResolver makes every domain look deliverable — the DNS seam under test
// control, so MX validation never reaches live DNS in unit tests.
type passResolver struct{}

func (passResolver) LookupMX(context.Context, string) ([]*net.MX, error) {
	return []*net.MX{{Host: "mx.example.test.", Pref: 10}}, nil
}

// newSvc builds an auth.Service whose DNS resolver always passes, so tests that
// use unroutable .test addresses still mail codes after MX validation landed.
func newSvc(db *sql.DB, mailer auth.Mailer) *auth.Service {
	return auth.NewService(db, mailer, auth.WithResolver(passResolver{}))
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

// Open signup: an unknown, deliverable email now gets a code mailed, but no
// user row is created until the code is verified (item 3 — account creation
// deferred to VerifyCode). The pending code is keyed by email with NULL user_id.
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

// Once the ~60s window passes, a new request issues a fresh code that
// verifies; the superseded code is dead (one active code per user).
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

// An expired code never verifies, even when it's the right code.
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

// Email is matched case/whitespace-insensitively and the code survives
// stray whitespace — what users actually paste from a mail client.
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

// fakeResolver returns canned MX results — lets a test stand in for live DNS.
type fakeResolver struct {
	mx  []*net.MX
	err error
}

func (f fakeResolver) LookupMX(context.Context, string) ([]*net.MX, error) {
	return f.mx, f.err
}

// A domain with no MX record is undeliverable: no code is mailed, and the
// response stays the non-committal nil (no enumeration).
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

// A syntactically invalid address is shed before any DNS lookup or send.
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

// A transient DNS error must fail open — a resolver hiccup can't lock a real
// user out, so the code still mails.
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
// farm a fresh 5 guesses by asking for a new code. The count carries forward.
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

// Carry-forward decays: a user who exhausts the budget recovers after the
// recovery window — re-requesting an old-enough code resets attempts to 0.
// This is story 14's recovery guarantee; carry-forward must not be permanent.
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

	// Age the budget past the recovery window, then re-request. Both clocks move:
	// created_at to clear the throttle, attempts_since to trip the decay.
	backdateCode(t, db, email, "created_at", 11*time.Minute)
	backdateCode(t, db, email, "attempts_since", 11*time.Minute)
	if err := svc.RequestCode(ctx, email); err != nil {
		t.Fatalf("recovery request: %v", err)
	}
	// Budget reset: the fresh code verifies again.
	if _, err := svc.VerifyCode(ctx, email, mailer.codes[1]); err != nil {
		t.Fatalf("after recovery window, fresh code must verify; got %v", err)
	}
}

// The recovery clock must survive a re-issue: re-requesting rewrites created_at
// (the throttle clock) but must NOT push the attempt-budget's recovery deadline
// out. Otherwise a user retrying after a lockout — or an attacker pinning a known
// email — stays locked indefinitely. Regression for the created_at/attempts_since
// coupling: decay keys on attempts_since (when the budget began), not created_at.
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

	// 5 min pass, then re-issue (throttle released). The re-issue must preserve
	// attempts_since (budget still 5 min old), not reset it to now.
	backdateCode(t, db, email, "created_at", 5*time.Minute)
	backdateCode(t, db, email, "attempts_since", 5*time.Minute)
	if err := svc.RequestCode(ctx, email); err != nil {
		t.Fatalf("re-issue: %v", err)
	}
	// Still locked: the fresh code is dead on the carried-forward budget.
	if _, err := svc.VerifyCode(ctx, email, mailer.codes[len(mailer.codes)-1]); !errors.Is(err, auth.ErrInvalidCode) {
		t.Fatalf("re-issue must not reset the budget; fresh code should fail, got %v", err)
	}

	// 6 more min elapse (budget now ~11 min old, past recoveryWindow). Had the
	// earlier re-issue reset attempts_since, the budget would read only ~6 min old
	// here and stay locked — the bug this guards against.
	backdateCode(t, db, email, "created_at", 61*time.Second)
	backdateCode(t, db, email, "attempts_since", 6*time.Minute)
	if err := svc.RequestCode(ctx, email); err != nil {
		t.Fatalf("recovery re-issue: %v", err)
	}
	if _, err := svc.VerifyCode(ctx, email, mailer.codes[len(mailer.codes)-1]); err != nil {
		t.Fatalf("budget must recover once attempts_since passes the window; got %v", err)
	}
}

// Carry-forward must not lock out an honest user: one mistype, then a resend,
// and the fresh code still verifies within the remaining budget.
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

// The global send ceiling is a reputation/cost backstop independent of source:
// once outbound OTP mail hits the ceiling in the rolling window, the breaker
// trips and further sends stop, with the same non-committal nil response (no
// enumeration). Distinct from the per-source rate limit (item 1).
func TestRequestCodeSendCeilingTrips(t *testing.T) {
	db := newDB(t)
	mailer := &captureMailer{}
	svc := auth.NewService(db, mailer,
		auth.WithResolver(passResolver{}),
		auth.WithSendCeiling(3, time.Minute))
	ctx := context.Background()

	// 4 distinct users → 4 real sends attempted, but the ceiling is 3.
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

// No ceiling configured (dev/unset env) is permissive: every send goes out.
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

// userCount reports how many user rows exist for an email — proves whether
// VerifyCode created (or refrained from creating) an account.
func userCount(t *testing.T, db *sql.DB, email string) int {
	t.Helper()
	var n int
	if err := db.QueryRowContext(context.Background(),
		`SELECT count(*) FROM "user" WHERE email = ?`, email).Scan(&n); err != nil {
		t.Fatalf("count users: %v", err)
	}
	return n
}

// makePendingCode requests a code for an unregistered email — post-flip this
// naturally yields the pre-account state: a login_code keyed by email with a
// NULL user_id and no user row. Returns the live code.
func makePendingCode(t *testing.T, svc *auth.Service, mailer *captureMailer, email string) string {
	t.Helper()
	if err := svc.RequestCode(context.Background(), email); err != nil {
		t.Fatalf("request: %v", err)
	}
	return mailer.codes[len(mailer.codes)-1]
}

// A correct code for an email with no user row is the account-creation point:
// VerifyCode mints the user and returns a working session (item 3 plumbing).
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

// A wrong code for a pending email creates no user — an unverified requester
// can't conjure an account for an address they don't control.
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
