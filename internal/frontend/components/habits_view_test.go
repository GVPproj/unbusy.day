package components_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/GVPproj/unbusy.day/internal/frontend/components"
	"github.com/GVPproj/unbusy.day/internal/habit"
)

func TestHabitViewSelectors(t *testing.T) {
	view := components.HabitView{Refresh: strings.Repeat("a", 32), View: 7, Read: 13}
	want := `#habit-grid[data-month="2024-03"][data-refresh="aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"][data-view="7"]`
	if got := view.GridSelector("2024-03"); got != want {
		t.Fatalf("grid selector = %q, want %q", got, want)
	}
	if got := view.ReadSelector("2024-03"); got != want+`[data-read-request="13"]` {
		t.Fatalf("read selector = %q", got)
	}
	if got := view.GridSelector(""); got != `#habit-grid[data-refresh="aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"][data-view="7"]` {
		t.Fatalf("monthless feedback selector = %q", got)
	}
}

func TestHabitGridPreservesCorrelationAttributes(t *testing.T) {
	view := components.HabitView{Refresh: strings.Repeat("b", 32), View: 7, Read: 13}
	month := habit.Month{Key: "2024-03"}
	var got bytes.Buffer
	if err := components.HabitGrid(nil, month, view).Render(context.Background(), &got); err != nil {
		t.Fatal(err)
	}
	for _, attr := range []string{
		`data-month="2024-03"`, `data-refresh="bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"`,
		`data-view="7"`, `data-read="13"`, `data-read-request="13"`,
		`data-attr:data-month="$habitmonth"`, `data-attr:data-refresh="$habitrefresh"`,
		`data-attr:data-view="$habitview"`, `data-attr:data-read-request="$habitread"`,
	} {
		if !strings.Contains(got.String(), attr) {
			t.Errorf("grid missing %s", attr)
		}
	}
	if strings.Contains(got.String(), `data-attr:data-read=`) {
		t.Fatal("rendered read must remain immutable")
	}
}
