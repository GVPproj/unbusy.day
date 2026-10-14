package migrate

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/GVPproj/unbusy.day/internal/jot"
	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
)

func newTestDB(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "migrate_test.db")
	return "file:" + path + "?_pragma=foreign_keys(1)"
}

func assertSchema(t *testing.T, dbURL string) {
	t.Helper()
	db, err := sql.Open("sqlite", dbURL)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	for _, table := range []string{"block", "user", "login_code", "session", "habit", "habit_checkin", "jot_note"} {
		var name string
		err := db.QueryRow(
			`SELECT name FROM sqlite_master WHERE type='table' AND name = ?`,
			table).Scan(&name)
		if err == sql.ErrNoRows {
			t.Errorf("table %q missing", table)
			continue
		}
		if err != nil {
			t.Fatalf("check table %s: %v", table, err)
		}
	}
}

func recordedVersions(t *testing.T, dbURL string) []int64 {
	t.Helper()
	db, err := sql.Open("sqlite", dbURL)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	rows, err := db.Query(
		`SELECT version_id FROM goose_db_version WHERE version_id > 0 ORDER BY version_id`)
	if err != nil {
		t.Fatalf("query goose_db_version: %v", err)
	}
	defer rows.Close()
	var versions []int64
	for rows.Next() {
		var v int64
		if err := rows.Scan(&v); err != nil {
			t.Fatalf("scan: %v", err)
		}
		versions = append(versions, v)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	return versions
}

// embeddedVersions derives the expected versions from the embedded migration filenames.
func embeddedVersions(t *testing.T) []int64 {
	t.Helper()
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		t.Fatalf("read migrations: %v", err)
	}
	versions := make([]int64, 0, len(entries))
	for _, e := range entries {
		prefix, _, _ := strings.Cut(e.Name(), "_")
		v, err := strconv.ParseInt(prefix, 10, 64)
		if err != nil {
			t.Fatalf("parse version from %s: %v", e.Name(), err)
		}
		versions = append(versions, v)
	}
	return versions
}

func TestMigrate_JotNotesPreserveDailyAndItsMergeBase(t *testing.T) {
	ctx := context.Background()
	dsn := newTestDB(t)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	sub, err := fs.Sub(migrationsFS, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	provider, err := goose.NewProvider(goose.DialectSQLite3, db, sub)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, 20260820120000); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO "user" (id, email, jot, jot_version) VALUES ('a', 'a@test', '  preserved
', 7), ('b', 'b@test', 'private', 2);
	INSERT INTO jot_base (owner, version, text) VALUES ('a', 6, 'previous');`); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := Run(ctx, dsn); err != nil {
			t.Fatal(err)
		}
		for _, tc := range []struct {
			owner, note, text string
			version           int64
		}{
			{"a", "daily", "  preserved\n", 7}, {"b", "daily", "private", 2},
			{"a", "weekly", "", 0}, {"a", "quarterly", "", 0},
			{"b", "weekly", "", 0}, {"b", "quarterly", "", 0},
		} {
			var text string
			var version int64
			if err := db.QueryRow(`SELECT text, version FROM jot_note WHERE owner = ? AND note = ?`, tc.owner, tc.note).Scan(&text, &version); err != nil {
				t.Fatal(err)
			}
			if text != tc.text || version != tc.version {
				t.Errorf("%s/%s = %q v%d", tc.owner, tc.note, text, version)
			}
		}
		var base string
		var version int64
		if err := db.QueryRow(`SELECT base_text, base_version FROM jot_note WHERE owner = 'a' AND note = 'daily'`).Scan(&base, &version); err != nil {
			t.Fatal(err)
		}
		if base != "previous" || version != 6 {
			t.Errorf("base = %q v%d", base, version)
		}
	}
	// An unchanged stale client must retain the migrated current text.
	svc := jot.NewService(db, nil)
	want := jot.Pad{Text: "  preserved\n", Version: 8}
	got, err := svc.Set(ctx, "a", jot.Daily, "previous", 6)
	if err != nil || got != want {
		t.Fatalf("migrated merge: %+v, %v", got, err)
	}
	if err := Run(ctx, dsn); err != nil {
		t.Fatal(err)
	}
	got, err = svc.Get(ctx, "a", jot.Daily)
	if err != nil || got != want {
		t.Fatalf("rerun reset note: %+v, %v", got, err)
	}
	// A legacy note without a shadow uses its current text as the merge base.
	got, err = svc.Set(ctx, "b", jot.Daily, "private edit", 1)
	if err != nil || got != (jot.Pad{Text: "private edit", Version: 3}) {
		t.Fatalf("migrated note without shadow: %+v, %v", got, err)
	}
}

func TestMigrate_FreshDatabase(t *testing.T) {
	dbURL := newTestDB(t)

	if err := Run(context.Background(), dbURL); err != nil {
		t.Fatalf("runMigrations: %v", err)
	}

	assertSchema(t, dbURL)
	got := recordedVersions(t, dbURL)
	want := embeddedVersions(t)
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("recorded versions = %v, want %v", got, want)
	}
}

// Regression: a second deploy once failed on already-applied DDL.
func TestMigrate_RerunIsNoOp(t *testing.T) {
	dbURL := newTestDB(t)
	ctx := context.Background()

	if err := Run(ctx, dbURL); err != nil {
		t.Fatalf("first run: %v", err)
	}
	if err := Run(ctx, dbURL); err != nil {
		t.Fatalf("second run: %v", err)
	}

	assertSchema(t, dbURL)
	got := recordedVersions(t, dbURL)
	want := embeddedVersions(t)
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("recorded versions after rerun = %v, want %v", got, want)
	}
}
