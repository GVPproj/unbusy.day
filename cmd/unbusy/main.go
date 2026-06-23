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

// newMailer picks the SMTP provider when SMTP_HOST is set (production),
// else LogMailer so `task dev` needs no email service.
func newMailer() auth.Mailer {
	host := os.Getenv("SMTP_HOST")
	if host == "" {
		log.Print("auth: LogMailer (login codes to stdout)")
		return auth.LogMailer{}
	}
	log.Printf("auth: SMTP mailer via %s", host)
	return auth.NewSMTPMailer(
		host,
		envOr("SMTP_PORT", "587"),
		os.Getenv("SMTP_USERNAME"),
		os.Getenv("SMTP_PASSWORD"),
		envOr("SMTP_FROM", "login@unbusy.day"),
	)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	ctx := context.Background()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL is required")
	}

	// `unbusy migrate` applies migrations and exits
	if len(os.Args) > 1 && os.Args[1] == "migrate" {
		if err := migrate.Run(ctx, dbURL); err != nil {
			log.Fatalf("migrate: %v", err)
		}
		log.Println("migrations applied")
		return
	}

	// Migrate on boot: 	persistent volume holding the SQLite file
	if err := migrate.Run(ctx, dbURL); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	db, err := sql.Open("sqlite", dbURL)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer func() { _ = db.Close() }()
	if err := db.PingContext(ctx); err != nil {
		log.Fatalf("ping db: %v", err)
	}

	broker := pubsub.New()
	blockSvc := block.NewService(db, broker)
	authSvc := auth.NewService(db, newMailer())

	// Secure cookies in production (ADR 0002). Set SECURE_COOKIES=1 wherever the
	// app sits behind HTTPS (Fly, for example, does so via fly.app.toml)
	secureCookies := os.Getenv("SECURE_COOKIES") == "1"

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
	mux.Handle("GET /{$}", frontend.RequireSession(authSvc, frontend.PageHandler(blockSvc)))
	mux.Handle("GET /login", frontend.LoginPageHandler())
	mux.Handle("POST /login/code", frontend.RequestCodeHandler(authSvc))
	mux.Handle("POST /login/verify", frontend.VerifyCodeHandler(authSvc, blockSvc, secureCookies))
	mux.Handle("POST /logout", frontend.LogoutHandler(authSvc, secureCookies))
	mux.Handle("GET /events", frontend.RequireSession(authSvc, frontend.EventsHandler(blockSvc, broker)))
	mux.Handle("POST /blocks/layout", frontend.RequireSession(authSvc, frontend.LayoutHandler(blockSvc)))
	mux.Handle("POST /blocks/bounds", frontend.RequireSession(authSvc, frontend.BoundsHandler(blockSvc)))
	mux.Handle("POST /blocks", frontend.RequireSession(authSvc, frontend.CreateHandler(blockSvc)))
	mux.Handle("POST /blocks/delete", frontend.RequireSession(authSvc, frontend.DeleteHandler(blockSvc)))
	mux.Handle("POST /blocks/clear", frontend.RequireSession(authSvc, frontend.ClearHandler(blockSvc)))
	mux.Handle("POST /blocks/rename", frontend.RequireSession(authSvc, frontend.RenameHandler(blockSvc)))

	mux.Handle("GET /static/", frontend.StaticHandler())

	// Wiring canary for the pinned Datastar SDK + templ versions
	mux.Handle("GET /_smoke", frontend.SmokeHandler())
	mux.Handle("GET /_smoke/events", frontend.SmokeEventsHandler())

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// WriteTimeout stays 0 (disabled): SSE streams are long-lived and the
	// handler manages its own liveness via the 25s keepalive.
	// ReadHeaderTimeout guards the against slow-loris attacks.
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
