package habit_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GVPproj/unbusy.day/internal/habit"
	"github.com/GVPproj/unbusy.day/internal/migrate"
	_ "modernc.org/sqlite"
)

func database(t *testing.T) (*sql.DB, string) {
	t.Helper()
	dsn := "file:" + filepath.Join(t.TempDir(), "habits.db") + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_txlock=immediate"
	if err := migrate.Run(context.Background(), dsn); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	for _, owner := range []string{"alice", "bob"} {
		if _, err := db.Exec(`INSERT INTO "user" (id,email) VALUES (?,?)`, owner, owner+"@example.test"); err != nil {
			t.Fatal(err)
		}
	}
	return db, dsn
}

func TestCreateListsOwnedHabitsInCreationOrderAcrossReopen(t *testing.T) {
	db, dsn := database(t)
	ctx := context.Background()
	s := habit.NewService(db, nil)
	empty, err := s.List(ctx, "alice")
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty: %v %v", empty, err)
	}
	first, err := s.Create(ctx, "alice", "Read", "2020-12-31", "UTC")
	if err != nil || len(first) != 1 {
		t.Fatalf("create: %v %v", first, err)
	}
	second, err := s.Create(ctx, "alice", "Walk", "2021-02-01", "UTC")
	if err != nil || len(second) != 2 {
		t.Fatalf("create: %v %v", second, err)
	}
	if second[0] != first[0] || second[1].ID <= second[0].ID || second[1].Name != "Walk" || second[0].StartDate != "2020-12-31" {
		t.Fatalf("order/state: %+v", second)
	}
	other, err := s.List(ctx, "bob")
	if err != nil || len(other) != 0 {
		t.Fatalf("ownership: %v %v", other, err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	got, err := habit.NewService(reopened, nil).List(ctx, "alice")
	if err != nil || !reflect.DeepEqual(got, second) {
		t.Fatalf("persisted: %v %v", got, err)
	}
}

type publisherFunc func(habit.Event)

func (f publisherFunc) PublishHabit(e habit.Event) { f(e) }

func TestPublicationExposesOnlyCommittedState(t *testing.T) {
	db, _ := database(t)
	ctx := context.Background()
	reader := habit.NewService(db, nil)
	published := 0
	s := habit.NewService(db, publisherFunc(func(e habit.Event) {
		published++
		if e.Owner != "alice" {
			t.Errorf("event owner: %q", e.Owner)
		}
		readCtx, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		got, err := reader.List(readCtx, e.Owner)
		if err != nil || len(got) != 1 || got[0].Name != "Read" {
			t.Errorf("postcommit state: %v %v", got, err)
		}
	}))
	if _, err := s.Create(ctx, "alice", "Read", "2020-01-01", "UTC"); err != nil {
		t.Fatal(err)
	}
	if published != 1 {
		t.Fatalf("missing committed invalidation: %d", published)
	}
	if _, err := s.Create(ctx, "alice", "READ", "2020-01-01", "UTC"); !habit.IsRejection(err) {
		t.Fatal(err)
	}
	if _, err := s.Create(ctx, "alice", " ", "2020-01-01", "UTC"); !habit.IsRejection(err) {
		t.Fatal(err)
	}
	if _, err := s.Create(ctx, "missing-owner", "Walk", "2020-01-01", "UTC"); err == nil || habit.IsRejection(err) {
		t.Fatalf("foreign key failure: %v", err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := s.Create(cancelled, "alice", "Walk", "2020-01-01", "UTC"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
	if published != 1 {
		t.Fatalf("published failed mutation: %d", published)
	}
	got, err := reader.List(ctx, "alice")
	if err != nil || len(got) != 1 {
		t.Fatalf("failed mutations changed list: %v %v", got, err)
	}
}

func TestConcurrentUnicodeDuplicatesAreRejectedPerOwner(t *testing.T) {
	db, _ := database(t)
	ctx := context.Background()
	for i, pair := range [][2]string{{"Read", "rEAD"}, {"Σ", "ς"}, {"K", "K"}, {"S", "ſ"}, {"ẞ", "ß"}} {
		t.Run(pair[0], func(t *testing.T) {
			var wg sync.WaitGroup
			results := make(chan error, 12)
			start := make(chan struct{})
			for n := 0; n < 12; n++ {
				wg.Add(1)
				go func(n int) {
					defer wg.Done()
					<-start
					_, err := habit.NewService(db, nil).Create(ctx, "alice", fmt.Sprint(i)+pair[n%2], "2020-01-01", "UTC")
					results <- err
				}(n)
			}
			close(start)
			wg.Wait()
			close(results)
			successes := 0
			for err := range results {
				if err == nil {
					successes++
				} else if !habit.IsRejection(err) {
					t.Fatalf("unexpected error: %v", err)
				}
			}
			if successes != 1 {
				t.Fatalf("successful duplicates: %d", successes)
			}
			s := habit.NewService(db, nil)
			if _, err := s.Create(ctx, "bob", fmt.Sprint(i)+pair[1], "2020-01-01", "UTC"); err != nil {
				t.Fatalf("other owner: %v", err)
			}
		})
	}
	got, err := habit.NewService(db, nil).List(ctx, "alice")
	if err != nil || len(got) != 5 {
		t.Fatalf("stored duplicates: %v %v", got, err)
	}
}

func TestCreateValidatesWithoutChangingState(t *testing.T) {
	db, _ := database(t)
	s := habit.NewService(db, nil)
	ctx := context.Background()
	for _, tc := range []struct{ name, date, zone string }{
		{" \t\n", "2020-01-01", "UTC"},
		{strings.Repeat("界", 81), "2020-01-01", "UTC"},
		{"Read", "2024-02-30", "UTC"},
		{"Read", "2024-2-01", "UTC"},
		{"Read", "", "UTC"},
		{"Read", "9999-01-01", "UTC"},
		{"Read", "2020-01-01", "bad/zone"},
		{"Read", "2020-01-01", ""},
	} {
		_, err := s.Create(ctx, "alice", tc.name, tc.date, tc.zone)
		if !habit.IsRejection(err) || err.Error() == "" {
			t.Fatalf("expected rejection for %+v: %v", tc, err)
		}
		got, err := s.List(ctx, "alice")
		if err != nil || len(got) != 0 {
			t.Fatalf("rejection changed state: %v %v", got, err)
		}
	}
	name := strings.Repeat("界", 80)
	got, err := s.Create(ctx, "alice", " \t"+name+"\n", "2020-02-29", "UTC")
	if err != nil || len(got) != 1 || got[0].Name != name {
		t.Fatalf("trimmed 80 runes: %v %v", got, err)
	}
	if habit.IsRejection(nil) || habit.IsRejection(errors.New("storage failure")) {
		t.Fatal("non-domain error classified as rejection")
	}
}

func TestCalendarUsesLocalMonthAndCivilDates(t *testing.T) {
	for _, tc := range []struct {
		zone, instant, today, label, first, last string
		days                                     int
	}{
		{"America/Los_Angeles", "2024-03-01T00:30:00Z", "2024-02-29", "February 2024", "2024-02-01", "2024-02-29", 29},
		{"Pacific/Kiritimati", "2024-12-31T12:30:00Z", "2025-01-01", "January 2025", "2025-01-01", "2025-01-31", 31},
		{"America/New_York", "2025-03-10T02:00:00Z", "2025-03-09", "March 2025", "2025-03-01", "2025-03-31", 31},
		{"UTC", "2023-02-28T12:00:00Z", "2023-02-28", "February 2023", "2023-02-01", "2023-02-28", 28},
	} {
		t.Run(tc.zone+tc.today, func(t *testing.T) {
			now, err := time.Parse(time.RFC3339, tc.instant)
			if err != nil {
				t.Fatal(err)
			}
			m, err := habit.Calendar(tc.zone, now)
			if err != nil {
				t.Fatal(err)
			}
			if m.Today != tc.today || m.Label != tc.label || len(m.Dates) != tc.days || m.Dates[0] != tc.first || m.Dates[len(m.Dates)-1] != tc.last {
				t.Fatalf("calendar: %+v", m)
			}
			for i, date := range m.Dates {
				parsed, err := time.Parse("2006-01-02", date)
				if err != nil || parsed.Day() != i+1 {
					t.Fatalf("date %d: %s %v", i, date, err)
				}
			}
		})
	}
	for _, zone := range []string{"", "not/a-zone", "Local"} {
		if _, err := habit.Calendar(zone, time.Now()); err == nil {
			t.Errorf("accepted zone %q", zone)
		}
	}
}
