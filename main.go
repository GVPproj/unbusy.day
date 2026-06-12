package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/grahamvanpelt/unbusy.day/cards"
	"github.com/grahamvanpelt/unbusy.day/ds"
	"github.com/grahamvanpelt/unbusy.day/pubsub"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	ctx := context.Background()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL is required")
	}
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Fatalf("pgxpool: %v", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("ping db: %v", err)
	}

	// `hello-cards migrate` applies embedded migrations and exits. Fly's
	// release_command runs this before the rollout so schema lands ahead of
	// the new binary (expand-then-deploy).
	if len(os.Args) > 1 && os.Args[1] == "migrate" {
		if err := runMigrations(ctx, pool); err != nil {
			log.Fatalf("migrate: %v", err)
		}
		log.Println("migrations applied")
		return
	}

	broker := pubsub.New()
	svc := cards.NewService(pool, broker)

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
	mux.Handle("GET /{$}", ds.PageHandler(svc))
	mux.Handle("GET /login", ds.LoginPageHandler())
	mux.Handle("POST /login", ds.LoginActionHandler())
	mux.Handle("GET /events", ds.EventsHandler(svc, broker))
	mux.Handle("POST /cards/reorder", ds.ReorderHandler(svc))
	mux.Handle("POST /cards/resize", ds.ResizeHandler(svc))

	// Wiring canary for the pinned Datastar SDK + templ versions.
	mux.Handle("GET /_smoke", ds.SmokeHandler())
	mux.Handle("GET /_smoke/events", ds.SmokeEventsHandler())

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
