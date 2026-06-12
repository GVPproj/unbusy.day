package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/GVPproj/unbusy.day/auth"
	"github.com/GVPproj/unbusy.day/cards"
	"github.com/GVPproj/unbusy.day/frontend"
	"github.com/GVPproj/unbusy.day/pubsub"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	ctx := context.Background()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL is required")
	}
	// `hello-cards migrate` applies embedded migrations and exits. Fly's
	// release_command runs this before the rollout so schema lands ahead of
	// the new binary (expand-then-deploy).
	if len(os.Args) > 1 && os.Args[1] == "migrate" {
		if err := runMigrations(ctx, dbURL); err != nil {
			log.Fatalf("migrate: %v", err)
		}
		log.Println("migrations applied")
		return
	}

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Fatalf("pgxpool: %v", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("ping db: %v", err)
	}

	broker := pubsub.New()
	svc := cards.NewService(pool, broker)
	authSvc := auth.NewService(pool, auth.LogMailer{})
	// Secure cookies in production only (ADR 0002) — Fly sets FLY_APP_NAME.
	secureCookies := os.Getenv("FLY_APP_NAME") != ""

	mux := http.NewServeMux()

	// In-process 200 only. Never query Postgres here — Fly's health check
	// fires every few seconds and a DB ping would defeat Neon's scale-to-zero.
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})

	// Datastar + templ frontend, server-rendered over the cards service and
	// pub/sub: the page renders the authoritative order, the events stream
	// fans every mutation to all subscribers as element patches, and the
	// reorder endpoint commits a drop and patches the column back.
	mux.Handle("GET /{$}", frontend.RequireSession(authSvc, frontend.PageHandler(svc)))
	mux.Handle("GET /login", frontend.LoginPageHandler())
	mux.Handle("POST /login/code", frontend.RequestCodeHandler(authSvc))
	mux.Handle("POST /login/verify", frontend.VerifyCodeHandler(authSvc, svc, secureCookies))
	mux.Handle("POST /logout", frontend.LogoutHandler(authSvc, secureCookies))
	mux.Handle("GET /events", frontend.RequireSession(authSvc, frontend.EventsHandler(svc, broker)))
	mux.Handle("POST /cards/reorder", frontend.RequireSession(authSvc, frontend.ReorderHandler(svc)))
	mux.Handle("POST /cards/resize", frontend.RequireSession(authSvc, frontend.ResizeHandler(svc)))

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
	log.Printf("hello-cards listening on :%s", port)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
