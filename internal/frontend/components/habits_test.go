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

func TestHabitMonthHeading(t *testing.T) {
	for _, tc := range []struct{ key, short string }{
		{"2026-01", "Jan '26"},
		{"2026-05", "May '26"},
		{"2026-09", "Sept '26"},
		{"2026-12", "Dec '26"},
	} {
		t.Run(tc.key, func(t *testing.T) {
			month, err := habit.CalendarMonth("UTC", tc.key, time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC))
			if err != nil {
				t.Fatal(err)
			}
			var out strings.Builder
			if err := components.HabitGrid(nil, month, components.HabitView{}).Render(context.Background(), &out); err != nil {
				t.Fatal(err)
			}
			body := html.UnescapeString(out.String())
			want := `<time datetime="` + tc.key + `" aria-label="` + month.Label + `">` + tc.short + `</time>`
			if !strings.Contains(body, want) {
				t.Errorf("missing compact heading with full accessible label: %s", want)
			}
		})
	}
}
