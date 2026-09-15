package components_test

import (
	"context"
	"html"
	"strings"
	"testing"
	"time"

	"github.com/GVPproj/unbusy.day/internal/frontend/components"
	"github.com/GVPproj/unbusy.day/internal/habit"
)

func TestHabitWeekHeadersRunSundayThroughSaturday(t *testing.T) {
	week, err := habit.CalendarWeek("UTC", "2026-05-31", time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	if err := components.HabitGrid([]habit.Habit{{ID: 1, Name: "Read", StartDate: "2020-01-01"}}, week, components.HabitView{}).Render(context.Background(), &out); err != nil {
		t.Fatal(err)
	}
	body := html.UnescapeString(out.String())
	if !strings.Contains(body, `<span>Sun</span> <time datetime="2026-05-31"`) || !strings.Contains(body, `<span>Sat</span> <time datetime="2026-06-06"`) {
		t.Fatalf("weekly headers do not run Sunday through Saturday: %s", body)
	}
}

func TestHabitWeekHeading(t *testing.T) {
	for _, tc := range []struct{ key, short string }{
		{"2026-01-04", "Jan 4–10 '26"},
		{"2026-05-31", "May 31–Jun 6 '26"},
		{"2026-09-27", "Sept 27–Oct 3 '26"},
		{"2026-12-27", "Dec 27 '26–Jan 2 '27"},
	} {
		t.Run(tc.key, func(t *testing.T) {
			week, err := habit.CalendarWeek("UTC", tc.key, time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC))
			if err != nil {
				t.Fatal(err)
			}
			var out strings.Builder
			if err := components.HabitGrid(nil, week, components.HabitView{}).Render(context.Background(), &out); err != nil {
				t.Fatal(err)
			}
			body := html.UnescapeString(out.String())
			want := `<time datetime="` + tc.key + `" aria-label="` + week.Label + `">` + tc.short + `</time>`
			if !strings.Contains(body, want) {
				t.Errorf("missing compact heading with full accessible label: %s", want)
			}
		})
	}
}
