# PRD: Adopt goose for run-once database migrations

Status: ready-for-agent

## Problem Statement

As the operator of unbusy.day, every deploy re-applies every migration file
against production, so the system is only as safe as the weakest idempotency
guard in any migration ever written. A subtle hole in one guard (a `DO` block
catching `duplicate_object` when Postgres raises `duplicate_table`) passed on
first apply and then failed the Fly release command on the *next* deploy,
blocking all deployment. The product is heading toward real production use
with tenant data; "every statement must be perfectly re-runnable forever" is a
bet that has already lost once and gets riskier with every migration added.

## Solution

Replace the re-apply-everything migration runner with goose's run-once version
tracking, embedded in the existing binary. Each migration applies exactly once
and is recorded in a `goose_db_version` table; new migrations are plain DDL
with no idempotency scaffolding. The deploy interface does not change: the Fly
release command still runs the binary's `migrate` subcommand, and `task
migrate` still migrates the dev database. Existing databases (production Neon,
dev) baseline themselves automatically: migrations 0001–0004 stay idempotent
as written, so goose's first run re-applies them harmlessly and records them —
no manual steps on any environment.

## User Stories

1. As the operator, I want each migration to run exactly once per database, so that a re-run can never fail on already-applied DDL and block a deploy.
2. As the operator, I want existing databases (prod Neon, local dev) to baseline automatically on the first goose run, so that the cutover needs no hand-run SQL anywhere.
3. As a developer, I want new migrations to be plain DDL without `IF NOT EXISTS` / `DO $$ EXCEPTION` scaffolding, so that there are fewer places to get a guard subtly wrong.
4. As the operator, I want the Fly release command interface (`migrate` subcommand on the binary) unchanged, so that `fly.app.toml` and CI need no changes.
5. As a developer, I want `task migrate` to keep working as the one local command, so that my workflow is unchanged.
6. As the operator, I want the deploy image to remain a single static binary (migrations embedded via `go:embed`), so that the scratch image stays shell-free and small.
7. As a developer, I want migrations to remain plain `.sql` files in `migrations/`, so that they stay reviewable and greppable.
8. As a developer, I want new migrations named with timestamp versions, so that two branches adding migrations concurrently cannot collide on a sequence number.
9. As a developer, I want the existing `0001`–`0004` filenames left untouched, so that git history, ADRs, and incident notes keep pointing at real files.
10. As the operator, I want a forward-only posture (no Down sections), so that nobody can destroy tenant data by rolling a migration back in production; mistakes are fixed by new forward migrations.
11. As a developer, I want to reset a broken local schema with `task nuke` + re-migrate, so that I don't need rollback machinery while iterating.
12. As a developer, I want the dev-allowlist seed (`u_test`) to survive the cutover, so that local login keeps working after the switch.
13. As the operator, I want migration 0001's seed cards to stop being re-inserted and re-deleted on every deploy, so that deploys do no pointless writes.
14. As a developer, I want `migrate` to fail loudly with the failing file's name, so that a bad migration is diagnosable from Fly release logs alone.
15. As a developer, I want CLAUDE.md's migration guidance rewritten for the run-once model, so that future agents write plain-DDL migrations instead of idempotent ones.
16. As a maintainer, I want an ADR recording the switch and its rationale, so that the retired "additive + idempotent" invariant isn't accidentally reintroduced.
17. As a developer, I want the option to use the goose CLI locally for `goose create`/`goose status` conveniences, so that authoring is easy without the CLI being required anywhere.

## Implementation Decisions

- **Library, not CLI**: `github.com/pressly/goose/v3` runs embedded in the
  binary (dependency approved during design grilling). The runtime image stays
  one static binary; no goose binary ships anywhere.
- **Single migration code path**: the existing migration runner module keeps
  its `go:embed migrations/*.sql` filesystem and its public seam
  (`runMigrations(ctx, …)` invoked by the `migrate` subcommand); its body
  becomes a goose `Up` call against the embedded FS. Dev, CI, and prod all go
  through it. `task migrate` switches from a psql loop to invoking the
  subcommand (`go run . migrate`), removing the local psql dependency.
- **Driver bridge**: goose wants `database/sql`; open a `*sql.DB` via pgx's
  stdlib adapter (already an indirect dependency) for the migration run only.
  The app's `pgxpool` usage is untouched.
- **Baselining**: migrations 0001–0004 keep their idempotent bodies (including
  the `duplicate_table` fix from the incident). Goose's first run on an
  existing database re-applies them as no-ops and records versions 1–4. A
  fresh database takes the identical path. No manual `goose_db_version`
  surgery on any environment.
- **Annotations**: each existing file gains the `-- +goose Up` header; multi-
  statement `DO $$ … $$` blocks (0003, 0004) are wrapped in
  `-- +goose StatementBegin` / `-- +goose StatementEnd`.
- **New migrations**: plain DDL, timestamp-versioned filenames, no Down
  sections, forward-only. Expand-then-deploy is preserved by discipline
  (additive DDL, explicit column lists in queries) rather than by
  re-runnability.
- **Docs**: CLAUDE.md's "migrations are expand-then-deploy by construction"
  section is rewritten for the run-once model; "reference app" framing is
  dropped per product direction. A new ADR (0004) records the migration-model
  switch.

## Testing Decisions

- Test at the existing highest seam: the migration runner function the
  `migrate` subcommand calls, run against a real Postgres (the compose dev DB,
  port 5433), asserting external behavior only — schema state and
  `goose_db_version` contents, never goose internals.
- The load-bearing cases:
  1. **Fresh database**: run migrate once → all migrations recorded, schema
     complete (tables `card`, `user`, `login_code`, `session`; constraint
     `card_owner_position_unique`).
  2. **Re-run is a no-op**: run migrate twice → second run applies nothing,
     exits clean (the exact failure mode from the incident).
  3. **Baseline**: apply 0001–0004 the old way (raw psql-style execution),
     then run goose migrate → it records versions without error and the
     schema is unchanged.
- Prior art: the `cards` package tests already exercise a `pgxpool.Pool`
  against the dev database; follow their setup conventions.
- CI already runs `go test -race ./...` with Postgres available; the deploy
  itself is the final integration check (release command on Neon).

## Out of Scope

- Renaming or squashing migrations 0001–0004.
- Down migrations and any rollback tooling.
- Requiring the goose CLI in dev, CI, or the Docker image.
- Multi-instance migration locking concerns (Fly runs exactly one release
  machine; goose's session locking is unnecessary).
- Removing the 0001 seed-card insert (it becomes a one-time no-op under
  run-once; cleanup can ride along but is not the goal).
- Broader CLAUDE.md rewrites beyond the migration section and reference-app
  framing.

## Further Notes

- The incident fix (`EXCEPTION WHEN duplicate_object OR duplicate_table`) in
  0004 is a prerequisite: automatic baselining depends on 0001–0004 actually
  being idempotent. It is in the working tree, uncommitted, as of this PRD.
- After the first production goose run, migration files are history: editing
  an applied file does nothing. Mistakes are fixed by new forward migrations.
- Origin: CI deploy failure on 2026-06-12 (release command, `relation
  "card_owner_position_unique" already exists`, SQLSTATE 42P07), followed by a
  design grilling that settled the decisions recorded here.
