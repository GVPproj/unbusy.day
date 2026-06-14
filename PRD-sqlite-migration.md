# PRD: Migrate from Neon Postgres to colocated SQLite

Status: Draft · Owner: Graham Van Pelt · Date: 2026-06-13

> **Scope note (2026-06-14):** Litestream continuous streaming backup is **out of
> scope / deferred to backlog** (`docs/backlog/002-litestream-streaming-backup.md`).
> The shipping design is **SQLite on a Fly volume**, with durability provided by
> Fly's scheduled volume snapshots. All Litestream references below are retained
> as historical/future-feature context, not committed work for this migration.

## Problem Statement

As the operator of unbusy.day, I run the app on a single always-on Fly machine
backed by a managed Neon Postgres in another region. That arrangement costs me
more than the workload warrants, adds a network boundary I'm already coding
around (the `/healthz` no-DB hack exists purely to preserve Neon's
scale-to-zero), and makes the app hard to hand to a self-hoster — asking
someone to stand up Postgres (or sign up for Neon) is a real adoption barrier.
I want lower long-term cost, simpler deploys, and a plausible self-hosting story
later.

## Solution

Replace the external Postgres with a SQLite database file colocated with the app
on a Fly volume. Durability is provided by Fly's scheduled volume snapshots.
(Continuous streaming backup to S3-compatible object storage via Litestream was
the original aspiration but is **deferred to backlog** — see the scope note above.)
The app keeps its single-machine, in-process pub/sub architecture (which it
already commits to by design, ADR 0003) — SQLite matches that constraint rather
than fighting it. Reads become in-process library calls (no network hop, no
cold-start), the managed-DB bill goes away, and self-hosting becomes "run one Go
binary that keeps its own `.db` file."

This is a database-layer swap, not an architectural rewrite. The
transport-agnostic core service and the pure layout-validation logic barely
move; the work clusters in three predictable places: auth's timestamp/interval
math, the migration DDL, and the deploy/Litestream plumbing.

## User Stories

1. As the operator, I want the app to read and write its data from a local
   SQLite file, so that queries don't pay a cross-region network round trip.
2. As the operator, I want the database to live on a persistent Fly volume, so
   that data survives machine restarts and redeploys.
3. *(Deferred — backlog)* As the operator, I want continuous backup of the
   SQLite file to object storage, so that I lose at most a second or two of
   writes if the machine dies. **For now durability is Fly's scheduled volume
   snapshots.**
4. *(Deferred — backlog)* As the operator, I want the app to restore the
   database from object storage on boot when the volume is empty, so that
   recovery is automatic.
5. As the operator, I want no managed-database bill, so that long-term cost
   approaches the cost of a few cents of object storage per month.
6. As the operator, I want the runtime image to stay a pure-Go `scratch` build,
   so that deploys stay small and the build stays cgo-free.
7. As the operator, I want migrations to run on boot in the machine that mounts
   the volume, so that schema lands against the real database file.
8. As a self-hoster, I want to run the app as a single binary pointed at a local
   file and an S3-compatible bucket, so that I don't have to operate Postgres.
9. As a developer, I want local dev to use a SQLite file instead of a Postgres
   container, so that `task dev` no longer depends on Docker Compose.
10. As a developer, I want the test suite to run against an ephemeral SQLite
    database, so that tests need no external container and run faster under
    `-race`.
11. As a user of the day plan, I want my blocks, bounds, and layout mutations to
    behave exactly as before, so that the database swap is invisible to me.
12. As a user, I want layout invariants (same block set, in bounds, no overlap,
    span ≥ 1) still enforced on every mutation, so that an invalid drag still
    snaps back to the authoritative column.
13. As a user, I want passwordless email-OTP login, single-use codes, attempt
    limits, request throttling, and absolute session expiry to keep working
    identically, so that authentication is unaffected by the storage change.
14. As a user, I want live SSE reads and optimistic patches to behave as before,
    so that the multi-tab live experience is unchanged.
15. As the operator, I want the deprecated `/healthz` no-DB rationale and Neon
    scale-to-zero comments cleaned up, so that the codebase stops referencing an
    external DB that no longer exists.
16. As the operator, I want a documented record of the overlap-backstop tradeoff,
    so that a future reader understands why the gist exclusion constraint is gone.

## Implementation Decisions

### Driver
- Use `modernc.org/sqlite` (pure Go, no cgo). This is required to keep the
  existing `CGO_ENABLED=0 … → scratch` Dockerfile working. `mattn/go-sqlite3`
  is rejected because it needs cgo and would break the scratch image.
- Drop `github.com/jackc/pgx/v5` and its `pgxpool`/`stdlib` usage. Keep
  `pressly/goose/v3` (already used as a library; supports SQLite dialect).
- DSN carries pragmas: WAL journal mode, a `busy_timeout`, and
  `foreign_keys=on`. Writes use immediate-transaction locking (`BEGIN IMMEDIATE`
  semantics) to avoid `SQLITE_BUSY` mid-transaction.

### Core service (`block/`)
- `Service` holds a `*sql.DB` instead of `*pgxpool.Pool`. The shared `querier`
  read interface re-points at `QueryContext`/`QueryRowContext` (satisfied by both
  `*sql.DB` and `*sql.Tx`, preserving the existing pool-or-tx pattern).
- Transactions: `BeginTx(ctx, nil)`; `Rollback()`/`ExecContext`/`QueryContext`
  replace the pgx equivalents. `pgx.ErrNoRows` → `sql.ErrNoRows`.
- `FOR UPDATE` is removed. SQLite serializes writes at the database level via the
  immediate write lock, which is a stronger guarantee than row-level
  `FOR UPDATE`; the `lock` parameter on the read helper becomes a no-op (kept or
  removed at author's discretion).
- The bulk `UPDATE … SET … FROM (VALUES …)` shape survives (SQLite 3.33+
  supports `UPDATE … FROM`). Edits: remove `::text`/`::int` casts and switch
  `$N` placeholders to `?`. Same edits apply to the `Seed` `INSERT … FROM
  (VALUES …)`.
- Pure layout validation (`ValidateLayout` and the `Bounds`/`Placement` types) is
  untouched — no DB dependency.

### Auth (`auth/`)
- Same mechanical pgx→`database/sql` swap.
- Postgres interval/`now()` arithmetic is moved into Go. Code TTL, request
  throttle cutoff, and session expiry are computed as concrete `time.Time`
  values in Go and passed as parameters, rather than `now() + $n::interval` in
  SQL. This also removes business time-math from SQL.
- Timestamps are stored in a single consistent representation (RFC3339 TEXT,
  chosen so values remain string-sortable for `expires_at` comparisons). The
  only timestamp scanned back into Go (`RETURNING expires_at`) is parsed
  explicitly, where pgx previously auto-decoded `time.Time`.
- The one-active-code upsert (`ON CONFLICT (user_id) DO UPDATE … WHERE`) and
  `RETURNING` are portable to SQLite and retained. `FOR UPDATE OF lc` is removed.

### Migrations (`internal/migrate/`)
- `migrate.Run` opens with the `sqlite` driver and uses `goose.DialectSQLite3`;
  the embed + provider machinery is unchanged. Forward-only discipline is kept.
- The historical Postgres migration files are collapsed into a single fresh
  SQLite baseline migration (the old files are history; a new SQLite database
  starts clean). Translation: `TIMESTAMPTZ`/`SMALLINT` → SQLite types,
  `now()` defaults → `datetime('now')`, drop all `IF NOT EXISTS` / `DO $$
  EXCEPTION` idempotency scaffolding (a single baseline doesn't need it).
- **Lost feature (accepted):** the `btree_gist` `EXCLUDE` overlap constraint
  (`block_owner_slots_excl`, `DEFERRABLE`) has no SQLite equivalent. It was a
  defense-in-depth backstop behind `ValidateLayout`, which remains the primary
  overlap guard and runs first on every mutation. The tradeoff is documented in
  a new ADR.

### Wiring (`cmd/unbusy/main.go`)
- `pgxpool.New` → `sql.Open("sqlite", dsn)`; `Ping` → `PingContext`. The
  `*sql.DB` threads into both `NewService` constructors.
- Migrations run on boot in the app machine (which mounts the volume) rather than
  in a separate Fly release machine that cannot see the volume.
- Remove the Neon scale-to-zero rationale from the `/healthz` handler comment.

### Deploy
- Add a Fly volume mounted at a data directory; the DSN points the SQLite file
  there. Durability is Fly's scheduled volume snapshots.
- *(Deferred — backlog, docs/backlog/002)* Litestream streaming backup. The
  original plan baked the static `litestream` binary into the image (later
  revised to embed Litestream as a Go library); both are parked. No Litestream
  binary, `litestream.yml`, or replica wiring ships in this migration.
- The Fly `release_command = "migrate"` step is removed in favor of boot-time
  migration in the app machine.
- Local dev drops the Compose Postgres: `task up/down/nuke` and the compose file
  are removed; `task dev` points at a local `.db` file; `.env` `DATABASE_URL`
  becomes a SQLite DSN.

## Testing Decisions

- Good tests assert external behavior through the highest existing seam — the
  `block.Service` and `auth.Service` public methods — not SQL or driver
  internals. The database swap must not change what these methods promise, so the
  existing test intent is the specification.
- Tests run against an ephemeral SQLite database (temp file, or `:memory:` with
  shared cache) created per test/suite, replacing the Postgres-backed fixtures.
  This removes the external-container dependency and keeps `-race` clean by
  routing through a single writer.
- Modules tested: `block` (layout commit, bounds edit, seed, list ordering,
  invariant rejections), `auth` (request throttle, single-use code, attempt
  limit, expiry, session resolution/revocation), and `migrate` (the baseline
  applies cleanly and is idempotent under goose).
- Prior art: the existing `block_test.go`, `layout_test.go`, `auth_test.go`, and
  `migrate_test.go` define the behavioral contracts; this work re-points their
  fixtures rather than rewriting their assertions. `layout_test.go` is pure and
  unchanged.
- Add a regression test asserting overlap rejection at the service layer, since
  the database-level gist backstop is gone and `ValidateLayout` is now the sole
  guard.

## Out of Scope

- Multi-machine / horizontal write scaling. The app is single-machine by design
  (ADR 0003); SQLite matches that and does not attempt to enable scaling. A
  future need for multi-instance writes is a separate re-architecture (it would
  also require replacing the in-process broker) regardless of this change.
- **Litestream continuous streaming backup (deferred to backlog,
  docs/backlog/002).** Durability for this migration is Fly's scheduled volume
  snapshots. Streaming backup is a future feature, not part of shipping the swap.
- Live SQLite read replicas (Litestream's newer replication mode, LiteFS,
  rqlite, Turso). Only single-machine streaming backup is in scope.
- Cross-instance pub/sub (LISTEN/NOTIFY, Redis) — already out of scope per
  existing ADRs and unchanged here.
- Reproducing the gist exclusion constraint via triggers or other SQLite
  mechanisms. Application-level validation is the accepted guard.
- Any change to the Datastar + templ frontend, SSE read path, or pub/sub broker
  behavior. User-visible behavior is held constant.

## Further Notes

- Estimated effort: roughly 2–3 focused days. The work concentrates in auth's
  timestamp math (medium, the trickiest core change because Postgres intervals
  don't exist in SQLite) and the deploy/Litestream plumbing (medium, genuinely
  new vs. ported). The core service swap is small; pure validation does not move.
- Strategic note: the self-hosting goal is the deciding factor. Asking a
  self-hoster to provision Postgres is real friction; a single Go binary keeping
  its own `.db` file is nearly none — the same reasoning behind self-hostable
  tools like Plausible, Pocketbase, and Miniflux.
- A new ADR should record (a) the choice of SQLite + `modernc` driver and its
  rationale (Litestream noted as a deferred future-feature), and (b) the loss of
  the gist overlap backstop and why application-level validation is sufficient.
- The existing expand-then-deploy discipline (additive DDL, explicit column
  lists, never `SELECT *`) still applies and is preserved.

## Work Breakdown

Sequenced into rounds ordered to keep per-package tests green at every boundary.
The full binary will **not** compile from the start of Round 2 to the end of
Round 4 (pgx is still in `go.mod` and `main` still wires `pgxpool`), but each
swapped package's own tests pass in isolation as it lands. The whole tree builds
and the full suite is green again at the end of Round 4. Each round closes with a
checkpoint; tick it before starting the next.

### Round 1 — Migration baseline + `migrate/` package
The most self-contained piece: the `migrate` package opens its own DB, so it can
be swapped and tested without touching anything else.
- [x] Add `modernc.org/sqlite` dependency (`go get`) — surface it per the deps rule before running.
- [x] Write the fresh single SQLite baseline migration: translate `TIMESTAMPTZ`/`SMALLINT` → SQLite types, `now()` → `datetime('now')`, drop all `IF NOT EXISTS` / `DO $$ EXCEPTION` scaffolding, drop the `btree_gist` `EXCLUDE` constraint.
- [x] Point `migrate.Run` at the `sqlite` driver + `goose.DialectSQLite3`; leave embed/provider machinery and forward-only discipline unchanged.
- [x] Repoint `migrate_test.go` to an ephemeral SQLite DB; assert baseline applies cleanly and is idempotent under goose.
- [x] **Checkpoint:** `go test ./internal/migrate` green. ✅

### Round 2 — Core service (`block/`)
- [x] `Service` holds `*sql.DB`; re-point the `querier` read interface at `QueryContext`/`QueryRowContext`.
- [x] Swap transactions to `BeginTx(ctx, nil)` / `Rollback` / `ExecContext` / `QueryContext`; `pgx.ErrNoRows` → `sql.ErrNoRows`; remove `FOR UPDATE` (dropped the read helper's `lock` param).
- [x] Fix the bulk `UPDATE … FROM (VALUES …)` and the `Seed` `INSERT … FROM (VALUES …)`: drop `::text`/`::int` casts, `$N` → `?`. (SQLite has no column-alias on a VALUES subquery, so both use a `WITH v(...) AS (VALUES …)` CTE.)
- [x] Confirm `ValidateLayout` / `Bounds` / `Placement` are untouched (no DB dependency).
- [x] Repoint `block_test.go` fixtures to ephemeral SQLite; `layout_test.go` is pure — leave assertions as-is.
- [x] Add the overlap-rejection regression test at the service layer (gist backstop is gone; `ValidateLayout` is now the sole guard).
- [x] **Checkpoint:** `go test ./internal/block` green. ✅ (`-race` clean; writes use `_txlock=immediate` to serialize cleanly under `busy_timeout`.)

### Round 3 — Auth (`auth/`) — the trickiest core change
- [x] Mechanical pgx → `database/sql` swap (mirrors Round 2).
- [x] Move interval/`now()` math into Go: compute code TTL, request-throttle cutoff, and session expiry as concrete `time.Time` values, passed as parameters.
- [x] Store timestamps as RFC3339 TEXT (string-sortable for `expires_at`); parse the `RETURNING expires_at` value explicitly where pgx used to auto-decode.
- [x] Keep the one-active-code upsert (`ON CONFLICT (user_id) DO UPDATE … WHERE`) and `RETURNING`; remove `FOR UPDATE OF lc`.
- [x] Repoint `auth_test.go` fixtures to ephemeral SQLite; keep behavioral assertions (throttle, single-use code, attempt limit, expiry, session resolution/revocation).
- [x] **Checkpoint:** `go test ./internal/auth` green. ✅

### Round 4 — Wiring (`cmd/unbusy/main.go`) + drop pgx
First point the whole binary builds again.
- [x] `pgxpool.New` → `sql.Open("sqlite", dsn)` with the pragma DSN (WAL, `busy_timeout`, `foreign_keys=on`, immediate-transaction locking); `Ping` → `PingContext`; thread `*sql.DB` into both `NewService` constructors.
- [x] Run migrations on boot in the app process (call `migrate.Run` at startup) instead of relying on a separate release machine.
- [x] Remove `github.com/jackc/pgx/v5` from imports; `go mod tidy`.
- [x] **Checkpoint:** `templ generate` then full `go test -race ./...` green; binary builds. ✅

### Round 5 — Local dev experience
- [x] `task dev` points at a local `.db` file; remove the Compose Postgres dependency.
- [x] Remove `task up` / `task down` / `task nuke` and the compose file.
- [x] Update `.env` / `.env.example`: `DATABASE_URL` becomes a SQLite DSN; document the local data path.
- [x] **Checkpoint:** `task dev` runs end-to-end with no Docker. ✅ (app boots, migrates on boot, creates `tmp/unbusy.db`, serves `/healthz` + `/login`)

### Round 6 — Deploy (Fly volume; Litestream deferred)
> **Update 2026-06-14:** Litestream streaming backup was implemented and
> deployed but hit a Tigris signed-payload corruption issue and was **reverted**
> to unblock the migration. SQLite-on-a-Fly-volume ships; durability is Fly's
> scheduled volume snapshots for now. The streaming-backup work (diagnosis, fix,
> resume plan) is parked in `docs/backlog/002-litestream-streaming-backup.md`.
> The `internal/replica` package, the S3 secrets/env plumbing, and the ADR 0007
> Litestream section were removed. Items below are kept as historical record.

Backup target: **Fly Tigris** (S3-compatible, built into Fly). `flyctl` is the
only CLI that needs an authenticated session (`flyctl auth login`); `fly storage
create` provisions the bucket *and* stages the S3 credentials as app secrets, so
no second account/CLI is required.
**Design change from the original plan:** Litestream runs **embedded as a Go
library** (`internal/replica`), not as a supervising CLI binary. The scratch
image has no shell to chain `restore` before `replicate`, so the binary calls
`db.EnsureExists` (restore-if-empty) then `Store.Open` (background stream)
itself — keeping a single pure-Go scratch binary, no sidecar, no `litestream.yml`,
no `ENTRYPOINT` change. Replication is gated on `BUCKET_NAME` (local dev/tests
skip it). Rationale recorded in ADR 0007. This makes the Dockerfile and
`litestream.yml` steps below obsolete.
- [x] Mount + DSN wired in `fly.app.toml`: `[mounts]` (`source = "data"`, `destination = "/data"`) and `[env] DATABASE_URL` pointing at `/data/unbusy.db` with the pragma DSN.
- [x] Litestream embedded in the binary (`internal/replica`): `EnsureExists` restores from S3 if the volume is empty, runs *before* `migrate.Run`; `Store.Open` streams in the background. Pure-Go, stays `CGO_ENABLED=0` scratch — Dockerfile unchanged. ~~Bake the static binary~~ / ~~add `litestream.yml`~~ / ~~`ENTRYPOINT` change~~ no longer needed.
- [x] Removed the Fly `release_command = "migrate"` step (migration runs on boot, Round 4); `fly.app.toml` still pins exactly one always-on machine (`auto_stop_machines = "off"`, `min_machines_running = 1`).
- [x] `.env.example` documents the optional S3 replica vars (`BUCKET_NAME`, `AWS_*`) for local/self-host use.
- [x] **Operator:** create the volume in the app's region — `fly volumes create data --config fly.app.toml --size 1`. ✅ (`vol_vp290zo9xqplglk4`, 1GB, sjc, attached)
- ~~Provision the Tigris bucket / S3 secrets~~ — **deferred to backlog (docs/backlog/002)** with the rest of Litestream streaming backup.
- [x] **Checkpoint:** deploy boots and reads/writes against the volume-backed SQLite file (no Litestream; durability = Fly volume snapshots). ✅ (v37 boots clean: `migrate: applied 20260614000000_sqlite_baseline.sql` against `/data`, then listening on :8080; `/healthz` + `/login` live 200; health checks passing. **Note:** the pre-revert deploy left an inert corrupted `/data/.unbusy.db-litestream/` dir on the volume — harmless, optional cleanup during soak.)

### Round 7 — Cleanup & documentation
- [x] Remove the Neon scale-to-zero rationale from the `/healthz` comment and any other external-DB references. (`/healthz` comment was already a pure-liveness note; cleaned stale `FOR UPDATE`/`without Postgres` comments in `block.go`, `adapter.go`, `adapter_test.go`. **Note:** `fly.app.toml` still carries Neon/release-machine comments — owned by Round 6, which rewrites that file.)
- [x] Write the ADR recording (a) SQLite + Litestream + `modernc` choice and rationale, and (b) loss of the gist overlap backstop and why `ValidateLayout` suffices. (`docs/adr/0007-sqlite-litestream-storage.md`)
- [x] Update `CLAUDE.md` (Postgres/compose/Neon references, deploy notes, dev loop). Also updated `README.md` quickstart (Docker → SQLite).
- [x] **Checkpoint:** no lingering Postgres/Neon references; ADR merged. ✅ (code + main docs clean; `fly.app.toml` Neon/release-machine comments removed in the Round 6 deploy rewrite)

### Round 8 — Decommission Neon (destructive — do last, after a soak)
**Split out to its own PRD: `PRD-neon-teardown.md`.** Tearing down the now-unused
Neon Postgres is destructive and gated on a production soak, so it's tracked
separately. This migration PRD is complete through Round 7; Neon decommission is
the standalone follow-up.
