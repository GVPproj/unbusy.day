# PRD: Decommission Neon Postgres

Status: Draft · Owner: Graham Van Pelt · Date: 2026-06-14

> Split out from `PRD-sqlite-migration.md` (Round 8). The SQLite migration is
> complete through Round 7 and the new SQLite-on-a-Fly-volume deploy is live;
> this PRD covers tearing down the now-unused Neon Postgres. It is **destructive
> and gated on a production soak** — do it last.

## Problem Statement

The migration off Neon Postgres to colocated SQLite has shipped — the app reads
and writes a SQLite file on a Fly volume (ADR 0007), and the production deploy
(v37, 2026-06-14) boots clean, migrates on boot, and passes health checks. The
Neon project is now unused but **still provisioned and still billing**. Removing
the managed-DB cost was the deciding goal of the migration (see the migration
PRD's Problem Statement), so Neon must be decommissioned to realize it.

Deletion is irreversible, so it is deliberately the final phase: it happens only
after the new deploy is proven in production over a soak window.

## Preconditions / Gating

- **Round 6 verified in production.** SQLite-on-Fly-volume deploy is live and
  serving real reads/writes.
- **Soak window passed.** Live reads/writes, multi-tab SSE, and Fly volume
  snapshots confirmed running for an agreed window before anything is deleted.
- **Clean start — no data migration.** This was a greenfield launch with no
  production data in Neon; the SQLite database starts empty and new users get
  starter blocks seeded on first login. There is nothing to export, so Neon can
  be deleted outright once the new deploy is proven.

## Work Breakdown

Destructive — do last, after the soak. Sequence so nothing references the old DSN
before the project is deleted.

- [ ] Let the new deploy soak for an agreed window — confirm live reads/writes,
  multi-tab SSE, and that Fly volume snapshots are running.
- [ ] Remove the old Neon `DATABASE_URL` secret from Fly
  (`fly secrets unset DATABASE_URL`) once nothing references the Postgres DSN.
  (The app already reads its SQLite DSN from `fly.app.toml [env]`, which
  overrides the secret — verify, then unset.)
- [ ] Delete the Neon project/branch (Neon console or MCP/CLI) once backups are
  confirmed and the soak passes.
- [ ] Cancel/confirm the managed-DB bill is gone (the cost goal in the migration
  PRD's problem statement).
- [ ] **Checkpoint:** Neon project deleted, no app references the old DSN,
  managed-DB cost is $0. ✅

## Out of Scope

- Any data export or migration from Neon — none exists (greenfield launch).
- Reverting to Postgres. The migration is complete; this is teardown only.
