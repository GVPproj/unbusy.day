# 005 — Scaling to other regions: data-source & host options

Status: backlog (exploration / no commitment)
Date: 2026-06-17

## Problem

The product may want to serve users outside the primary region (e.g. Europe)
with acceptable latency. The current architecture is **deliberately
single-machine** (ADR 0007, `AGENTS.md`): one always-on Fly machine, a colocated
SQLite file on one volume in one region, and an **in-process** pub/sub broker
with no external bus and no event history. None of these cross a region
boundary. A Fly volume lives in one region, so "add a Europe machine" is not a
config flag — it breaks the single-writer SQLite and in-process-broker
invariants at once.

This note captures the options explored so a future decision starts from here
rather than from scratch.

## First: name the actual goal

The right answer depends entirely on *which* problem we're solving. Don't pick
an architecture before naming this.

| Goal | Real cause | Cheapest honest fix |
|------|-----------|---------------------|
| EU page loads slow | RTT to primary region per request | Move the one machine to EU, or read replicas |
| EU drag/SSE laggy | Same RTT, on the write round-trip | Replicas don't help writes; needs replicated/forwarded writes |
| Contractual/regulatory residency | A specific policy requires a User's data to remain in-region | Per-region placement / sharding by tenant |
| Redundancy / DR | Single region = single point of failure | Streaming backup (backlog/002) first |

Most "serve Europe" asks are really latency. If users are *mostly* EU, the
cheapest answer is just **move the single machine**. One region cannot be fast
on both sides of the Atlantic without replication.

## Data-source options

Planner data is owner-scoped with no cross-tenant product queries, which makes
per-user placement plausible. The auth/control plane is global, however:
`user`, `session`, `login_code`, and `suppression` must remain shared or gain a
routing layer that can resolve a session before opening the User's database.

### A. Replicated SQLite, one primary (LiteFS / libSQL / Turso)
Primary writer + read replicas in other regions. EU machines serve **reads**
locally (fast page loads, fast SSE first-frame); **writes** forward to the
primary and pay RTT. Closest to "the same app, but multi-region." Replicates
*data*, not our pub/sub — the broker fan-out still needs solving separately.

### B. DB-per-user (one SQLite file per tenant)
After a shared control plane resolves the session and home region, each User's
planner data comes from a file opened on demand by owner id. This creates a clean
placement boundary, but plain local files do **not** provide replication,
multi-region routing, or backup by themselves; those require a storage layer
such as Turso/libSQL or explicit regional volumes and operations.

- **Buys:** tenant-sized placement and restore units, smaller blast radius, and a
  schema shape that aligns with the already user-keyed broker (ADR 0003). With a
  replication/placement layer, residency can become per-file placement rather
  than row-level sharding.
- **Costs:** migrations run N times (migrate on first open, not only on boot), an
  LRU must bound open `*sql.DB` handles, backup/restore must cover every file,
  and the hard boundary complicates future shared boards or teams. Moving the
  current services from one injected `*sql.DB` to request-selected databases is
  also a real interface change, not merely a different constructor argument.

### C. Per-region tenant sharding — N independent single-machine apps
A full independent copy of today's architecture per region (`us` app + volume,
`eu` app + volume), routed by tenant home region. The core services can remain
regional, but signup/session routing, provisioning, migrations, backups, and
operations must all understand the User's home region. Best fit for strict
residency; tenants do not move regions easily.

### D. External bus + distributed DB (Postgres + Redis/NATS)
The "normal SaaS" answer. Throws away the single-machine simplicity ADR 0007
deliberately chose. Only worth it at genuine horizontal scale — most code, most
ongoing complexity.

### On-device / local-first (different *product*, noted for completeness)
SQLite on the client with a sync engine. Collides head-on with three defining
choices in `AGENTS.md`: "logic lives once on the server," "no client-side business
logic / no JS build step," and "SSE carries the server's truth." Needs CRDTs or
server-authoritative reconcile for the layout overlap invariants (last-write-wins
is wrong — two devices dragging blocks produce overlaps `ValidateLayout` forbids).
Worth it only if *offline-first* is a product requirement we want to sell, not as
a latency fix. Tools: ElectricSQL, Zero, PowerSync, libSQL embedded replicas,
Automerge/Yjs.

## Does Fly still make sense if Turso becomes the data source?

Yes — and Turso *removes* the constraint that made Fly awkward, rather than
weakening the case for it.

- Taking the DB off local disk deletes the volume and the "exactly one machine"
  **data** constraint. The only remaining single-machine constraint is the
  in-process broker — a smaller, more movable problem.
- Machines become stateless and disposable — the shape Fly is good at. Fly's
  multi-region deploy + anycast routing then gives the EU-latency story we
  wanted, without us building the routing layer that made option C painful.
- Caveat: a stateless container near users is a **commodity** (Railway, Render,
  Cloud Run, etc. all do it). Turso *broadens* host options rather than
  confirming Fly. Fly stays a good choice; it stops being the only sensible one.

## The catch that survives every option above

1. **The in-process broker is still single-machine.** Going multi-machine means
   two sessions for the same User on different machines will not see each
   other's live updates — the broker is per-process, with no external bus (out
   of scope in `AGENTS.md`). Replicated data ≠
   replicated fan-out. Either stay on one machine (Turso still buys DR + clean
   backups + region flexibility there) or replace the broker with an external
   bus / a Turso change-stream driving SSE. **This is the real decision — not
   the host.**
2. **Write latency.** Any one-primary model (LiteFS, Turso) forwards remote
   writes to the primary and pays RTT. Reads local and fast; the drag-commit
   write is not. Usually fine for a planner — measure it.
3. **Remote versus embedded reads.** A remote database client adds a network hop
   per query. An embedded replica keeps reads in-process but pays network and
   consistency costs during synchronization. Benchmark the mode actually under
   consideration rather than treating both as equivalent.

## Recommendation / sequencing

1. **Prerequisite:** ship streaming backup (backlog/002). The moment we
   replicate or shard, "one file, one volume, nightly snapshot" stops being
   enough.
2. **Low-risk first experiment:** benchmark one machine on Fly against a managed
   Turso/libSQL database while keeping the broker unchanged. That can remove the
   app volume and delegate replication/backup, at the cost of a database network
   path. Evaluate DB-per-user separately; local per-user files retain the volume
   and create more backup/migration work unless paired with a managed layer.
3. **Full EU story:** multi-region only after solving broker fan-out — and at
   that point re-ask whether Fly is still the best host.
4. **Decision question to answer first:** is this product ever collaborative
   (shared boards, teams) or strictly one-user-one-workspace? Collaboration
   changes the per-user boundary (B/C) and the conflict model (local-first), and
   should be answered before committing.

## Related

- ADR 0007 (SQLite, single-machine) — the constraints this note works against.
- ADR 0003 (user-keyed broker) — why per-user DBs line up with the broker.
- backlog/002 (Litestream streaming backup) — prerequisite for any of this.
