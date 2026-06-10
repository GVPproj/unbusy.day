package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/grahamvanpelt/unbusy.day/cards"
	"github.com/grahamvanpelt/unbusy.day/ds"
	"github.com/grahamvanpelt/unbusy.day/pubsub"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ringSize bounds the SSE replay buffer (PRD F2). On overflow the client
// refetches; see eventsHandler / pubsub.Broker.
const ringSize = 1024

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

	broker := pubsub.New(ringSize)
	svc := cards.NewService(pool, broker)

	mux := http.NewServeMux()

	// PRD F3 / D6: in-process 200 only. Never query Postgres here — Fly's
	// health check fires every few seconds and a DB ping would defeat
	// Neon's scale-to-zero (~183 CU-h/mo, over the free tier).
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})

	// Authoritative read for initial sync and the F2 overflow contract: when
	// the SSE replay ring can't cover a client's gap (or a reconnect carries
	// no Last-Event-ID), the client refetches the full state here.
	mux.HandleFunc("GET /api/cards", func(w http.ResponseWriter, r *http.Request) {
		cs, err := svc.List(r.Context())
		if err != nil {
			log.Printf("list: %v", err)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"cards": cs})
	})

	// PRD F1: reorder mutation. Returns {cards, txid} with txid as a
	// decimal string. F5: errors return structured JSON with 4xx/5xx so
	// the client (TanStack DB) rolls back the optimistic order.
	mux.HandleFunc("POST /api/cards/reorder", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Order []string `json:"order"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		res, err := svc.Reorder(r.Context(), body.Order)
		switch {
		case errors.Is(err, cards.ErrNotPermutation):
			writeError(w, http.StatusBadRequest, err.Error())
			return
		case err != nil:
			log.Printf("reorder: %v", err)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(res)
	})

	// PRD F2: live read path. SSE off the same pub/sub the reorder handler
	// publishes to; one mutation fans to every subscriber.
	mux.HandleFunc("GET /api/events", eventsHandler(broker))

	// PRD §5 / M2.5b: Adapter B (FE2, Datastar + templ) over the same cards
	// service and pub/sub as Adapter A — one mutation fans to both adapters'
	// subscribers (criterion 9). F12 reorder, F13 patch stream, F14 page.
	mux.Handle("GET /ds/{$}", ds.PageHandler(svc))
	mux.Handle("GET /ds/events", ds.EventsHandler(svc, broker))
	mux.Handle("POST /ds/cards/reorder", ds.ReorderHandler(svc))

	// PRD F16 / M2.5a: Adapter B's pin-and-verify smoke. Kept mounted as a
	// cheap wiring canary for the pinned Datastar SDK + templ versions.
	mux.Handle("GET /ds/_smoke", ds.SmokeHandler())
	mux.Handle("GET /ds/_smoke/events", ds.SmokeEventsHandler())

	// PRD F4: everything not matched above serves the embedded FE1 SPA with an
	// index.html fallback (/assets/* immutable, index.html no-cache). The "/"
	// pattern is lowest precedence in net/http's mux, so /api/*, /ds/*, and
	// /healthz keep their specific handlers. Released builds embed the Vite
	// output (-tags embedassets); dev builds use the spa_stub.go no-op.
	mux.Handle("/", spaHandler())

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// WriteTimeout stays 0 (disabled): SSE streams are long-lived and the
	// handler manages its own liveness via the 25s keepalive (PRD D1/F2).
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

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
