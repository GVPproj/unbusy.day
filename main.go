package main

import (
	"log"
	"net/http"
	"os"
)

func main() {
	mux := http.NewServeMux()

	// PRD F3 / D6: in-process 200 only. Never query Postgres here — Fly's
	// health check fires every few seconds and a DB ping would defeat
	// Neon's scale-to-zero (~183 CU-h/mo, over the free tier).
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
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
