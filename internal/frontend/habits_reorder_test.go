package frontend

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GVPproj/unbusy.day/internal/habit"
)

func TestHabitReorderPatchesTheCurrentAuthoritativeGrid(t *testing.T) {
	db := habitTestDB(t)
	now := time.Date(2024, 3, 1, 12, 0, 0, 0, time.UTC)
	svc := habit.NewService(db, nil, habit.WithClock(func() time.Time { return now }))
	ctx := context.Background()
	for _, name := range []string{"Read", "Walk", "Write"} {
		if err := svc.Create(ctx, testOwner, name, "2020-01-01", "UTC"); err != nil {
			t.Fatal(err)
		}
	}
	hs, err := svc.List(ctx, testOwner)
	if err != nil {
		t.Fatal(err)
	}
	body := fmt.Sprintf(`{"habitorder":[{"id":%d,"sortOrder":0},{"id":%d,"sortOrder":1},{"id":%d,"sortOrder":2}],"habitweek":"2024-02-25","habitrefresh":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","habitview":7,"habitread":11,"timezone":"UTC"}`, hs[2].ID, hs[0].ID, hs[1].ID)
	rec := httptest.NewRecorder()
	HabitReorderHandler(svc).ServeHTTP(rec, authedRequest(http.MethodPost, "/habits/reorder", body))

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	response := rec.Body.String()
	wantSelector := `selector #habit-grid[data-week="2024-02-25"][data-refresh="aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"][data-view="7"][data-read-request="11"]`
	if !strings.Contains(response, wantSelector) {
		t.Fatalf("missing current-view selector %q: %s", wantSelector, response)
	}
	writeAt, readAt, walkAt := strings.Index(response, ">Write</span>"), strings.Index(response, ">Read</span>"), strings.Index(response, ">Walk</span>")
	if writeAt < 0 || !(writeAt < readAt && readAt < walkAt) {
		t.Fatalf("grid order was not authoritative: %s", response)
	}
	for i, id := range []int64{hs[2].ID, hs[0].ID, hs[1].ID} {
		if !strings.Contains(response, fmt.Sprintf(`id="habit-%d" data-sort-order="%d"`, id, i)) {
			t.Errorf("missing stored order for habit %d: %s", id, response)
		}
	}
}

func TestHabitReorderRejectionRestoresAuthoritativeOrderAndFeedback(t *testing.T) {
	db := habitTestDB(t)
	now := time.Date(2024, 3, 1, 12, 0, 0, 0, time.UTC)
	svc := habit.NewService(db, nil, habit.WithClock(func() time.Time { return now }))
	ctx := context.Background()
	for _, name := range []string{"Read", "Walk"} {
		if err := svc.Create(ctx, testOwner, name, "2020-01-01", "UTC"); err != nil {
			t.Fatal(err)
		}
	}
	hs, _ := svc.List(ctx, testOwner)
	body := fmt.Sprintf(`{"habitorder":[{"id":%d,"sortOrder":0},{"id":%d,"sortOrder":1}],"habitweek":"2024-02-25","habitrefresh":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","habitview":4,"habitread":8,"timezone":"UTC"}`, hs[0].ID, hs[0].ID)
	rec := httptest.NewRecorder()
	HabitReorderHandler(svc).ServeHTTP(rec, authedRequest(http.MethodPost, "/habits/reorder", body))

	response := rec.Body.String()
	if rec.Code != http.StatusOK || !strings.Contains(response, "current habit set") {
		t.Fatalf("rejection: %d %s", rec.Code, response)
	}
	if !strings.Contains(response, `id="habit-grid"`) || !strings.Contains(response, `#habit-reorder-feedback`) {
		t.Fatalf("rejection must restore the grid and explain why: %s", response)
	}
	if readAt, walkAt := strings.Index(response, ">Read</span>"), strings.Index(response, ">Walk</span>"); readAt < 0 || walkAt < readAt {
		t.Fatalf("rejection did not restore stored order: %s", response)
	}
}

func TestHistoricalHabitGridCannotReorderTheGlobalList(t *testing.T) {
	db := habitTestDB(t)
	now := time.Date(2024, 3, 1, 12, 0, 0, 0, time.UTC)
	svc := habit.NewService(db, nil, habit.WithClock(func() time.Time { return now }))
	ctx := context.Background()
	for _, name := range []string{"Read", "Walk"} {
		if err := svc.Create(ctx, testOwner, name, "2020-01-01", "UTC"); err != nil {
			t.Fatal(err)
		}
	}
	hs, _ := svc.List(ctx, testOwner)
	body := fmt.Sprintf(`{"habitorder":[{"id":%d,"sortOrder":0},{"id":%d,"sortOrder":1}],"habitweek":"2024-02-18","habitrefresh":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","habitview":4,"habitread":8,"timezone":"UTC"}`, hs[1].ID, hs[0].ID)
	rec := httptest.NewRecorder()
	HabitReorderHandler(svc).ServeHTTP(rec, authedRequest(http.MethodPost, "/habits/reorder", body))

	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Return to this week") {
		t.Fatalf("historical reorder: %d %s", rec.Code, rec.Body.String())
	}
	after, err := svc.List(ctx, testOwner)
	if err != nil || after[0].ID != hs[0].ID || after[1].ID != hs[1].ID {
		t.Fatalf("historical reorder changed state: %+v, %v", after, err)
	}
}

func TestHabitReorderRejectsMalformedSignals(t *testing.T) {
	rec := httptest.NewRecorder()
	HabitReorderHandler(newTestHabits(t)).ServeHTTP(rec, authedRequest(http.MethodPost, "/habits/reorder", `not json`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
}
