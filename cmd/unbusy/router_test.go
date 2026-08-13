package main

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/GVPproj/unbusy.day/internal/auth"
	"github.com/GVPproj/unbusy.day/internal/block"
	"github.com/GVPproj/unbusy.day/internal/jot"
	"github.com/GVPproj/unbusy.day/internal/migrate"
	"github.com/GVPproj/unbusy.day/internal/pubsub"
)

func testRouter(t *testing.T) *http.ServeMux {
	t.Helper()
	dsn := "file:" + filepath.Join(t.TempDir(), "router.db")
	if err := migrate.Run(context.Background(), dsn); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	broker := pubsub.New()
	return newRouter(
		auth.NewService(db, auth.LogMailer{}),
		block.NewService(db, broker),
		jot.NewService(db, broker),
		broker,
		routerConfig{},
	)
}

// The route table's core contract: app routes sit behind RequireSession
// (anonymous → 303 for the page, 401 for SSE/mutations), while login,
// health, assets, and the smoke canary stay open.
func TestRouterSessionGating(t *testing.T) {
	mux := testRouter(t)

	cases := []struct {
		method, path string
		wantStatus   int
	}{
		// Gated: the page bounces, SSE and mutations get a bare 401.
		{"GET", "/", http.StatusSeeOther},
		{"GET", "/events", http.StatusUnauthorized},
		{"POST", "/blocks/layout", http.StatusUnauthorized},
		{"POST", "/blocks/bounds", http.StatusUnauthorized},
		{"POST", "/blocks", http.StatusUnauthorized},
		{"POST", "/blocks/delete", http.StatusUnauthorized},
		{"POST", "/blocks/clear", http.StatusUnauthorized},
		{"POST", "/blocks/rename", http.StatusUnauthorized},
		{"POST", "/jot", http.StatusUnauthorized},
		// Ungated.
		{"GET", "/healthz", http.StatusOK},
		{"GET", "/login", http.StatusOK},
		{"GET", "/static/css/app.css", http.StatusOK},
		{"GET", "/sw.js", http.StatusOK},
		{"GET", "/_smoke", http.StatusOK},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(tc.method, tc.path, nil))
		if rec.Code != tc.wantStatus {
			t.Errorf("%s %s: got %d, want %d", tc.method, tc.path, rec.Code, tc.wantStatus)
		}
	}

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if loc := rec.Header().Get("Location"); loc != "/login" {
		t.Errorf("anonymous GET / redirects to %q, want /login", loc)
	}
}

// The SES webhook mounts only when a topic ARN is configured.
func TestRouterSESWebhookGatedByConfig(t *testing.T) {
	mux := testRouter(t)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("POST", "/webhooks/ses", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("no ARN configured: POST /webhooks/ses got %d, want 404", rec.Code)
	}
}
