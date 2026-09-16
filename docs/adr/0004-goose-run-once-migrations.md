# Run-once Migrations via goose

Status: accepted (deployment and baseline details amended by ADR 0007)

> **Current state:** ADR 0007 replaced Postgres with SQLite, collapsed the old
> Postgres history into one SQLite baseline, and moved migration execution from
> a Fly release machine to application boot. The durable decision here is still
> run-once, embedded, forward-only goose migrations.

Migrations switch from "re-apply every file on every deploy, each file
idempotent forever" to run-once version tracking with
[goose](https://github.com/pressly/goose) embedded in the binary. Each
`migrations/*.sql` file applies exactly once per database and is recorded in
`goose_db_version`; new migrations are plain DDL with no `IF NOT EXISTS` /
`DO $$ EXCEPTION` scaffolding. Forward-only: no Down sections, ever — mistakes
are fixed by new forward migrations, broken local schemas by `task nuke`
(deletes the `tmp/` `.db` file and its WAL sidecars; re-migrates on next run).

## Context

The old runner re-ran every migration on every deploy, so safety rested on the
weakest idempotency guard ever written. That bet lost on 2026-06-12: a `DO`
block caught `duplicate_object` where Postgres raises `duplicate_table`, the
guard passed on first apply, and the *next* deploy's release command failed
(`relation "card_owner_position_unique" already exists`, SQLSTATE 42P07),
blocking all deployment.

## Considered Options

- **Keep idempotent re-apply, harden the guards** — rejected: "every statement
  perfectly re-runnable forever" already failed once and gets riskier with each
  migration as the product heads toward real tenant data.
- **goose CLI in the image / a migration sidecar** — rejected: the deploy
  image stays one static binary on scratch; goose runs as a library
  (`github.com/pressly/goose/v3`) against the `go:embed`-ed migrations.
- **Manual baselining (`goose_db_version` surgery on prod)** — rejected:
  0001–0004 keep their idempotent bodies, so goose's first run on an existing
  database re-applies them as harmless no-ops and records versions 1–4. Fresh
  and existing databases take the identical path.

## Consequences

- The app applies migrations on boot after opening the volume-backed SQLite
  database. `task migrate` invokes the same binary path explicitly with
  `go run ./cmd/unbusy migrate` for ad hoc use; Fly has no release command
  because a release machine cannot mount the app volume (ADR 0007).
- goose runs as a library against `database/sql` over the `modernc.org/sqlite`
  driver (`DialectSQLite3`); the app's own DB access is untouched.
- The retired Postgres files were collapsed into
  `20260614000000_sqlite_baseline.sql`. New migrations are plain DDL with
  timestamp-versioned filenames so concurrent branches cannot collide. Once
  applied, a file is history — fix mistakes with a new forward migration.
- Expand-then-deploy is preserved by **discipline** (additive DDL, explicit
  column lists in queries), no longer by re-runnability. The retired
  "additive + idempotent by construction" invariant must not be reintroduced.
- The app runs exactly one always-on machine, so concurrent cross-machine
  migration execution is outside the deployment model.
