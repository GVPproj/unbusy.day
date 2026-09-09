package components_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/GVPproj/unbusy.day/internal/frontend/components"
	"github.com/GVPproj/unbusy.day/internal/habit"
)

func TestHabitGridHighlightsLocalToday(t *testing.T) {
	// UTC has rolled over, but the viewer is still on September 9.
	now := time.Date(2026, 9, 10, 1, 0, 0, 0, time.UTC)
	for _, key := range []string{"2026-09", "2026-08"} {
		t.Run(key, func(t *testing.T) {
			month, err := habit.CalendarMonth("America/New_York", key, now)
			if err != nil {
				t.Fatal(err)
			}
			var out strings.Builder
			habits := []habit.Habit{{ID: 1, Name: "Read", StartDate: "2026-08-01"}}
			if err := components.HabitGrid(habits, month, "", 0).Render(context.Background(), &out); err != nil {
				t.Fatal(err)
			}
			body := out.String()
			if key == "2026-09" {
				if !strings.Contains(body, `<time datetime="2026-09-09" aria-current="date">`) {
					t.Error("local today must be marked as the current date")
				}
				if got := strings.Count(body, `class="today"`); got != 2 {
					t.Errorf("got %d today cells, want header and one habit cell", got)
				}
			} else if strings.Contains(body, `class="today"`) || strings.Contains(body, `aria-current="date"`) {
				t.Error("past month must not highlight today")
			}
		})
	}
}
