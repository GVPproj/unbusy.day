package main

import (
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/GVPproj/unbusy.day/internal/auth"
	"github.com/GVPproj/unbusy.day/internal/frontend"
	_ "modernc.org/sqlite"
)

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func newMailer() auth.Mailer {
	host := os.Getenv("SMTP_HOST")
	if host == "" {
		log.Print("auth: LogMailer (login codes to stdout)")
		return auth.LogMailer{}
	}
	log.Printf("auth: SMTP mailer via %s", host)

	logo, err := frontend.Asset("static/icons/icon-192.png")
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
