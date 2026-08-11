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

// set is the happy-path helper: write from the current version, fail the test
// on any error, return the committed state.
func set(t *testing.T, svc *jot.Service, owner, text string, base int64) jot.Pad {
	t.Helper()
	p, err := svc.Set(context.Background(), owner, text, base)
	if err != nil {
		t.Fatalf("set %q from v%d: %v", text, base, err)
	}
	return p
}

// The column default is the whole "new user" story: no seeding, no ErrNoRows
// branch, no upsert — a User row exists, so a Jotpad exists, empty at v0.
func TestGet_DefaultsToEmptyVersionZeroForANewUser(t *testing.T) {
	db := newDB(t)
	svc := jot.NewService(db, nil)
	owner := newOwner(t, db)

	got, err := svc.Get(context.Background(), owner)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Text != "" || got.Version != 0 {
		t.Errorf("new user's jot: want empty at v0, got %+v", got)
	}
}

func TestSet_CASHitStoresExactlyAndIncrementsTheVersion(t *testing.T) {
	db := newDB(t)
	svc := jot.NewService(db, nil)
	ctx := context.Background()
	owner := newOwner(t, db)

	// Exactly the characters typed: nothing renders it, nothing trims it.
	const text = "  - milk\n  - eggs\n\n   trailing spaces   "
	p := set(t, svc, owner, text, 0)
	if p.Text != text || p.Version != 1 {
		t.Errorf("committed: want %q at v1, got %+v", text, p)
	}
	got, err := svc.Get(ctx, owner)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got != p {
		t.Errorf("round trip: want %+v, got %+v", p, got)
	}

	p2 := set(t, svc, owner, "", 1)
	if p2.Text != "" || p2.Version != 2 {
		t.Errorf("clearing is a normal write: want empty at v2, got %+v", p2)
	}
}

// A stale writer one revision behind is three-way merged: base = the shadowed
// previous revision, ours = stored, theirs = the client's text. Both sides'
// edits survive.
func TestSet_CASMissMergesBothSidesEdits(t *testing.T) {
	db := newDB(t)
	svc := jot.NewService(db, nil)
	owner := newOwner(t, db)

	set(t, svc, owner, "## Monday\n- call Mom\n\n## Tuesday\n- dentist\n", 0) // v1, both devices load this
	set(t, svc, owner, "## Monday\n- call Mom\n- buy milk\n\n## Tuesday\n- dentist\n", 1) // phone edits Monday → v2

	// The laptop, still on v1, edits Tuesday and flushes.
	p, err := svc.Set(context.Background(), owner,
		"## Monday\n- call Mom\n\n## Tuesday\n- dentist at 3pm\n", 1)
	if err != nil {
		t.Fatalf("stale set: %v", err)
	}
	want := "## Monday\n- call Mom\n- buy milk\n\n## Tuesday\n- dentist at 3pm\n"
	if p.Text != want {
		t.Errorf("merge: want %q, got %q", want, p.Text)
	}
	if p.Version != 3 {
		t.Errorf("merge version: want 3, got %d", p.Version)
	}
}

// A miss older than the one shadow revision fuzzy-patches the client's edit
// against the current text — the shadow is the closest base available. The
// mismatched base means server edits *inside the client's diff context* can be
// reverted (the PRD's accepted one-paragraph blast radius); the client's edit
// and server changes elsewhere in the doc must both survive.
func TestSet_MissOlderThanTheShadowStillAppliesTheEdit(t *testing.T) {
	db := newDB(t)
	svc := jot.NewService(db, nil)
	owner := newOwner(t, db)

	set(t, svc, owner, "alpha\nbeta\ngamma\n", 0)          // v1, stale tab loads this
	set(t, svc, owner, "alpha one\nbeta\ngamma\n", 1)      // v2
	set(t, svc, owner, "alpha one\nbeta\ngamma\ndelta\n", 2) // v3; shadow now holds v2

	// The stale tab (base v1, two revisions behind) edits the beta line.
	p, err := svc.Set(context.Background(), owner, "alpha\nbeta two\ngamma\n", 1)
	if err != nil {
		t.Fatalf("older-than-shadow set: %v", err)
	}
	if !strings.Contains(p.Text, "beta two") {
		t.Errorf("stale edit lost: got %q", p.Text)
	}
	if !strings.Contains(p.Text, "delta") {
		t.Errorf("server text outside the diff context lost: got %q", p.Text)
	}
	if p.Version != 4 {
		t.Errorf("version: want 4, got %d", p.Version)
	}
}

// Over-cap writes are rejected whole and the caller gets the authoritative
// state back — the client must stop believing its over-cap doc is saved.
func TestSet_OverTheCapReturnsTheAuthoritativeStateWithErrTooLong(t *testing.T) {
	db := newDB(t)
	svc := jot.NewService(db, nil)
	ctx := context.Background()
	owner := newOwner(t, db)

	set(t, svc, owner, "keep me", 0)
	p, err := svc.Set(ctx, owner, strings.Repeat("a", jot.MaxLen+1), 1)
	if !errors.Is(err, jot.ErrTooLong) {
		t.Fatalf("over-cap set: want ErrTooLong, got %v", err)
	}
	if p.Text != "keep me" || p.Version != 1 {
		t.Errorf("rejected write must return the untouched state; got %+v", p)
	}
	got, gerr := svc.Get(ctx, owner)
	if gerr != nil {
		t.Fatalf("get: %v", gerr)
	}
	if got.Text != "keep me" || got.Version != 1 {
		t.Errorf("rejected write must not touch storage; got %+v", got)
	}
}

// A merge whose *result* exceeds the cap is rejected whole too: version and
// text stay put, and the stale client snaps to the authoritative state.
func TestSet_OverCapMergeResultIsRejectedWhole(t *testing.T) {
	db := newDB(t)
	svc := jot.NewService(db, nil)
	owner := newOwner(t, db)

	half := strings.Repeat("a", jot.MaxLen/2+100) + "\n"
	set(t, svc, owner, "seed\n", 0)
	set(t, svc, owner, "seed\n"+half, 1) // v2 grows the front

	// The stale tab (base v1) appends its own near-half; merged they exceed the cap.
	p, err := svc.Set(context.Background(), owner, "seed\n"+strings.Repeat("b", jot.MaxLen/2+100), 1)
	if !errors.Is(err, jot.ErrTooLong) {
		t.Fatalf("over-cap merge: want ErrTooLong, got %v", err)
	}
	if p.Version != 2 || p.Text != "seed\n"+half {
		t.Errorf("over-cap merge must leave the stored state untouched; got v%d", p.Version)
	}
}

// The cap counts characters, not bytes: a multi-byte jot at the character cap
// is well under the byte count and must be accepted.
func TestSet_CapCountsCharactersNotBytes(t *testing.T) {
	db := newDB(t)
	svc := jot.NewService(db, nil)
	ctx := context.Background()
	owner := newOwner(t, db)

	if _, err := svc.Set(ctx, owner, strings.Repeat("é", jot.MaxLen), 0); err != nil {
		t.Fatalf("set multi-byte at cap: %v", err)
	}
	if _, err := svc.Set(ctx, owner, strings.Repeat("é", jot.MaxLen+1), 1); !errors.Is(err, jot.ErrTooLong) {
		t.Errorf("multi-byte over cap: want ErrTooLong, got %v", err)
	}
}

func TestSet_IsOwnerScoped(t *testing.T) {
	db := newDB(t)
	svc := jot.NewService(db, nil)
	ctx := context.Background()
	a, b := newOwner(t, db), newOwner(t, db)

	set(t, svc, a, "a's private notes", 0)
	got, err := svc.Get(ctx, b)
	if err != nil {
		t.Fatalf("get b: %v", err)
	}
	if got.Text != "" {
		t.Errorf("b must not see a's jot; got %q", got.Text)
	}

	set(t, svc, b, "b's notes", 0)
	got, err = svc.Get(ctx, a)
	if err != nil {
		t.Fatalf("get a: %v", err)
	}
	if got.Text != "a's private notes" {
		t.Errorf("a's jot after b wrote: want unchanged, got %q", got.Text)
	}
}

// capturePub records fan-outs, and what Get returned at publish time — proving
// the publish is post-commit (the committed row is already readable).
type capturePub struct {
	events   []jot.Event
	readBack []jot.Pad
	svc      *jot.Service
	t        *testing.T
	owner    string
}

func (c *capturePub) PublishJot(e jot.Event) {
	c.events = append(c.events, e)
	p, err := c.svc.Get(context.Background(), c.owner)
	if err != nil {
		c.t.Errorf("get during publish: %v", err)
	}
	c.readBack = append(c.readBack, p)
}

func TestSet_PublishesTheCommittedStatePostCommit(t *testing.T) {
	db := newDB(t)
	pub := &capturePub{t: t}
	svc := jot.NewService(db, pub)
	pub.svc = svc
	owner := newOwner(t, db)
	pub.owner = owner

	p := set(t, svc, owner, "fan me out", 0)

	if len(pub.events) != 1 {
		t.Fatalf("publishes: want 1, got %d", len(pub.events))
	}
	e := pub.events[0]
	if e.Owner != owner || e.Version != p.Version || e.Text != p.Text {
		t.Errorf("event: want %+v for %s, got %+v", p, owner, e)
	}
	// Post-commit: a concurrent reader at publish time sees the new state.
	if pub.readBack[0] != p {
		t.Errorf("publish must be post-commit; reader saw %+v, want %+v", pub.readBack[0], p)
	}
}

func TestSet_RejectedWriteDoesNotPublish(t *testing.T) {
	db := newDB(t)
	pub := &capturePub{t: t}
	svc := jot.NewService(db, pub)
	pub.svc = svc
	owner := newOwner(t, db)
	pub.owner = owner

	if _, err := svc.Set(context.Background(), owner, strings.Repeat("a", jot.MaxLen+1), 0); !errors.Is(err, jot.ErrTooLong) {
		t.Fatalf("want ErrTooLong, got %v", err)
	}
	if len(pub.events) != 0 {
		t.Errorf("a rejected write must not fan out; got %d events", len(pub.events))
	}
}

// RequireSession guarantees the User row, so this can't happen in the app — but
// it must surface as an error rather than as an empty Jotpad, which a later Set
// would then quietly write over.
func TestGet_UnknownOwnerErrors(t *testing.T) {
	db := newDB(t)
	svc := jot.NewService(db, nil)

	if _, err := svc.Get(context.Background(), "no-such-user"); err == nil {
		t.Error("unknown owner Get: want an error, got nil")
	}
	if _, err := svc.Set(context.Background(), "no-such-user", "x", 0); err == nil {
		t.Error("unknown owner Set: want an error, got nil")
	}
}
