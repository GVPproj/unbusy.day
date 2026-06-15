# Run-once Migrations via goose

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

- The deploy interface is unchanged: Fly's release command still runs the
  binary's `migrate` subcommand; `task migrate` now invokes it too
  (`go run . migrate`), dropping the local psql dependency.
- goose runs as a library against `database/sql` over the `modernc.org/sqlite`
  driver (`DialectSQLite3`); the app's own DB access is untouched.
- New migrations: plain DDL, timestamp-versioned filenames
  (`goose create x sql`-style) so concurrent branches can't collide; 0001–0004
  keep their names so history, ADRs, and incident notes still point at real
  files. Once applied in prod, a file is history — editing it does nothing.
- Expand-then-deploy is preserved by **discipline** (additive DDL, explicit
  column lists in queries), no longer by re-runnability. The retired
  "additive + idempotent by construction" invariant must not be reintroduced.
- Fly runs exactly one release machine, so goose's session locking is
  unnecessary.
