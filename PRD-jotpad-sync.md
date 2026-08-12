# PRD — Jotpad cross-device sync + save-state honesty

Status: draft, for `/to-spec` breakdown
Depends on: the CodeMirror Jotpad graduating from `prototype/jotpad-codemirror`
(remote edits are applied as CM transactions; a bare textarea cannot do this).

## Problem

The Jotpad saves with a debounced full-text POST into one `jot` column,
last-write-wins. Three user-visible failures follow:

1. **Silent clobber.** A stale tab (laptop left open overnight) that flushes on
   blur overwrites newer text written on another device, with no warning.
2. **No cross-device liveness.** A second device never sees edits until a full
   page reload — `POST /jot` is deliberately a 204 with no patch, because
   nothing may re-render a textarea the user is typing into.
3. **Invisible save state.** The user cannot tell whether their last paragraph
   reached the server. Trust in autosave *is* the feature; today it is blind
   trust.

## Goals

- Edits made on one device appear live on the user's other open devices,
  cursor preserved, without a reload.
- Two devices editing near-simultaneously converge without silently losing
  either side's text.
- The UI always tells the truth about save state: saved, saving, or
  offline-with-retry.

## Non-goals

- **Real-time collaborative editing / CRDTs.** No Automerge, Yjs, or Loro. The
  server stays the single home of business logic and SQLite stays a plain-text
  source of truth (ADR 0007) — never an opaque CRDT blob. Concurrent
  same-second typing on two devices is resolved by a server-side three-way
  merge; when the merge guesses imperfectly the blast radius is one visible
  paragraph, which is acceptable for a single-user scratchpad.
- **Multi-user shared jotpads.** Out of scope; revisit CRDTs only then.
- **Offline-first app shell / snapshot history.** Separate efforts.
- **Changing the block column's sync model.** This touches `jot` only.

## Design

### Data model

Add a version counter alongside the text (forward-only migration, additive DDL
per ADR 0004):

```sql
ALTER TABLE "user" ADD COLUMN jot_version INTEGER NOT NULL DEFAULT 0;
```

Every committed write increments `jot_version`. `Get` returns `(text, version)`.

### Write path: compare-and-swap, merge on miss

`POST /jot` gains a base version: `{_jot: string, _jotVersion: int}`.

- **CAS hit** (`_jotVersion` == stored version): store text, increment version.
- **CAS miss** (stale base): the server three-way merges — base = the text at
  the client's version, ours = current stored text, theirs = the client's
  text — using diff-match-patch (`sergi/go-diff`), stores the merged result,
  increments version.
  - Merging needs the base text, which the row no longer holds after the
    concurrent write. Keep a single `jot_base (owner, version, text)` shadow
    row updated on each write — the previous revision only, not history.
    A miss older than one revision falls back to patch-apply of the client's
    diff against current text (diff-match-patch is built for fuzzy patching);
    an unpatchable hunk keeps the server text and the response makes the
    client's editor authoritative, visibly.
- All of this lives in `jot.Service` in one write transaction
  (`_txlock=immediate` serializes writers, as with blocks). Adapters never
  touch SQL; `MaxLen` still rejects whole writes (a merge that exceeds the cap
  is rejected whole too, CAS-miss response as below).
- **Response** is JSON `{version: int, text?: string}`: `text` present only
  when the stored result differs from what the client sent (merge happened).
  The client applies it as a CM transaction (see below) and adopts the new
  version. This replaces today's bare 204.

### Read path: jot events over the existing SSE stream

- `jot.Service` gains the same post-commit `Publisher` seam `block` has (nil =
  no fan-out in unit tests), publishing `{version, text}` on the user's
  channel of the existing in-process broker (ADR 0003, single machine).
- `EventsHandler` forwards jot events on the already-open EventSource — no
  second connection. The first frame on (re)connect includes the current jot
  snapshot alongside the block column, so a dropped consumer is made whole
  (the broker drops slow consumers, no event history — unchanged).
- Jot events are **not** element patches. Morphing the editor is exactly the
  re-render-under-the-typist problem. They are data events the client applies
  itself.

### Client: remote edits as CodeMirror transactions

In `jot-cm.js` (the seam is `initJotpadCM`):

- On a jot event or a merge response: if the incoming version ≤ the local
  version, or the text equals the local doc, ignore. Otherwise compute the
  minimal diff old→new and dispatch it as a single CM transaction. CM maps the
  selection through the change set — the cursor stays put unless the remote
  edit deleted the text under it.
- Echo suppression: a device adopts the version returned by its own POST, so
  its own fan-out arrives as version ≤ local and is ignored by the guard
  above — no flag-passing.
- While a local debounce or in-flight POST exists (dirty state), buffer the
  remote event and resolve through the POST's merge response instead of
  applying both — one convergence path, not two racing ones.
- The flush path (`blur`/`pagehide`/`visibilitychange`) keeps `sendBeacon` but
  the beacon body now carries the base version too; a beacon's response is
  unreadable, so its version adoption is reconciled by the next SSE event or
  page load. Non-teardown saves move from bare `fetch` to
  `fetch(…, {keepalive: true})` with a retry (below).

### Save-state honesty

A single small status element next to the Jotpad heading, three states:

- **Saved** — no pending debounce, no in-flight request, last write acked.
  Rendered as a quiet checkmark/label; this is the resting state and must not
  attract attention.
- **Saving…** — debounce pending or request in flight. Only shown if the state
  persists past ~500 ms, so normal typing doesn't flicker it.
- **Offline — will retry** — a save failed or `navigator.onLine` is false.
  Dirty text is retried with capped exponential backoff (e.g. 2 s, 4 s, 8 s,
  max 30 s) and immediately on the `online` event. This state is allowed to be
  noticeable.

Styling uses the theme tokens (`--ink`, `--accent`, …) per ADR 0011; the
element lives in the jotpad templ component, updated by `jot-cm.js` directly
(client-only state — no Datastar signal, the server doesn't care).

### Textarea fallback (`jot.js`)

If the non-CM textarea path still ships when this lands: it gets CAS + version
plumbing and the save-state indicator (both transport-level), but **not** live
remote application — a remote-newer event while the textarea is clean replaces
`el.value` wholesale; while dirty, it waits for the merge response. Cursor
preservation under remote edits is a CM-only feature by design.

## Error convention

Domain rejections stay in-band and visible, matching the app-wide convention:
a rejected/merged write answers 200 with the authoritative `{version, text}`
and the client's editor snaps to it. `ErrTooLong` keeps today's log-and-drop
behavior but now also returns the authoritative state so the client stops
believing its over-cap doc is saved (indicator would otherwise lie).

## Acceptance criteria

1. Type on device A; within ~2 s device B's open Jotpad shows the text, and
   B's cursor position survives if A's edit didn't touch it.
2. Overnight-stale tab flushes on blur → server merges; text typed meanwhile
   on the phone is still present afterwards on both devices.
3. Both devices edit different paragraphs within the same second → both edits
   present on both devices after convergence; no infinite echo loop.
4. Kill the network, type a paragraph, restore the network → indicator walks
   Saving… → Offline — will retry → Saved; the paragraph is on the server.
5. Reject/merge paths never morph the editor DOM; only CM transactions (or
   `el.value` on the clean textarea fallback) change the text.
6. EventSource reconnect fully restores jot state (snapshot-on-connect), as it
   already does for blocks.
7. `task test` covers: CAS hit/miss, one-revision merge, older-than-base fuzzy
   patch, over-cap merge rejection, publisher fan-out post-commit, and the
   client version-guard logic (`node --test` for the JS reducer parts, kept as
   pure functions like `keyboard-reducer.js`).

## Open questions

- Does the jot snapshot join the existing first SSE frame, or ride as a
  separate named event? (Leaning: separate `jot` event type; the block column
  patch stays pure element-patch.)
- Is the one-revision `jot_base` shadow row enough in practice, or do multi-
  device usage patterns produce older-than-base misses often enough to warrant
  keeping N revisions? Instrument the fallback path with a log line first.
- Indicator copy and placement — needs a quick design pass against both
  feelings/colorschemes.
