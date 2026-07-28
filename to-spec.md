# Deepening the Day Plan — Spec

Four refactors that turn shallow modules into deep ones, found by walking the
recent hot spots (`internal/frontend`, `static/gestures`, `internal/block`).
No user-visible feature changes; two deliberate behaviour changes are marked
inline.

Status: **not built.** Nothing here has been implemented.

Vocabulary is `CONTEXT.md` for the domain (Slot, Block, Day Plan, Push) and the
deep-module set for the architecture (module, interface, implementation, seam,
depth, leverage, locality).

## Dependencies

These are four largely independent changes, not a chain:

| | depends on |
| --- | --- |
| [1 · Slot and Bounds](#1--give-slot-and-bounds-an-implementation) | — |
| [2 · One commit path](#2--one-commit-path-for-the-six-mutators) | — |
| [3 · One mutation handler](#3--one-mutation-handler) | 2 |
| [4 · One gesture commit](#4--one-gesture-commit-path) | — |

Only 3→2 is real: `mutate` can only be uniform once every mutator returns the
same thing. 1 makes a *sub-part* of 2 cheap (routing `Create` and `SetBounds`
through `ValidateLayout`) but is not a prerequisite for it. 4 is JS-side and
shares nothing with the rest.

---

## 1 · Give Slot and Bounds an implementation

### The problem

A Slot is a bare `int` everywhere — Go, templ, JS, SQL — and `Bounds` is a
struct with no methods. So the "30 minutes per slot, counted from 00:00" rule
and the "is this placement inside the day" question are both restated at every
call site.

Four label formatters, all the same body:

- `components/column.templ:130` `timeLabel`
- `components/modals/hours.templ:38` `clockLabel`
- `components/modals/guide.templ:145` `gcTime` (hardcoded to count from 9:00)
- `static/keyboard-reducer.js:13` `timeLabel`

Four containment checks, and they do not agree:

- `block/layout.go:99` — `p.Slot < b.Start || p.Slot+p.Span > b.End`
- `block/block.go:105` — `slot < b.Start || slot >= b.End` (Create)
- `block/block.go:338` — `c.Position < start || c.Position+c.Span > end` (SetBounds)
- `components/column.templ:66` — `c.Position < b.Start || c.Position >= b.End`

Create's variant is correct only because Create is always span-1. The day it
isn't, that's a silent overlap.

Plus the span-floor (`span < 1 → 1`) in five places, and `DefaultDayStart` /
`DefaultDayEnd` restated as bare literals in
`migrations/20260614000000_sqlite_baseline.sql:15`.

### What changes

`internal/block/layout.go` gains a named type and methods. Nothing moves
packages.

```go
// Slot is a 30-minute interval index counted from 00:00.
type Slot int

func (s Slot) Label() string          // "9:00", "9:30"
func (s Slot) Range(span int) string  // "9:00 to 10:30"
func (s Slot) Add(span int) Slot

func (b Bounds) Contains(s Slot, span int) bool
func (b Bounds) Valid() bool          // MinDayStart/MaxDayEnd, End > Start
func (b Bounds) Slots() []Slot        // the render loop's iteration
func (b Bounds) Row(s Slot) int       // CSS grid-row, 1-based
func (b Bounds) Len() int
```

`Block.Position`, `Placement.Slot` and `Bounds.Start`/`End` become `Slot`.
`Span` stays `int` — the compiler then separates the two things that actually
get confused. `Slot` is an integer kind, so `encoding/json` and
`database/sql`'s reflect-based `Scan` both handle it with no wire or schema
change.

`spanOr1` moves out of `column_block.templ` into `block` as the one span floor.

### Call sites after

```
layout.go:99      → if !bounds.Contains(p.Slot, p.Span)
block.go:105      → if !bounds.Contains(slot, 1)
block.go:338      → if !next.Contains(c.Position, c.Span)
column.templ:66   → if !b.Contains(c.Position, c.Span)
column.templ:96   → bounds.Row(s)
column.templ:130  → s.Label()
hours.templ:38    → deleted, uses s.Label()
guide.templ:145   → deleted, uses Slot(i + 18).Label()
```

### The JS copy stays

ADR 0005 puts the Push cascade on the client, so `keyboard-reducer.js` keeps
its own `timeLabel`. This spec does not try to unify them — that would mean
codegen or a shared WASM core, both worse than the duplication.

What it *does* add is a drift check: a Go test emits the label for every slot
in `[MinDayStart, MaxDayEnd]` to a golden file, and a `node --test` case reads
the same file and asserts the JS agrees. Cheap, and it catches the only way the
two can diverge silently.

### Tests

All in `layout_test.go`, table-driven, no DB — the module with the best test
isolation in the repo already. `Contains` gets the span-1 vs span-N cases that
`Create`'s variant currently gets wrong.

### Deliberately not in scope

- **A `DayPlan` aggregate type.** `Bounds` + `[]Block` is enough; wrapping them
  buys nothing until something needs to own both.
- **Changing the hard limits or the slot size.** Same constants, one owner.
- **`CONTEXT.md` changes.** Slot and Day Plan are already defined there and the
  definitions are unchanged; this only gives them an implementation.

---

## 2 · One commit path for the six mutators

### The problem

`Create`, `Delete`, `Clear`, `Rename`, `SetLayout` and `SetBounds` each run the
same seven steps:

```
BeginTx → defer Rollback → queryBounds → mutate → queryBlocks → Commit → Publish
```

The fan-out block is verbatim six times (`block.go:134, 175, 211, 257, 310,
351`). There are five result types, each with one `Blocks` field, and the
adapter discards four of them with `_, err :=`.

The one that deviates is wrong: `SetBounds` publishes the blocks it read
*before* the update and hand-builds `Bounds{start, end}` (`block.go:352`)
instead of reading back. It is the only mutator whose Event is not a
post-mutation snapshot, and the only one with no result at all.

### What changes

One private seam in `internal/block`, used by its own methods and nothing else:

```go
type Snapshot struct {
    Blocks []Block `json:"blocks"`
    Bounds Bounds  `json:"bounds"`
}

// commit runs fn inside the write transaction, reads the committed state back,
// and fans it out post-commit. fn sees the pre-mutation state for validation.
func (s *Service) commit(
    ctx context.Context,
    owner string,
    fn func(ctx context.Context, tx *sql.Tx, bounds Bounds, current []Block) error,
) (*Snapshot, error)
```

`commit` owns `BeginTx`, `Rollback`, both pre-reads, both post-reads, `Commit`
and the `if s.pub != nil { Publish }` guard. Every mutator becomes its
validation plus its statement.

All six return `(*Snapshot, error)`. `LayoutResult`, `CreateResult`,
`DeleteResult`, `ClearResult`, `RenameResult` are deleted; `SetBounds` gains a
return value.

Reading bounds back after every mutation costs one extra query on four
mutators that don't change bounds. On a colocated SQLite file that is not worth
a special case, and it is what makes `SetBounds` stop being special.

**Behaviour change:** `SetBounds`'s Event now carries post-commit state. The
values are the same today; the guarantee is what changes.

### Two validation paths converge

While in here, route `Create` and `SetBounds` through `ValidateLayout` (or
`Bounds.Contains` from §1) instead of their own expressions. This is the part
that gets cheaper if §1 lands first, and it is what closes the span-1
assumption in `Create`.

### Deletion test

Delete `commit` and the transaction-and-fan-out ceremony reappears in six
places. It concentrates.

### Tests

`block_test.go` currently asserts the Event contents in exactly one place
(`TestSetLayout_PublishesEvent`). After this, one test over `commit` covers the
post-commit-only guarantee for all six, and `TestSetBounds` gains the Event
assertion it never had.

`Delete` has no test in `block_test.go` at all today — only a stub in the
adapter's `fakeService`. Add one against real SQL, including the cross-owner
case.

---

## 3 · One mutation handler

Depends on §2.

### The problem

Six handlers in `adapter.go`, each restating the same five steps: `ReadSignals`
+ 400, `ownerFrom`, the call, a `switch` classifying domain errors, `snapshot`,
`patchColumn`. `ClearHandler` is 18 lines of which one is the operation.

Counted across the package: `ownerFrom` ×9, `ReadSignals`+400 ×9, `snapshot`
×7, `patchColumn` ×6, `log.Printf` + 500 ×15 in `adapter.go` alone.

The real leak is the `switch`. Each handler keeps its own list of which
`block` errors mean "200 + re-render the authoritative column" — five
hand-maintained restatements of a fact that belongs next to the errors. A new
domain error is a five-place edit, and forgetting one turns a User-facing
rejection into a 500.

### What changes

**In `internal/block`** — the domain answers the question it owns:

```go
// IsRejection reports whether err is the User's doing (a rejected placement,
// a blank label, an unknown id) rather than a fault. Rejections surface as
// 200 + a re-render of the authoritative column.
func IsRejection(err error) bool
```

**In `internal/frontend`** — one generic implementation:

```go
func mutate[S any](
    apply func(ctx context.Context, owner string, sig S) (*block.Snapshot, error),
) http.Handler
```

It reads signals, pulls the owner, calls `apply`, classifies the error with
`block.IsRejection`, re-reads on rejection, and patches. Each handler is then a
signal struct and one call:

```go
func CreateHandler(svc BlockService) http.Handler {
    return mutate(func(ctx context.Context, owner string, sig createSignals) (*block.Snapshot, error) {
        return svc.Create(ctx, owner, sig.Label, sig.Slot, block.BlockType(sig.Type))
    })
}
```

`ClearHandler` uses `struct{}` as its signal type — Datastar always posts a
signals body, so the decode succeeds.

`snapshot()` and its double read go away: §2's `*Snapshot` already carries
both blocks and bounds, so the success path drops from 1 write + 2 reads to 1
write. Only the rejection path re-reads.

`BlockService` (`adapter.go:24`) narrows to match.

### What stays hand-written

`PageHandler`, `EventsHandler` and `JotHandler`. `EventsHandler` is the one
genuinely deep handler in the file (subscribe-before-snapshot ordering,
write-deadline clearing, keepalive) and must not be folded in. `JotHandler`
breaks the house convention on purpose — 204, no patch — and `JOTPAD-SPEC.md`
records why.

### Tests

`DeleteHandler` and `RenameHandler` have no adapter test today. After this they
are the same tested path as the other four, and the tests that matter are: one
per signal decode, and one over `mutate` covering the rejection→200 and
fault→500 split.

`block.IsRejection` gets a table test naming every error in the package — which
is also the guard against adding an error and forgetting to classify it.

---

## 4 · One gesture commit path

Independent of 1–3.

### The problem

`pointer.js` and `keyboard.js` both end a gesture by writing the layout,
announcing, and dispatching `layout`. They do it in three different orders:

```
pointer.js:191   same? → in-list? → dispatch
keyboard.js:145  announce → same? → in-list? → focus → dispatch
keyboard.js:217  same? → in-list? → announce → focus? → dispatch
```

The divergence is not designed, and it shows:

- A no-op drop announces "Dropped, …"; a no-op resize is silent.
- The pointer path never calls `announce` at all — `ctx.announce` is passed to
  `pointer.js` and never used.
- The pointer path updates neither the grip's `aria-valuenow`/`valuetext` nor
  the block's `.sr-only` clock range (`column_block.templ:27`). The keyboard
  resize path updates the grip only. `aria-valuemax` is a function of the slot
  and is never updated by anything, so it goes stale after any move.
- `sameLayout` (`grid.js:48`) is one-directional (`a.every(...)`, ignoring
  anything in `b` not in `a`) and vacuously `true` for `a === []`. Every
  "did anything change?" guard is built on it, so a layout that gained or lost
  a block can suppress the dispatch.
- `updateGripValue` (`keyboard.js:55`) and `springSibs` (`pointer.js:147`) both
  do an unguarded lookup and dereference. `writeLayout` guards the identical
  lookup with `if (!p) continue`. An SSE patch landing mid-gesture throws a
  TypeError in the other two.

### What changes

A new `static/gestures/commit.js`, one module both adapters call:

```js
// Finish a gesture: persist the layout, refresh the accessible mirrors,
// announce, steer focus across the morph, and tell the server. Returns
// whether anything was dispatched.
export function commitGesture(list, {
  el,        // the gesture's element
  id,
  from,      // grab-start layout
  to,        // committed layout
  bounds,
  say,       // announcement string, or null
  refocus,   // () => Element | null, or null to leave focus alone
})
```

It owns, in one order:

1. structural compare `from` vs `to` — bail if unchanged
2. still-in-list guard (`el.parentElement !== list` → the server already won)
3. `writeLayout`
4. refresh `aria-valuenow` / `aria-valuetext` / `aria-valuemax` on the grip and
   the `.sr-only` range on the block — all guarded lookups
5. `announce(say)`
6. `restoreFocusAfterMorph` if `refocus`
7. dispatch `layout`

`sameLayout` is fixed in place: length check plus bidirectional comparison.

Three call sites collapse to three `commitGesture` calls. `dispatchLayout`
(`pointer.js:191`) is deleted.

**Behaviour change:** the pointer path now announces. `#sr-announce` and
`#dnd-instructions` exist for exactly this, and a screen-reader user on a
touch device gets the drag path, not the keyboard path. A no-op gesture
announces nothing, on either path — which is the resize behaviour today, not
the drop behaviour.

### Two adapters justify the seam

Pointer and keyboard. Both already produce a full layout from the same
`grid.js` helpers and the same `push.js` cascade; only the commit differs, and
only accidentally.

### Tests

This is the first slice of the gesture wiring that can leave the browser.
`commit.js` touches no Motion import, no `getBoundingClientRect`, no
`PointerEvent`, no `matchMedia` — so it runs under `node --test` against a
small DOM stub, like `block-gestures.test.js` already does for arbitration.

Cases worth having: unchanged layout does not dispatch; a block that left the
list does not dispatch; a layout that gained or lost a block *does* dispatch
(the `sameLayout` bug); a missing element in the layout does not throw; the
ARIA mirrors match the committed placement.

`pointer.js`'s 404 lines of drag mechanics stay untestable here and stay
covered by `/verify`. That is not fixed by this spec and is not meant to be.

### Deliberately not in scope

- **Making `pointer.js` importable in Node.** The Motion CDN pin is
  deliberate (`AGENTS.md`, `task check:versions`); the dynamic import in
  `block-gestures.js:36` already keeps the entry importable.
- **Unifying the two clamp implementations.** `pointer.js:294` and
  `keyboard-reducer.js:61` clamp differently because one is continuous and one
  is discrete, with `push.js:22` as the shared backstop. That is fine.
- **A gesture state machine.** The two paths' module-level singletons are ugly
  but arbitration already has a tested contract (UNB-26).

---

## Build order

Each step is independently shippable and independently revertable.

1. **§1 Slot and Bounds** — smallest, purest, no DB, no behaviour change.
   Lands in `layout_test.go`. Includes the golden-file drift check against
   `keyboard-reducer.js`.
2. **§4 gesture commit** — independent of the Go work, so it can go in
   parallel or in either order. Fixes two latent crashes and the `sameLayout`
   bug on the way.
3. **§2 commit path** — includes converging `Create` and `SetBounds` onto
   `Bounds.Contains`, which is why it comes after §1.
4. **§3 mutation handler** — last, because it consumes §2's `*Snapshot`.

Steps 3 and 4 are one change seen from two sides of the same seam; doing them
in one branch is reasonable, doing them in one commit is not.

## Follow-up (not specified here)

`cmd/unbusy/main.go:79` `guardOpenSignup` is the open-signup interlock,
expressed as env-var string checks in `func main`, in a package with no test
file. The send path's seven defenses are owned by three packages
(`auth`, `frontend`, `main`) and no module can state whether the configuration
is safe. Worth a spec of its own; deliberately out of scope here because it is
security policy, not Day Plan structure.
