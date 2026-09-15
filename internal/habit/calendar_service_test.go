package habit_test

import (
	"testing"
	"time"

	"github.com/GVPproj/unbusy.day/internal/habit"
)

func TestCurrentWeekRemainsAvailableWithoutHabitStorage(t *testing.T) {
	db, _ := database(t)
	now := time.Date(2024, 3, 1, 0, 30, 0, 0, time.UTC)
	svc := habit.NewService(db, nil, habit.WithClock(func() time.Time { return now }))
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	week, err := svc.CurrentWeek("America/Los_Angeles")
	if err != nil || week.Today != "2024-02-29" || week.Key != "2024-02-25" || len(week.Dates) != 7 {
		t.Fatalf("local leap day without storage: %+v %v", week, err)
	}
	now = time.Date(2024, 12, 31, 12, 30, 0, 0, time.UTC)
	week, err = svc.CurrentWeek("Pacific/Kiritimati")
	if err != nil || week.Today != "2025-01-01" || week.Key != "2024-12-29" {
		t.Fatalf("local new year without storage: %+v %v", week, err)
	}
	for _, zone := range []string{"", "Local", "Not/AZone"} {
		if _, err := svc.CurrentWeek(zone); !habit.IsRejection(err) {
			t.Errorf("timezone %q: %v", zone, err)
		}
	}
}
