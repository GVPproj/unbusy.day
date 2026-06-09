package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"

	"github.com/grahamvanpelt/unbusy.day/cards"
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

	svc := cards.NewService(pool)

	mux := http.NewServeMux()

	// PRD F3 / D6: in-process 200 only. Never query Postgres here — Fly's
	// health check fires every few seconds and a DB ping would defeat
	// Neon's scale-to-zero (~183 CU-h/mo, over the free tier).
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
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

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("hello-cards listening on :%s", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatal(err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
