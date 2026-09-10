package habit

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/GVPproj/unbusy.day/internal/migrate"
	_ "modernc.org/sqlite"
)

func TestCreateUsesServerClockInRequestedTimezone(t *testing.T) {
	ctx := context.Background()
	dsn := "file:" + filepath.Join(t.TempDir(), "clock.db") + "?_pragma=foreign_keys(1)&_txlock=immediate"
	if err := migrate.Run(ctx, dsn); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO "user" (id,email) VALUES ('clock','clock@example.test')`); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2024, 3, 1, 0, 30, 0, 0, time.UTC)
	s := NewService(db, nil, WithClock(func() time.Time { return now }))
	if err := s.Create(ctx, "clock", "Tomorrow", "2024-03-01", "America/Los_Angeles"); !IsRejection(err) {
		t.Fatalf("local tomorrow: %v", err)
	}
	if err := s.Create(ctx, "clock", "Today", "2024-02-29", "America/Los_Angeles"); err != nil {
		t.Fatalf("local today: %v", err)
	}
	now = time.Date(2024, 12, 31, 12, 30, 0, 0, time.UTC)
	if err := s.Create(ctx, "clock", "New year", "2025-01-01", "Pacific/Kiritimati"); err != nil {
		t.Fatal(err)
	}
	got, err := s.List(ctx, "clock")
	if err != nil || len(got) != 2 || got[0].Name != "Today" {
		t.Fatalf("local new year and perpetual list: %v %v", got, err)
	}
}
