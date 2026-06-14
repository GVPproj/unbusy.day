package migrate

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

// newTestDB returns a DSN for a throwaway SQLite file in the test's temp dir.
// No external container needed; the file is removed with the temp dir.
func newTestDB(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "migrate_test.db")
	return "file:" + path + "?_pragma=foreign_keys(1)"
}

// assertSchema checks the externally visible result of a full migration run:
// all tables exist.
func assertSchema(t *testing.T, dbURL string) {
	t.Helper()
	db, err := sql.Open("sqlite", dbURL)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	for _, table := range []string{"block", "user", "login_code", "session"} {
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

// goose records each applied migration; a fresh database ends with every
// embedded version present.
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

// embeddedVersions derives the expected version list from the embedded
// migration filenames (the numeric prefix before the first underscore).
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

// The incident: a second deploy re-ran migrations and failed on
// already-applied DDL. Under run-once, a second run applies nothing.
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
