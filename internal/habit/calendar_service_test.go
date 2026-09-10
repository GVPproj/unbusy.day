package habit_test

import (
	"testing"
	"time"

	"github.com/GVPproj/unbusy.day/internal/habit"
)

func TestCurrentCalendarRemainsAvailableWithoutHabitStorage(t *testing.T) {
	db, _ := database(t)
	now := time.Date(2024, 3, 1, 0, 30, 0, 0, time.UTC)
	svc := habit.NewService(db, nil, habit.WithClock(func() time.Time { return now }))
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	month, err := svc.CurrentCalendar("America/Los_Angeles")
	if err != nil || month.Today != "2024-02-29" || month.Key != "2024-02" || len(month.Dates) != 29 {
		t.Fatalf("local leap day without storage: %+v %v", month, err)
	}
	now = time.Date(2024, 12, 31, 12, 30, 0, 0, time.UTC)
	month, err = svc.CurrentCalendar("Pacific/Kiritimati")
	if err != nil || month.Today != "2025-01-01" || month.Key != "2025-01" {
		t.Fatalf("local new year without storage: %+v %v", month, err)
	}
	for _, zone := range []string{"", "Local", "Not/AZone"} {
		if _, err := svc.CurrentCalendar(zone); !habit.IsRejection(err) {
			t.Errorf("timezone %q: %v", zone, err)
		}
	}
}
