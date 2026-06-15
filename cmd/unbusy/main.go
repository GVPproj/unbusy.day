package main

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/GVPproj/unbusy.day/internal/auth"
	"github.com/GVPproj/unbusy.day/internal/block"
	"github.com/GVPproj/unbusy.day/internal/frontend"
	"github.com/GVPproj/unbusy.day/internal/migrate"
	"github.com/GVPproj/unbusy.day/internal/pubsub"
	_ "modernc.org/sqlite"
)

func main() {
	ctx := context.Background()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL is required")
	}

	// `unbusy migrate` applies migrations and exits — an ops escape hatch.
	if len(os.Args) > 1 && os.Args[1] == "migrate" {
		if err := migrate.Run(ctx, dbURL); err != nil {
			log.Fatalf("migrate: %v", err)
		}
		log.Println("migrations applied")
		return
	}

	// Migrate on boot: this machine mounts the volume holding the SQLite file,
	// so schema lands against the real database before serving.
	if err := migrate.Run(ctx, dbURL); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	db, err := sql.Open("sqlite", dbURL)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		log.Fatalf("ping db: %v", err)
	}

	broker := pubsub.New()
	svc := block.NewService(db, broker)
	authSvc := auth.NewService(db, auth.LogMailer{})
	// Secure cookies in production only (ADR 0002) — Fly sets FLY_APP_NAME.
	secureCookies := os.Getenv("FLY_APP_NAME") != ""

	mux := http.NewServeMux()

	// In-process 200 only — a liveness probe, not a DB readiness check.
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})

	// Datastar + templ frontend, server-rendered over the blocks service and
	// pub/sub: the page renders the authoritative order, the events stream
	// fans every mutation to all subscribers as element patches, and the
	// reorder endpoint commits a drop and patches the column back.
	mux.Handle("GET /{$}", frontend.RequireSession(authSvc, frontend.PageHandler(svc)))
	mux.Handle("GET /login", frontend.LoginPageHandler())
	mux.Handle("POST /login/code", frontend.RequestCodeHandler(authSvc))
	mux.Handle("POST /login/verify", frontend.VerifyCodeHandler(authSvc, svc, secureCookies))
	mux.Handle("POST /logout", frontend.LogoutHandler(authSvc, secureCookies))
	mux.Handle("GET /events", frontend.RequireSession(authSvc, frontend.EventsHandler(svc, broker)))
	mux.Handle("POST /blocks/layout", frontend.RequireSession(authSvc, frontend.LayoutHandler(svc)))
	mux.Handle("POST /blocks/bounds", frontend.RequireSession(authSvc, frontend.BoundsHandler(svc)))

	// Embedded frontend assets (drag.js). Session-free by design: a cached
	// asset is not user data.
	mux.Handle("GET /static/", frontend.StaticHandler())

	// Wiring canary for the pinned Datastar SDK + templ versions.
	mux.Handle("GET /_smoke", frontend.SmokeHandler())
	mux.Handle("GET /_smoke/events", frontend.SmokeEventsHandler())

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// WriteTimeout stays 0 (disabled): SSE streams are long-lived and the
	// handler manages its own liveness via the 25s keepalive.
	// ReadHeaderTimeout guards the non-streaming routes against slow-loris.
	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Printf("unbusy listening on :%s", port)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
