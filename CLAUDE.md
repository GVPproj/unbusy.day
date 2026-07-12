# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

unbusy.day — a Cal Newport-style time-blocking day planner (Fly app name `hello-cards`, module `github.com/GVPproj/unbusy.day`). The stack is deliberately **Go-only**: business logic lives exactly once on the server, a colocated SQLite file is the source of truth (ADR 0007), SSE carries live reads, and the frontend is server-rendered **Datastar + templ** — no SPA, no client-side business logic, no Node. There is no CSS build step: styling is one hand-authored `app.css` (ADR 0011); templ generation is the only codegen. `CONTEXT.md` is the domain glossary; `docs/adr/` holds the decisions.

## Commands

Day-to-day uses [go-task](https://taskfile.dev) (`task`):

- `task dev` — full dev loop over a local SQLite file: templ watch with a reload proxy on **:7331** driving `go run ./cmd/unbusy` on `$PORT` (default :8080). Static assets (including `app.css`) are served from disk in dev, so CSS edits land on reload. Browse **:7331**, not the app port. Do **not** run `air` alongside it.
- `task test` — `go test ./...` plus `node --test internal/frontend/jstest/*.test.js`. Single Go test: `go test ./internal/block -run TestSetLayout`. CI runs `go test -race`.
- `task templ` — one-shot `templ generate`. **Never run this while `task dev` is up** — non-watch generation deletes the watch session's literal cache and the running server 500s until restarted.
- `task build` — templ + `go build -o tmp/unbusy ./cmd/unbusy`.
- `task migrate` / `task nuke` — apply migrations ad hoc / delete the local SQLite file (it re-migrates on next boot).

There is no database container: the SQLite file lives under `tmp/` (git-ignored) and the app migrates on boot. On a clean checkout, `templ generate` must run before `go vet`/`go build`/`go test` — `*_templ.go` is git-ignored and generated.

## Architecture

Entrypoint `cmd/unbusy/main.go`; packages under `internal/`, wired in main:

- **`block/`** — the transport-agnostic core `Service` over `*sql.DB`. All mutation logic lives here once; adapters never touch SQL. Mutations (`SetLayout`, `SetBounds`, `Create`, `Delete`, `Clear`, `Rename`) run in a write transaction and fan a full-state `block.Event` to the broker **post-commit**. `SetLayout` takes the whole client-computed layout and validates invariants via `ValidateLayout` (same block set, in bounds, no overlaps, span ≥ 1) — the sole overlap guard; SQLite's `_txlock=immediate` serializes writers (ADRs 0005/0007). The `Publisher` interface is the pub/sub seam (nil = skip fan-out in unit tests).
- **`pubsub/`** — in-process fan-out `Broker`, user-keyed (ADR 0003). **Single-machine only**; non-blocking delivery drops slow consumers, who recover via a full snapshot re-render on EventSource reconnect. No event history.
- **`frontend/`** — Datastar + templ route handlers. `PageHandler` server-renders the column; `EventsHandler` is the SSE read path (first frame is the full column, so any (re)connect is made whole); the `POST /blocks*` endpoints each respond with an SSE element-patch of the committed column. Views split into `layouts/` (document shell), `routes/` (pages), `components/` (fragments/patch targets). `static/` is `go:embed`-served at `/static/` (app.css, drag.js, push.js, keys.js, now-pill.js, sw.js). `RequireSession` middleware gates the app routes.
- **`auth/`** — passwordless email-OTP over DB-backed sessions (ADRs 0001/0002). `Mailer` is the email seam; dev uses `LogMailer` (codes on stdout). Signup is open — a correct code mints the account on first verify; the blast-radius bound is layered on the send path: per-email throttle, suppression list (ADR 0009), MX check, Turnstile presence check, per-IP/global rate limit, and a global send ceiling. `guardOpenSignup` in `main.go` refuses to boot a live mailer without Turnstile + ceiling unless `OPEN_SIGNUP_INSECURE=1`. Sessions are opaque tokens in an HttpOnly SameSite=Lax cookie; new users get starter blocks seeded on first login.

Key invariant: **page render and patch render share one templ component** (`components.BlockColumn`) so initial load and SSE patches can't drift. `#block-list` is the stable idiomorph anchor; `data-id` attributes are the per-block identity keys.

**Error convention:** domain rejections (overlap, out-of-bounds, occupied shrink, blank label, unknown id, …) are surfaced as **200 + a re-render of the authoritative column**, not a 4xx — the rejected optimistic change visibly snaps back.

**Migrations** (ADR 0004): plain forward-only `.sql` files in `internal/migrate/migrations/`, `go:embed`-ed, applied run-once by goose on boot. New migrations are plain DDL with a `-- +goose Up` header and timestamp-versioned filenames; no Down sections — fix mistakes with a new forward migration. Keep DDL additive and queries on explicit column lists (never `SELECT *`) so a migration can land before the code that reads it.

## Frontend gotchas (Datastar / templ)

- Datastar **keyed** attributes use a **colon** between plugin and key (`data-on:layout`, `data-signals:layout`). The dash form is silently skipped — no error.
- Drag/stretch lives in `frontend/static/drag.js` (Motion, pinned via CDN import): it transforms the real `<li>`, previews the client-computed push cascade (`push.js`, ADR 0005), commits as a FLIP, then dispatches one `layout` event carrying the full layout. Listeners are delegated to `#block-list` so wiring survives morphs; block state is persisted in `data-span`/`data-slot` attributes so server render and client gesture agree.
- `/_smoke` + `/_smoke/events` are a wiring canary for the pinned Datastar SDK + templ versions — keep them working when bumping those deps.

## Theming & HTML practices

- **Prefer native HTML over JS/signal plumbing**: native `<dialog>` opened via invoker commands (`commandfor`/`command`), `closedby="any"` for light dismiss. The theme modal is the exemplar. Add Datastar signals only for state the server cares about.
- Theming is two axes of CSS custom properties on `:root` (tokens in `app.css`, attributes toggled in `frontend/layouts/layout.templ`): **colorscheme** (`data-colorscheme`) sets color tokens, **feeling** (`data-feeling`) sets font/icon set. Both persist to localStorage with a pre-paint script to avoid FOUC. New styles must use the tokens (`--bg`, `--ink`, `--surface`, `--accent`, …), never hardcoded colors.
- **Plain CSS in one hand-authored stylesheet** (ADR 0011): `frontend/static/app.css`, committed and `go:embed`-served as the single cached `<link>` — SSE patches never re-ship CSS, and CSS never renders inside a patched fragment. Cascade layers own ordering (`reset, tokens, base, layout, components`); the `components` layer is a small shared class tier (`.btn`, `.field`, `.option-row`, …) plus one `@scope` block per leaf component (`.login-main`, `.app-dialog`, `.sidenav`, `.blocks`), bare element selectors inside, section comments naming the templ file each styles. Markup carries semantic hooks only — the JS hook classes (`.block-item`, `.open`, `.dragging`, …) double as styling anchors; conditionals use `templ.KV`; no templ `css` blocks. Grow the shared tier deliberately, not by default. Media queries: `@media (width < 40rem)` is the mobile breakpoint, `@media (pointer: coarse)` the touch variants.

## Code style

- **Keep comments succinct — 1 or 2 lines** — and only where they state something the code can't (a constraint, a browser quirk, a why). Prefer self-explanatory code over comments.
- **Prefer the Go standard library.** Don't add a dependency until demonstrably necessary; the current deps (templ, modernc.org/sqlite, goose, datastar-go) are the deliberate minimum.
- **Always check the docs before writing Datastar or templ code** — Datastar 1.x (https://data-star.dev) and templ (https://templ.guide). Both APIs shift; verified docs beat training-data memory.
- **templ naming:** one lowercase file per feature (`column.templ`), parts in prefix-grouped siblings (`column_block.templ`), a folder only for a real boundary with a small exported API (`modals/`). Exported components PascalCase; helpers camelCase.

## Conventions & deploy

- **Sign off every commit**: `git commit -s` (DCO; CI enforces it). Open an issue before non-trivial work.
- **Tool versions have one source each**: templ in `go.mod` (CLI installed via `go list -m`), Datastar client bundle and Motion as exact CDN tags in the templ shells / `drag.js`. `task check:versions` flags drift for all of them.
- `DATABASE_URL` is a SQLite DSN in `.env` (copy `.env.example`) — `file:./tmp/unbusy.db` plus pragmas (WAL, `busy_timeout`, `foreign_keys`, `_txlock=immediate`).
- Deploy is Fly via **`fly.app.toml`**: `flyctl deploy --config fly.app.toml`; CI auto-deploys on push to `main`. Production data is a SQLite file on a Fly volume backed up by scheduled volume snapshots (streaming backup deferred, docs/backlog/002). The app runs **exactly one always-on machine** — never scale to >1 or enable auto-stop (in-process pub/sub, single-writer SQLite).
- `/healthz` is an in-process 200 only — a liveness probe, never a DB readiness check.

## Agent skills

### Issue tracker

Issues live in Linear, team **Unbusy**. See `docs/agents/issue-tracker.md`.

### Domain docs

Single-context: `CONTEXT.md` + `docs/adr/` at the repo root. See `docs/agents/domain.md`.
