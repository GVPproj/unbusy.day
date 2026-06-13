# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

A minimal full-stack Go app (module `github.com/GVPproj/unbusy.day`, app name `hello-cards`) — the production architecture for a Trello-like, multi-tenant product with live, optimistic UI over flaky networks. The stack is deliberately **Go-only**: business logic lives exactly once on the server, Postgres is the source of truth, SSE carries live reads, and the frontend is server-rendered **Datastar + templ** — there is no JS build step, no SPA, no client-side business logic.

## Commands

Day-to-day uses [go-task](https://taskfile.dev) (`task`):

- `task dev` — full dev loop: starts Postgres (compose), applies pending migrations, runs `templ generate --watch` with a reload proxy on **:7331** driving `go run ./cmd/unbusy` on `$PORT` (default :8080; set `PORT` in `.env`). Browse **:7331**, not the app port. Do **not** run `air` alongside it (air rebuilds on every `.templ` edit and defeats templ's text-only fast path).
- `task test` — `go test ./...`. Run a single test: `go test ./internal/block -run TestSetLayout`. Note CI runs `go test -race ./...`.
- `task migrate` — applies pending migrations against `$DATABASE_URL` via the binary's `migrate` subcommand (`go run ./cmd/unbusy migrate`; goose, run-once, safe to re-run).
- `task templ` — one-shot `templ generate`. **Never run this while `task dev` is up** — non-watch generation deletes the watch session's literal cache and the running server 500s on every render until restarted.
- `task up` / `task down` / `task nuke` — Postgres lifecycle (`nuke` deletes the data volume).
- `task build` — `templ generate` + `go build -o tmp/hello-cards ./cmd/unbusy`.

`templ generate` must run before `go vet`/`go build`/`go test` on a clean checkout: `*_templ.go` is git-ignored and generated (regenerated in dev, in the Docker build, and in CI).

## Architecture

The entrypoint is `cmd/unbusy/main.go`; all application packages live under `internal/` (import path `github.com/GVPproj/unbusy.day/internal/<pkg>`). Package paths named below (`block/`, `frontend/…`) are relative to `internal/`. Three layers, each a package, wired in `cmd/unbusy/main.go`:

- **`block/`** — the transport-agnostic core `Service` over a `pgxpool.Pool`. All mutation logic lives here exactly once; adapters never touch SQL. `SetLayout` takes the whole client-computed layout, validates the invariants via `ValidateLayout` (same block set, in bounds, no overlaps, span ≥ 1) and commits via a single bulk `UPDATE … FROM (VALUES …)` under `SELECT … FOR UPDATE` (ADR 0005). `SetBounds` edits the owner's day extent, rejecting a shrink onto an occupied slot. Both fan a full-state `block.Event` out to the broker **post-commit** so subscribers never see uncommitted state. The `Publisher` interface is the seam to pub/sub (nil Publisher = skip fan-out, used in unit tests).
- **`pubsub/`** — an in-process fan-out `Broker` (implements `block.Publisher`). **Single-machine only** — non-blocking delivery drops slow consumers, who recover on EventSource reconnect. There is no event history/ring buffer; recovery is a full snapshot re-render on the read path. Cross-instance fan-out would require an external bus (LISTEN/NOTIFY or Redis) and is explicitly out of scope.
- **`frontend/`** — the Datastar + templ frontend adapter (route handlers). `PageHandler` server-renders the authoritative column; `EventsHandler` is the live SSE read path (first frame is the full current column, so any (re)connect is made whole by one render); `LayoutHandler` (`POST /blocks/layout`) and `BoundsHandler` (`POST /blocks/bounds`) are the mutation endpoints, each responding with an SSE element-patch of the committed column. templ views are split into `frontend/layouts/` (document shell; routes pass page styles in), `frontend/routes/` (page-level views), and `frontend/components/` (fragments/patch targets); component styles are colocated in their `.templ` files. `frontend/static/` is a `go:embed`-served folder (mounted at `/static/`) holding the few real JS files (currently `drag.js`). `RequireSession` middleware gates `/`, `/events`, and the mutation endpoints, stashing the owner id in the request context.
- **`auth/`** — passwordless email-OTP auth over DB-backed sessions (ADRs 0001/0002 in `docs/adr/`). The `Mailer` interface is the email seam; dev uses `LogMailer` (code lands on stdout — no email service needed locally). The allowlist is the `user` table; sessions are opaque tokens in an HttpOnly SameSite=Lax cookie, revoked by row delete. Blocks are owner-scoped and the broker is user-keyed (ADR 0003); new users get starter blocks seeded on first login.

Key invariant: **page render and patch render share one templ component** (`components.BlockColumn`) so the initial load and every SSE patch can't drift. The `#block-list` element id is the stable idiomorph anchor every patch lands on; `data-id` attributes are the per-block identity keys.

### Error handling / rollback convention

Domain rejections (`block.ErrNotSameBlocks`, `block.ErrOutOfBounds`, `block.ErrOverlap`, `block.ErrInvalidSpan` from layout; `block.ErrInvalidBounds`, `block.ErrBoundsOccupied` from bounds) are surfaced as **200 + a re-render of the authoritative column**, not a 4xx. The rejected optimistic change visibly snaps back because the server patches the truth back over it. (Datastar applying patches on error statuses is unverified, and there's no separate client rollback path.)

### Migrations are run-once via goose (ADR 0004)

Migrations are plain `.sql` files in `internal/migrate/migrations/`, embedded via `go:embed` by the `internal/migrate` package (which exposes `migrate.Run`; `main` calls it on the `migrate` subcommand), and applied exactly once per database by goose (`github.com/pressly/goose/v3` as a library; versions recorded in `goose_db_version`). The Fly release command and `task migrate` both run the binary's `migrate` subcommand.

- **New migrations are plain DDL** — no `IF NOT EXISTS` / `DO $$ EXCEPTION` idempotency scaffolding. Each file needs a `-- +goose Up` header; wrap multi-statement `DO $$ … $$` blocks in `-- +goose StatementBegin` / `-- +goose StatementEnd`.
- **Timestamp-versioned filenames** for new migrations (e.g. via `goose create <name> sql`) so concurrent branches can't collide. The legacy `0001`–`0004` names stay as-is; their idempotent bodies are what lets goose baseline pre-goose databases automatically.
- **Forward-only**: no Down sections, no rollback tooling. Fix mistakes with a new forward migration; reset a broken local schema with `task nuke` + re-migrate. An applied file is history — editing it does nothing.
- **Expand-then-deploy is preserved by discipline, not re-runnability**: keep DDL additive and keep the binary on explicit column lists (`SELECT id, label, position, span`), never `SELECT *`, so a migration can land before the code that reads a new column and never changes the wire shape by itself.

## Frontend gotchas (Datastar / templ)

- Datastar **keyed** attributes use a **colon** between plugin and key (`data-on:layout`, `data-signals:layout`). The dash form is silently skipped as a nonexistent plugin — no error. See `frontend/components/column.templ`.
- The drag/stretch interaction lives in `frontend/static/drag.js` (loaded as a module by the `DragInit` component in `column.templ`): a Motion-powered script (Motion pinned via CDN import) that transforms the real `<li>` under the pointer (not a ghost), commits the DOM placement in one synchronous FLIP frame, then dispatches the `layout` event carrying the full client-computed layout (ADR 0005). Listeners are delegated to `#block-list` so wiring survives morphs. Block `span`/slot state is persisted and re-rendered into every morph (`data-span`, `--span`, `.stretched`, `.consumed`) so server render and client gesture agree and morphs stay idempotent.
- `/_smoke` + `/_smoke/events` are a wiring canary for the pinned Datastar SDK + templ versions — keep them working when bumping those deps.

## Theming & HTML practices

- **Prefer native HTML over JS/signal plumbing.** The theme picker (`frontend/components/theme.templ`) is the exemplar: a native `<dialog>` opened declaratively via `commandfor="theme-modal" command="show-modal"` buttons — no open/close signal, `closedby="any"` for light dismiss, Esc free from the platform. New UI should reach for native elements/invoker commands first and add Datastar signals only for state the server cares about.
- Theming is CSS custom properties scoped by `body[data-theme="…"]` in `frontend/layouts/layout.templ` (Solarized Light, Solarized Dark Osaka, Catppuccin Mocha). Picking a theme writes the `$_theme` signal, which the body mirrors into `data-theme` and persists to localStorage — the swap is live. New styles must use the existing tokens (`--bg`, `--ink`, `--surface`, `--accent`, …), never hardcoded colors.
- Navigation (`frontend/components/nav.templ`) is a desktop icon rail that becomes a mobile hamburger + off-canvas drawer (`$_navopen` signal) at ≤768px.

## Code style

- **Keep comments succinct — 1 or 2 lines.** (Some existing comments run longer than this; new code should not.)
- **Prefer the Go standard library.** Don't add a dependency until it's demonstrably necessary; the current deps (templ, pgx, datastar-go) are the deliberate minimum.
- **Always check the docs before writing Datastar or templ code** — Datastar 1.x (https://data-star.dev) and templ (https://templ.guide) — rather than guessing at attribute/SDK/template behavior. Both libraries are young and their APIs shift; verified docs beat training-data memory.
- **Use modern, semantic HTML first** — native `<dialog>`, invoker commands (`commandfor`/`command`), real buttons/forms — before adding custom JS or signal state. See the theme modal for the house style.

## Conventions & deploy

- **Sign off every commit**: `git commit -s` (DCO; CI enforces it, git author must match the sign-off). Open an issue before non-trivial work.
- The templ CLI version is pinned identically in three places — `go.mod`, the `Dockerfile`, and `.github/workflows/ci.yml` (`v0.3.1020`). Bump all together.
- Local Postgres runs on host port **5433** (compose). `DATABASE_URL` lives in `.env` (git-ignored; copy `.env.example`).
- Deploy is via Fly using **`fly.app.toml`** (not the default `fly.toml`): `flyctl deploy --config fly.app.toml`. CI auto-deploys on push to `main`. Production Postgres is Neon. The Fly app runs **exactly one always-on machine** — in-process pub/sub is single-machine by design; never scale to >1 machine or enable auto-stop.
- `/healthz` is an in-process 200 only — never query Postgres there (Fly's frequent health check would defeat Neon's scale-to-zero).
