package frontend

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHabitCreationAcknowledgementIsScopedToItsSubmittingOpening(t *testing.T) {
	svc := newTestHabits(t)
	rec := httptest.NewRecorder()
	HabitCreateHandler(svc).ServeHTTP(rec, authedRequest(http.MethodPost, "/habits", `{"habitname":"Read","habitstart":"2020-01-01","habitcreateview":7,"timezone":"UTC"}`))
	body := rec.Body.String()
	if rec.Code != http.StatusOK || !strings.Contains(body, `signals {"_habitcreatesavedview":7}`) {
		t.Fatalf("creation acknowledgement lost its opening: %d %s", rec.Code, body)
	}
	for _, absent := range []string{"datastar-patch-elements", `"_habitcreated":true`, `"habitcreateview":`} {
		if strings.Contains(body, absent) {
			t.Errorf("creation response updates unscoped state %q: %s", absent, body)
		}
	}
	hs, err := svc.List(context.Background(), testOwner)
	if err != nil || len(hs) != 1 || hs[0].Name != "Read" {
		t.Fatalf("creation was not persisted: %+v %v", hs, err)
	}
}

func TestHabitCreationRejectionIsScopedToItsSubmittingOpening(t *testing.T) {
	svc := newTestHabits(t)
	rec := httptest.NewRecorder()
	HabitCreateHandler(svc).ServeHTTP(rec, authedRequest(http.MethodPost, "/habits", `{"habitname":"   ","habitstart":"2020-01-01","habitcreateview":9,"timezone":"UTC"}`))
	body := rec.Body.String()
	for _, want := range []string{"datastar-patch-signals", `"_habitcreateerror":`, `"_habitcreateerrorview":9`, "name"} {
		if rec.Code != http.StatusOK || !strings.Contains(body, want) {
			t.Errorf("creation rejection missing %q: %d %s", want, rec.Code, body)
		}
	}
	for _, absent := range []string{"datastar-patch-elements", "_habitcreatesavedview", `"habitcreateview":`} {
		if strings.Contains(body, absent) {
			t.Errorf("creation rejection updates unscoped state %q: %s", absent, body)
		}
	}
	hs, err := svc.List(context.Background(), testOwner)
	if err != nil || len(hs) != 0 {
		t.Fatalf("rejection persisted a habit: %+v %v", hs, err)
	}
}
