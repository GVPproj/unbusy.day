# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

A minimal full-stack Go app (module `github.com/GVPproj/unbusy.day`, app name `hello-cards`) — the production architecture for a Trello-like, multi-tenant product with live, optimistic UI over flaky networks. The stack is deliberately **Go-only**: business logic lives exactly once on the server, a colocated SQLite file (ADR 0007; durability via Fly volume snapshots — streaming backup deferred, docs/backlog/002) is the source of truth, SSE carries live reads, and the frontend is server-rendered **Datastar + templ** — no SPA, no client-side business logic, no JS/Node build step. (The one build step is CSS: a Tailwind v4 standalone binary compiles utilities — no Node, see ADR 0008.)

## Commands

Day-to-day uses [go-task](https://taskfile.dev) (`task`):

- `task dev` — full dev loop over a local SQLite file (no Docker): the app migrates on boot, so there's no separate migrate step. Runs `templ generate --watch` with a reload proxy on **:7331** driving `go run ./cmd/unbusy` on `$PORT` (default :8080; set `PORT` in `.env`). Browse **:7331**, not the app port. Do **not** run `air` alongside it (air rebuilds on every `.templ` edit and defeats templ's text-only fast path).
- `task test` — `go test ./...`. Run a single test: `go test ./internal/block -run TestSetLayout`. Note CI runs `go test -race ./...`.
- `task migrate` — applies pending migrations against `$DATABASE_URL` via the binary's `migrate` subcommand (`go run ./cmd/unbusy migrate`; goose, run-once, safe to re-run).
- `task templ` — one-shot `templ generate`. **Never run this while `task dev` is up** — non-watch generation deletes the watch session's literal cache and the running server 500s on every render until restarted.
- `task build` — `templ generate` + `go build -o tmp/hello-cards ./cmd/unbusy`.

There is no database container to manage: the SQLite file lives under `tmp/` (git-ignored), is created and migrated on boot, and is reset with `task nuke`. `templ generate` must run before `go vet`/`go build`/`go test` on a clean checkout: `*_templ.go` is git-ignored and generated (regenerated in dev, in the Docker build, and in CI).

## Architecture

The entrypoint is `cmd/unbusy/main.go`; all application packages live under `internal/` (import path `github.com/GVPproj/unbusy.day/internal/<pkg>`). Package paths named below (`block/`, `frontend/…`) are relative to `internal/`. Three layers, each a package, wired in `cmd/unbusy/main.go`:

- **`block/`** — the transport-agnostic core `Service` over a `*sql.DB` (SQLite). All mutation logic lives here exactly once; adapters never touch SQL. `SetLayout` takes the whole client-computed layout, validates the invariants via `ValidateLayout` (same block set, in bounds, no overlaps, span ≥ 1) and commits via a single bulk `UPDATE … FROM (VALUES …)` CTE inside a write transaction (ADR 0005). SQLite serializes writes at the database level (immediate write lock), so there's no row-level `FOR UPDATE`. `ValidateLayout` is the sole overlap guard now that the Postgres `btree_gist` exclusion constraint is gone (ADR 0007). `SetBounds` edits the owner's day extent, rejecting a shrink onto an occupied slot. Both fan a full-state `block.Event` out to the broker **post-commit** so subscribers never see uncommitted state. The `Publisher` interface is the seam to pub/sub (nil Publisher = skip fan-out, used in unit tests).
- **`pubsub/`** — an in-process fan-out `Broker` (implements `block.Publisher`). **Single-machine only** — non-blocking delivery drops slow consumers, who recover on EventSource reconnect. There is no event history/ring buffer; recovery is a full snapshot re-render on the read path. Cross-instance fan-out would require an external bus (LISTEN/NOTIFY or Redis) and is explicitly out of scope.
- **`frontend/`** — the Datastar + templ frontend adapter (route handlers). `PageHandler` server-renders the authoritative column; `EventsHandler` is the live SSE read path (first frame is the full current column, so any (re)connect is made whole by one render); `LayoutHandler` (`POST /blocks/layout`) and `BoundsHandler` (`POST /blocks/bounds`) are the mutation endpoints, each responding with an SSE element-patch of the committed column. templ views are split into `frontend/layouts/` (document shell; routes pass page styles in), `frontend/routes/` (page-level views), and `frontend/components/` (fragments/patch targets); component styles are colocated in their `.templ` files. `frontend/static/` is a `go:embed`-served folder (mounted at `/static/`) holding the few real JS files (currently `drag.js`). `RequireSession` middleware gates `/`, `/events`, and the mutation endpoints, stashing the owner id in the request context.
- **`auth/`** — passwordless email-OTP auth over DB-backed sessions (ADRs 0001/0002 in `docs/adr/`). The `Mailer` interface is the email seam; dev uses `LogMailer` (code lands on stdout — no email service needed locally). The allowlist is the `user` table; sessions are opaque tokens in an HttpOnly SameSite=Lax cookie, revoked by row delete. Blocks are owner-scoped and the broker is user-keyed (ADR 0003); new users get starter blocks seeded on first login.

Key invariant: **page render and patch render share one templ component** (`components.BlockColumn`) so the initial load and every SSE patch can't drift. The `#block-list` element id is the stable idiomorph anchor every patch lands on; `data-id` attributes are the per-block identity keys.

### Error handling / rollback convention

Domain rejections (`block.ErrNotSameBlocks`, `block.ErrOutOfBounds`, `block.ErrOverlap`, `block.ErrInvalidSpan` from layout; `block.ErrInvalidBounds`, `block.ErrBoundsOccupied` from bounds) are surfaced as **200 + a re-render of the authoritative column**, not a 4xx. The rejected optimistic change visibly snaps back because the server patches the truth back over it. (Datastar applying patches on error statuses is unverified, and there's no separate client rollback path.)

### Migrations are run-once via goose (ADR 0004)

Migrations are plain `.sql` files in `internal/migrate/migrations/`, embedded via `go:embed` by the `internal/migrate` package (which exposes `migrate.Run`; `main` calls it on the `migrate` subcommand), and applied exactly once per database by goose (`github.com/pressly/goose/v3` as a library, `DialectSQLite3`; versions recorded in `goose_db_version`). Migrations run on boot in the app process (`main` calls `migrate.Run` at startup) since the app machine is the one that mounts the volume; `task migrate` also runs the binary's `migrate` subcommand for ad-hoc local use.

- **New migrations are plain DDL** — no `IF NOT EXISTS` / `DO $$ EXCEPTION` idempotency scaffolding. Each file needs a `-- +goose Up` header; wrap multi-statement `DO $$ … $$` blocks in `-- +goose StatementBegin` / `-- +goose StatementEnd`.
- **Timestamp-versioned filenames** for new migrations (e.g. via `goose create <name> sql`) so concurrent branches can't collide. The legacy `0001`–`0004` names stay as-is; their idempotent bodies are what lets goose baseline pre-goose databases automatically.
- **Forward-only**: no Down sections, no rollback tooling. Fix mistakes with a new forward migration; reset a broken local schema with `task nuke` (deletes the `tmp/` `.db` file and WAL sidecars; re-migrates on next run). An applied file is history — editing it does nothing.
- **Expand-then-deploy is preserved by discipline, not re-runnability**: keep DDL additive and keep the binary on explicit column lists (`SELECT id, label, position, span`), never `SELECT *`, so a migration can land before the code that reads a new column and never changes the wire shape by itself.

## Frontend gotchas (Datastar / templ)

- Datastar **keyed** attributes use a **colon** between plugin and key (`data-on:layout`, `data-signals:layout`). The dash form is silently skipped as a nonexistent plugin — no error. See `frontend/components/column.templ`.
- The drag/stretch interaction lives in `frontend/static/drag.js` (loaded as a module by the `DragInit` component in `column.templ`): a Motion-powered script (Motion pinned via CDN import) that transforms the real `<li>` under the pointer (not a ghost), commits the DOM placement in one synchronous FLIP frame, then dispatches the `layout` event carrying the full client-computed layout (ADR 0005). Listeners are delegated to `#block-list` so wiring survives morphs. Block `span`/slot state is persisted and re-rendered into every morph (`data-span`, `--span`, `.stretched`, `.consumed`) so server render and client gesture agree and morphs stay idempotent.
- `/_smoke` + `/_smoke/events` are a wiring canary for the pinned Datastar SDK + templ versions — keep them working when bumping those deps.

## Theming & HTML practices

- **Prefer native HTML over JS/signal plumbing.** The theme picker (`frontend/components/modals/theme.templ`) is the exemplar: a native `<dialog>` opened declaratively via `commandfor="theme-modal" command="show-modal"` buttons — no open/close signal, `closedby="any"` for light dismiss, Esc free from the platform. New UI should reach for native elements/invoker commands first and add Datastar signals only for state the server cares about.
- Theming is CSS custom properties scoped by `body[data-theme="…"]` in `frontend/layouts/layout.templ` (Solarized Light, Solarized Dark Osaka, Catppuccin Mocha). Picking a theme writes the `$_theme` signal, which the body mirrors into `data-theme` and persists to localStorage — the swap is live. New styles must use the existing tokens (`--bg`, `--ink`, `--surface`, `--accent`, …), never hardcoded colors.
- Navigation (`frontend/components/nav.templ`) is a desktop icon rail that becomes a mobile hamburger + off-canvas drawer (`$_navopen` signal) at ≤768px.
- **Utility-first CSS via Tailwind v4** (ADR 0008 / PRD-tailwind-migration.md). Style one-off elements with inline utilities (`bg-accent`, `py-[0.6rem]`, `text-ink-muted`) — don't invent single-use class names. The tokens above are bridged into Tailwind via `@theme inline` in `frontend/input.css`, so `bg-surface` etc. resolve to `var(--surface)` and the live `data-colorscheme`/`data-feeling` swap keeps working. The CSS scan is restricted to `**/*.templ` (`source(none)` + `@source`), never `.go`/`*_templ.go`. Generated `output.css` is git-ignored, `go:embed`-served as one cached `<link>` (so SSE patches never re-ship CSS), and regenerated by the pinned standalone binary in dev/CI/Docker. **Structural/stateful CSS stays as co-located `@scope` blocks** (ADR 0006) — day-grid geometry, `data-slot` rules, `.grip`/`.dragging`/`.resizing` hooks `drag.js` reads, and stateful overlays like login's `.otp` comb. `login.templ` is the migrated exemplar; `column.templ` stays `@scope`-heavy. `task dev` runs `tailwindcss --watch` alongside templ.

## Code style

- **Keep comments succinct — 1 or 2 lines.** (Some existing comments run longer than this; new code should not.)
- **Prefer the Go standard library.** Don't add a dependency until it's demonstrably necessary; the current deps (templ, modernc.org/sqlite, goose, datastar-go) are the deliberate minimum.
- **Always check the docs before writing Datastar or templ code** — Datastar 1.x (https://data-star.dev) and templ (https://templ.guide) — rather than guessing at attribute/SDK/template behavior. Both libraries are young and their APIs shift; verified docs beat training-data memory.
- **Use modern, semantic HTML first** — native `<dialog>`, invoker commands (`commandfor`/`command`), real buttons/forms — before adding custom JS or signal state. See the theme modal for the house style.
- **templ file/symbol naming.** One file per feature, lowercase, named after the feature (`column.templ`, `nav.templ`); the base file holds the primary exported component. Parts of that feature go in prefix-grouped siblings (`column_block.templ`) so they sort together — same package, no exporting. A folder (subpackage) is only for a self-contained unit with a small exported API (`modals/`); reserve it for a real boundary, not for grouping. Exported components are PascalCase and form the package's public surface; parts/helpers stay camelCase (unexported). Each feature owns a `<Feature>Styles` component for its scoped CSS, co-located in the base file until the file grows unwieldy.

## Conventions & deploy

- **Sign off every commit**: `git commit -s` (DCO; CI enforces it, git author must match the sign-off). Open an issue before non-trivial work.
- The templ CLI version is pinned identically in three places — `go.mod`, the `Dockerfile`, and `.github/workflows/ci.yml` (`v0.3.1020`). Bump all together.
- The Tailwind standalone binary is pinned the same way in three places — the `Taskfile.yml` (`TAILWIND_VERSION`, in place of go.mod since it's a non-Go binary), the `Dockerfile`, and `.github/workflows/ci.yml` (`v4.3.1`). Bump all together.
- The database is a local SQLite file. `DATABASE_URL` is a SQLite DSN in `.env` (git-ignored; copy `.env.example`) — `file:./tmp/unbusy.db` plus pragmas (WAL, `busy_timeout`, `foreign_keys`, `_txlock=immediate`).
- Deploy is via Fly using **`fly.app.toml`** (not the default `fly.toml`): `flyctl deploy --config fly.app.toml`. CI auto-deploys on push to `main`. Production data is a SQLite file on a Fly volume (ADR 0007), backed up by Fly's scheduled volume snapshots; continuous streaming backup (Litestream) is deferred (docs/backlog/002). The Fly app runs **exactly one always-on machine** — in-process pub/sub and the single-writer SQLite file are single-machine by design; never scale to >1 machine or enable auto-stop.
- `/healthz` is an in-process 200 only — a pure liveness probe, never a DB readiness check.
