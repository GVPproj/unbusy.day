package frontend

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GVPproj/unbusy.day/internal/block"
	"github.com/GVPproj/unbusy.day/internal/jot"
)

func TestPageHabitLoadingShellDoesNotRequireHabitStorage(t *testing.T) {
	db := habitTestDB(t)
	// Keep the real Plan and Jotpad available, but remove Habit storage.
	for _, query := range []string{`DROP TABLE habit_checkin`, `DROP TABLE habit`} {
		if _, err := db.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	rec := httptest.NewRecorder()
	PageHandler(block.NewService(db, nil), jot.NewService(db, nil)).ServeHTTP(rec, authedRequest(http.MethodGet, "/", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("loading shell: %d %s", rec.Code, rec.Body.String())
	}
	for _, want := range []string{`id="habit-grid"`, "Loading", "/habits/month", `id="block-list"`, `id="jot-cm"`} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Errorf("loading shell missing %q", want)
		}
	}
}
