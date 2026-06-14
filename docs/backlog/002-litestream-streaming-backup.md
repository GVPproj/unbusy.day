# 002 — Streaming backup of the SQLite file (Litestream, deferred)

Status: backlog
Date: 2026-06-14

## Problem

The SQLite migration (ADR 0007) landed, but the **streaming backup** half did
not. Production durability currently rests only on Fly's scheduled volume
snapshots (daily-ish, coarse RPO) — there is no continuous, second-granularity
backup to object storage, and no automatic restore-on-empty-volume. If the
machine and its volume are lost between snapshots, recent writes are gone.

The original plan was to embed Litestream as a Go library (`internal/replica`)
streaming to Fly Tigris. It was implemented, deployed, and then **reverted** —
not because the design was wrong, but because we hit a wall getting it healthy
against Tigris in the time available and chose to ship SQLite without it rather
than block the migration.

## What was tried (and where it got stuck)

- Embedded `github.com/benbjohnson/litestream` v0.5.12 in `internal/replica`,
  building the S3 client by hand via `s3.NewReplicaClient()` and setting
  endpoint/creds from the `AWS_*` / `BUCKET_NAME` env vars that
  `fly storage create` stages.
- Symptom: persistent `compaction failed` / `sync error` with
  `read lz4 trailer: expected lz4 end frame` — uploaded LTX objects came back
  corrupt.
- **Root cause found:** litestream's *URL-parse* constructor applies
  Tigris-specific defaults (`replica_client.go`: `if isTigris {
  client.SignPayload = true; client.RequireContentMD5 = false }`). Building the
  client field-by-field skips that, so payloads went unsigned and Tigris stored
  corrupt objects. Setting `client.SignPayload = true` /
  `client.RequireContentMD5 = false` is the fix.
- **Where it stalled:** after the fix, the *first* (pre-fix) boot had already
  written a corrupt LTX file to the volume's `.unbusy.db-litestream/` sidecar
  dir and a corrupt object to the bucket. Litestream does not auto-recover by
  default, and the `scratch` image has no shell (`fly ssh console` can't delete
  the sidecar dir; no local `aws` CLI to empty the bucket). Clearing the
  one-time corrupt state cleanly is what we ran out of runway on.

## Path forward when resumed

1. Re-add `internal/replica` with `client.SignPayload = true` and
   `client.RequireContentMD5 = false` set explicitly (or switch to litestream's
   URL/DSN-parse constructor so provider defaults apply automatically — likely
   the more durable choice than hand-building the client).
2. Start from clean state: fresh (empty) volume **and** empty bucket so no
   pre-fix corrupt LTX poisons the first snapshot. Greenfield, so a destroy +
   recreate of both is fine.
3. Consider enabling litestream auto-recover (if exposed in the pinned version)
   so transient LTX corruption self-heals instead of wedging.
4. Verify a clean object lands in the bucket and `sync`/`compaction` log no
   errors over several intervals before calling it done.
5. Re-instate the deploy plumbing: `BUCKET_NAME`/`AWS_*` secrets (already
   staged by `fly storage create`), the `.env.example` replica block, and the
   ADR 0007 Litestream section (the reverting commit removed all three).

## Related

- ADR 0007 (SQLite replaces Neon) — records the storage swap; its Litestream
  section was trimmed to a "deferred" note pointing here.
- Fly volume scheduled snapshots are the interim durability story (`fly volumes
  snapshots list`). Coarser RPO than Litestream; acceptable while greenfield.
</content>
</invoke>
