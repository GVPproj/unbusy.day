package jot_test

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GVPproj/unbusy.day/internal/jot"
	"github.com/GVPproj/unbusy.day/internal/migrate"
	_ "modernc.org/sqlite"
)

func newDB(t *testing.T) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "jot_test.db")
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

func newOwner(t *testing.T, db *sql.DB) string {
	t.Helper()
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		t.Fatalf("rand: %v", err)
	}
	id := "test-" + hex.EncodeToString(b)
	if _, err := db.ExecContext(context.Background(),
		`INSERT INTO "user" (id, email) VALUES (?, ?)`, id, id+"@example.test"); err != nil {
		t.Fatalf("insert test user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), `DELETE FROM "user" WHERE id = ?`, id)
	})
	return id
}

// The column default is the whole "new user" story: no seeding, no ErrNoRows
// branch, no upsert — a User row exists, so a Jotpad exists and it is empty.
func TestGet_DefaultsToEmptyForANewUser(t *testing.T) {
	db := newDB(t)
	svc := jot.NewService(db)
	owner := newOwner(t, db)

	got, err := svc.Get(context.Background(), owner)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got != "" {
		t.Errorf("new user's jot: want empty, got %q", got)
	}
}

func TestSet_RoundTrips(t *testing.T) {
	db := newDB(t)
	svc := jot.NewService(db)
	ctx := context.Background()
	owner := newOwner(t, db)

	// Exactly the characters typed: nothing renders it, nothing trims it.
	const text = "  - milk\n  - eggs\n\n   trailing spaces   "
	if err := svc.Set(ctx, owner, text); err != nil {
		t.Fatalf("set: %v", err)
	}
	got, err := svc.Get(ctx, owner)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got != text {
		t.Errorf("round trip: want %q, got %q", text, got)
	}
}

func TestSet_EmptyIsAValidValue(t *testing.T) {
	db := newDB(t)
	svc := jot.NewService(db)
	ctx := context.Background()
	owner := newOwner(t, db)

	if err := svc.Set(ctx, owner, "notes"); err != nil {
		t.Fatalf("set: %v", err)
	}
	// The User is the only thing that empties it, so clearing must persist.
	if err := svc.Set(ctx, owner, ""); err != nil {
		t.Fatalf("set empty: %v", err)
	}
	got, err := svc.Get(ctx, owner)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got != "" {
		t.Errorf("after clearing: want empty, got %q", got)
	}
}

func TestSet_AtTheCapIsAccepted(t *testing.T) {
	db := newDB(t)
	svc := jot.NewService(db)
	ctx := context.Background()
	owner := newOwner(t, db)

	text := strings.Repeat("a", jot.MaxLen)
	if err := svc.Set(ctx, owner, text); err != nil {
		t.Fatalf("set at cap: %v", err)
	}
	got, err := svc.Get(ctx, owner)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(got) != jot.MaxLen {
		t.Errorf("length at cap: want %d, got %d", jot.MaxLen, len(got))
	}
}

// Over-cap writes are rejected whole — the stored text is untouched, because a
// partial write would silently truncate prose.
func TestSet_OverTheCapIsRejectedAndLeavesTheStoredTextIntact(t *testing.T) {
	db := newDB(t)
	svc := jot.NewService(db)
	ctx := context.Background()
	owner := newOwner(t, db)

	if err := svc.Set(ctx, owner, "keep me"); err != nil {
		t.Fatalf("set: %v", err)
	}
	err := svc.Set(ctx, owner, strings.Repeat("a", jot.MaxLen+1))
	if !errors.Is(err, jot.ErrTooLong) {
		t.Fatalf("over-cap set: want ErrTooLong, got %v", err)
	}
	got, gerr := svc.Get(ctx, owner)
	if gerr != nil {
		t.Fatalf("get: %v", gerr)
	}
	if got != "keep me" {
		t.Errorf("rejected write must not touch the stored text; got %q", got)
	}
}

// The cap counts characters, not bytes: a multi-byte jot at the character cap
// is well under the byte count and must be accepted.
func TestSet_CapCountsCharactersNotBytes(t *testing.T) {
	db := newDB(t)
	svc := jot.NewService(db)
	ctx := context.Background()
	owner := newOwner(t, db)

	text := strings.Repeat("é", jot.MaxLen)
	if err := svc.Set(ctx, owner, text); err != nil {
		t.Fatalf("set multi-byte at cap: %v", err)
	}
	if err := svc.Set(ctx, owner, strings.Repeat("é", jot.MaxLen+1)); !errors.Is(err, jot.ErrTooLong) {
		t.Errorf("multi-byte over cap: want ErrTooLong, got %v", err)
	}
}

func TestSet_IsOwnerScoped(t *testing.T) {
	db := newDB(t)
	svc := jot.NewService(db)
	ctx := context.Background()
	a, b := newOwner(t, db), newOwner(t, db)

	if err := svc.Set(ctx, a, "a's private notes"); err != nil {
		t.Fatalf("set a: %v", err)
	}
	got, err := svc.Get(ctx, b)
	if err != nil {
		t.Fatalf("get b: %v", err)
	}
	if got != "" {
		t.Errorf("b must not see a's jot; got %q", got)
	}

	if err := svc.Set(ctx, b, "b's notes"); err != nil {
		t.Fatalf("set b: %v", err)
	}
	got, err = svc.Get(ctx, a)
	if err != nil {
		t.Fatalf("get a: %v", err)
	}
	if got != "a's private notes" {
		t.Errorf("a's jot after b wrote: want unchanged, got %q", got)
	}
}

// RequireSession guarantees the User row, so this can't happen in the app — but
// it must surface as an error rather than as an empty Jotpad, which a later Set
// would then quietly write over.
func TestGet_UnknownOwnerErrors(t *testing.T) {
	db := newDB(t)
	svc := jot.NewService(db)

	if _, err := svc.Get(context.Background(), "no-such-user"); err == nil {
		t.Error("unknown owner Get: want an error, got nil")
	}
}
