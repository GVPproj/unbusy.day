# Colocated SQLite replaces Neon Postgres

The external managed Postgres (Neon) is replaced by a SQLite database file
colocated with the app on a Fly volume. The driver is `modernc.org/sqlite`
(pure Go, no cgo). Reads become in-process library calls. Durability is Fly's
scheduled volume snapshots; **continuous streaming backup to object storage
(Litestream) was planned here but deferred — see docs/backlog/002.**

## Context

The app runs on a single always-on Fly machine (in-process pub/sub, ADR 0003)
but talked to a managed Postgres in another region. That arrangement cost more
than the workload warranted, added a network boundary we were already coding
around (the `/healthz` no-DB hack existed purely to preserve Neon's
scale-to-zero), and made self-hosting a non-starter — asking a self-hoster to
stand up Postgres is real adoption friction. SQLite matches the
single-machine-by-design constraint rather than fighting it.

## Considered Options

- **`mattn/go-sqlite3`** — rejected: needs cgo, which breaks the
  `CGO_ENABLED=0 → scratch` image. `modernc.org/sqlite` is pure Go and keeps the
  cgo-free static build.
- **LiteFS / rqlite / Turso / Litestream live read replicas** — rejected
  (out of scope): the app is single-machine by design, so distributed SQLite
  buys nothing here. Only single-machine streaming backup is needed.
- **Stay on Neon** — rejected: the managed-DB bill, the cross-region hop, and
  the self-hosting friction are the whole motivation for the change.

## Consequences

- The `block.Service` and `auth.Service` hold a `*sql.DB` instead of a
  `pgxpool.Pool`; the transport-agnostic core and pure layout validation are
  otherwise unchanged. `FOR UPDATE` is gone — SQLite serializes writes at the
  database level via the immediate write lock (`_txlock=immediate` +
  `busy_timeout`), a stronger guarantee than row-level locking.
- Auth's Postgres interval/`now()` arithmetic moves into Go: code TTL, request
  throttle cutoff, and session expiry are concrete `time.Time` values passed as
  parameters. Timestamps are stored as RFC3339 TEXT (string-sortable for
  `expires_at`).
- Migrations are a single fresh SQLite baseline (the Postgres files are
  history); goose runs with `DialectSQLite3` and applies on boot in the app
  machine that mounts the volume, replacing the Fly release-machine step (which
  cannot see the volume).
- **Lost: the `btree_gist` `EXCLUDE` overlap constraint
  (`block_owner_slots_excl`).** SQLite has no equivalent, and reproducing it via
  triggers is out of scope. This was defense-in-depth *behind* `ValidateLayout`,
  which remains the primary overlap guard and runs first on every mutation
  (ADR 0005). A service-layer regression test now asserts overlap rejection
  directly, since `ValidateLayout` is the sole guard. The accepted risk: a bug
  in `ValidateLayout` could commit an overlap that the database would previously
  have refused.
- Self-hosting becomes "run one Go binary pointed at a local `.db` file" — the
  Plausible/Pocketbase/Miniflux model.
- Local dev drops the Compose Postgres entirely: `task dev` points at a local
  `.db` file, no Docker required.
- **Streaming backup (Litestream) is deferred.** The intended design embedded
  `github.com/benbjohnson/litestream` as a Go library (`internal/replica`)
  streaming to Fly Tigris — keeping the image a single pure-Go `scratch` binary
  with no sidecar. It was implemented and deployed, but we hit a Tigris
  signed-payload corruption issue (`read lz4 trailer: expected lz4 end frame`)
  and reverted rather than block the migration on it. Until it returns,
  durability rests on Fly's scheduled volume snapshots (coarser RPO). The
  diagnosis, the fix, and the resume plan are recorded in docs/backlog/002.
