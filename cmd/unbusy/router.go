package main

import (
	"log"
	"net/http"

	"github.com/GVPproj/unbusy.day/internal/auth"
	"github.com/GVPproj/unbusy.day/internal/block"
	"github.com/GVPproj/unbusy.day/internal/frontend"
	"github.com/GVPproj/unbusy.day/internal/jot"
	"github.com/GVPproj/unbusy.day/internal/pubsub"
	"github.com/GVPproj/unbusy.day/internal/web"
)

type routerConfig struct {
	secureCookies    bool
	turnstileSiteKey string
	turnstileSecret  string
	sesTopicARN      string
}

// newRouter builds the route table over already-constructed services — the
// env/flag surface stays in main, so tests can exercise the real gating.
func newRouter(authSvc *auth.Service, blockSvc *block.Service, jotSvc *jot.Service, broker *pubsub.Broker, cfg routerConfig) *http.ServeMux {
	mux := http.NewServeMux()

	// In-process 200 only — a liveness probe, not a DB readiness check.
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})

	mux.Handle("GET /{$}", web.RequireSession(authSvc, frontend.PageHandler(blockSvc, jotSvc)))
	mux.Handle("GET /login", frontend.LoginPageHandler(cfg.turnstileSiteKey))
	// Per-IP + global rate limit on the pre-auth send path. Fly-Client-IP is
	// trusted only behind Fly's proxy.
	loginRL := web.NewLoginRateLimiter(cfg.secureCookies)
	// Turnstile presence gate; no secret set → dev no-op.
	presence := auth.NewPresenceVerifier(cfg.turnstileSecret)

	mux.Handle("POST /login/code", loginRL.Limit(frontend.RequestCodeHandler(authSvc, presence)))
	mux.Handle("POST /login/verify", frontend.VerifyCodeHandler(authSvc, cfg.secureCookies))
	mux.Handle("POST /logout", frontend.LogoutHandler(authSvc, cfg.secureCookies))
	mux.Handle("GET /events", web.RequireSession(authSvc, frontend.EventsHandler(blockSvc, jotSvc, broker)))
	mux.Handle("POST /blocks/layout", web.RequireSession(authSvc, frontend.LayoutHandler(blockSvc)))
	mux.Handle("POST /blocks/bounds", web.RequireSession(authSvc, frontend.BoundsHandler(blockSvc)))
	mux.Handle("POST /blocks", web.RequireSession(authSvc, frontend.CreateHandler(blockSvc)))
	mux.Handle("POST /blocks/delete", web.RequireSession(authSvc, frontend.DeleteHandler(blockSvc)))
	mux.Handle("POST /blocks/clear", web.RequireSession(authSvc, frontend.ClearHandler(blockSvc)))
	mux.Handle("POST /blocks/rename", web.RequireSession(authSvc, frontend.RenameHandler(blockSvc)))
	mux.Handle("POST /jot", web.RequireSession(authSvc, frontend.JotHandler(jotSvc)))

	// SES bounce/complaint feedback. Unauthenticated (SNS calls it) but locked
	// to our topic ARN + SNS signature verification.
	if cfg.sesTopicARN != "" {
		log.Printf("auth: SES feedback webhook mounted for %s", cfg.sesTopicARN)
		mux.Handle("POST /webhooks/ses", auth.SESWebhookHandler(authSvc, cfg.sesTopicARN))
	}

	mux.Handle("GET /static/", frontend.StaticHandler())
	// Served from root so its control scope is the whole app (iOS PWA); see sw.js.
	mux.Handle("GET /sw.js", frontend.ServiceWorkerHandler())

	// Wiring canary for the pinned Datastar SDK + templ versions.
	mux.Handle("GET /_smoke", frontend.SmokeHandler())
	mux.Handle("GET /_smoke/events", frontend.SmokeEventsHandler())
	mux.Handle("POST /_smoke/echo", frontend.SmokeEchoHandler())

	return mux
}
