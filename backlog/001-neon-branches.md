# 001 — Test migrations against a Neon branch in CI

Status: backlog
Date: 2026-06-12

## Problem

CI applies `migrations/*.sql` to a fresh, empty Postgres. That proves the set
bootstraps from nothing, but not the case our re-apply-everything scheme is
actually exposed to: applying against a database that already has prod's
schema, data, and full migration history. The two known hazards —

1. a non-idempotent statement breaks every future deploy at the Fly
   release_command (idempotency is convention, nothing enforces it), and
2. data statements re-run forever (e.g. 0004's
   `DELETE FROM card WHERE owner_id IS NULL` executes on every deploy)

— only surface against a prod-shaped database. Neon branches are copy-on-write
clones of prod, created in seconds, so this test is nearly free.

## Plan

Add one CI job, gated to PRs that touch `migrations/`:

1. Create a branch off prod via `neondatabase/create-branch-action`
   (needs `NEON_API_KEY` + project id as repo secrets).
2. Apply all of `migrations/*.sql` against the branch — same psql loop as the
   existing test job — and run the loop **twice**, mechanically proving
   idempotency instead of trusting it.
3. Optionally run `go test ./...` with `DATABASE_URL` pointed at the branch,
   exercising the PR's code against post-migration prod schema.
4. Delete the branch in an `always()` cleanup step (free tier: 10 branches).

Keep the existing empty-Postgres job: it proves bootstrap-from-nothing
(new dev machine, `task nuke`); the Neon job proves upgrade-in-place.

## Caveats

- **Forks:** fork PRs can't read secrets, and arbitrary PR SQL shouldn't run
  against a clone of prod data. Gate on
  `github.event.pull_request.head.repo.full_name == github.repository`.
- **Prod data in CI:** the branch contains real user data. Fine with one user;
  revisit before there are real tenants.

## Related

- Neon instant restore (PITR) covers fast-noticed mistakes in prod, but only
  within the history window (check Console → Settings → Instant restore).
  This job is the preventative complement.
- If migrations ever move to run-once semantics (goose or a small stdlib
  version tracker), the double-apply check becomes redundant; the
  prod-clone apply test stays valuable.
