
# PRD: hello-cards (v3 — Go-native sync + edge)

**Status:** Draft · **Owner:** you · **Last updated:** 2026-06-09 · **Supersedes:** v2 (Electric sync-engine spike)

> **v3 domain object:** a single sortable column of 3 cards (reorder-only — no create/delete/rename). Smallest domain that exercises ordering; mirrors the eventual Trello target. The reorder mutation replaces the v2 increment; all architecture is unchanged.

## 1. Summary

Minimal full-stack reference app validating the production architecture for a Trello-like multi-tenant product with optimistic UI over flaky networks, globally distributed:

- **Postgres** (Neon `us-west-2`, paired with Fly `sea`) as source of truth.
- **Go API** as the *only* home of business logic: HTTP mutations write in a transaction and return the Postgres `txid` for client reconciliation. Live reads via SSE off the same server; in-process pub/sub fans events to subscribers.
- **TanStack DB** client: live queries + optimistic writes with automatic rollback, over a custom SSE collection.
- **Two frontends, one Go core** — validate the "logic exactly once" thesis:
  - **FE1 — Vite + React + TanStack DB**: rich-client, automatic optimistic + rollback, txid handshake. Embedded via `go:embed`.
  - **FE2 — Datastar + templ**: server-driven hypermedia; Go renders `templ` fragments and pushes them as SSE patches.
  Both drive the same Go core through thin adapters. FE2 is a **comparison spike**, not a committed dual-stack.
- Deployed to Fly.io (`flyctl`); **Cloudflare** proxied DNS in front: edge-cached static assets; `/api/*` and SSE pass through.

Domain is one column of 3 fixed cards the user drags to reorder — smallest domain that exercises ordering, which is the production mutation shape.

## 2. Goals

- Business logic exists exactly once, in Go. **Proof:** two structurally different FEs drive the same core. If logic can't cleanly split from serialization, that itself is the finding.
- Instant optimistic UI: drag-reorder renders <1 frame; survives flaky wifi; reconciles cleanly (no snap-back).
- Live multi-client sync: two browsers see each other's reorders within ~1s via SSE.
- Globally distributed static delivery: first paint bounded by edge latency, not trans-Pacific RTT.
- One app artifact (Go binary + embedded SPA) + Postgres + Cloudflare; all CLI-provisioned.
- The SSE handler is the auth seam, ready for per-tenant filtering.

## 3. Non-Goals

- True offline-first (long disconnected sessions, queued writes). Target is optimistic UI over flaky connections; offline durability is a later decision (TanStack `offline-transactions` is the upgrade path).
- Cross-instance fan-out: in-process pub/sub is single-machine. Scale-out plan documented in §9 but not built.
- Auth/users, multi-region origin, horizontal scaling, migration tooling beyond `psql`, observability beyond Fly logs.
- Full card CRUD and broader Trello domain. v1 = one list of 3 fixed cards + reorder. **Fractional indexing** is out of scope (v1 uses server-rewritten integer positions — F1); fractional is the §9 upgrade path.
- CDN-fronted shape sync (Electric). Re-evaluate when board fan-out demands it.

## 4. Functional Requirements

### API (Go, stdlib `net/http`)

|ID |Requirement|
|---|-----------|
|F1 |`POST /api/cards/reorder` → body `{"order": ["<id>",…]}` (new top-to-bottom order). In a transaction: validate `order` is a permutation of current ids (reject per F5), then rewrite every card's `position` to its index in `order` via **a single bulk `UPDATE … FROM (VALUES …)` against a `DEFERRABLE` unique constraint on `position`** so intermediate states don't trip the constraint (§11). Returns `{"cards":[…], "txid": "<pg txid>"}`. Capture txid in-tx with `SELECT pg_current_xact_id()::text`. **Never cast to `xid`** — `pg_current_xact_id()` returns 64-bit `xid8`; `::xid` truncates to 32 bits and breaks handshake matching. txid is a **string** end-to-end (JS `Number` loses precision above 2^53). After commit, handler publishes `{txid, cards}` to in-process pub/sub.|
|F2 |`GET /api/events` → SSE (`Content-Type: text/event-stream`). Subscribes to pub/sub; v1 hardcodes the `cards` topic. Each mutation = one SSE message with `id: <txid>` and `data:` (full ordered card list). Constraints: `http.Flusher.Flush` per event; per-route read/write timeouts disabled or ≥1h; `Cache-Control: no-cache`; `X-Accel-Buffering: no`; **server emits `:keepalive\n\n` every 25s** to defeat intermediary idle closes. Supports `Last-Event-ID` reconnect: replays events with txid > Last-Event-ID from an in-memory ring buffer (v1: 1024). On ring overflow, client falls back to a full state refetch.|
|F3 |`GET /healthz` → 200. Fly health check.|
|F4 |Non-`/api` paths serve embedded SPA with `index.html` fallback. `/assets/*` (Vite content-hashed) → `Cache-Control: public, max-age=31536000, immutable`. `index.html` → `Cache-Control: no-cache` so Cloudflare revalidates the entry point (atomic deploys at the client level).|
|F5 |Mutation errors (including malformed/stale non-permutation `order`) return structured JSON with 4xx/5xx so TanStack DB rolls back (dragged card snaps back).|

### Frontend (Vite + React + TanStack DB)

|ID |Requirement|
|---|-----------|
|F6 |`cards` = TanStack DB collection backed by a **custom SSE adapter**: opens `EventSource` against `/api/events`, dispatches events into the change stream, supplies the `onUpdate` mutation handler. Native EventSource reconnect carries `Last-Event-ID`; adapter adds exponential-backoff retry over hard close.|
|F7 |Column is a live query ordered by `position`; reorders from other clients re-sort the stack on SSE arrival.|
|F8 |Drag-to-reorder uses **dnd-kit** sortable + optimistic update through the collection; `onUpdate` POSTs to F1 and returns `{txid}`. TanStack DB holds the optimistic order until that txid arrives via SSE (no snap-back flicker); rolls back on F5.|
|F9 |Airplane-mode: with network off, drag shows optimistic order; on reconnect within the session it commits or visibly snaps back. SSE reconnect restores stream; missed events replayed via `Last-Event-ID` if within ring, else full refetch. (No cross-restart offline persistence in v1.)|

### Frontend B (Datastar + templ) — comparison spike

Mounted at `/ds/…` in the same binary, over the same `cards` service and pub/sub. No client build step beyond `templ generate`; Datastar runtime is a single cacheable JS file.

|ID |Requirement|
|---|-----------|
|F12|`POST /ds/cards/reorder` → calls the **same core mutation** as F1. Responds with an SSE-formatted patch (rendered `templ` fragment) so the dragging client updates on response. Shared core means F1/F12 differ only in serialization.|
|F13|`GET /ds/events` → SSE on the same `cards` topic, but each event renders a `templ` element-patch rather than JSON. Reuses all F2 connection hardening verbatim. **Reconnect contract differs:** server renders the current authoritative fragment rather than replaying deltas; F2's ring buffer / `Last-Event-ID` is an FE1-shaped feature, not required here.|
|F14|Column rendered server-side via `templ`; reorders from other clients arrive as element patches and apply with no client reconciliation (server authoritative).|
|F15|**Optimistic UX is opt-in and manual.** Datastar has no native DnD, so FE2 still needs a client DnD library (**SortableJS**) whose `onEnd` posts the new `order` to F12 — a key comparison finding (DnD is a client concern in *both* stacks; they differ in what happens *after* the drop). Default is a server round-trip (latency = RTT; card may briefly jump back until the server patch lands). Document whether the spike implements an optimistic Datastar signal (hold dropped order, corrected by next server patch) or stays purely server-driven — primary UX dimension compared against F8. txid is not load-bearing here; keep only if optimistic signals need flicker-suppression dedup.|
|F16|Datastar's element-patch event names and Go SDK surface **must be verified against current (1.0+) docs** — naming changed during the 1.0 RC era; don't bake names in from memory. **Also verify SortableJS ↔ Datastar wiring** (reading post-drop DOM order without the two libraries fighting — Datastar patches the same elements SortableJS reorders).|

### Data (Postgres)

|ID |Requirement|
|---|-----------|
|F10|`card(id TEXT PRIMARY KEY, label TEXT NOT NULL, position INTEGER NOT NULL, CONSTRAINT card_position_unique UNIQUE (position) DEFERRABLE INITIALLY DEFERRED)`; seeded by idempotent migration with 3 cards at positions 0,1,2 via `task migrate` (psql).|
|F11|Pooled Neon connection string with `sslmode=require`. No `wal_level=logical` and no direct/session endpoint in v1 (nothing tails WAL; no `LISTEN/NOTIFY`). Transaction-mode pooling fine throughout.|

## 5. Architecture

```
browser
  │
  ├── static assets ──> Cloudflare edge (cached on hashed filenames) ─┐
  │                                                                    │
  ├── reads: TanStack DB SSE collection                                │
  │           └── EventSource ──> Cloudflare (pass-through) ──> Go /api/events
  │                                                              ▲
  │                                                              │ in-process pub/sub
  └── writes: optimistic reorder ──> Cloudflare (pass-through) ──> Go /api/cards/reorder
                                                                   └── Postgres txn (Neon, us-west-2)
                                                                        returns txid
                                                                        publish(txid, cards) ─┘

(client holds optimistic order until matching txid arrives via SSE)
```

- **Core/adapter split (keystone for the two-FE proof):** a transport-agnostic `cards` service owns the logic — permutation validation, bulk position-rewrite tx, `pg_current_xact_id()::text` capture, `publish(txid, cards)`. Two adapters sit over it and over the same pub/sub: **A** = JSON handlers + JSON-SSE (FE1); **B** = Datastar handlers + `templ`-fragment SSE (FE2). One mutation event fans to both adapters' subscribers.
- **Mounting:** one binary, two route trees — `/api/*` + embedded SPA (FE1), `/ds/*` + server-rendered templ (FE2). Chosen over content-negotiation on one `/api/events` to keep each FE's contract legible and avoid `Accept`-header subtlety through Cloudflare. One Neon, one pub/sub, one deploy.
- Repo layout: `main.go`, `cards/` (service), `frontend/` (FE1), `ds/` (FE2 templ + handlers), `migrations/`, `Dockerfile`, `fly.app.toml`, `Taskfile.yml`, `compose.yml`.
- Build: multi-stage Dockerfile (node → go → scratch); Go stage runs `templ generate` before `go build`; `fly deploy --remote-only`.
- **Edge / global-latency trade (key comparison finding):** FE1's content-hashed assets edge-cache so first paint is bounded by edge latency. FE2's HTML shell is origin-rendered from `sea` — only the Datastar runtime edge-caches, so initial paint costs an origin round-trip. The §2 global-latency goal is an FE1 advantage; record FE2's distant-colo first-paint number as part of the comparison.
- Dev loop (`task dev`): `docker compose up` (Postgres), `air` (Go hot reload), `pnpm dev` (Vite proxy `/api -> :8080`; streams SSE without buffering by default). Embed only at release. Cloudflare is not in the dev path (criterion 6 covers prod verification).

## 6. Deployment Requirements

|ID |Requirement|
|---|-----------|
|D1 |**App (Fly `sea`)**: `fly launch --no-deploy`; shared-cpu-1x; `min_machines_running = 1`. No volume. `/api/events` route timeouts disabled or ≥1h; other routes normal.|
|D2 |**Postgres (Neon `us-west-2`)**: closest to Fly `sea` (~10–20ms). Pooled string only. `sslmode=require`. Auto-suspend ok — no persistent replication consumer. Free-tier branch sufficient.|
|D3 |**Cloudflare proxied DNS**: zone added, registrar nameservers updated, `A`/`AAAA` (or `CNAME`) at Fly anycast with proxy enabled. Cache Rules: (a) **`/events` suffix (covers `/api/events` AND `/ds/events`)** — bypass cache, disable response buffering; (b) `/api/*` — bypass cache; (c) default — respect origin `Cache-Control`. SSL: **Full (strict)** (Fly terminates TLS).|
|D4 |Fly health checks on `/healthz`. SSE liveness monitored via heartbeats (F2), not Fly checks.|
|D5 |Strategy `rolling` once SSE is verified. On drain, in-flight SSE close cleanly; clients reconnect via EventSource + `Last-Event-ID`. M1–M2 use `immediate`.|
|D6 |**Cost:** app ~$3 + Cloudflare free + Postgres. **Neon free tier (100 CU-h/mo, 0.5 GB) covers spike AND solo/low-traffic production**: DB scales to zero after 5 min idle, so daily solo use is ~15–30 CU-h/mo. **Caveat:** anything that pings Postgres on a timer defeats scale-to-zero — keep `/healthz` (F3) a pure in-process 200 with no DB query, else Fly's health checks keep DB warm permanently (~183 CU-h/mo → over budget). Smallest paid plan (~$19/mo) only needed for steady multi-user traffic or PITR/branching/SLA. Floor for solo dev/prod: ~**$3/mo**.|
|D7 |Egress/TLS: app ↔ Neon over TLS (`sslmode=require`). Client ↔ Cloudflare = HTTPS (CF cert). Cloudflare ↔ Fly = HTTPS Full (strict).|
|D8 |Cloudflare proxy timeouts (100s on Free/Pro/Business) don't strictly cap SSE under active traffic, but the contract tolerates periodic reconnects via `Last-Event-ID`. 25s keepalive keeps idle conns warm. Confirm empirically in M3.|

## 7. Acceptance Criteria

1. `task dev` gives hot-reload; drag-reorder is optimistic and persists in dev Postgres.
2. Two browsers: reordering one updates the other within ~1s via SSE.
3. Flaky-net (devtools throttle/offline): dragged card lands instantly; no snap-back flicker on commit (txid handshake); a rejected reorder (F5) snaps back; SSE reconnect catches up via `Last-Event-ID` or full refetch.
4. `fly deploy` succeeds; the Cloudflare URL serves the React page; drag-reorder works end-to-end.
5. `fly machine restart` → order unchanged; SSE clients reconnect within seconds and resume from `Last-Event-ID`.
6. **Edge caching verified globally:** `curl -I` on `/assets/<hashed>.js` returns `cf-cache-status: HIT` after warm-up; `/api/*` returns `BYPASS` or `DYNAMIC`. Spot-check from a distant Cloudflare colo via `cf-ray`.
7. SSE survives extended idle: with a client connected and zero traffic for ≥5 minutes (several 25s keepalive cycles, comfortably past the ~100s intermediary idle window), the next reorder reaches all clients without refresh. A forced drop (server restart) recovers via EventSource reconnect + `Last-Event-ID`. *(Simplified from overnight idle — the failure mode is intermediary idle timeout, which manifests within minutes, not hours; D8/§10 keep the long-idle question open for observation in normal use.)*
8. Schema drill: add a column, run migration, document client behavior (does it need reconnect/refetch to observe the new shape?).
9. **Two-FE parity (comparison payoff):** FE1 and FE2 side by side; reordering in either updates both within ~1s — one mutation, two adapters off one pub/sub. Record the comparison: lines of client code, **DnD-library integration cost (dnd-kit + optimistic vs SortableJS + server patch, including F16 DOM-ownership wiring)**, drop-to-settled latency (FE1 instant vs FE2 round-trip), first-paint-from-distant-colo, reconnect behavior. This table decides which stack the Trello PRD adopts.

## 8. Milestones

Each phase independently testable; tick `Status` as you ship.

- **M0 — repo bootstrap** · ☑
  `git init`, `LICENSE.md` (FSL-1.1-Apache-2.0 — L1), `.gitignore` + `.env.example` (S1), `README.md` (L2), `CONTRIBUTING.md` (DCO — C1). Enable GitHub secret scanning + push protection; add `gitleaks` pre-commit; add DCO check action (S3). Scaffold `Taskfile.yml`, `compose.yml`, multi-stage `Dockerfile`, empty `cards/` `migrations/` `frontend/` `ds/`, and `main.go` serving `/healthz` (F3). **Done when:** `task dev` brings up Postgres, `curl localhost:8080/healthz` → 200; DCO check green on throwaway PR; `gitleaks` blocks a deliberate fake-secret commit.

- **M1a — write path + txid** · ☑
  `card` migration (F10) with `DEFERRABLE` unique on `position`, seeded with 3 cards. `cards` service owns permutation validation, bulk `UPDATE … FROM (VALUES …)`, `pg_current_xact_id()::text` capture as a **string**. `POST /api/cards/reorder` (F1) wired. **Done when:** `curl -X POST … '{"order":["c","a","b"]}'` returns `{cards, txid}` with txid as decimal string; non-permutation `order` → 4xx (F5); a 100-random-reorder fuzz never trips the unique constraint.

- **M1b — SSE fan-out** · ☑
  In-process pub/sub; reorder handler publishes `{txid, cards}` post-commit. `GET /api/events` (F2) with full hardening: text/event-stream, no-cache, `X-Accel-Buffering: no`, `Flush` per event, 25s `:keepalive`, route timeouts disabled. In-memory ring (1024) + `Last-Event-ID` replay; overflow contract documented. **Done when:** `curl --no-buffer -N /api/events` receives `id: <txid>` after `curl POST /api/cards/reorder`; reconnect with `Last-Event-ID: <old txid>` replays the gap; an overflow returns the documented sentinel.

- **M2a — FE1 read path** · ☑
  Vite + React under `frontend/` with dev proxy `/api → :8080`. Custom SSE-backed TanStack DB collection (F6); column via live query ordered by `position` (F7); native reconnect carries `Last-Event-ID`; adapter adds exp-backoff. **Done when:** two tabs open; `curl POST /api/cards/reorder` updates both within ~1s; restarting the Go server triggers clean reconnect with no missed events.

- **M2b — FE1 optimistic write** · ☑
  dnd-kit sortable; `onUpdate` POSTs and returns `{txid}` so TanStack DB holds the optimistic order until matching txid arrives via SSE (no flicker); rolls back on F5. The core/adapter split (`cards` service + Adapter A) crystallizes here so M2.5 is purely additive. **Done when:** criteria 1–3 pass locally.

- **M2.5a — Datastar/SortableJS pin & verify** · ☑
  Pin a Datastar 1.0+ release and a SortableJS version; verify the SSE element/signal-patch event names, Go SDK helpers, and SortableJS↔Datastar DOM-ownership wiring (F16). Output: `ds/NOTES.md` listing the verified API surface and any 1.0-RC-era changes. **Done when:** smoke `templ` page on `/ds/_smoke` receives a hand-crafted element patch and updates; pinned versions in `go.mod`/`package.json`/notes.

- **M2.5b — FE2 over shared core** · ☑
  Adapter B at `/ds/*` over the existing `cards` service and pub/sub: `POST /ds/cards/reorder` (F12), `GET /ds/events` rendering templ-fragment patches (F13) reusing F2 hardening, server-rendered column updated via element patches (F14), SortableJS-driven post-drop (F15) with the optimistic-vs-server-driven choice documented (server-driven — ds/NOTES.md). `templ generate` runs in dev (`task dev` backgrounds `templ generate --watch` alongside air) and as a Docker build step. **Done when:** criterion 9 passes — both FEs side by side, reorder in either updates both within ~1s; comparison table filled in. *(Verified by headless-browser harness: real mouse drag in FE2 commits and fans out cross-tab/cross-adapter <1s; comparison table in ds/NOTES.md — first-paint-from-distant-colo and drop-to-settled numbers measured at M3 post-deploy.)*

- **M3a — Neon + Fly origin** · ☑
  Neon project in `aws-us-west-2` (D2), pooled string in `fly secrets`. `fly launch --no-deploy --copy-config --name hello-cards --region sea`, then `fly deploy --remote-only --strategy immediate`. `min_machines_running = 1`; `/api/events` timeouts disabled or ≥1h (D1). **Done when:** criterion 4 passes against raw `*.fly.dev` URL (no CF yet); `/healthz` is in-process 200 with no DB query (D6 — keeps Neon scale-to-zero intact); drag-reorder works end-to-end on FE1.
  - **Shipped at `https://hello-cards.fly.dev` (`fly.app.toml`).** Verified via curl: `/healthz` 200, embedded React shell (`no-cache` + hashed assets), Neon-backed `/api/cards`, `/assets/*` `immutable`, reorder→SSE `id`⇄`txid` string handshake, non-permutation→400 (F5).
  - **F4 landed here, not earlier.** No milestone had implemented the `go:embed` SPA serving; M3a's FE1 done-criterion forced it. Now `spa.go`/`spa_stub.go` (build tag `embedassets`) + catch-all `/` route in `main.go`; Dockerfile frontend stage uncommented and builds with `-tags embedassets`.
  - **Deviation — region: `sea` → `sjc`.** Fly deprecated `sea` (D2's pin) — "cannot have new resources provisioned." `sjc` is the West-Coast replacement (~20–30ms to Neon Oregon, still low-RTT). **D2/D3/§12 commands that say `sea` are stale — read as `sjc`.**
  - **Deviation — Docker `node:20` → `node:22`** (§12 floor was ≥20): current pnpm (11.x via corepack) requires `node:sqlite`, Node 22+.
  - **Single machine enforced.** `fly deploy` auto-created an HA pair; scaled to 1 (`fly scale count 1`) — in-process pub/sub is single-machine (§3/§11), so a second machine would silently drop cross-client SSE fan-out.

- **M3b — Cloudflare proxy + Cache Rules** · ☐
  Proxied A/AAAA at apex/subdomain (D3); SSL Full (strict). `ops/cloudflare/cache-rules.json` committed and PUT against `http_request_cache_settings` ruleset entrypoint with three rules: (a) `/events` suffix bypass + disable buffering; (b) `/api/*` bypass; (c) default respects origin. **Done when:** criterion 6 passes — `/assets/*` HIT after warm-up, `/api/*` BYPASS/DYNAMIC, `curl --no-buffer -N` on `/api/events` and `/ds/events` delivers without buffering, distant-colo spot-check via `cf-ray`.

- **M3c — production hardening drills** · ☐
  `fly machine restart` (criterion 5). Extended-idle drill, ≥5 min (criterion 7 — simplified from overnight). Schema-change (criterion 8). Switch strategy from `immediate` to `rolling` (D5). **Done when:** criteria 5, 7, 8 pass; `fly.app.toml` carries `strategy = "rolling"`; observed concurrent-connection ceiling on `shared-cpu-1x` recorded so §9 scale-out trigger is data-driven.

- **Fast-follow** · ☐
  GHA `fly deploy` on push to main (Fly token in `gh` secrets). Spike TanStack DB persistence + `offline-transactions` for true-offline (§3). Spike cross-instance fan-out (LISTEN/NOTIFY or Redis) for multi-machine Go (§9). None gate v1 sign-off.

## 9. Forward-looking notes (Trello PRD inputs)

- **Multitenancy** rides on the SSE handler: per authenticated user/org, subscribe only to permitted topics. Per-board vs per-org topic granularity drives fan-out cost (events × subscribers) and authz complexity — decide early.
- **Card ordering:** v1 uses server-rewritten integer positions (F1) over one fixed list — enough to exercise the mutation and handshake. Production upgrade is **fractional indexing** (lexicographic keys) with server-side rebalance, enforced only in Go: avoids rewriting every row on each move, supports inserts/cross-list moves. Concurrent same-position writes resolve server-side; clients reconcile via txid. Swap touches only the `cards` service's rewrite step.
- **Mutation surface** stays small HTTP handlers: createBoard/List/Card, move, rename, archive — each a transaction returning txid, each publishing to relevant topics. Single-machine case needs no new infra beyond v1.
- **Scale-out** (multi-machine): in-process pub/sub no longer crosses instances. Two paths:
  - Postgres `LISTEN/NOTIFY` on a session-mode or direct endpoint (txn-mode pooling doesn't support `LISTEN`). Cheapest, 8KB NOTIFY payload cap.
  - Small Redis (Upstash, Fly Redis) for cross-instance pub/sub. More flexible, slight cost.
  - SSE handler shape unchanged either way.
- **CDN-fronted shape sync** (Electric or equivalent): worth re-evaluating only when board read fan-out justifies trading the gatekeeper auth seam for edge-cached partial-replication shapes. Own spike.

### Edge topology & the "client-as-edge" trade

The most important finding the spike will surface — the axis the Trello frontend choice turns on.

- **"Edge" compresses three layers, only one cheap.** (1) *Compute* — Go in N regions is trivial on Fly. (2) *Cross-region fan-out* — in-process pub/sub no longer reaches subscribers in other regions; the §9 scale-out bus becomes mandatory AND cross-region; latency floor is commit-region → subscriber-region propagation. (3) *Data* — the hard layer.
- **Writes stay anchored to one primary.** Edge nodes forward mutations to it (Fly `fly-replay: region=<primary>` is canonical: one cross-region hop, writes only). Multi-primary (CRDT/multi-master) is the wrong model for transactional with server-side ordering + txid authority. Edge buys fast *reads/renders*, never fast *writes*.
- **Latency that flips:** only FE2's initial render and read-only paths, and only with per-region read replicas. FE1 static assets already edge-cache for free.
- **Neon wrinkle:** D2 pins Neon `us-west-2` ~10–20ms from Fly `sea`. Multi-region Go forces either slow cross-region reads or per-region replicas — Neon's cross-region replica story is thinner than Fly Postgres's. "Go at the edge" silently re-opens DB topology and may push off Neon toward Fly Postgres.
- **Consistency hazard, worst for FE2.** Edge read replicas + server-authoritative render interact badly: client reorders (committed txid N via primary) but local replica is at N−1 → server renders stale order → SSE patch corrects → *snap*. Making edge safe for FE2 means rebuilding FE1's reconciliation inside the server model (read-from-primary-until-replica≥txid, or txid-keyed optimistic signals) — re-importing the complexity Datastar was chosen to avoid.
- **What server-driven apps do at scale:** (1) **tenant-pinned regions** — each org gets a home region; writes are local there; users elsewhere accept RTT (Linear/Notion/Turso shape; cross-tenant features get harder, migrations are surgery). (2) **Per-region read replicas + write-to-primary** — to dodge the replica-lag snap requires the txid handshake server-side, rebuilding FE1's reconciliation inside FE2's render path. (3) **Optimistic islands inside the server-driven shell** — Stimulus/Alpine on Hotwire, signals on Datastar — hold hot interactions (drag, type-ahead) locally. At scale FE2 converges toward FE1 on hot paths.
- **The fork.** Server-driven (FE2)'s only lever for global latency is *physically relocating the server* — edge compute + edge data + cross-region fan-out, expensive and consistency-fraught, dragging DB topology along. Local-first (FE1)'s answer is "**the client *is* the edge node**": holds state, applies optimistically, reconciles via txid, so global low-latency interaction is the default with zero extra infra. "Where can the server live?" is really "where do state and authority live?"
- **For the spike:** don't build edge (§3 non-goal); just measure FE2's distant-colo first-paint (criterion 9). Cheap FE2 mitigations if needed: stream fast skeleton then SSE-patch data in (perceived latency only); edge-cache the anonymous shell (evaporates the moment per-tenant auth lands — itself a warning).

## 10. Open Questions

- Cloudflare SSE at scale: confirm in M3 that 25s keepalive + `/api/events` Cache Rule reliably holds through CF across low-traffic minutes. If not, raise cadence; `Last-Event-ID` makes periodic reconnects tolerable.
- Ring-buffer size (F2): 1024 is arbitrary. Trello chatty boards need per-topic sizing, or replacement with "on reconnect, refetch current via paginated read" — decide before per-board fan-out.
- Cross-instance fan-out choice (LISTEN/NOTIFY vs Redis): defer until horizontal scaling.
- **Reorder payload shape:** v1 sends the full new `order` (fine for 3 cards). Trello needs single-card "move X after Y" deltas (full-arrays don't scale, lose intent under concurrency). Decide when the second list lands — interacts with the fractional-indexing switch.
- TanStack DB pre-1.0 — pin exact versions, expect breaking changes. Custom SSE collection is the regression surface on upgrade.
- Domain on CF: registered domain on a CF zone required; no free wildcard for generic origin proxying. ~$10/yr.
- ~~Datastar version/API (FE2): pin 1.0+ release and verify SSE event names + Go SDK against docs before F12–F14 (F16). 1.0-RC-era naming changed — a stale name is a silent no-op. Also pin SortableJS and confirm DOM ownership (F16).~~ **Resolved (M2.5a/b):** versions pinned and full surface verified in `ds/NOTES.md`. The silent-no-op class was real twice over: `data-on-load` → `data-init`, and keyed attributes are colon-separated (`data-on:reorder`, not `data-on-reorder` — dash forms are skipped without error). Pinned by test.
- Cloudflare cache-purge in deploys: `no-cache` on `index.html` (F4) should make purge unnecessary; confirm in M3. If CF ignores `no-cache` under load, add `POST /zones/$ZONE_ID/purge_cache` to the deploy.

## 11. Foreseen bugs / engineering risks

Index of sharp edges; mitigations are wired into §4–§6.

- **64-bit txid truncation.** `pg_current_xact_id()` returns `xid8`; `::xid` silently truncates to 32 bits. Symptom: handshake "works mostly, occasionally sticks/snaps" as wrapped values collide. Mitigation: F1 — never cast; treat as string in Go/JSON/JS.
- **Unique-position collision during reorder.** Naive per-row `UPDATE` of `position` against an immediate `UNIQUE(position)` trips mid-tx duplicate-key. Symptom: random 500s depending on swap direction. Mitigation: F10 `DEFERRABLE INITIALLY DEFERRED` + F1 bulk `UPDATE … FROM (VALUES …)`.
- **Stale-order reorder under concurrency.** Two clients drag from different starting orders; second POST is built against a no-longer-current order. Symptom: silently undoes another client's move, or references a shifted list. Mitigation: F1 validates `order` is a permutation of *current* ids (reject unknown/missing); txid handshake (F8) reconciles. Single-card delta intent deferred (§10).
- **SSE buffering at the Cloudflare edge.** Without the explicit Cache Rule (D3), CF can buffer/transform on the proxy path, defeating event-by-event delivery. Mitigation: D3 rule disables buffering; F2 `Cache-Control: no-cache` + `X-Accel-Buffering: no`; verify with `curl --no-buffer -N`.
- **SSE drops on intermediaries.** Browsers, mobile NATs, proxies, CF enforce idle timeouts. Symptom: clients silently stop, no error surfaced. Mitigation: F2 25s `:keepalive` + EventSource reconnect + `Last-Event-ID`.
- **Lost events on reconnect.** Between close and reopen, reorders commit but bypass the dropped client. Mitigation: F2 `Last-Event-ID` replay from ring; overflow → full refetch.
- **DnD vs server patch fighting over DOM (FE2).** SortableJS mutates on drop while Datastar patches the same elements from SSE; uncoordinated, the card double-applies or reverts mid-animation. Mitigation: F16 — let SortableJS own the drag, hand the order to Datastar, ensure server patch is idempotent against already-applied order.
- **In-process pub/sub doesn't survive horizontal scaling.** Two Go machines = mutation on A is invisible to subscribers on B. Mitigation: §9 scale-out plan. By-design for v1 but flagged so it isn't forgotten.
- **LISTEN/NOTIFY incompatible with txn-mode pooling.** Pasting the pooled Neon string into a `LISTEN` consumer appears to work once, then silently drops under load. Mitigation: §9 — session-mode or direct endpoint for the NOTIFY consumer.
- **Cloudflare cache poisoning of `index.html`.** Stale `index.html` at the edge points clients at old asset hashes. Mitigation: F4 `no-cache`; M3 verifies. Add `POST /zones/$ZONE_ID/purge_cache` to deploy if observation shows `no-cache` insufficient.
- **TanStack DB pre-1.0 version skew.** Frequent minor breaking changes. Mitigation: pin exact versions; custom SSE collection is the regression surface.
- **Truncate-style snapshot sync pins stale overlays (TanStack DB).** Applying each SSE frame as truncate+reinsert makes the library re-apply the client's optimistic snapshot on top of every later frame: once a local drag completes, foreign reorders are silently overridden — persistent two-browser desync (hit in M2b). Mitigation: F6 adapter applies frames as keyed upserts (update/insert + delete for vanished rows); two-client convergence tests sweep interleaved drag storms across timing offsets.
- **Connection-count bottleneck on Go app.** Each SSE client = one TCP conn; 10k clients = 10k FDs on `shared-cpu-1x`. Mitigation: vertical scale (`fly scale vm`) until §9 horizontal lands. Record observed ceiling in M3 so trigger is data-driven.

## 12. CLI prerequisites & commands

Backs §2's "all CLI-provisioned." Three services need login/token; everything else is local toolchain.

### Services requiring login

|Service|CLI|Login|Used for|
|---|---|---|---|
|Fly.io|`flyctl` (alias `fly`)|`fly auth login`|App create/deploy, secrets, machines, logs (D1, D4–D5)|
|Neon|`neon` (npm `neonctl`)|`neon auth`|Postgres project + connection strings (D2, F11)|
|Cloudflare|*(none — `curl` v4 REST API)*|`CLOUDFLARE_API_TOKEN` env|DNS, proxy, Cache Rules (D3)|
|GitHub *(fast-follow)*|`gh`|`gh auth login`|Storing Fly deploy token as repo secret (§8)|

### Local toolchain (no login)

`docker` + `compose`, `go` ≥ 1.26, `node` ≥ 20 + `pnpm` ≥ 9 (`pnpm-lock.yaml`), `psql`, `task`, `air`, `templ` (`go install github.com/a-h/templ/cmd/templ@latest`), `git`, `curl`, `jq`.

### Install

```bash
curl -L https://fly.io/install.sh | sh                       # Fly
corepack enable && corepack prepare pnpm@latest --activate    # pnpm
pnpm add -g neonctl                                           # Neon
brew install gh                                               # GitHub (fast-follow)
# Cloudflare — no CLI; curl + v4 REST API.
```

### Bootstrap

```bash
fly auth login
neon auth
export CLOUDFLARE_API_TOKEN='<dashboard: My Profile > API Tokens, scoped to the zone>'
gh auth login   # fast-follow only
```

### Neon: provision (M1, D2)

```bash
neon projects create --name hello-cards --region-id aws-us-west-2
neon set-context --project-id <project-id>
neon connection-string --pooled true   # pooled only; no replication/session endpoint in v1
```

### Fly: deploy (M3, D1)

```bash
fly launch --no-deploy --copy-config --name hello-cards --region sea
fly secrets set DATABASE_URL='<neon pooled string>' -a hello-cards
fly deploy --remote-only --strategy immediate -a hello-cards
```

### Cloudflare: DNS + Cache Rules (M3, D3)

Pre-req: zone added, registrar pointed at CF nameservers. All CF provisioning is `curl` against the v4 REST API.

```bash
ZONE=example.com
APP_FQDN=hello-cards.example.com
FLY_APP_IP=$(fly ips list -a hello-cards | awk '/v4/ {print $2}' | head -n 1)
ZONE_ID=$(curl -s -H "Authorization: Bearer $CLOUDFLARE_API_TOKEN" \
  "https://api.cloudflare.com/client/v4/zones?name=$ZONE" | jq -r '.result[0].id')

# Proxied A record (orange cloud)
curl -X POST "https://api.cloudflare.com/client/v4/zones/$ZONE_ID/dns_records" \
  -H "Authorization: Bearer $CLOUDFLARE_API_TOKEN" -H 'Content-Type: application/json' \
  --data "$(jq -n --arg name "$APP_FQDN" --arg ip "$FLY_APP_IP" \
    '{type:"A", name:$name, content:$ip, ttl:1, proxied:true}')"

# Cache Rules — commit JSON to repo (ops/cloudflare/cache-rules.json) for reproducibility.
# (a) /events suffix → bypass + disable buffering (covers /api/events AND /ds/events)
# (b) /api/*         → bypass (defense in depth over F4)
# (c) default        → respect origin Cache-Control (year-long /assets/*; revalidate index.html)
curl -X PUT "https://api.cloudflare.com/client/v4/zones/$ZONE_ID/rulesets/phases/http_request_cache_settings/entrypoint" \
  -H "Authorization: Bearer $CLOUDFLARE_API_TOKEN" -H 'Content-Type: application/json' \
  --data @ops/cloudflare/cache-rules.json
```

### Test & ops (criteria 4, 5, 6, 7)

```bash
fly status -a hello-cards
fly logs   -a hello-cards

# Criterion 4 — public URL through CF
curl -i https://hello-cards.example.com/healthz

# Criterion 6 — edge cache
curl -I https://hello-cards.example.com/assets/<hashed>.js   # cf-cache-status: HIT after warm
curl -I https://hello-cards.example.com/api/cards/reorder    # BYPASS or DYNAMIC

# F2 streaming sanity
curl --no-buffer -N -H 'Accept: text/event-stream' \
  https://hello-cards.example.com/api/events

# Criterion 5 — restart, order persists, SSE resumes
fly machine list -a hello-cards
fly machine restart <id> -a hello-cards

# Reorder smoke (F1 — txid must be string; order is permutation of current ids)
curl -i -X POST https://hello-cards.example.com/api/cards/reorder \
  -H 'Content-Type: application/json' -d '{"order":["c","a","b"]}'
```

### Tear-down

```bash
RECORD_ID=$(curl -s -H "Authorization: Bearer $CLOUDFLARE_API_TOKEN" \
  "https://api.cloudflare.com/client/v4/zones/$ZONE_ID/dns_records?name=$APP_FQDN" \
  | jq -r '.result[0].id')
curl -X DELETE "https://api.cloudflare.com/client/v4/zones/$ZONE_ID/dns_records/$RECORD_ID" \
  -H "Authorization: Bearer $CLOUDFLARE_API_TOKEN"

fly apps destroy hello-cards
neon projects delete <project-id>
```

## 13. Repository, licensing & contributions

Repo is **public from day one** and stays public through production. The entire secret surface (Neon `DATABASE_URL`, `CLOUDFLARE_API_TOKEN`, Fly deploy token, future auth secrets) is injected out-of-band via `fly secrets` / env / GH repo secrets and never committed. No security-through-obscurity: the auth seam (§9) must be secure with source fully visible.

### Business model & license

End product is a **paid hosted service.** License keeps source open and PR-friendly while preventing competing commercial use.

|ID |Requirement|
|---|-----------|
|L1 |License: **FSL-1.1-Apache-2.0** (Functional Source License). Source-available, not OSI "open source." Permits all use *except* competing commercially with the licensor; each release **auto-converts to Apache-2.0 two years after publication.** Drop canonical text in `LICENSE.md` (from fsl.software — don't hand-edit). State model in `README`: *"Source-available under FSL-1.1-Apache-2.0; the hosted service is the commercial offering; contributions welcome."*|
|L2 |**Public repo ≠ open source.** FSL is source-available — say so plainly so readers don't assume MIT/Apache rights before the 2-year conversion. A missing `LICENSE` defaults to all-rights-reserved and silently blocks the contributions we want — L1 ships in the first commit.|

### Contributions

|ID |Requirement|
|---|-----------|
|C1 |Inbound contributions governed by **DCO** (Developer Certificate of Origin), not a CLA. `Signed-off-by` via `git commit -s`; no paperwork. `CONTRIBUTING.md` with DCO text (developercertificate.org) + sign-off instruction; enforce with a DCO check on PRs.|
|C2 |Licensor retains copyright on its own commits; DCO + FSL grant preserve the right to operate/commercialize. Revisit a CLA only if a future need to *relicense the whole project* emerges — out of scope.|

### Secret hygiene (keeps the repo safely public)

|ID |Requirement|
|---|-----------|
|S1 |**`.gitignore` in first commit:** `.env`, `.env.*` (allow an `.env.example` with dummies), local Postgres dumps, editor/OS cruft. Single habit that prevents leaking on commit one.|
|S2 |No real secrets in tracked files: `fly.app.toml` = app/region/timeouts (DB string in `fly secrets`, D2/F11); `compose.yml` dev Postgres uses dummy local creds (`postgres:postgres`); `ops/cloudflare/cache-rules.json` is routing-only (`ZONE_ID` fetched at runtime).|
|S3 |Enable **GitHub secret scanning + push protection**; add `gitleaks` (or `trufflehog`) pre-commit. Cheap insurance.|
