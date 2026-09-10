package habit_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/GVPproj/unbusy.day/internal/habit"
)

func TestMutationsRequireCanonicalCivilDates(t *testing.T) {
	db, _ := database(t)
	ctx := context.Background()
	published := 0
	s := habit.NewService(db, publisherFunc(func(habit.Event) { published++ }),
		habit.WithClock(func() time.Time { return time.Date(2024, 3, 1, 12, 0, 0, 0, time.UTC) }))
	if err := s.Create(ctx, "alice", "Read", "2024-01-01", "UTC"); err != nil {
		t.Fatal(err)
	}
	before, err := s.List(ctx, "alice")
	if err != nil {
		t.Fatal(err)
	}
	id := before[0].ID
	for _, date := range []string{"2024-02- 1", "2024-02-1", "2024-2-01", "2023-02-29", "2024-02-30", "2024-02-01T00:00:00Z", " 2024-02-01", ""} {
		t.Run(date, func(t *testing.T) {
			for name, mutate := range map[string]func() error{
				"create":  func() error { return s.Create(ctx, "alice", "Walk", date, "UTC") },
				"edit":    func() error { return s.Edit(ctx, "alice", id, "Renamed", date, "UTC") },
				"check":   func() error { return s.SetCheckIn(ctx, "alice", id, date, true, "UTC") },
				"uncheck": func() error { return s.SetCheckIn(ctx, "alice", id, date, false, "UTC") },
			} {
				t.Run(name, func(t *testing.T) {
					if err := mutate(); !habit.IsRejection(err) {
						t.Fatalf("expected invalid date rejection, got %v", err)
					}
				})
			}
		})
	}
	after, err := s.List(ctx, "alice")
	if err != nil || !reflect.DeepEqual(before, after) || published != 1 {
		t.Fatalf("rejected dates changed state: habits=%+v publications=%d err=%v", after, published, err)
	}
	if err := s.Edit(ctx, "alice", id, "Read", "2024-02-29", "UTC"); err != nil {
		t.Fatalf("valid leap-day start: %v", err)
	}
	if err := s.SetCheckIn(ctx, "alice", id, "2024-02-29", true, "UTC"); err != nil {
		t.Fatalf("valid leap-day check-in: %v", err)
	}
}
