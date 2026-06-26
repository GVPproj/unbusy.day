# Passthrough service worker for iOS PWA storage durability

## Context

The app installs to the iOS home screen as a PWA (`manifest.webmanifest`,
`display: standalone`, apple-touch-icon). In practice an installed instance
prompted for a fresh email-OTP login roughly **every day**, despite:

- a 30-day **absolute** server session (ADR 0002), still valid in the DB, and
- a correctly-formed **persistent** auth cookie — explicit 30-day `Expires`,
  `HttpOnly`, `Secure` (prod), `SameSite=Lax`. Nothing server-side expires daily.

So the session row outlives the cookie: WebKit was evicting the cookie
client-side. A home-screen app **without a service worker** is treated by iOS
closer to an ephemeral *web clip* than an installed PWA — its storage container
is reclaimed aggressively (ITP idle caps, memory pressure when the backgrounded
app is killed). The tell: another PWA on the same device — one that registers a
service worker — stays logged in for far longer, reliably. Redeploys were ruled
out (cookies are domain-scoped not build-scoped; sessions are DB rows on the Fly
volume that survive deploys).

## Decision

Ship a **minimal passthrough service worker** (`internal/frontend/static/sw.js`)
whose only job is to exist, so iOS classifies the app as an installed PWA and
gives it the durable storage tier that keeps the auth cookie alive.

- **Caches nothing, intercepts nothing.** It registers a `fetch` listener that
  never calls `respondWith()`, so every request goes to the network normally.
  No precache, no offline mode, no stale-while-revalidate.
- **Served from the site root** (`GET /sw.js` via `ServiceWorkerHandler`), not
  `/static/`, because a service worker's control scope is capped to its own URL
  path — root is what lets it claim scope `/` and cover the whole app. `no-cache`
  so a new `sw.js` lands on the next visit.
- **Registered** from the layout shell with a guarded
  `navigator.serviceWorker.register("/sw.js")` on `load`.

## Why this doesn't violate "no SPA / no client-side business logic"

The architecture bans client-side *business logic* and SPA caching, not all JS
(`drag.js` already exists). This service worker holds **zero** logic and
deliberately **no offline cache** — a cache would fight the "server render is the
source of truth" invariant by serving stale state. It is purely an installability
signal. Keeping it a strict passthrough is the load-bearing constraint: the moment
it caches responses, it starts lying about the column, so it must not.

## Consequences

- Plausibly fixes the daily iOS logout by moving the app into the durable-storage
  tier. **Not proven** — iOS storage eviction is undocumented and a service worker
  is not an absolute guarantee against ITP; it is the strongest lead given the
  "other PWA is fine" data point. Validate by living with an installed instance.
- One more piece of client JS to keep honest. If anyone is ever tempted to add
  offline caching here, that's a new decision (and a new ADR), not an extension
  of this one.
- Passkeys/WebAuthn (which survive storage eviction and make re-auth a Face ID
  tap) remain the real remedy if eviction persists — backlogged separately.
