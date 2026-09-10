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

func TestHabitFormResponsesPreserveDialogCorrelation(t *testing.T) {
	for _, action := range []string{"create", "edit"} {
		for _, outcome := range []string{"saved", "rejected", "failed"} {
			t.Run(action+"/"+outcome, func(t *testing.T) {
				db := habitTestDB(t)
				svc := habit.NewService(db, nil)
				if err := svc.Create(context.Background(), testOwner, "Read", "2020-01-01", "UTC"); err != nil {
					t.Fatal(err)
				}
				habits, err := svc.List(context.Background(), testOwner)
				if err != nil {
					t.Fatal(err)
				}
				name := "Walk"
				if outcome == "rejected" {
					name = ""
				}
				if outcome == "failed" {
					if err := db.Close(); err != nil {
						t.Fatal(err)
					}
				}
				handler := HabitCreateHandler(svc)
				path := "/habits"
				body := fmt.Sprintf(`{"habitname":%q,"habitstart":"2020-01-01","habitcreateview":17,"timezone":"UTC"}`, name)
				if action == "edit" {
					handler = HabitEditHandler(svc)
					path = "/habits/edit"
					body = fmt.Sprintf(`{"habiteditid":%d,"habiteditname":%q,"habiteditstart":"2020-01-01","habiteditview":17,"timezone":"UTC"}`, habits[0].ID, name)
				}
				rec := httptest.NewRecorder()
				handler.ServeHTTP(rec, authedRequest(http.MethodPost, path, body))
				body = rec.Body.String()
				status := http.StatusOK
				want := `data: signals {"_habit` + action + `savedview":17}`
				switch outcome {
				case "rejected":
					want = `"_habit` + action + `errorview":17`
					if !strings.Contains(body, "Enter a habit name.") || strings.Contains(body, "savedview") {
						t.Fatalf("rejection response = %s", body)
					}
				case "failed":
					status = http.StatusInternalServerError
					want = "Unable to " + action + " habit. Please try again.\n"
				}
				if rec.Code != status || !strings.Contains(body, want) {
					t.Fatalf("response = %d %s; want %d containing %q", rec.Code, body, status, want)
				}
				if strings.Contains(body, "datastar-patch-elements") || strings.Contains(body, "database is closed") {
					t.Fatalf("unexpected response content: %s", body)
				}
			})
		}
	}
}
