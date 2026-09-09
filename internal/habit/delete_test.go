package habit_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/GVPproj/unbusy.day/internal/block"
	"github.com/GVPproj/unbusy.day/internal/habit"
	"github.com/GVPproj/unbusy.day/internal/jot"
	"github.com/GVPproj/unbusy.day/internal/migrate"
	"github.com/pressly/goose/v3"
)

func TestDeleteAfterUpgradeRetainsExistingIdentityHighWaterMark(t *testing.T) {
	ctx := context.Background()
	dsn := "file:" + filepath.Join(t.TempDir(), "upgrade.db") + "?_pragma=foreign_keys(1)&_txlock=immediate"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	provider, err := goose.NewProvider(goose.DialectSQLite3, db, os.DirFS("../migrate/migrations"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, 20260818120000); err != nil {
		t.Fatal(err)
	}
	// Fixture represents a deployed database before durable allocation existed.
	for _, query := range []string{
		`INSERT INTO "user" (id,email) VALUES ('alice','alice@example.test')`,
		`INSERT INTO habit (id,owner_id,name,name_key,start_date) VALUES (41,'alice','Read','READ','2024-01-01')`,
		`INSERT INTO habit_checkin (habit_id,date) VALUES (41,'2024-02-29')`,
	} {
		if _, err := db.ExecContext(ctx, query); err != nil {
			t.Fatal(err)
		}
	}
	if err := migrate.Run(ctx, dsn); err != nil {
		t.Fatal(err)
	}
	s := habit.NewService(db, nil)
	hs, err := s.List(ctx, "alice")
	if err != nil || len(hs) != 1 || hs[0].ID != 41 || !reflect.DeepEqual(hs[0].CheckedDates, []string{"2024-02-29"}) {
		t.Fatalf("upgrade lost history: %+v %v", hs, err)
	}
	if err := s.Delete(ctx, "alice", 41); err != nil {
		t.Fatal(err)
	}
	hs, err = s.Create(ctx, "alice", "Read", "2024-01-01", "UTC")
	if err != nil || len(hs) != 1 || hs[0].ID <= 41 || len(hs[0].CheckedDates) != 0 {
		t.Fatalf("upgrade reused identity/history: %+v %v", hs, err)
	}
}

func TestDeleteNeverReusesMaximumOrLastIdentityAcrossReopen(t *testing.T) {
	db, dsn := database(t)
	ctx := context.Background()
	s := habit.NewService(db, nil)
	hs, err := s.Create(ctx, "alice", "Keep", "2024-01-01", "UTC")
	if err != nil {
		t.Fatal(err)
	}
	keep := hs[0].ID
	hs, err = s.Create(ctx, "alice", "Read", "2024-01-01", "UTC")
	if err != nil {
		t.Fatal(err)
	}
	id := hs[1].ID
	for cycle := range 2 {
		for _, date := range []string{"2024-01-31", "2024-02-29"} {
			if _, err := s.SetCheckIn(ctx, "alice", id, date, true, "UTC"); err != nil {
				t.Fatal(err)
			}
		}
		if err := s.Delete(ctx, "alice", id); err != nil {
			t.Fatal(err)
		}
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
		if err := migrate.Run(ctx, dsn); err != nil {
			t.Fatal(err)
		}
		db, err = sql.Open("sqlite", dsn)
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		s = habit.NewService(db, nil)
		hs, err = s.Create(ctx, "alice", "Read", "2024-01-01", "UTC")
		if err != nil {
			t.Fatal(err)
		}
		fresh := hs[len(hs)-1]
		if fresh.ID <= id || fresh.Name != "Read" || len(fresh.CheckedDates) != 0 {
			t.Fatalf("cycle %d inherited identity/history: %+v (deleted %d)", cycle, fresh, id)
		}
		id = fresh.ID
		if cycle == 0 {
			if err := s.Delete(ctx, "alice", keep); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestDeleteIsOwnerScopedAndPublishesCommittedStateEvenWhenMissing(t *testing.T) {
	db, _ := database(t)
	// A callback read must acquire a new transaction, not observe the deleting one.
	db.SetMaxOpenConns(1)
	ctx := context.Background()
	reader := habit.NewService(db, nil)
	alice, err := reader.Create(ctx, "alice", "Read", "2024-01-01", "UTC")
	if err != nil {
		t.Fatal(err)
	}
	bob, err := reader.Create(ctx, "bob", "Private", "2024-01-01", "UTC")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reader.SetCheckIn(ctx, "bob", bob[0].ID, "2024-02-29", true, "UTC"); err != nil {
		t.Fatal(err)
	}
	bob, err = reader.List(ctx, "bob")
	if err != nil {
		t.Fatal(err)
	}
	want := alice
	published := 0
	s := habit.NewService(db, publisherFunc(func(e habit.Event) {
		published++
		if e.Owner != "alice" {
			t.Errorf("invalidated other owner: %q", e.Owner)
		}
		readCtx, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		got, err := reader.List(readCtx, "alice")
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("postcommit state: %+v %v, want %+v", got, err, want)
		}
	}))
	for _, id := range []int64{bob[0].ID, 999999, -1} {
		if err := s.Delete(ctx, "alice", id); err != nil {
			t.Fatal(err)
		}
	}
	want = []habit.Habit{}
	for range 2 {
		if err := s.Delete(ctx, "alice", alice[0].ID); err != nil {
			t.Fatal(err)
		}
	}
	if published != 5 {
		t.Fatalf("missing reconciliation events: %d", published)
	}
	got, err := reader.List(ctx, "bob")
	if err != nil || !reflect.DeepEqual(got, bob) {
		t.Fatalf("other owner changed: %+v %v", got, err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err := s.Delete(cancelled, "alice", alice[0].ID); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(ctx, "alice", alice[0].ID); err == nil {
		t.Fatal("closed database accepted delete")
	}
	if published != 5 {
		t.Fatalf("failed delete published: %d", published)
	}
}

func TestDeleteConcurrentEditsAndCheckInRetriesNeverRecreate(t *testing.T) {
	db, _ := database(t)
	ctx := context.Background()
	s := habit.NewService(db, nil)
	for range 10 {
		hs, err := s.Create(ctx, "alice", "Read", "2024-01-01", "UTC")
		if err != nil {
			t.Fatal(err)
		}
		id := hs[0].ID
		start := make(chan struct{})
		results := make(chan error, 12)
		for n := range 12 {
			go func(n int) {
				<-start
				writer := habit.NewService(db, nil)
				var err error
				switch n % 3 {
				case 0:
					err = writer.Delete(ctx, "alice", id)
				case 1:
					_, err = writer.Edit(ctx, "alice", id, "Books", "2024-01-01", "UTC")
				case 2:
					_, err = writer.SetCheckIn(ctx, "alice", id, "2024-02-29", true, "UTC")
				}
				if n%3 == 0 && err != nil {
					results <- err
					return
				}
				if habit.IsRejection(err) {
					err = nil
				}
				results <- err
			}(n)
		}
		close(start)
		for range 12 {
			if err := <-results; err != nil {
				t.Fatal(err)
			}
		}
		fresh, err := s.Create(ctx, "alice", "Read", "2024-01-01", "UTC")
		if err != nil {
			t.Fatal(err)
		}
		for range 2 {
			if _, err := s.Edit(ctx, "alice", id, "Resurrected", "2024-01-01", "UTC"); !habit.IsRejection(err) {
				t.Fatalf("stale edit: %v", err)
			}
			for _, checked := range []bool{true, false} {
				if _, err := s.SetCheckIn(ctx, "alice", id, "2024-02-29", checked, "UTC"); !habit.IsRejection(err) {
					t.Fatalf("stale check-in: %v", err)
				}
			}
			if err := s.Delete(ctx, "alice", id); err != nil {
				t.Fatal(err)
			}
		}
		got, err := s.List(ctx, "alice")
		if err != nil || !reflect.DeepEqual(got, fresh) || len(got) != 1 || got[0].ID <= id || len(got[0].CheckedDates) != 0 {
			t.Fatalf("recreated/inherited habit: %+v %v", got, err)
		}
		if err := s.Delete(ctx, "alice", fresh[0].ID); err != nil {
			t.Fatal(err)
		}
		var checkins int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM habit_checkin`).Scan(&checkins); err != nil || checkins != 0 {
			t.Fatalf("orphan check-ins after concurrent deletion: %d %v", checkins, err)
		}
	}
}

func TestDeleteLeavesOtherHabitsDayPlanAndJotpadUntouched(t *testing.T) {
	db, _ := database(t)
	ctx := context.Background()
	s := habit.NewService(db, nil)
	plans := block.NewService(db, nil)
	pads := jot.NewService(db, nil)
	bounds, err := plans.Bounds(ctx, "alice")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := plans.Create(ctx, "alice", "Focus", bounds.Start, block.BlockDeep); err != nil {
		t.Fatal(err)
	}
	plan, err := plans.List(ctx, "alice")
	if err != nil {
		t.Fatal(err)
	}
	pad, err := pads.Set(ctx, "alice", "Keep my notes", 0)
	if err != nil {
		t.Fatal(err)
	}
	hs, err := s.Create(ctx, "alice", "Read", "2024-01-01", "UTC")
	if err != nil {
		t.Fatal(err)
	}
	deleted := hs[0].ID
	hs, err = s.Create(ctx, "alice", "Walk", "2024-01-01", "UTC")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetCheckIn(ctx, "alice", hs[1].ID, "2024-02-29", true, "UTC"); err != nil {
		t.Fatal(err)
	}
	hs, err = s.List(ctx, "alice")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(ctx, "alice", deleted); err != nil {
		t.Fatal(err)
	}
	got, err := s.List(ctx, "alice")
	if err != nil || !reflect.DeepEqual(got, hs[1:]) {
		t.Fatalf("other habit changed: %+v %v", got, err)
	}
	gotPlan, err := plans.List(ctx, "alice")
	if err != nil || !reflect.DeepEqual(gotPlan, plan) {
		t.Fatalf("plan changed: %+v %v", gotPlan, err)
	}
	gotBounds, err := plans.Bounds(ctx, "alice")
	if err != nil || gotBounds != bounds {
		t.Fatalf("bounds changed: %+v %v", gotBounds, err)
	}
	gotPad, err := pads.Get(ctx, "alice")
	if err != nil || gotPad != pad {
		t.Fatalf("jotpad changed: %+v %v", gotPad, err)
	}
	if _, err := plans.Clear(ctx, "alice"); err != nil {
		t.Fatal(err)
	}
	got, err = s.List(ctx, "alice")
	if err != nil || !reflect.DeepEqual(got, hs[1:]) {
		t.Fatalf("plan clear changed habits: %+v %v", got, err)
	}
}

func TestDeleteRemovesHabitAcrossMonths(t *testing.T) {
	db, _ := database(t)
	ctx := context.Background()
	s := habit.NewService(db, nil)
	hs, err := s.Create(ctx, "alice", "Read", "2024-01-01", "UTC")
	if err != nil {
		t.Fatal(err)
	}
	id := hs[0].ID
	for _, date := range []string{"2024-01-31", "2024-02-29", "2024-03-01"} {
		if _, err := s.SetCheckIn(ctx, "alice", id, date, true, "UTC"); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Delete(ctx, "alice", id); err != nil {
		t.Fatal(err)
	}
	got, err := s.List(ctx, "alice")
	if err != nil || len(got) != 0 {
		t.Fatalf("deleted list: %+v %v", got, err)
	}
	// List cannot reveal orphan rows; verify the storage boundary erased history too.
	var checkins int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM habit_checkin WHERE habit_id = ?`, id).Scan(&checkins); err != nil || checkins != 0 {
		t.Fatalf("persisted check-ins after deletion: %d %v", checkins, err)
	}
	for _, month := range []string{"2024-01", "2024-02", "2024-03"} {
		got, err := s.MonthSnapshot(ctx, "alice", "UTC", month)
		if err != nil || len(got.Habits) != 0 {
			t.Fatalf("deleted month %s: %+v %v", month, got, err)
		}
	}
}
