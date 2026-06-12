package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

// newTestDB creates a throwaway database on the same server as $DATABASE_URL
// and returns its URL. Dropped on cleanup.
func newTestDB(t *testing.T) string {
	t.Helper()
	base := os.Getenv("DATABASE_URL")
	if base == "" {
		t.Skip("DATABASE_URL not set — start `task up`")
	}
	u, err := url.Parse(base)
	if err != nil {
		t.Fatalf("parse DATABASE_URL: %v", err)
	}
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		t.Fatalf("rand: %v", err)
	}
	name := "migrate_test_" + hex.EncodeToString(b)

	ctx := context.Background()
	admin, err := pgx.Connect(ctx, base)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatalf("create db: %v", err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(context.Background(), "DROP DATABASE "+name+" WITH (FORCE)")
		_ = admin.Close(context.Background())
	})
	u.Path = "/" + name
	return u.String()
}

// assertSchema checks the externally visible result of a full migration run:
// all tables exist and the per-owner position constraint is in place.
func assertSchema(t *testing.T, dbURL string) {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dbURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Close(ctx)

	for _, table := range []string{"card", "user", "login_code", "session"} {
		var exists bool
		if err := conn.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = 'public' AND table_name = $1)`,
			table).Scan(&exists); err != nil {
			t.Fatalf("check table %s: %v", table, err)
		}
		if !exists {
			t.Errorf("table %q missing", table)
		}
	}
	var exists bool
	if err := conn.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'card_owner_position_unique')`).Scan(&exists); err != nil {
		t.Fatalf("check constraint: %v", err)
	}
	if !exists {
		t.Error("constraint card_owner_position_unique missing")
	}
}

// goose records each applied migration; a fresh database ends with every
// embedded version present.
func recordedVersions(t *testing.T, dbURL string) []int64 {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dbURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Close(ctx)
	rows, err := conn.Query(ctx,
		`SELECT version_id FROM goose_db_version WHERE version_id > 0 ORDER BY version_id`)
	if err != nil {
		t.Fatalf("query goose_db_version: %v", err)
	}
	versions, err := pgx.CollectRows(rows, pgx.RowTo[int64])
	if err != nil {
		t.Fatalf("collect: %v", err)
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

	if err := runMigrations(context.Background(), dbURL); err != nil {
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

	if err := runMigrations(ctx, dbURL); err != nil {
		t.Fatalf("first run: %v", err)
	}
	if err := runMigrations(ctx, dbURL); err != nil {
		t.Fatalf("second run: %v", err)
	}

	assertSchema(t, dbURL)
	got := recordedVersions(t, dbURL)
	want := embeddedVersions(t)
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("recorded versions after rerun = %v, want %v", got, want)
	}
}

// An existing database (prod Neon, local dev) was migrated by the old psql
// loop and has no goose_db_version. The first goose run baselines it: the
// idempotent 0001–0004 re-apply as no-ops and get recorded.
func TestMigrate_BaselinesExistingDatabase(t *testing.T) {
	dbURL := newTestDB(t)
	ctx := context.Background()

	// Apply the old way: each file executed whole, like `psql -f`.
	conn, err := pgx.Connect(ctx, dbURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		t.Fatalf("read migrations: %v", err)
	}
	for _, e := range entries {
		sqlBytes, err := migrationsFS.ReadFile("migrations/" + e.Name())
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		if _, err := conn.Exec(ctx, string(sqlBytes), pgx.QueryExecModeSimpleProtocol); err != nil {
			t.Fatalf("old-style apply %s: %v", e.Name(), err)
		}
	}
	conn.Close(ctx)

	if err := runMigrations(ctx, dbURL); err != nil {
		t.Fatalf("goose run on existing db: %v", err)
	}

	assertSchema(t, dbURL)
	got := recordedVersions(t, dbURL)
	want := embeddedVersions(t)
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("recorded versions = %v, want %v", got, want)
	}
}
