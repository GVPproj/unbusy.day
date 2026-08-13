# Architecture Decision Records

One record per decision, numbered in the order they were made. Each file opens
with a `Status:` line — `accepted` or `superseded by NNNN`; superseded records
are kept as history, never rewritten.

| ADR | Decision | Status |
| --- | --- | --- |
| [0001](0001-passwordless-email-otp-auth.md) | Passwordless authentication via email one-time codes | accepted |
| [0002](0002-db-backed-sessions.md) | Server-side sessions in the database, not stateless cookies | accepted |
| [0003](0003-per-user-tenancy-keyed-broker.md) | Per-user Day Plans: owner-scoped blocks and a user-keyed broker | accepted |
| [0004](0004-goose-run-once-migrations.md) | Run-once migrations via goose | accepted |
| [0005](0005-client-computed-push-server-enforced-invariants.md) | Client-computed Push, server-enforced invariants | accepted |
| [0006](0006-scoped-component-css-via-at-scope.md) | Component CSS scoped per leaf via native `@scope` | superseded by 0008 (revived in amended form by 0011) |
| [0007](0007-sqlite-litestream-storage.md) | Colocated SQLite replaces Neon Postgres | accepted (streaming backup deferred — [docs/backlog/002](../backlog/002-litestream-streaming-backup.md)) |
| [0008](0008-utility-first-tailwind-v4.md) | Utility-first CSS via the Tailwind v4 standalone binary | superseded by 0011 |
| [0009](0009-ses-bounce-complaint-suppression.md) | SES bounce/complaint suppression via an SNS feedback webhook | accepted |
| [0010](0010-passthrough-service-worker-ios-pwa.md) | Passthrough service worker for iOS PWA storage durability | accepted |
| [0011](0011-plain-css-cascade-layers-and-scope.md) | Plain CSS via cascade layers and `@scope` in one hand-authored stylesheet | accepted |
