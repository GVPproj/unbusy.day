# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

A minimal full-stack reference app (module `github.com/grahamvanpelt/unbusy.day`, app name `hello-cards`) validating the production architecture for a Trello-like, multi-tenant product with live, optimistic UI over flaky networks. The stack is deliberately **Go-only**: business logic lives exactly once on the server, Postgres is the source of truth, SSE carries live reads, and the frontend is server-rendered **Datastar + templ** — there is no JS build step, no SPA, no client-side business logic.

## Commands

Day-to-day uses [go-task](https://taskfile.dev) (`task`):

- `task dev` — full dev loop: starts Postgres (compose), runs `templ generate --watch` with a reload proxy on **:7331** driving `go run .` on :8080. Browse **:7331**, not :8080. Do **not** run `air` alongside it (air rebuilds on every `.templ` edit and defeats templ's text-only fast path).
- `task test` — `go test ./...`. Run a single test: `go test ./cards -run TestReorder`. Note CI runs `go test -race ./...`.
- `task migrate` — applies `migrations/*.sql` in order against `$DATABASE_URL` via `psql` (idempotent; safe to re-run).
- `task templ` — one-shot `templ generate`. **Never run this while `task dev` is up** — non-watch generation deletes the watch session's literal cache and the running server 500s on every render until restarted.
- `task up` / `task down` / `task nuke` — Postgres lifecycle (`nuke` deletes the data volume).
- `task build` — `templ generate` + `go build -o tmp/hello-cards .`.

`templ generate` must run before `go vet`/`go build`/`go test` on a clean checkout: `*_templ.go` is git-ignored and generated (regenerated in dev, in the Docker build, and in CI).

## Architecture

Three layers, each a package, wired in `main.go`:

- **`cards/`** — the transport-agnostic core `Service` over a `pgxpool.Pool`. All mutation logic (reorder, resize) lives here exactly once; adapters never touch SQL. `Reorder` validates that the incoming order is a true permutation of current ids and commits via a single bulk `UPDATE … FROM (VALUES …)` under `SELECT … FOR UPDATE`. `Resize` persists a card's `span` (height in slots). Both fan a full-state `cards.Event` out to the broker **post-commit** so subscribers never see uncommitted state. The `Publisher` interface is the seam to pub/sub (nil Publisher = skip fan-out, used in unit tests).
- **`pubsub/`** — an in-process fan-out `Broker` (implements `cards.Publisher`). **Single-machine only** — non-blocking delivery drops slow consumers, who recover on EventSource reconnect. There is no event history/ring buffer; recovery is a full snapshot re-render on the read path. Cross-instance fan-out would require an external bus (LISTEN/NOTIFY or Redis) and is explicitly out of scope.
- **`ds/`** — the Datastar + templ frontend adapter (route handlers). `PageHandler` server-renders the authoritative column; `EventsHandler` is the live SSE read path (first frame is the full current column, so any (re)connect is made whole by one render); `ReorderHandler`/`ResizeHandler` are the mutation endpoints, each responding with an SSE element-patch of the committed column. templ components are in `ds/components/`. `RequireSession` middleware gates `/`, `/events`, and the mutation endpoints, stashing the owner id in the request context.
- **`auth/`** — passwordless email-OTP auth over DB-backed sessions (ADRs 0001/0002 in `docs/adr/`). The `Mailer` interface is the email seam; dev uses `LogMailer` (code lands on stdout — no email service needed locally). The allowlist is the `user` table; sessions are opaque tokens in an HttpOnly SameSite=Lax cookie, revoked by row delete. Cards are owner-scoped and the broker is user-keyed (ADR 0003); new users get starter cards seeded on first login.

Key invariant: **page render and patch render share one templ component** (`components.CardColumn`) so the initial load and every SSE patch can't drift. The `#card-list` element id is the stable idiomorph anchor every patch lands on; `data-id` attributes are the reorder identity keys.

### Error handling / rollback convention

Domain rejections (`cards.ErrNotPermutation`, `cards.ErrInvalidSpan`) are surfaced as **200 + a re-render of the authoritative column**, not a 4xx. The rejected optimistic change visibly snaps back because the server patches the truth back over it. (Datastar applying patches on error statuses is unverified, and there's no separate client rollback path.)

### Migrations are expand-then-deploy by construction

All migrations are additive + idempotent (`ADD COLUMN IF NOT EXISTS`, etc.). The running binary uses **explicit column lists** (`SELECT id, label, position, span`), never `SELECT *`, so a migration can land before the code that reads a new column, and a migration alone never changes the wire shape. Exposing a new field always needs a code change + deploy.

## Frontend gotchas (Datastar / templ)

- Datastar **keyed** attributes use a **colon** between plugin and key (`data-on:reorder`, `data-signals:order`). The dash form is silently skipped as a nonexistent plugin — no error. See `ds/components/column.templ`.
- The drag/resize interaction (`dragInit` in `column.templ`) is a Motion-powered script that transforms the real `<li>` under the pointer (not a ghost), then commits the DOM reorder in one synchronous FLIP frame before dispatching the `reorder` event. Card `span`/slot state is persisted and re-rendered into every morph (`data-span`, `--span`, `.stretched`, `.consumed`) so server render and client gesture agree and morphs stay idempotent.
- `/_smoke` + `/_smoke/events` are a wiring canary for the pinned Datastar SDK + templ versions — keep them working when bumping those deps.

## Code style

- **Keep comments succinct — 1 or 2 lines.** (Some existing comments run longer than this; new code should not.)
- **Prefer the Go standard library.** Don't add a dependency until it's demonstrably necessary; the current deps (templ, pgx, datastar-go) are the deliberate minimum.
- **Follow Datastar 1.x best practices and consult their docs** (https://data-star.dev) rather than guessing at attribute/SDK behavior.

## Conventions & deploy

- **Sign off every commit**: `git commit -s` (DCO; CI enforces it, git author must match the sign-off). Open an issue before non-trivial work.
- The templ CLI version is pinned identically in three places — `go.mod`, the `Dockerfile`, and `.github/workflows/ci.yml` (`v0.3.1020`). Bump all together.
- Local Postgres runs on host port **5433** (compose). `DATABASE_URL` lives in `.env` (git-ignored; copy `.env.example`).
- Deploy is via Fly using **`fly.app.toml`** (not the default `fly.toml`): `flyctl deploy --config fly.app.toml`. CI auto-deploys on push to `main`. Production Postgres is Neon. The Fly app runs **exactly one always-on machine** — in-process pub/sub is single-machine by design; never scale to >1 machine or enable auto-stop.
- `/healthz` is an in-process 200 only — never query Postgres there (Fly's frequent health check would defeat Neon's scale-to-zero).
