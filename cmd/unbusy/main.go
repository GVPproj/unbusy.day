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
	"github.com/GVPproj/unbusy.day/internal/jot"
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

	// `unbusy migrate` applies migrations and exits.
	if len(os.Args) > 1 && os.Args[1] == "migrate" {
		if err := migrate.Run(ctx, dbURL); err != nil {
			log.Fatalf("migrate: %v", err)
		}
		log.Println("migrations applied")
		return
	}

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
	jotSvc := jot.NewService(db, broker)
	authSvc := auth.NewService(db, newMailer(), authOptions()...)

	guardOpenSignup()

	mux := newRouter(authSvc, blockSvc, jotSvc, broker, routerConfig{
		// Set SECURE_COOKIES=1 wherever the app sits behind HTTPS (ADR 0002).
		secureCookies:    os.Getenv("SECURE_COOKIES") == "1",
		turnstileSiteKey: os.Getenv("TURNSTILE_SITEKEY"),
		turnstileSecret:  os.Getenv("TURNSTILE_SECRET"),
		sesTopicARN:      os.Getenv("SES_SNS_TOPIC_ARN"),
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// HOST narrows the bind (e.g. 127.0.0.1 in dev so tailscale serve can
	// hold the same port on the tailnet IP); empty means all interfaces.
	addr := os.Getenv("HOST") + ":" + port

	// WriteTimeout stays 0: SSE streams are long-lived and keep their own
	// 25s keepalive. ReadHeaderTimeout guards against slow-loris.
	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Printf("unbusy listening on %s", addr)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
