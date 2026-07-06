package main

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/GVPproj/unbusy.day/internal/auth"
	"github.com/GVPproj/unbusy.day/internal/block"
	"github.com/GVPproj/unbusy.day/internal/frontend"
	"github.com/GVPproj/unbusy.day/internal/migrate"
	"github.com/GVPproj/unbusy.day/internal/pubsub"
	_ "modernc.org/sqlite"
)

// newMailer picks SMTP when SMTP_HOST is set, else LogMailer for dev.
func newMailer() auth.Mailer {
	host := os.Getenv("SMTP_HOST")
	if host == "" {
		log.Print("auth: LogMailer (login codes to stdout)")
		return auth.LogMailer{}
	}
	log.Printf("auth: SMTP mailer via %s", host)
	// Missing logo asset falls back to the text wordmark, not a boot failure.
	logo, err := frontend.Asset("static/icon-192.png")
	if err != nil {
		log.Printf("auth: email logo unavailable, using text wordmark: %v", err)
	}
	return auth.NewSMTPMailer(
		host,
		envOr("SMTP_PORT", "587"),
		os.Getenv("SMTP_USERNAME"),
		os.Getenv("SMTP_PASSWORD"),
		envOr("SMTP_FROM", "hi@unbusy.day"),
		logo,
	)
}

// authOptions enables the global send ceiling only when OTP_SEND_CEILING is
// set; OTP_SEND_WINDOW defaults to 1h.
func authOptions() []auth.Option {
	var opts []auth.Option
	if v := os.Getenv("OTP_SEND_CEILING"); v != "" {
		max, err := strconv.Atoi(v)
		if err != nil || max <= 0 {
			log.Fatalf("OTP_SEND_CEILING must be a positive integer, got %q", v)
		}
		window := time.Hour
		if w := os.Getenv("OTP_SEND_WINDOW"); w != "" {
			window, err = time.ParseDuration(w)
			if err != nil {
				log.Fatalf("OTP_SEND_WINDOW must be a Go duration, got %q", w)
			}
		}
		log.Printf("auth: send ceiling %d per %s (circuit breaker)", max, window)
		opts = append(opts, auth.WithSendCeiling(max, window))
	}
	return opts
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// guardOpenSignup fails fast when a live mailer (SMTP_HOST) would serve open
// signup with Turnstile or the send ceiling silently off — that would make
// /login/code an open email relay. LogMailer carries no relay risk.
func guardOpenSignup() {
	if os.Getenv("SMTP_HOST") == "" {
		return
	}
	var missing []string
	if os.Getenv("TURNSTILE_SECRET") == "" {
		missing = append(missing, "TURNSTILE_SECRET (human-presence check)")
	}
	if os.Getenv("OTP_SEND_CEILING") == "" {
		missing = append(missing, "OTP_SEND_CEILING (global send ceiling)")
	}
	if len(missing) == 0 {
		return
	}
	msg := "open signup with a live mailer but these defensive layers are DISABLED: " + strings.Join(missing, ", ")
	if os.Getenv("OPEN_SIGNUP_INSECURE") == "1" {
		log.Printf("WARNING: %s — proceeding because OPEN_SIGNUP_INSECURE=1", msg)
		return
	}
	log.Fatalf("refusing to start: %s. Set them, or OPEN_SIGNUP_INSECURE=1 to override.", msg)
}

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
	authSvc := auth.NewService(db, newMailer(), authOptions()...)

	guardOpenSignup()

	// Set SECURE_COOKIES=1 wherever the app sits behind HTTPS (ADR 0002).
	secureCookies := os.Getenv("SECURE_COOKIES") == "1"

	mux := http.NewServeMux()

	// In-process 200 only — a liveness probe, not a DB readiness check.
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})

	mux.Handle("GET /{$}", frontend.RequireSession(authSvc, frontend.PageHandler(blockSvc)))
	turnstileSiteKey := os.Getenv("TURNSTILE_SITEKEY")
	mux.Handle("GET /login", frontend.LoginPageHandler(turnstileSiteKey))
	// Per-IP + global rate limit on the pre-auth send path. Fly-Client-IP is
	// trusted only behind Fly's proxy.
	loginRL := frontend.NewLoginRateLimiter(secureCookies)
	// Turnstile presence gate; no secret set → dev no-op.
	presence := frontend.NewPresenceVerifier(os.Getenv("TURNSTILE_SECRET"))
	mux.Handle("POST /login/code", loginRL.Limit(frontend.RequestCodeHandler(authSvc, presence)))
	mux.Handle("POST /login/verify", frontend.VerifyCodeHandler(authSvc, blockSvc, secureCookies))
	mux.Handle("POST /logout", frontend.LogoutHandler(authSvc, secureCookies))
	mux.Handle("GET /events", frontend.RequireSession(authSvc, frontend.EventsHandler(blockSvc, broker)))
	mux.Handle("POST /blocks/layout", frontend.RequireSession(authSvc, frontend.LayoutHandler(blockSvc)))
	mux.Handle("POST /blocks/bounds", frontend.RequireSession(authSvc, frontend.BoundsHandler(blockSvc)))
	mux.Handle("POST /blocks", frontend.RequireSession(authSvc, frontend.CreateHandler(blockSvc)))
	mux.Handle("POST /blocks/delete", frontend.RequireSession(authSvc, frontend.DeleteHandler(blockSvc)))
	mux.Handle("POST /blocks/clear", frontend.RequireSession(authSvc, frontend.ClearHandler(blockSvc)))
	mux.Handle("POST /blocks/rename", frontend.RequireSession(authSvc, frontend.RenameHandler(blockSvc)))

	// SES bounce/complaint feedback. Unauthenticated (SNS calls it) but locked
	// to our topic ARN + SNS signature verification.
	if arn := os.Getenv("SES_SNS_TOPIC_ARN"); arn != "" {
		log.Printf("auth: SES feedback webhook mounted for %s", arn)
		mux.Handle("POST /webhooks/ses", frontend.SESWebhookHandler(authSvc, arn))
	}

	mux.Handle("GET /static/", frontend.StaticHandler())
	// Served from root so its control scope is the whole app (iOS PWA); see sw.js.
	mux.Handle("GET /sw.js", frontend.ServiceWorkerHandler())

	// Wiring canary for the pinned Datastar SDK + templ versions.
	mux.Handle("GET /_smoke", frontend.SmokeHandler())
	mux.Handle("GET /_smoke/events", frontend.SmokeEventsHandler())

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// WriteTimeout stays 0: SSE streams are long-lived and keep their own
	// 25s keepalive. ReadHeaderTimeout guards against slow-loris.
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
