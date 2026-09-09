package frontend

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestHabitCheckInReceiptFencesAnAuthoritativeReadAfterSupersession(t *testing.T) {
	svc := newTestHabits(t)
	ctx := context.Background()
	if err := svc.Create(ctx, testOwner, "Read", "2020-01-01", "UTC"); err != nil {
		t.Fatal(err)
	}
	hs, err := svc.List(ctx, testOwner)
	if err != nil {
		t.Fatal(err)
	}
	today := time.Now().UTC().Format(time.DateOnly)
	post := httptest.NewRecorder()
	HabitCheckInHandler(svc).ServeHTTP(post, authedRequest(http.MethodPost, "/habits/check-in", fmt.Sprintf(`{"habitid":%d,"habitdate":%q,"habitchecked":true,"habitcheckinattempt":17,"timezone":"UTC"}`, hs[0].ID, today)))
	if post.Code != http.StatusOK || !strings.Contains(post.Body.String(), `"_habitcheckinack":"17:committed"`) {
		t.Fatalf("missing correlated commit receipt: %d %s", post.Code, post.Body.String())
	}
	if strings.Contains(post.Body.String(), `id="habit-grid"`) {
		t.Fatal("mutation response must not render a potentially stale grid")
	}
	if err := svc.SetCheckIn(ctx, testOwner, hs[0].ID, today, false, "UTC"); err != nil {
		t.Fatal(err)
	}
	read := httptest.NewRecorder()
	signals := fmt.Sprintf(`{"habitmonth":%q,"habitrefresh":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","habitview":3,"habitread":8,"timezone":"UTC"}`, today[:7])
	HabitMonthHandler(svc).ServeHTTP(read, authedRequest(http.MethodGet, "/habits/month?datastar="+url.QueryEscape(signals), ""))
	for _, want := range []string{
		`selector #habit-grid[data-month="` + today[:7] + `"][data-refresh="aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"][data-view="3"][data-read-request="8"]`,
		`data-read="8"`, `aria-pressed="false"`,
	} {
		if !strings.Contains(read.Body.String(), want) {
			t.Errorf("authoritative read missing %q: %s", want, read.Body.String())
		}
	}
	if strings.Contains(read.Body.String(), `aria-pressed="true"`) {
		t.Fatal("read must contain the last committed state, not the acknowledged desired value")
	}
}

func TestHabitCheckInRejectionReceiptIdentifiesOnlyItsAttempt(t *testing.T) {
	svc := newTestHabits(t)
	post := httptest.NewRecorder()
	HabitCheckInHandler(svc).ServeHTTP(post, authedRequest(http.MethodPost, "/habits/check-in", `{"habitid":123,"habitdate":"2020-01-01","habitchecked":true,"habitcheckinattempt":19,"timezone":"UTC"}`))
	if post.Code != http.StatusOK || !strings.Contains(post.Body.String(), `"_habitcheckinack":"19:rejected"`) || strings.Contains(post.Body.String(), `19:committed`) {
		t.Fatalf("rejection receipt: %d %s", post.Code, post.Body.String())
	}
}
