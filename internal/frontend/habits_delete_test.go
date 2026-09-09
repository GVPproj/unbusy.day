package frontend

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GVPproj/unbusy.day/internal/habit"
)

func TestHabitDeleteStorageFailureDoesNotAcknowledgeDeletion(t *testing.T) {
	db := habitTestDB(t)
	svc := habit.NewService(db, nil)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	HabitDeleteHandler(svc).ServeHTTP(rec, authedRequest(http.MethodPost, "/habits/delete", `{"habitdeleteid":1,"habitdeleteview":7}`))
	if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), "_habitdeletesavedview") {
		t.Fatalf("storage failure: %d %s", rec.Code, rec.Body.String())
	}
}

func TestHabitDeleteAcknowledgesOwnedDeletionAndRetriesWithoutGridPatches(t *testing.T) {
	svc := newTestHabits(t)
	ctx := context.Background()
	if err := svc.Create(ctx, testOwner, "Read", "2020-01-01", "UTC"); err != nil {
		t.Fatal(err)
	}
	mine, err := svc.List(ctx, testOwner)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Create(ctx, "another-owner", "Private", "2020-01-01", "UTC"); err != nil {
		t.Fatal(err)
	}
	other, err := svc.List(ctx, "another-owner")
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{other[0].ID, mine[0].ID, mine[0].ID, 999999} {
		rec := httptest.NewRecorder()
		body := fmt.Sprintf(`{"habitdeleteid":%d,"habitdeleteview":7,"owner":"another-owner"}`, id)
		HabitDeleteHandler(svc).ServeHTTP(rec, authedRequest(http.MethodPost, "/habits/delete", body))
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"_habitdeletesavedview":7`) {
			t.Fatalf("ack: %d %s", rec.Code, rec.Body.String())
		}
		for _, absent := range []string{`id="habit-grid"`, `id="jot-cm"`, `id="block-list"`, "Private"} {
			if strings.Contains(rec.Body.String(), absent) {
				t.Errorf("unrelated patch %q: %s", absent, rec.Body.String())
			}
		}
	}
	got, err := svc.List(ctx, testOwner)
	if err != nil || len(got) != 0 {
		t.Fatalf("owned state: %+v %v", got, err)
	}
	if err := svc.Create(ctx, testOwner, "Read", "2020-01-01", "UTC"); err != nil {
		t.Fatal(err)
	}
	replacement, err := svc.List(ctx, testOwner)
	if err != nil {
		t.Fatal(err)
	}
	stale := httptest.NewRecorder()
	HabitCheckInHandler(svc).ServeHTTP(stale, authedRequest(http.MethodPost, "/habits/check-in", fmt.Sprintf(`{"habitid":%d,"habitdate":"2024-01-01","habitchecked":true,"timezone":"UTC"}`, mine[0].ID)))
	if stale.Code != http.StatusOK || !strings.Contains(stale.Body.String(), "Habit not found") {
		t.Fatalf("stale check-in: %d %s", stale.Code, stale.Body.String())
	}
	got, err = svc.List(ctx, testOwner)
	if err != nil || len(got) != 1 || got[0].ID != replacement[0].ID || len(got[0].CheckedDates) != 0 {
		t.Fatalf("stale check-in changed replacement: %+v %v", got, err)
	}
	got, err = svc.List(ctx, "another-owner")
	if err != nil || len(got) != 1 || got[0].ID != other[0].ID {
		t.Fatalf("private state: %+v %v", got, err)
	}
	rec := httptest.NewRecorder()
	HabitDeleteHandler(svc).ServeHTTP(rec, authedRequest(http.MethodPost, "/habits/delete", "not json"))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("malformed: %d", rec.Code)
	}
}
