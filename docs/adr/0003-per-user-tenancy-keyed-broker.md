# Per-user Day Plans: owner-scoped blocks and a user-keyed broker

Each User privately owns their Blocks. `block` (then named `card`) gains an
`owner_id`, the uniqueness constraint becomes `UNIQUE(owner_id, position)` (so
two Users can both hold position 0), every `block.Service` query is scoped by
owner, and the in-process `pubsub.Broker` is **keyed by user** — `block.Event`
and the `Publisher` interface carry an owner key so a mutation fans out only to
that User's subscribers. We rejected a single global board behind a login gate, and rejected
a global broker that fans every event everywhere and filters in the handler.

## Considered Options

- **Login gate over a shared board** — rejected: the product is multi-tenant;
  a gate would defer the real tenancy work and the schema would need redoing.
- **Global broker, filter by owner in the SSE handler** — rejected: every
  mutation wakes every connection and leaks cross-user activity timing. Keying by
  user is the shape we'd want if this ever sharded.

## Consequences

- Migration adds `owner_id`, swaps the unique constraint, and **deletes the old
  ownerless global seed rows** (`a`/`b`/`c`). Safe under expand-then-deploy: the
  running binary's explicit column lists don't read `owner_id`, and it never
  inserts blocks.
- Blocks now need **generated unique ids** (hand-picked ids can't repeat across
  Users).
- New Users are **seeded** with starter blocks on first login, so the day plan
  is populated before any create-block UI exists. An add/delete-block UI is deferred.
- Still single-machine by design (see CLAUDE.md): a user-keyed *in-process*
  broker does not change the "never scale past one machine" constraint;
  cross-instance fan-out remains out of scope.
