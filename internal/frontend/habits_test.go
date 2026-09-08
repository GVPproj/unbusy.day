package frontend

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

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

func TestHabitCreationPatchesOnlyTheOwnersAuthoritativeMatrix(t *testing.T) {
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
	for _, want := range []string{"datastar-patch-elements", `id="habit-matrix"`, "Read &lt;books&gt;", `id="habit-feedback"`, "Habit created"} {
		if !strings.Contains(body, want) {
			t.Errorf("response missing %q: %s", want, body)
		}
	}
	for _, absent := range []string{"Private habit", `id="jot-cm"`, `id="block-list"`, `id="habit-create"`} {
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
	for _, want := range []string{`id="habit-matrix"`, "Morning walk", "Read"} {
		if !strings.Contains(frame, want) {
			t.Errorf("live snapshot missing %q: %s", want, frame)
		}
	}
	for _, absent := range []string{`id="jot-cm"`, `id="block-list"`, `id="habit-create"`, "Private habit"} {
		if strings.Contains(frame, absent) {
			t.Errorf("unrelated/private content in live habit patch: %s", frame)
		}
	}
}
