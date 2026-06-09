
# PRD: hello-cards (v3 — Go-native sync + edge)

**Status:** Draft · **Owner:** you · **Last updated:** 2026-06-09 · **Supersedes:** v2 (Electric sync-engine spike)

> **v3 domain-object note:** the spike's placeholder domain object is a **single sortable column of 3 cards** (reorder-only — no card create/delete/rename), chosen so the demo mirrors the eventual Trello target more closely than a bare counter did. The reorder mutation replaces the increment everywhere below; all architecture (Go-as-logic, txid handshake, SSE fan-out, two-FE proof, edge delivery) is unchanged.

## 1. Summary

A minimal full-stack reference app that validates the production architecture for a Trello-like, multi-tenant product with optimistic/instant UI over flaky networks, served to a globally distributed user base:

- **Postgres** (Neon, region `us-west-2` to pair with Fly `sea`) as the single source of truth.
- **Go API** as the *only* home of business logic: plain HTTP mutation endpoints that write to Postgres in a transaction and return the Postgres `txid` for client reconciliation. Live read-path delivered as **Server-Sent Events** off the same Go server, with in-process pub/sub fanning mutation events to subscribers.
- **TanStack DB** on the client for live queries and optimistic writes with automatic rollback, backed by a custom SSE-driven collection.
- **Two frontends over one Go core**, to validate the "logic exactly once" thesis against opposite ends of the client-complexity spectrum:
  - **FE1 — Vite + React + TanStack DB**: rich-client, automatic optimistic apply + rollback, txid-handshake reconciliation. Embedded into the Go binary via `go:embed`.
  - **FE2 — Datastar + templ**: server-driven hypermedia; the Go server renders `templ` fragments and pushes them as SSE patches. Minimal client JS, no client-side mutator/state machine.
  Both are exercised against the same business logic and the same in-process pub/sub; only a thin presentation adapter differs. FE2 is a **comparison spike** (measure the trades, then pick one), not a committed dual-stack.
- Everything deployed to Fly.io with `flyctl`.
- **Cloudflare** proxied DNS (orange-cloud) in front of the Fly app: edge-cached static assets for a global user base; `/api/*` and the SSE event stream pass through to origin.

Purpose: validate the spine of the production architecture — Go-as-business-logic, txid-handshake reconciliation, SSE fan-out, and edge-cached delivery — before building the full board/list/card feature set. The domain object is a **single column of 3 cards that the user can drag to reorder** (no create/delete/rename) — the smallest domain that still exercises ordering, the mutation shape the real product is built around.

## 2. Goals

- Business logic exists exactly once, in Go. No client-side mutator duplication. **Proof:** two structurally different frontends (FE1 rich-client, FE2 server-driven) drive the same Go core through thin presentation adapters; if logic can't be cleanly split from serialization, that itself is the finding.
- Instant optimistic UI: a drag-reorder renders in <1 frame; survives flaky wifi; reconciles cleanly when the server response lands (the dragged card stays put — no snap-back).
- Live multi-client sync: two browsers see each other's reorders within ~1s without refresh, via SSE.
- Globally distributed static delivery: first paint cost from a distant region is bounded by edge latency, not trans-Pacific RTT to `sea`.
- One deployable app artifact (Go binary with embedded SPA) plus one infrastructure piece (Postgres) and one edge config (Cloudflare) — all provisioned and deployed via CLI.
- Establish the auth seam: the SSE handler is the gatekeeper for live reads, ready for per-tenant filtering later.

## 3. Non-Goals

- True offline-first (long disconnected sessions, queued offline writes). Target is optimistic UI over flaky connections; offline durability is a later, separate decision (TanStack `offline-transactions` + persistence is the upgrade path).
- Cross-instance fan-out for multi-machine Go: in-process pub/sub is sufficient for the spike (one machine). The scale-out plan (LISTEN/NOTIFY on a session-mode connection, or Redis pub/sub) is documented but not built (§9).
- Auth/users (the SSE handler is the placeholder for it), multi-region origin, horizontal scaling of the Go app, migrations tooling beyond `psql`, observability beyond Fly logs.
- **Full card CRUD and the broader Trello domain** (creating/deleting/renaming cards, multiple lists, multiple boards, cross-list moves). v1 has exactly one list of 3 fixed cards and one mutation: reorder. **Fractional indexing** for ordering is also out of scope — v1 uses server-rewritten integer positions (F1); fractional indexing is the documented upgrade path (§9).
- CDN-fronted shape-based sync (Electric or equivalent). Out of scope; re-evaluate when board fan-out demands it.

## 4. Functional Requirements

### API (Go, stdlib `net/http`)

|ID |Requirement|
|---|-----------|
|F1 |`POST /api/cards/reorder` → body `{"order": ["<id>", "<id>", "<id>"]}`, the new top-to-bottom card order. Inside a transaction: validates `order` is a permutation of the current card ids (reject otherwise per F5), then rewrites every card's `position` to its index in `order` and `RETURNING` the new ordered rows. Do the rewrite as a **single bulk `UPDATE … FROM (VALUES …)` statement** against a **`DEFERRABLE` unique constraint on `position`** so intermediate states don't trip the constraint mid-statement (see §11). Returns `{"cards": [{id,label,position}…], "txid": "<pg txid>"}`. The `txid` is captured in the same transaction with `SELECT pg_current_xact_id()::text`. **Must not cast to `xid` first** — `pg_current_xact_id()` returns 64-bit `xid8`; `::xid` truncates to 32 bits and breaks txid matching against the SSE stream. The txid is a **string** end-to-end (Go, JSON, JS) because JS `Number` loses precision above 2^53. After commit, the handler publishes `{txid, cards}` to an in-process pub/sub for SSE fan-out (F2).|
|F2 |`GET /api/events` → SSE endpoint (`Content-Type: text/event-stream`). Subscribes to the in-process pub/sub for the requested topic(s); v1 hardcodes the `cards` topic. Each mutation event is emitted as one SSE message with `id: <txid>` and `data: {...json...}` (the full ordered card list). Implementation constraints: call `http.Flusher.Flush` after each event; per-route server read/write timeouts disabled or set very high (SSE is long-lived); explicit `Cache-Control: no-cache` and `X-Accel-Buffering: no` response headers. **Server emits a `:keepalive\n\n` comment every 25s** to keep intermediaries (Cloudflare proxy, mobile NATs, browser idle detectors) from closing the connection. Supports the `Last-Event-ID` request header on reconnect: replays any events with txid > Last-Event-ID from a small in-memory ring buffer (v1: 1024 events). On ring overflow, client falls back to refetching current state from a regular read endpoint.|
|F3 |`GET /healthz` → 200, used by Fly health checks.|
|F4 |All non-`/api` paths serve embedded SPA with `index.html` fallback. Hashed assets (`/assets/*`) carry `Cache-Control: public, max-age=31536000, immutable` (Vite emits content-hashed filenames). `index.html` carries `Cache-Control: no-cache` so Cloudflare always revalidates the entry point against origin — keeps deploys atomic at the client level.|
|F5 |Errors from mutation endpoints (including a malformed/stale `order` that is not a permutation of current ids) return structured JSON with a 4xx/5xx status so TanStack DB rolls back the optimistic state (the dragged card snaps back to its prior position).|

### Frontend (Vite + React + TanStack DB)

|ID |Requirement|
|---|-----------|
|F6 |`cards` is a TanStack DB collection backed by a **custom SSE adapter**: opens an `EventSource` against `/api/events`, dispatches incoming events into the collection's change stream, and supplies the `onUpdate` mutation handler. Native EventSource reconnect carries `Last-Event-ID` automatically; the adapter also implements an exponential-backoff retry over hard close.|
|F7 |The column is rendered via a live query against the collection ordered by `position`; reorders from *other* clients re-sort the stack without user action (SSE event arrival).|
|F8 |Drag-to-reorder uses a client DnD library (**dnd-kit** sortable) and performs an optimistic update through the collection; the `onUpdate` handler POSTs the new `order` to F1 and returns `{ txid }`, so TanStack DB holds the optimistic order until that txid appears on the SSE stream (no snap-back flicker), and rolls back automatically on error (F5). The dragged card lands instantly under the cursor.|
|F9 |Airplane-mode test: with network disabled, dragging a card shows the optimistic new order; on reconnect within the session it either commits or visibly snaps back. SSE reconnect restores the live stream; missed events during disconnect are replayed via `Last-Event-ID` if within the ring (F2), otherwise the adapter triggers a full state refetch. (No cross-restart offline persistence in v1.)|

### Frontend B (Datastar + templ) — comparison spike

Mounted under its own route prefix (`/ds/…`) in the same binary, over the same `cards` service and the same in-process pub/sub. No client build step beyond `templ generate`; the Datastar runtime is a single small JS file served as a cacheable static asset.

|ID |Requirement|
|---|-----------|
|F12|`POST /ds/cards/reorder` → calls the **same** core mutation as F1 (shared `cards` service: validate permutation, bulk position rewrite, `pg_current_xact_id()::text`, `publish(txid, cards)`). Responds with an SSE-formatted patch (rendered `templ` fragment for the reordered column) so the dragging client updates immediately on response. The shared core means F1 and F12 differ only in serialization, never in logic.|
|F13|`GET /ds/events` → SSE endpoint subscribing to the same `cards` topic, but each event renders the current column as a **`templ` fragment patch** (element patch) rather than JSON. Reuses all F2 connection hardening verbatim: 25s `:keepalive`, `Cache-Control: no-cache`, `X-Accel-Buffering: no`, `http.Flusher.Flush` per event. **Reconnect contract differs from F1:** on (re)connect the server renders the current authoritative fragment rather than replaying deltas — the F2 ring buffer / `Last-Event-ID` delta-replay is an FE1-shaped feature and is *not* required here.|
|F14|Column rendered server-side via `templ`; reorders from other clients arrive as element patches over the F13 stream and apply with no client-side reconciliation (server is authoritative).|
|F15|**Optimistic UX is opt-in and manual.** Datastar has no native drag-and-drop, so FE2 still needs a client DnD library (**SortableJS**) whose `onEnd` posts the new `order` to F12 — this is a key comparison finding (DnD is a client concern in *both* stacks; the stacks differ only in what happens *after* the drop). Default behavior is then a server round-trip (latency = RTT to origin; the card may briefly jump back to its drop-origin until the server patch lands). Document whether the spike implements an optimistic Datastar signal (hold the dropped order on click, corrected by the next server patch) or stays purely server-driven — this is the primary UX dimension being compared against F8. txid is *not* load-bearing here: keep it only if optimistic signals need flicker-suppression dedup.|
|F16|Datastar's element-patch event names and Go SDK surface **must be verified against current (1.0+) Datastar docs** before implementing — event naming changed during the 1.0 RC era. Do not bake names in from memory. **Also verify the SortableJS ↔ Datastar wiring** (reading the post-drop order out of the DOM and posting it without the two libraries fighting over DOM ownership — Datastar patches the same elements SortableJS reorders).|

### Data (Postgres)

|ID |Requirement|
|---|-----------|
|F10|Single table: `card(id TEXT PRIMARY KEY, label TEXT NOT NULL, position INTEGER NOT NULL, CONSTRAINT card_position_unique UNIQUE (position) DEFERRABLE INITIALLY DEFERRED)`; seeded by an idempotent migration script with exactly 3 cards at positions 0,1,2, run via `task migrate` (psql).|
|F11|Go API uses the pooled Neon connection string with `sslmode=require`. No `wal_level=logical` requirement and no direct/session-mode endpoint needed in v1 — nothing tails WAL and nothing uses `LISTEN/NOTIFY` yet. Transaction-mode pooling is fine for the entire mutation path.|

## 5. Architecture

```
browser
  │
  ├── static assets ──> Cloudflare edge (cached on hashed filenames) ─────────┐
  │                                                                            │
  ├── reads:  TanStack DB SSE collection                                       │
  │             └── EventSource ──> Cloudflare (pass-through) ──> Go /api/events
  │                                                                  ▲
  │                                                                  │  in-process pub/sub
  └── writes: optimistic reorder ──> Cloudflare (pass-through) ──> Go /api/cards/reorder
                                                                       └── Postgres txn (Neon, us-west-2)
                                                                            returns txid
                                                                            publish(txid, cards) ─┘

(client holds optimistic card order until matching txid arrives via SSE)
```

- **Core / adapter split (keystone for the two-FE proof):** a transport-agnostic `cards` service package owns the business logic — permutation validation, the bulk position-rewrite transaction, `pg_current_xact_id()::text` capture, and `publish(txid, cards)` to the in-process pub/sub. Two thin presentation adapters sit over it and over the same pub/sub: **Adapter A** = JSON mutation handlers + SSE-emitting-JSON (FE1/TanStack); **Adapter B** = Datastar handlers + SSE-emitting-`templ`-fragments (FE2). The pub/sub fans one mutation event to both adapters' subscribers simultaneously.
- **Mounting:** one binary, two route trees — `/api/*` + embedded SPA for FE1, `/ds/*` + server-rendered templ for FE2. Chosen over content-negotiation on a single `/api/events` to keep each FE's contract legible and avoid `Accept`-header subtlety through Cloudflare. One Neon, one pub/sub, one deploy.
- Repo layout: `main.go`, `cards/` (shared service), `frontend/` (Vite app, FE1), `ds/` (templ components + Datastar handlers, FE2), `migrations/`, `Dockerfile`, `fly.app.toml`, `Taskfile.yml`, `compose.yml` (dev Postgres only).
- Build: multi-stage Dockerfile (node → go → scratch) for the app; the Go stage runs `templ generate` (FE2 components) before `go build`; `fly deploy --remote-only`.
- **Edge / global-latency trade (key comparison finding):** FE1's content-hashed assets edge-cache (criterion 6) so first paint is bounded by edge latency. FE2's HTML shell is origin-rendered from `sea` — only the Datastar runtime JS is edge-cacheable, so initial paint costs a round-trip to origin. For the no-auth card column the shell *could* be cached, but for the real per-tenant product server-rendered HTML at the edge is hard, and multi-region Go is a §3 non-goal. Net: the §2 global-latency goal is an FE1 advantage; record FE2's first-paint-from-a-distant-colo number as part of the comparison.
- Dev loop (`task dev`): `docker compose up` (Postgres), `air` for Go hot reload, `pnpm dev` with Vite proxy `/api -> :8080` (Vite's dev proxy streams SSE without buffering — no special config needed). Embed only at release build. Cloudflare is not in the dev path; criterion 6 covers verifying it in production.

## 6. Deployment Requirements

|ID |Requirement|
|---|-----------|
|D1 |**App (Fly, region `sea`)**: `fly launch --no-deploy`; shared-cpu-1x; `min_machines_running = 1`. No volume needed (state lives in Postgres). Server read/write timeouts on the `/api/events` route disabled or set to ≥ 1h; other routes use normal idle timeouts.|
|D2 |**Postgres (Neon, `us-west-2`)**: closest region to Fly `sea` (~10–20ms). Pooled connection string only. `sslmode=require`. Auto-suspend can be on or off — no persistent replication consumer to keep warm. Free-tier branch is sufficient for the spike; production plan justified later by PITR / branching / SLA, not by this architecture.|
|D3 |**Cloudflare proxied DNS** in front of the Fly app: add the zone to Cloudflare, update registrar nameservers, create an `A`/`AAAA` (or `CNAME`) record pointing at Fly's anycast IPs with the orange-cloud proxy *enabled*. Cache behavior governed by Cache Rules: (a) **SSE endpoints (`/api/events` *and* `/ds/events`)** — bypass cache *and* disable response buffering (the SSE pass-through rule; widen the matcher to cover both, e.g. a path suffix of `/events`); (b) `/api/*` — bypass cache (defense in depth on top of `no-cache` headers); (c) default — respect origin `Cache-Control` (hashed `/assets/*` get the year-long edge cache; `index.html` revalidates per F4). SSL mode: **Full (strict)**, leveraging Fly's TLS termination.|
|D4 |Health checks: Fly app on `/healthz`. SSE liveness is monitored at the application level via heartbeats (F2), not Fly health checks.|
|D5 |Deploy strategy `rolling` for the app once SSE is verified end-to-end. On drain, in-flight SSE connections close cleanly; clients auto-reconnect via EventSource and resume with `Last-Event-ID`. During M1–M2, `immediate` is fine.|
|D6 |Cost note: app machine (~$3) + Cloudflare free tier + Postgres. **Neon free tier (100 CU-hours/mo, 0.5 GB) covers the spike *and* solo/low-traffic production**, not just the spike: the DB scales to zero after 5 min idle, so a single developer's daily use burns only ~15–30 CU-hours/mo (well under 100). Caveat: anything that pings Postgres on a timer defeats scale-to-zero — keep `/healthz` (F3) a pure in-process 200 with no DB query, or Fly's health checks keep the DB permanently warm (~183 CU-hours/mo → over budget). The smallest Neon **paid** plan (~$19/mo) is needed only when *steady multi-user traffic* keeps the DB awake past the compute budget, or for PITR/branching/SLA — i.e. justified by ops features, not by this architecture. Cheaper alternative once paid Postgres is warranted: a Postgres machine co-located on Fly in `sea` (~$3–7/mo, lower latency, no cold start, but you manage backups). Net floor for solo dev/production: app (~$3) + Neon free ($0) + domain (~$10/yr) ≈ **~$3/mo**.|
|D7 |Egress / TLS: app ↔ Neon traverses public internet over TLS (`sslmode=require`). Client ↔ Cloudflare is HTTPS (Cloudflare-issued cert). Cloudflare ↔ Fly is HTTPS with Full (strict).|
|D8 |Cloudflare proxy connection timeouts (100s on Free/Pro/Business) do not strictly cap SSE under active traffic, but the SSE contract tolerates periodic reconnects regardless via `Last-Event-ID` (F2). The 25s keepalive in F2 keeps idle connections warm within the window. Confirm empirically in M3.|

## 7. Acceptance Criteria

1. `task dev` gives a working hot-reload loop; drag-reorder is optimistic and persists in dev Postgres (refresh shows the new order).
2. Two browser windows: reordering the column in one updates the order in the other within ~1s via SSE, no refresh.
3. Flaky-network test (devtools throttling/offline toggle): dragged card lands instantly; no snap-back flicker on commit (txid matching works); a rejected reorder (F5) visibly snaps back; SSE reconnect catches up missed reorders via `Last-Event-ID` or full refetch.
4. `fly deploy` of the app succeeds; the Cloudflare-fronted public URL serves the React page; drag-reorder works end-to-end.
5. `fly machine restart` on the app → card order unchanged; SSE clients reconnect cleanly within a few seconds and resume from their last `Last-Event-ID`.
6. **Edge caching verified globally**: `curl -I https://<domain>/assets/<hashed>.js` returns `cf-cache-status: HIT` after one warm-up; `curl -I` on `/api/*` returns `BYPASS` or `DYNAMIC`. Spot-check from at least one distant Cloudflare colo (via `cf-ray` header / `curl --resolve` or a third-party probe).
7. SSE survives overnight idle: leave a browser open overnight; on next reorder, all open clients receive the event without manual refresh — confirms keepalive + EventSource reconnect both working.
8. Schema change drill: add a column, run migration, document client behavior — particularly whether a reconnect/refetch is needed for clients to observe the new shape.
9. **Two-FE parity (the comparison payoff):** with FE1 and FE2 open side by side, reordering in *either* updates *both* within ~1s — proving both adapters fan out from the same pub/sub off one mutation. Record the per-FE comparison: lines of client code, **DnD-library integration cost (dnd-kit + optimistic collection vs SortableJS + server patch, including the DOM-ownership wiring of F16)**, drop-to-settled latency (FE1 instant vs FE2 round-trip), first-paint-from-distant-colo, and reconnect behavior. This table is the deliverable that decides which stack the Trello PRD adopts.

## 8. Milestones

Each phase is independently testable; tick `Status` here as you ship so the PRD doubles as the build log.

- **M0 — repo bootstrap** · Status: ☐
  `git init` with `LICENSE.md` (FSL-1.1-Apache-2.0, L1), `.gitignore` + `.env.example` (S1), `README.md` stating the source-available model (L2), `CONTRIBUTING.md` with the DCO text and sign-off instruction (C1). Enable GitHub secret scanning + push protection and add a `gitleaks` pre-commit hook (S3); add the DCO check action. Scaffold `Taskfile.yml`, `compose.yml` (dev Postgres only), a multi-stage `Dockerfile` skeleton, empty `cards/` `migrations/` `frontend/` `ds/` directories, and a minimal `main.go` serving `/healthz` only (F3). **Done when:** `task dev` brings up Postgres and `curl localhost:8080/healthz` returns 200; the DCO check runs green on a throwaway PR; `gitleaks` blocks a deliberate fake-secret commit.

- **M1a — write path + txid** · Status: ☐
  `card` table migration (F10) with `DEFERRABLE INITIALLY DEFERRED` unique on `position`, seeded with 3 fixed cards via `task migrate`. `cards` service package owns permutation validation, the bulk `UPDATE … FROM (VALUES …)` rewrite, and `pg_current_xact_id()::text` capture as a **string** (never cast to `xid`, never marshalled as a JS number). `POST /api/cards/reorder` (F1) wired to the service. **Done when:** `curl -X POST … '{"order":["c","a","b"]}'` returns `{cards, txid}` with txid as a decimal string; a malformed/non-permutation `order` returns 4xx (F5); a fuzz of 100 random reorders never trips the unique constraint (validates the `DEFERRABLE` + bulk-UPDATE mitigation in §11).

- **M1b — SSE fan-out** · Status: ☐
  In-process pub/sub; the reorder handler publishes `{txid, cards}` after commit. `GET /api/events` (F2) with full connection hardening: `Content-Type: text/event-stream`, `Cache-Control: no-cache`, `X-Accel-Buffering: no`, `http.Flusher.Flush` per event, 25s `:keepalive` comments, server read/write timeouts disabled on this route. In-memory ring buffer (1024) with `Last-Event-ID` replay; ring-overflow contract (client falls back to a full state refetch) documented in the handler. **Done when:** `curl --no-buffer -N /api/events` in one terminal receives an event with `id: <txid>` immediately after `curl POST /api/cards/reorder` in another; reconnecting with `Last-Event-ID: <old txid>` replays the events between; an overflow-forcing replay returns the documented sentinel.

- **M2a — FE1 read path** · Status: ☐
  Vite + React scaffold under `frontend/` with dev proxy `/api → :8080`. Custom SSE-backed TanStack DB collection (F6); column rendered via a live query ordered by `position` (F7); native EventSource reconnect carries `Last-Event-ID`; the adapter adds exponential-backoff retry over hard close. **Done when:** two browser tabs open; a `curl POST /api/cards/reorder` updates both tabs within ~1s with no refresh (the read half of criterion 2); killing and restarting the Go server triggers a clean reconnect with no missed events.

- **M2b — FE1 optimistic write** · Status: ☐
  dnd-kit sortable column; `onUpdate` posts the new `order` and returns `{txid}` so TanStack DB holds the optimistic order until the matching txid arrives via SSE (no snap-back flicker on commit), and rolls back automatically on F5 errors (the dragged card snaps back to its prior position). This is also where the core/adapter split (`cards` service + Adapter A) crystallizes — the artifact that lets M2.5 be purely additive. **Done when:** acceptance criteria 1–3 all pass locally — drag-reorder is instant and persists across refresh; two windows sync within ~1s; a devtools-offline drag either commits or visibly snaps back on reconnect; the txid handshake suppresses the snap-back flicker on the commit path.

- **M2.5a — Datastar/SortableJS pin & verify** · Status: ☐
  Pin a specific Datastar 1.0+ release and a SortableJS version; verify the SSE element/signal-patch event names, the Go SDK helpers, and the SortableJS↔Datastar DOM-ownership wiring against current docs (F16). Output: a short note checked into the repo (`ds/NOTES.md`) listing the verified API surface and any naming changes from the 1.0-RC era. **Done when:** a smoke `templ` page connected to a `/ds/_smoke` endpoint receives a hand-crafted element patch and updates correctly; the pinned versions are recorded in `go.mod` / `package.json` / the notes file.

- **M2.5b — FE2 over the shared core** · Status: ☐
  Adapter B mounted at `/ds/*` over the existing `cards` service and the same pub/sub: `POST /ds/cards/reorder` (F12), `GET /ds/events` rendering templ-fragment patches (F13) reusing F2 connection hardening verbatim, server-rendered column updated via element patches (F14), SortableJS-driven post-drop (F15) with the optimistic-or-pure-server-driven choice documented. `templ generate` runs in dev (`task dev --watch`) and as a step in the multi-stage build. **Done when:** acceptance criterion 9 passes — FE1 and FE2 open side by side, a reorder in either updates both within ~1s, proving one mutation fans out through both adapters; the comparison table (lines of client code, DnD-library integration cost incl. the F16 DOM-ownership wiring, drop-to-settled latency, first-paint-from-distant-colo, reconnect behavior) is filled in.

- **M3a — Neon + Fly origin** · Status: ☐
  Neon project in `aws-us-west-2` (D2), pooled connection string pushed to `fly secrets`. `fly launch --no-deploy --copy-config --name hello-cards --region sea`, then `fly deploy --remote-only --strategy immediate`. `min_machines_running = 1`; `/api/events` route timeouts disabled or ≥1h (D1). **Done when:** acceptance criterion 4 passes against the raw `*.fly.dev` URL (Cloudflare not yet in front); `/healthz` returns 200 from the public origin URL with no DB query in the handler (D6 — keeps Neon scale-to-zero intact); drag-reorder works end-to-end on the FE1 path.

- **M3b — Cloudflare proxy + Cache Rules** · Status: ☐
  Proxied A/AAAA record at the apex/subdomain pointed at the Fly anycast IP (D3); SSL mode Full (strict). `ops/cloudflare/cache-rules.json` committed to the repo and PUT against the zone's `http_request_cache_settings` ruleset entrypoint with three rules: (a) `/events` matcher (covers `/api/events` **and** `/ds/events`) bypasses cache and disables response buffering; (b) `/api/*` bypasses cache (defense in depth on top of F4 headers); (c) default respects origin `Cache-Control`. **Done when:** acceptance criterion 6 passes — hashed `/assets/*` returns `cf-cache-status: HIT` after warm-up, `/api/*` returns BYPASS or DYNAMIC, `curl --no-buffer -N` against `/api/events` and `/ds/events` delivers events without proxy buffering, and a spot-check from a distant Cloudflare colo confirms edge proximity via `cf-ray`.

- **M3c — production hardening drills** · Status: ☐
  `fly machine restart` drill (criterion 5: card order persists; SSE clients reconnect and resume from `Last-Event-ID`). Overnight-idle SSE drill (criterion 7: keepalive + EventSource reconnect both prove out over an unattended window). Schema-change drill (criterion 8: add a column, run the migration, document whether a reconnect/refetch is needed for clients to observe the new shape). Switch deploy strategy from `immediate` to `rolling` (D5). **Done when:** criteria 5, 7, 8 all pass and `fly.app.toml` carries `strategy = "rolling"`; observed concurrent-connection ceiling on `shared-cpu-1x` is recorded in the repo so the §9 scale-out trigger is data-driven (§11 last bullet).

- **Fast-follow** · Status: ☐
  GitHub Actions `fly deploy` on push to main (uses the Fly deploy token from `gh` repo secrets). Spike TanStack DB persistence + `offline-transactions` to scope the true-offline upgrade (§3). Spike cross-instance fan-out (LISTEN/NOTIFY on a session-mode Neon endpoint, or a small Redis) to unblock multi-machine Go (§9). None of these gate the v1 spike sign-off.

## 9. Forward-looking notes (Trello PRD inputs)

- **Multitenancy** rides on the SSE handler: per authenticated user/org, the handler subscribes only to topics that user is permitted to see. Per-board vs per-org topic granularity drives both fan-out cost (one event per topic per subscriber) and authorization complexity — decide early.
- **Card ordering**: v1 deliberately uses **server-rewritten integer positions** (F1) over a single fixed list — enough to exercise the reorder mutation and txid handshake without the ordering scheme itself becoming the spike. The production upgrade is **fractional indexing** (e.g. lexicographic keys) with server-side rebalance, enforced only in Go mutation endpoints: it avoids rewriting every row on each move and supports inserts/cross-list moves, which the integer scheme does not. Concurrent same-position writes resolve server-side; clients reconcile via txid. Swapping integer positions for fractional keys touches only the `cards` service's rewrite step, not the adapters or FE contracts.
- **Mutation surface** stays small HTTP handlers: createBoard/List/Card, move, rename, archive — each a transaction returning txid, each publishing affected entities to relevant topics. Single-machine case needs no new infrastructure beyond v1. (v1 ships only `reorder`; the others are this list's natural extension.)
- **Scale-out** (multiple Go machines): in-process pub/sub no longer fans out across instances. Two paths:
  - Postgres `LISTEN/NOTIFY` on a session-mode or direct endpoint (transaction-mode pooling does not support `LISTEN`). Cheapest, NOTIFY payload capped at 8KB.
  - Small Redis (Upstash, Fly Redis) for cross-instance pub/sub. More flexible, slight cost bump.
  - SSE handler shape is unchanged either way.
- **CDN-fronted shape sync** (Electric or equivalent): worth re-evaluating only when board read fan-out justifies trading the gatekeeper auth seam for edge-cached partial-replication shapes. Out of scope here; would be its own spike.

- **Edge topology & the "client-as-edge" trade.** FE2's first-paint tax (origin-rendered HTML from `sea`; see §5) raises the question "move Go to the edge?" — which is not a deploy-config change but a category jump, and which reframes the whole FE1-vs-FE2 decision. Capturing it so the Trello PRD inherits the reasoning, not just the one-line caveat:
  - **"Edge" compresses three layers, only one of them cheap.** (1) *Compute* — running the Go binary in N regions is trivial on Fly (`fly regions add`, anycast routes to nearest). (2) *Cross-region fan-out* — in-process pub/sub can no longer reach SSE subscribers in other regions, so the §9 scale-out bus becomes mandatory *and* cross-region; its latency floor is the propagation time from the commit region to the subscriber region (edge compute does not beat physics on the live-read path). (3) *Data* — the genuinely hard layer.
  - **Writes stay anchored to one primary.** There is still exactly one writable Postgres. Edge nodes forward mutations to it (Fly `fly-replay: region=<primary>` is the canonical pattern: one cross-region hop, writes only). You cannot edge away write RTT without multi-primary (CRDT/multi-master), which is the wrong model for a transactional app with server-side ordering and txid authority. So edge buys fast *renders/reads*, never fast *writes*.
  - **Latency that actually flips:** only FE2's initial render (and read-only paths), and only if a read replica is co-located per region. Static assets (FE1) already edge-cache for free; writes don't improve; live fan-out improves only partially.
  - **Neon wrinkle.** D2 deliberately pins Neon `us-west-2` ~10–20ms from Fly `sea`. Multi-region Go puts edge nodes far from that single Neon, forcing either slow cross-region reads or per-region read replicas — and Neon's cross-region replica story is thinner than Fly Postgres's built-in replica+replay. So "Go at the edge" silently re-opens the DB topology decision and may push off Neon toward Fly Postgres for replica ergonomics.
  - **Consistency hazard, worst for FE2.** Edge read replicas + Datastar's server-authoritative render interact badly: client reorders (committed at txid N via the primary) but the local replica is still at N−1 → server renders the stale order → SSE patch corrects → *card snaps* — the exact failure F8's txid handshake prevents. Making edge safe for FE2 means rebuilding FE1's reconciliation inside the server-driven model (read-from-primary-until-replica≥txid, or txid-keyed optimistic signals) — re-importing the client complexity Datastar was chosen to avoid.
  - **What production server-driven apps actually do at scale.** Three patterns dominate, and naming them is what makes the fork below honest: (1) **tenant-pinned regions** — each workspace/org gets a home region at signup; writes are local there; users elsewhere accept the RTT but it's their org's choice. Fly's `fly-replay: region=<home>` header is the canonical mechanism (anycast lands you anywhere, app replays to home); Linear/Notion/Turso shape this way. Cost: cross-tenant features (global search, marketplaces) get structurally harder, and region migrations are real surgery. (2) **Per-region read replicas + write-to-primary** — reads stay local, writes forward to the single primary; to dodge the replica-lag snap (see *Consistency hazard* above) you must re-import the txid handshake server-side ("read from primary until replica ≥ committed txid"), which rebuilds FE1's reconciliation inside FE2's render path. (3) **Optimistic islands inside the server-driven shell** — Stimulus/Alpine on Hotwire, signals on Datastar — hold the hot interactions (drag, type-ahead, hover) locally so RTT only shows up on commit. This is FE1's reconciliation in miniature; the honest finding is that at scale FE2 converges toward FE1 on the hot paths. Trello-relevant takeaway: server-driven globally is fine if you pick (1), but a globally-distributed multi-tenant product with cross-tenant surfaces collides with the structural cost of (1) and with the FE1-shaped complexity (2) and (3) re-introduce.
  - **The fork (most important finding the spike will surface).** Server-driven (FE2)'s only lever for global latency is *physically relocating the server* — edge compute + edge data + cross-region fan-out, expensive and consistency-fraught, dragging DB topology along. Local-first (FE1)'s answer is "**the client *is* the edge node**": it holds state, applies optimistically, reconciles via txid, so global low-latency interaction is the default with zero extra infrastructure. "Where can the server live?" is really "where do state and authority live?" — and that is the axis the Trello frontend choice turns on.
  - **For the spike:** do *not* build edge (a §3 non-goal); just measure FE2's distant-colo first-paint (criterion 9). Cheaper FE2 mitigations if needed: stream a fast skeleton then SSE-patch the data in (moves perceived latency only); edge-cache the shell *while anonymous* (evaporates the moment per-tenant auth lands — itself a useful warning).

## 10. Open Questions

- Cloudflare SSE behavior at scale: confirm in M3 that 25s keepalive + the `/api/events` Cache Rule reliably holds connections through Cloudflare across multiple minutes of low/no traffic. If not, raise keepalive cadence; `Last-Event-ID` makes periodic reconnects tolerable regardless.
- Ring-buffer size for SSE replay (F2): 1024 events is an arbitrary starting point. For Trello with chatty boards this needs per-topic sizing, or replacement with "on reconnect, refetch current state from a paginated read endpoint" — decide before per-board fan-out is real.
- Cross-instance fan-out choice (LISTEN/NOTIFY vs Redis): defer until horizontal Go scaling is on the roadmap.
- **Reorder payload shape:** v1 sends the full new `order` array (robust for 3 fixed cards). The Trello product will need single-card "move card X after Y" deltas (full-order arrays don't scale to long lists and lose intent under concurrency). Decide when the second list lands — it interacts with the fractional-indexing switch.
- TanStack DB pre-1.0 — pin exact versions, expect breaking changes between minor releases until ~1.0. The custom SSE collection wrapper is the regression surface to retest on upgrade.
- Domain on Cloudflare: a registered domain on a Cloudflare zone is required; no free wildcard exists for generic origin proxying (Workers/Pages `*.workers.dev` is for their compute, not arbitrary origin pass-through). ~$10/yr for a `.com`.
- Datastar version/API surface (FE2): pin a specific 1.0+ release and verify the SSE element/signal-patch event names and Go SDK helpers against its docs before implementing F12–F14 (F16). The patch event naming changed during the 1.0 RC era — a stale name is a silent no-op. Also pin SortableJS and confirm its DOM mutations and Datastar's patches don't fight (F16).
- Cloudflare cache-purge step in deploys: `Cache-Control: no-cache` on `index.html` (F4) should make a purge unnecessary, but worth empirically confirming during M3 — if Cloudflare ignores `no-cache` under load, add a `POST /zones/$ZONE_ID/purge_cache` step (curl, same auth as D3) to the deploy.

## 11. Foreseen bugs / engineering risks

Known sharp edges in this stack — mitigations are wired into the requirements above; this section is the index so they don't get lost in a refactor.

- **64-bit txid truncation.** `pg_current_xact_id()` returns `xid8`; casting to `xid` silently truncates to 32 bits. Symptom: txid handshake "works most of the time, occasionally sticks/snaps" as wrapped values collide. Mitigation: F1 — never cast to `xid`, treat the value as a string in Go/JSON/JS (JS `Number` loses precision above 2^53).
- **Unique-position collision during reorder.** A naive per-row `UPDATE` of `position` against an immediate `UNIQUE(position)` constraint trips a duplicate-key violation mid-transaction (e.g. setting card B to position 0 while A still holds 0). Symptom: reorder randomly 500s depending on the swap direction. Mitigation: F10 — `DEFERRABLE INITIALLY DEFERRED` unique constraint (checked at commit, not per-row); F1 — apply the new order as a single bulk `UPDATE … FROM (VALUES …)` statement.
- **Stale-order reorder under concurrency.** Two clients drag from different starting orders; the second POST carries an `order` array built against an order that no longer holds. Symptom: a reorder silently "undoes" another client's move, or references a card list that has shifted. Mitigation: F1 validates `order` is a permutation of the *current* ids (rejecting unknown/missing ids), and the txid handshake (F8) reconciles each client to the committed order; full single-card-delta intent is deferred (§10).
- **SSE buffering at the Cloudflare edge.** Without the explicit Cache Rule (D3), Cloudflare can buffer/transform responses on the proxy path, defeating event-by-event delivery. Mitigation: D3 — `/api/events` Cache Rule that disables buffering; F2 — `Cache-Control: no-cache` and `X-Accel-Buffering: no` as belt-and-braces; verify with `curl --no-buffer -N` against the public URL.
- **SSE connection drops on intermediaries.** Browsers, mobile NATs, proxies, and Cloudflare itself enforce idle timeouts. Symptom: clients silently stop receiving events after N seconds, no error surfaced. Mitigation: F2 — 25s server-side `:keepalive` comments; EventSource native reconnect + `Last-Event-ID` for resilience.
- **Lost events on reconnect.** Between EventSource close and reopen, reorders commit to Postgres but bypass the dropped client. Mitigation: F2 — `Last-Event-ID` replay from in-memory ring; ring overflow falls back to a full state refetch through the SSE collection wrapper. Document this contract.
- **DnD library vs server patch fighting over the DOM (FE2).** SortableJS mutates the DOM on drop while Datastar patches the same elements from the SSE stream; uncoordinated, the card double-applies or reverts mid-animation. Mitigation: F16 — verify the wiring (let SortableJS own the drag, hand the resulting order to Datastar, and ensure the server patch is idempotent against the already-applied order).
- **In-process pub/sub does not survive horizontal scaling.** Two Go machines behind a load balancer means a mutation handled on machine A is invisible to SSE subscribers on machine B. Mitigation: §9 — explicit scale-out plan. For the v1 single-machine spike this is by-design but flagged so it isn't forgotten when machine count goes >1.
- **LISTEN/NOTIFY incompatible with transaction-mode pooling.** When the scale-out moment arrives, pasting the pooled Neon string into the `LISTEN` consumer appears to work for a single test, then silently drops notifications under load. Mitigation: §9 — use a session-mode or direct endpoint specifically for the NOTIFY consumer; document at the point of switch.
- **Cloudflare cache poisoning of `index.html`.** A stale `index.html` cached at the edge after a deploy points clients at old asset hashes that no longer exist. Mitigation: F4 — `index.html` ships `no-cache`; M3 verifies behavior. Add a `POST /zones/$ZONE_ID/purge_cache` step (curl, same auth as D3) to the deploy pipeline if observation shows `no-cache` insufficient under Cloudflare's aggressive cache.
- **TanStack DB pre-1.0 version skew.** Frequent minor breaking changes. Mitigation: pin exact versions; treat the custom SSE collection wrapper as the regression surface on upgrade.
- **Connection-count bottleneck on the Go app.** Each SSE client holds one TCP connection on the single Fly machine; at 10k concurrent clients that's 10k file descriptors on `shared-cpu-1x`. Mitigation: vertical scale (`fly scale vm`) until the §9 horizontal scale-out lands. Record observed concurrent-connection ceiling in M3 so the scale-out trigger is data-driven.

## 12. CLI prerequisites & commands

Backs the §2 goal of "all provisioned and deployed via CLI." Three services require login or an API token; everything else is local toolchain.

### Services requiring login

|Service|CLI binary|Login|Used for|
|---|---|---|---|
|Fly.io|`flyctl` (alias `fly`)|`fly auth login`|App create/deploy, secrets, machines, logs (D1, D4–D5)|
|Neon|`neon` (npm `neonctl`)|`neon auth`|Postgres project + connection strings (D2, F11)|
|Cloudflare|*(none — use the v4 REST API via `curl`)*|API token via env (`CLOUDFLARE_API_TOKEN`)|DNS, proxy, Cache Rules (D3)|
|GitHub *(fast-follow only)*|`gh`|`gh auth login`|Storing the Fly deploy token as a repo secret for the GHA pipeline (§8)|

### Local toolchain (no login)

`docker` + `docker compose` (dev Postgres), `go` ≥ 1.22, `node` ≥ 20 + `pnpm` ≥ 9 (lockfile is `pnpm-lock.yaml`), `psql` (libpq, for migrations), `task` (Taskfile runner), `air` (Go hot reload, dev only), `templ` (FE2 component codegen — `go install github.com/a-h/templ/cmd/templ@latest`; `task dev` runs `templ generate --watch`), `git`, `curl` (SSE / streaming checks; also the Cloudflare provisioning interface — see D3), `jq` (parse Cloudflare API responses).

### Install

```bash
# Fly
curl -L https://fly.io/install.sh | sh

# pnpm (via Corepack — ships with Node ≥ 16.10)
corepack enable
corepack prepare pnpm@latest --activate

# Neon
pnpm add -g neonctl

# Cloudflare — no CLI binary; everything is curl against the v4 REST API (D3).
# jq is only used to parse responses; install via your package manager if missing.

# GitHub (fast-follow only)
brew install gh
```

### First-time bootstrap

```bash
fly auth login
neon auth
export CLOUDFLARE_API_TOKEN='<dashboard: My Profile > API Tokens, scoped to the zone>'
gh auth login        # fast-follow only
```

### Neon: provision Postgres (M1, D2)

```bash
neon projects create --name hello-cards --region-id aws-us-west-2
neon set-context --project-id <project-id>

# Pooled string only — no replication / session endpoint required in v1.
neon connection-string --pooled true
```

### Fly: deploy the Go app (M3, D1)

```bash
fly launch --no-deploy --copy-config --name hello-cards --region sea
fly secrets set \
  DATABASE_URL='<neon pooled string>' \
  -a hello-cards
fly deploy --remote-only --strategy immediate -a hello-cards
```

### Cloudflare: proxy DNS + Cache Rules (M3, D3)

```bash
# Pre-req: zone already added to Cloudflare, registrar pointed at CF nameservers.
# All Cloudflare provisioning is curl against the v4 REST API — one auth pattern
# (CLOUDFLARE_API_TOKEN), one error surface, no extra CLI binary to install.
ZONE=example.com
APP_FQDN=hello-cards.example.com
FLY_APP_IP=$(fly ips list -a hello-cards | awk '/v4/ {print $2}' | head -n 1)
ZONE_ID=$(curl -s -H "Authorization: Bearer $CLOUDFLARE_API_TOKEN" \
  "https://api.cloudflare.com/client/v4/zones?name=$ZONE" | jq -r '.result[0].id')

# Proxied A record (orange cloud)
curl -X POST "https://api.cloudflare.com/client/v4/zones/$ZONE_ID/dns_records" \
  -H "Authorization: Bearer $CLOUDFLARE_API_TOKEN" \
  -H 'Content-Type: application/json' \
  --data "$(jq -n --arg name "$APP_FQDN" --arg ip "$FLY_APP_IP" \
    '{type:"A", name:$name, content:$ip, ttl:1, proxied:true}')"

# Cache Rules — Cloudflare's Rules engine is API-only at the CLI layer.
# 1) /events suffix → bypass cache, disable buffering (covers /api/events and /ds/events)
# 2) /api/*         → bypass cache (defense in depth on top of F4 headers)
# 3) (default)      → respect origin Cache-Control (year-long for /assets/*; revalidate for index.html)
# Commit the rule JSON to the repo so it's reproducible: ops/cloudflare/cache-rules.json
curl -X PUT "https://api.cloudflare.com/client/v4/zones/$ZONE_ID/rulesets/phases/http_request_cache_settings/entrypoint" \
  -H "Authorization: Bearer $CLOUDFLARE_API_TOKEN" \
  -H 'Content-Type: application/json' \
  --data @ops/cloudflare/cache-rules.json
```

### Test & ops (criteria 4, 5, 6, 7)

```bash
# Status + logs
fly status -a hello-cards
fly logs   -a hello-cards

# Criterion 4 — public URL end-to-end (through Cloudflare)
curl -i https://hello-cards.example.com/healthz

# Criterion 6 — edge cache verification
curl -I https://hello-cards.example.com/assets/<hashed>.js   # expect cf-cache-status: HIT after warm
curl -I https://hello-cards.example.com/api/cards/reorder    # expect cf-cache-status: BYPASS or DYNAMIC

# F2 streaming sanity — SSE must not buffer through Cloudflare
curl --no-buffer -N -H 'Accept: text/event-stream' \
  https://hello-cards.example.com/api/events

# Criterion 5 — restart the app, card order must persist, SSE resumes
fly machine list -a hello-cards
fly machine restart <id> -a hello-cards

# Reorder smoke (F1 — txid must be a string, not a number; order is a permutation of current ids)
curl -i -X POST https://hello-cards.example.com/api/cards/reorder \
  -H 'Content-Type: application/json' \
  -d '{"order":["c","a","b"]}'
```

### Tear-down

```bash
# Look up the DNS record id, then delete it.
RECORD_ID=$(curl -s -H "Authorization: Bearer $CLOUDFLARE_API_TOKEN" \
  "https://api.cloudflare.com/client/v4/zones/$ZONE_ID/dns_records?name=$APP_FQDN" \
  | jq -r '.result[0].id')
curl -X DELETE "https://api.cloudflare.com/client/v4/zones/$ZONE_ID/dns_records/$RECORD_ID" \
  -H "Authorization: Bearer $CLOUDFLARE_API_TOKEN"

fly apps destroy hello-cards
neon projects delete <project-id>
```

## 13. Repository, licensing & contributions

The repo is **public from day one and stays public through production.** Nothing in the architecture forces it private: the entire secret surface (Neon `DATABASE_URL`, `CLOUDFLARE_API_TOKEN`, the Fly deploy token, and later any auth signing/OAuth secrets) is injected out-of-band via `fly secrets` / env / GitHub repo secrets and is never committed. There is no security-through-obscurity dependency — the auth seam (§9) must be secure with the source fully visible.

### Business model & license

The end product is a **paid hosted service.** The license is chosen to keep the source open and PR-friendly while preventing someone from taking it and standing up a competing commercial service.

|ID |Requirement|
|---|-----------|
|L1 |License: **FSL-1.1-Apache-2.0** (Functional Source License). Source-available, not OSI "open source." Permits all use *except* competing commercially with the licensor's product/service; each release **auto-converts to Apache-2.0 two years after publication.** Drop the canonical text in `LICENSE.md` at repo root (from fsl.software — do not hand-edit the terms). State the model explicitly in `README`: *"Source-available under FSL-1.1-Apache-2.0; the hosted service is the commercial offering; contributions welcome."*|
|L2 |**Public repo ≠ open source.** Because FSL is source-available, say so plainly so readers don't assume MIT/Apache rights before the 2-year conversion. A missing `LICENSE` would default to all-rights-reserved and silently block the contributions we want — so L1 ships in the first commit, not later.|

### Contributions

|ID |Requirement|
|---|-----------|
|C1 |Inbound contributions are governed by a **DCO (Developer Certificate of Origin)**, not a CLA. Contributors certify origin with a `Signed-off-by` line (`git commit -s`); no paperwork, low friction. Add `CONTRIBUTING.md` with the DCO text (developercertificate.org) and the sign-off instruction; enforce with a DCO check (GitHub Action / app) on PRs.|
|C2 |The licensor retains copyright on its own commits; the DCO sign-off plus the FSL grant preserve the right to operate and commercialize the hosted service. Revisit a CLA only if a future need to *relicense the whole project* (e.g. dual-licensing) emerges — out of scope now.|

### Secret hygiene (keeps the repo safely public)

|ID |Requirement|
|---|-----------|
|S1 |**`.gitignore` in the first commit:** `.env`, `.env.*` (allow an `.env.example` with dummy values), local Postgres dumps, editor/OS cruft. This is the single habit that prevents leaking the Neon password on commit one.|
|S2 |No real secrets in tracked files: `fly.app.toml` carries app/region/timeouts only (DB string stays in `fly secrets`, D2/F11); `compose.yml` dev Postgres uses obviously-local dummy creds (`postgres:postgres`); `ops/cloudflare/cache-rules.json` is routing-only (`ZONE_ID` is fetched at runtime, §12).|
|S3 |Enable **GitHub secret scanning + push protection** on the repo; add a `gitleaks` (or `trufflehog`) pre-commit hook as belt-and-braces against an accidental paste. Cheap insurance for a public repo.|
