package frontend

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GVPproj/unbusy.day/internal/block"
	"github.com/GVPproj/unbusy.day/internal/habit"
	"github.com/GVPproj/unbusy.day/internal/jot"
	"github.com/GVPproj/unbusy.day/internal/migrate"
	"github.com/GVPproj/unbusy.day/internal/pubsub"
	"github.com/GVPproj/unbusy.day/internal/web"
	_ "modernc.org/sqlite"
)

func habitTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := "file:" + filepath.Join(t.TempDir(), "habits.db") + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_txlock=immediate"
	if err := migrate.Run(context.Background(), dsn); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	for _, owner := range []string{testOwner, "another-owner"} {
		if _, err := db.Exec(`INSERT INTO "user" (id, email) VALUES (?, ?)`, owner, owner+"@example.test"); err != nil {
			t.Fatal(err)
		}
	}
	return db
}

func newTestHabits(t *testing.T) *habit.Service {
	t.Helper()
	return habit.NewService(habitTestDB(t), nil)
}

func TestHabitCheckInMutationConfirmsCommittedOwnedStateWithoutAStaleGridPatch(t *testing.T) {
	svc := newTestHabits(t)
	ctx := context.Background()
	mine, err := svc.Create(ctx, testOwner, "Read", "2020-01-01", "UTC")
	if err != nil {
		t.Fatal(err)
	}
	other, err := svc.Create(ctx, "another-owner", "Private", "2020-01-01", "UTC")
	if err != nil {
		t.Fatal(err)
	}
	today := time.Now().UTC().Format(time.DateOnly)

	post := func(body string) *httptest.ResponseRecorder {
		t.Helper()
		rec := httptest.NewRecorder()
		HabitCheckInHandler(svc).ServeHTTP(rec, authedRequest(http.MethodPost, "/habits/check-in", body))
		return rec
	}
	body := fmt.Sprintf(`{"habitid":%d,"habitdate":%q,"habitchecked":true,"timezone":"UTC","owner":"another-owner"}`, mine[0].ID, today)
	for range 2 {
		rec := post(body)
		if rec.Code != http.StatusOK {
			t.Fatalf("check status %d: %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "Saved") {
			t.Errorf("checked response omitted confirmation: %s", rec.Body.String())
		}
		for _, absent := range []string{"Private", `id="habit-grid"`, `id="jot-cm"`, `id="block-list"`} {
			if strings.Contains(rec.Body.String(), absent) {
				t.Errorf("checked patch contains %q", absent)
			}
		}
	}
	got, err := svc.List(ctx, testOwner)
	if err != nil || !reflect.DeepEqual(got[0].CheckedDates, []string{today}) {
		t.Fatalf("checked state: %+v %v", got, err)
	}

	rec := post(fmt.Sprintf(`{"habitid":%d,"habitdate":%q,"habitchecked":false,"timezone":"UTC"}`, mine[0].ID, today))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Saved") {
		t.Fatalf("uncheck: %d %s", rec.Code, rec.Body.String())
	}
	got, err = svc.List(ctx, testOwner)
	if err != nil || len(got[0].CheckedDates) != 0 {
		t.Fatalf("unchecked state: %+v %v", got, err)
	}

	rec = post(fmt.Sprintf(`{"habitid":%d,"habitdate":%q,"habitchecked":true,"timezone":"UTC"}`, other[0].ID, today))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Habit not found") || strings.Contains(rec.Body.String(), "Private") {
		t.Fatalf("cross-owner rejection: %d %s", rec.Code, rec.Body.String())
	}
}

func TestHabitCheckInRequiresAnExplicitDesiredState(t *testing.T) {
	svc := newTestHabits(t)
	hs, err := svc.Create(context.Background(), testOwner, "Read", "2020-01-01", "UTC")
	if err != nil {
		t.Fatal(err)
	}
	today := time.Now().UTC().Format(time.DateOnly)
	for _, body := range []string{
		fmt.Sprintf(`{"habitid":%d,"habitdate":%q,"timezone":"UTC"}`, hs[0].ID, today),
		`not json`,
	} {
		rec := httptest.NewRecorder()
		HabitCheckInHandler(svc).ServeHTTP(rec, authedRequest(http.MethodPost, "/habits/check-in", body))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("missing/malformed explicit state: %d %s", rec.Code, rec.Body.String())
		}
	}
}

func TestHabitCheckInRejectsInvalidInputWithAuthoritativeReconciliation(t *testing.T) {
	svc := newTestHabits(t)
	hs, err := svc.Create(context.Background(), testOwner, "Read", "2024-02-28", "UTC")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, date, zone, feedback string }{
		{"pre-start", "2024-02-27", "UTC", "before"},
		{"invalid date", "2024-02-30", "UTC", "valid"},
		{"future", "9999-01-01", "UTC", "after today"},
		{"invalid timezone", "2024-02-28", "Not/AZone", "timezone"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			body := fmt.Sprintf(`{"habitid":%d,"habitdate":%q,"habitchecked":true,"timezone":%q}`, hs[0].ID, tc.date, tc.zone)
			HabitCheckInHandler(svc).ServeHTTP(rec, authedRequest(http.MethodPost, "/habits/check-in", body))
			if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), tc.feedback) {
				t.Fatalf("rejection: %d %s", rec.Code, rec.Body.String())
			}
			if strings.Contains(rec.Body.String(), `id="habit-grid"`) {
				t.Fatalf("rejection bypassed the ordered live grid stream: %s", rec.Body.String())
			}
		})
	}
}

func TestHabitCreationConfirmsWithoutBypassingTheOwnersLiveGridStream(t *testing.T) {
	svc := newTestHabits(t)
	if _, err := svc.Create(context.Background(), "another-owner", "Private habit", "2020-01-01", "UTC"); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	HabitCreateHandler(svc).ServeHTTP(rec, authedRequest(http.MethodPost, "/habits", `{"habitname":"  Read <books>  ","habitstart":"2020-01-01","timezone":"Pacific/Kiritimati","owner":"another-owner"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{"datastar-patch-elements", `id="habit-feedback"`, "Habit created"} {
		if !strings.Contains(body, want) {
			t.Errorf("response missing %q: %s", want, body)
		}
	}
	for _, absent := range []string{"Private habit", "Read &lt;books&gt;", `id="habit-grid"`, `id="jot-cm"`, `id="block-list"`, `id="habit-create"`} {
		if strings.Contains(body, absent) {
			t.Errorf("habit write patched unrelated/private content %q", absent)
		}
	}
	got, err := svc.List(context.Background(), testOwner)
	if err != nil || len(got) != 1 || got[0].Name != "Read <books>" {
		t.Fatalf("persisted habits: %+v, %v", got, err)
	}
}

func TestHabitRejectionsKeepTheDraftAndExplainTheProblem(t *testing.T) {
	svc := newTestHabits(t)
	if _, err := svc.Create(context.Background(), testOwner, "Read", "2020-01-01", "UTC"); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, body, feedback string }{
		{"duplicate", `{"habitname":"READ","habitstart":"2020-01-01","timezone":"UTC"}`, "already exists"},
		{"blank", `{"habitname":"   ","habitstart":"2020-01-01","timezone":"UTC"}`, "name"},
		{"future", `{"habitname":"Walk","habitstart":"9999-01-01","timezone":"UTC"}`, "after today"},
		{"invalid date", `{"habitname":"Walk","habitstart":"2025-02-29","timezone":"UTC"}`, "valid start date"},
		{"invalid timezone", `{"habitname":"Walk","habitstart":"2020-01-01","timezone":"Not/AZone"}`, "timezone"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			HabitCreateHandler(svc).ServeHTTP(rec, authedRequest(http.MethodPost, "/habits", tc.body))
			body := rec.Body.String()
			if rec.Code != http.StatusOK || !strings.Contains(body, tc.feedback) || !strings.Contains(body, `id="habit-feedback"`) {
				t.Fatalf("rejection: %d %s", rec.Code, body)
			}
			for _, absent := range []string{`id="habit-create"`, "datastar-patch-signals", `id="jot-cm"`} {
				if strings.Contains(body, absent) {
					t.Errorf("rejection overwrites draft/editor: %s", body)
				}
			}
		})
	}
}

func TestHabitCreationSurvivesPageReloadAndIsIndependentOfPlanAndJotpad(t *testing.T) {
	db := habitTestDB(t)
	habits := habit.NewService(db, nil)
	blocks := block.NewService(db, nil)
	jots := jot.NewService(db, nil)
	ctx := context.Background()
	if _, err := jots.Set(ctx, testOwner, "Keep these notes", 0); err != nil {
		t.Fatal(err)
	}
	if _, err := blocks.Create(ctx, testOwner, "Focus", 18, block.BlockType("deep")); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	HabitCreateHandler(habits).ServeHTTP(rec, authedRequest(http.MethodPost, "/habits", `{"habitname":"Read","habitstart":"2020-01-01","timezone":"UTC"}`))
	if rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	bs, err := blocks.List(ctx, testOwner)
	if err != nil || len(bs) != 1 || bs[0].Label != "Focus" {
		t.Fatalf("habit write changed plan: %+v, %v", bs, err)
	}
	clear := httptest.NewRecorder()
	ClearHandler(blocks).ServeHTTP(clear, authedRequest(http.MethodPost, "/blocks/clear", `{}`))
	if clear.Code != 200 {
		t.Fatal(clear.Body.String())
	}
	page := httptest.NewRecorder()
	PageHandler(blocks, jots, habit.NewService(db, nil)).ServeHTTP(page, authedRequest(http.MethodGet, "/", ""))
	if page.Code != 200 || !strings.Contains(page.Body.String(), "Read</th>") {
		t.Fatalf("reload lost habit: %d %s", page.Code, page.Body.String())
	}
	if got := jotPayload(t, page.Body.String()); got != "Keep these notes" {
		t.Fatalf("notes changed: %q", got)
	}
	other := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	PageHandler(blocks, jots, habits).ServeHTTP(other, req.WithContext(web.WithOwner(req.Context(), "another-owner")))
	if strings.Contains(other.Body.String(), "Read</th>") || strings.Contains(other.Body.String(), "Keep these notes") {
		t.Fatal("page leaked another owner's data")
	}
}

func TestHabitCreationRejectsMalformedJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	HabitCreateHandler(newTestHabits(t)).ServeHTTP(rec, authedRequest(http.MethodPost, "/habits", `not json`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d", rec.Code)
	}
}

func TestHabitEventsRejectInvalidTimezoneBeforeOpeningTheStream(t *testing.T) {
	for _, query := range []string{"", `?datastar=%7B%22timezone%22%3A%22Not/AZone%22%7D`, `?datastar=broken`} {
		rec := httptest.NewRecorder()
		EventsHandler(&fakeService{}, newFakeJot(), pubsub.New(), newTestHabits(t)).ServeHTTP(rec, authedRequest(http.MethodGet, "/events"+query, ""))
		if rec.Code != http.StatusBadRequest || strings.Contains(rec.Body.String(), "datastar-patch-elements") {
			t.Fatalf("invalid timezone started stream: %d %s", rec.Code, rec.Body.String())
		}
	}
}

func TestHabitCheckInStorageFailuresDoNotClaimSuccess(t *testing.T) {
	db := habitTestDB(t)
	svc := habit.NewService(db, nil)
	hs, err := svc.Create(context.Background(), testOwner, "Read", "2020-01-01", "UTC")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	body := fmt.Sprintf(`{"habitid":%d,"habitdate":%q,"habitchecked":true,"timezone":"UTC"}`, hs[0].ID, time.Now().UTC().Format(time.DateOnly))
	HabitCheckInHandler(svc).ServeHTTP(rec, authedRequest(http.MethodPost, "/habits/check-in", body))
	if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), "Saved") || strings.Contains(rec.Body.String(), `id="habit-grid"`) {
		t.Fatalf("storage failure: %d %s", rec.Code, rec.Body.String())
	}
}

func TestHabitStorageFailuresDoNotClaimCreationSucceeded(t *testing.T) {
	db := habitTestDB(t)
	svc := habit.NewService(db, nil)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	HabitCreateHandler(svc).ServeHTTP(rec, authedRequest(http.MethodPost, "/habits", `{"habitname":"Read","habitstart":"2020-01-01","timezone":"UTC"}`))
	if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), "Habit created") {
		t.Fatalf("storage failure: %d %s", rec.Code, rec.Body.String())
	}
}

func TestHabitEventsRefreshEligibilityAtBrowserLocalMidnight(t *testing.T) {
	db := habitTestDB(t)
	created, err := habit.NewService(db, nil).Create(context.Background(), testOwner, "Read", "2024-03-02", "UTC")
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	now := time.Date(2024, 3, 1, 23, 59, 59, 0, time.UTC)
	svc := habit.NewService(db, nil, habit.WithClock(func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		return now
	}))
	oldInterval := keepaliveInterval
	keepaliveInterval = 20 * time.Millisecond
	t.Cleanup(func() { keepaliveInterval = oldInterval })

	_, br := openEvents(t, EventsHandler(&fakeService{}, newFakeJot(), pubsub.New(), svc))
	readFrame(t, br)
	readFrame(t, br)
	readFrame(t, br)
	initial := readFrame(t, br)
	buttonID := fmt.Sprintf(`id="checkin-%d-2024-03-02"`, created[0].ID)
	if strings.Contains(initial, buttonID) {
		t.Fatalf("tomorrow was eligible before midnight: %s", initial)
	}
	mu.Lock()
	now = time.Date(2024, 3, 2, 0, 0, 1, 0, time.UTC)
	mu.Unlock()
	refreshed := readFrame(t, br)
	if !strings.Contains(refreshed, buttonID) {
		t.Fatalf("today did not become eligible after midnight: %s", refreshed)
	}
}

func TestHabitEventsReconnectAndLiveWritesPatchOnlyTheMatrix(t *testing.T) {
	db := habitTestDB(t)
	broker := pubsub.New()
	svc := habit.NewService(db, broker)
	if _, err := svc.Create(context.Background(), testOwner, "Morning walk", "2020-01-01", "UTC"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Create(context.Background(), "another-owner", "Private habit", "2020-01-01", "UTC"); err != nil {
		t.Fatal(err)
	}
	_, br := openEvents(t, EventsHandler(&fakeService{blocks: threeBlocks()}, newFakeJot(), broker, svc))
	readFrame(t, br)
	readFrame(t, br)
	readFrame(t, br)
	frame := readFrame(t, br)
	if !strings.Contains(frame, "Morning walk") || strings.Contains(frame, "Private habit") {
		t.Fatalf("initial habit snapshot: %s", frame)
	}
	if _, err := svc.Create(context.Background(), testOwner, "Read", "2020-01-01", "UTC"); err != nil {
		t.Fatal(err)
	}
	frame = readFrame(t, br)
	for _, want := range []string{`id="habit-grid"`, "Morning walk", "Read"} {
		if !strings.Contains(frame, want) {
			t.Errorf("live snapshot missing %q: %s", want, frame)
		}
	}
	for _, absent := range []string{`id="jot-cm"`, `id="block-list"`, `id="habit-create"`, "Private habit"} {
		if strings.Contains(frame, absent) {
			t.Errorf("unrelated/private content in live habit patch: %s", frame)
		}
	}

	today := time.Now().UTC().Format(time.DateOnly)
	hs, err := svc.List(context.Background(), testOwner)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetCheckIn(context.Background(), testOwner, hs[0].ID, today, true, "UTC"); err != nil {
		t.Fatal(err)
	}
	frame = readFrame(t, br)
	for _, want := range []string{`id="checkin-`, today, `aria-pressed="true"`} {
		if !strings.Contains(frame, want) {
			t.Errorf("live check-in snapshot missing %q: %s", want, frame)
		}
	}
	if strings.Contains(frame, `id="jot-cm"`) || strings.Contains(frame, `id="block-list"`) {
		t.Fatalf("check-in patch disturbed another feature: %s", frame)
	}

	_, reconnect := openEvents(t, EventsHandler(&fakeService{blocks: threeBlocks()}, newFakeJot(), broker, svc))
	readFrame(t, reconnect)
	readFrame(t, reconnect)
	readFrame(t, reconnect)
	frame = readFrame(t, reconnect)
	if !strings.Contains(frame, today) || !strings.Contains(frame, `aria-pressed="true"`) {
		t.Fatalf("reconnect missed authoritative check-in: %s", frame)
	}
}
