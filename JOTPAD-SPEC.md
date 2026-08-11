# Jotpad — Spec

A second panel beside the Day Plan holding one persistent, free-text scratchpad
per User. Written continuously with no save button; read back on every page
load.

Status: **built, then superseded in part.** All six [build-order](#build-order)
steps shipped in `feat/jotpad` as specced. Since then the editing surface moved
from the plain textarea to CodeMirror 6 (`jot-cm.js`; `jot.js` and its Enter
list continuation are gone — CM's markdown keymap owns that), and the deferred
cross-device live sync shipped (versioned CAS writes with server-side three-way
merge, jot events over the existing SSE stream, and a save-state indicator).
References to `jot.js`/textarea mechanics below are kept as the record of that
first build.

The design below is unchanged by the build: nothing in it was contradicted.
What the build added is marked **Built:** inline, so this file stays a record of
the reasoning rather than turning into a changelog.

## Domain

**Jotpad** — a User's private, perpetual scratchpad: one free-text document,
owned by exactly one User, with no lifecycle of its own. It does not reset,
roll over, or expire; the User is the only thing that empties it. It is
markdown-*flavoured* by convention only — nothing renders it, and the stored
value is exactly the characters the User typed.

Unlike the Day Plan, the Jotpad is **not** "a rolling today." The Day Plan has
no history because a day ends; the Jotpad has no history because it never does.
That asymmetry is intentional, and it is the reason the two panels — identical
in styling — are not peers in the document outline (see
[Structure](#structure-and-accessibility)).

`CONTEXT.md` gains a `## Jotpad` entry saying the above, definitions only.

### Deliberately not in scope

- **Markdown rendering or syntax highlighting.** A `<textarea>` cannot render
  styled text; every route to it (transparent-overlay-on-`<pre>`,
  `contenteditable`, CodeMirror from a CDN, server-side goldmark + a preview
  mode) costs either font-metric fragility, the `contenteditable` swamp on iOS,
  a third pinned CDN dependency against `docs/backlog/004`, or a mode toggle
  that makes a scratchpad worse. The data model is a string either way, so this
  stays addable later on evidence rather than upfront on intuition.
- **Clickable checkboxes.** Needs rendering. Same reasoning.
- **Multiple notes, note history, undo beyond the browser's own.**
- **Clearing on any schedule.** There is no day boundary in this system to hang
  it on, and a timezone-aware reset would be the first server-side use of the
  viewer's clock — which the app deliberately avoids (`now.js` exists precisely
  because the server cannot know it).

## Behaviour

### Persistence

- Bound to a Datastar signal; a **1s debounce** on `input` POSTs to `/jot`.
- The debounce is **flushed** on `blur`, `visibilitychange`, and `pagehide`.
  `pagehide` matters specifically because iOS PWAs (ADR 0010) get suspended
  without a clean unload — that is where a paragraph would otherwise be lost.

  **Built:** not literally a flush. Datastar exposes no way to drain a pending
  `__debounce`, and a `fetch` started during teardown is routinely cancelled —
  precisely the case `pagehide` exists for. So `jot.js` sends an independent
  `navigator.sendBeacon` on all three events instead. It tracks only its own
  sends, so a beacon after a debounced `@post` repeats it: one duplicate write
  beats a lost one, and the endpoint is a whole-text replace either way.
- `POST /jot` responds **204, no patch.** The DOM is already correct; patching
  a textarea the user is typing into is exactly the thing to avoid.
- No save button, no save indicator, no dirty marker.

### Length cap

- **100,000 characters**, enforced twice: `maxlength` on the textarea, and a
  check in `jot.Service`.
- Over-cap writes are **rejected and logged, with no UI**. A browser cannot
  produce one; only a crafted client can.
- This is the one place the house "domain rejection → 200 + re-render of the
  authoritative state" convention is **deliberately not followed.** That
  convention exists so a rejected optimistic gesture visibly snaps back — good
  for a block, unforgivable for prose, where it would destroy text the user
  typed.

### "Clear" does not touch the Jotpad

The nav's Clear action is understood as clearing the day's blocks. Silently
wiping a persistent notes document from a button about the schedule would be a
data-loss trap.

### Enter list continuation

The only editing assistance. Pressing Enter on a line beginning with a list
marker inserts the marker on the next line; pressing Enter on an otherwise
empty marker line clears it.

**Built:** the markers are `-`/`*`/`+`, ordered `N.`/`N)` (which continue with
the next number), and — a judgement call worth flagging — `- [ ]`/`- [x]`,
which continue as a fresh **unticked** box. That is plain text, not the
*clickable* checkbox ruled out above, and it needs no rendering; but it is
wider than "inserts the marker," and it is one regex group and two tests to
remove if that reads as scope creep. A caret inside the marker, a selection, or
a marker with no trailing space all get no assistance — the browser's own Enter
stands. The edit is applied through `execCommand`, the only path that both
preserves the browser's undo stack and fires the native `input` event
`data-bind` needs.

**Tab does not indent.** Intercepting Tab inside a textarea removes the only
way a keyboard user can leave the control — WCAG 2.1.2 (No Keyboard Trap), a
Level A failure. Every escape-hatch workaround (Esc-then-Tab, a modifier key)
needs documenting in the shortcuts modal and is undiscoverable, to buy nested
bullets in a document nothing renders. Type spaces.

### No new keyboard shortcut

`shortcuts.js` documents `?` as "the one page-global printable shortcut,"
because WCAG 2.1.4 (Character Key Shortcuts) is satisfied everywhere else by
scoping letter keys to a focused block or grip. A global key to jump to the
Jotpad would be a second such exception, and `j`/`k` are already bound
block-scoped. Above 52rem the panel is always visible and Tab-reachable; below
it, the nav drawer carries it.

### Empty state

A `placeholder`, and nothing seeded — same as the block column, which now also
starts empty. Putting words into someone's notes document is presumptuous and
they would have to delete them. The placeholder is not load-bearing for
accessibility — the accessible name comes from `aria-labelledby`.

### Discoverability

One sentence in the guide modal, which is the narrative onboarding and already
walks through blocks, types, and theming. Without it the second panel is
unexplained.

## Storage

```sql
-- +goose Up
ALTER TABLE "user" ADD COLUMN jot TEXT NOT NULL DEFAULT '';
```

A column on `user`, not a `jot` table. Because a Session cannot exist without a
User row, **the jot row is guaranteed to exist** — no upsert, no
"row not created yet" branch, no `sql.ErrNoRows` case, no seeding on first
login. Every one of those is a branch that a separate table would force into a
service whose entire data model is a string. It also matches what
`day_start`/`day_end` already do: per-user singleton domain state on `user`,
written by a non-auth package (`block` already runs
`UPDATE "user" SET day_start = ?, day_end = ?`).

Rejected: a `jot(owner_id PK, body, updated_at)` table. It buys `updated_at`
(unused — the focus guard needs no timestamps) and an extension path to
multiple notes (a different feature with a different UI, and a clean forward
migration when it arrives).

The session hot path is unaffected: `RequireSession` runs
`SELECT user_id FROM session WHERE token = ?` and never reads the `user` row.
Per house convention, all queries name explicit columns, so nothing picks the
blob up incidentally.

## Package seam

A new **`internal/jot`** package: a transport-agnostic `Service` over `*sql.DB`
with `Get(ctx, owner)` and `Set(ctx, owner, text)`, owner-scoped like
everything else (ADR 0003).

**`block` never learns the word "jot."** `block.Service` is the deep module —
one job, one set of invariants (overlap, bounds, push, span). A Jotpad has none
of them: no overlap, no push cascade, no validation beyond a length cap.
Folding it in would make that module wider and shallower for zero shared logic.

`frontend` gains a `JotService` interface alongside `BlockService`, satisfied by
`*jot.Service`, wired in `main.go`.

## Layout

Two responsive regimes, reusing the **existing 52rem breakpoint**. No new
breakpoint is introduced — the stylesheet has exactly one plus
`pointer: coarse`, and adding a third value is a permanent tax on every future
component.

### ≥ 52rem — side by side

- A `.panels` flex wrapper inside `.shell` holds both columns with a gap.
- Both panels are `flex: 1 1 0; max-width: 460px`. They **share and shrink
  together** rather than demanding the ~74rem that two 520px panels would need
  — a 13" laptop at default scaling does not reach that.
- This narrows the Day Plan from its current 520px to 460px. That is a visible
  change to the existing app, accepted.
- Blocks left, Jotpad right — reading order, and the schedule stays where it is
  today so the app does not feel like it moved.
- **`.shortcuts-fab` collides** with the Jotpad's bottom-right corner
  (`position: fixed; right: 1rem; bottom: 0.75rem` above 52rem, currently
  floating over empty space). Fix by padding the Jotpad's bottom by the FAB's
  height — not by moving the FAB, which deliberately mirrors the rail's
  baseline.

### < 52rem — one panel at a time

- Both panels stay **in the DOM**, hidden by CSS. Never conditionally rendered:
  the toggle is then instant with no round trip, and the textarea keeps its
  value, caret, and scroll position across switches.
- Toggled by a **client-only signal `$_jotopen`** — the leading underscore
  matches `$_navopen` and marks it as never sent to the server, per the house
  rule that Datastar signals are for state the server cares about. Panel
  visibility is not.
- Toggled via **`data-class:jot-open="$_jotopen"`** on `.panels`, with the
  show/hide rules scoped inside `@media (width < 52rem)`. **Not `data-show`** —
  that sets `display` unconditionally and would hide a panel at desktop widths
  too. Above 52rem the class is inert. This matches the `.open` idiom the nav
  drawer already uses.
- The switch is a **`SideNav` button**, label swapping "View Jotpad" ⇄ "View
  Blocks". It costs two taps behind the hamburger and its label lives inside a
  drawer that is closed almost always — accepted, because the Jotpad is a
  persistent notes document opened a few times a day, not a capture inbox
  opened every few minutes. At that frequency consistency with every other
  global action wins. If real use proves otherwise, promoting it to a segmented
  control in `DateHeading` is a contained change.

  **Built:** the icon swaps with the label, because a note icon beside "View
  Blocks" reads wrong. That needed one new icon, `IconJot`, in all three feeling
  sets — drawn in each set's idiom rather than lifted from it, since none ships
  a note glyph that reads at 24px beside the others. The Blocks state reuses
  `IconBlocks`, which already existed unused.
- **Resets to Blocks on every load.** A planner should open on the plan, and
  persisting the choice means adopting the theme's pre-paint-script pattern for
  a third thing to avoid a visible panel flip.

### Panel internals

- The textarea **fills its panel and scrolls internally** — no auto-grow.
  `.column` is already capped at `100svh - 6rem` with a single internal scroll
  region; auto-grow would either fight that cap or reintroduce document scroll.
- **`resize: none`.** A user-resizable box inside a fixed-height flex panel
  breaks the layout and gains nothing.
- All styling uses existing theme tokens, in `app.css`, as one `@scope` block
  per ADR 0011. No hardcoded colors.

## Structure and accessibility

```
.shell
  SideNav
  .panels            (flex; data-class:jot-open)
    <main class="column">          — Day Plan, unchanged, keeps the page's <h1>
    <aside class="column jotpad">  — <h2 id="jot-heading">Jotpad</h2>
                                     <textarea id="jot" aria-labelledby="jot-heading">
```

Two panels cannot both be `<main>` — invalid HTML, not a style preference. The
Day Plan stays `<main>` and the Jotpad becomes `<aside>`: the smaller diff, and
the more honest one, since this is a day planner and a scratchpad beside the
plan is genuinely complementary. It also gives screen-reader users two named
landmarks to jump between, which is the navigation affordance a two-panel
layout owes them.

`<h2>`, not a second `<h1>`. On mobile, when the Jotpad is the only visible
panel, its `h2` is the only heading in the tree — a mild smell, not an error,
and not worth a second `h1` to avoid.

No collision work is needed for existing keyboard handling: `shortcuts.js`
already guards `INPUT`/`TEXTAREA`/`isContentEditable`, and
`gestures/keyboard.js` is delegated via `e.target.closest(".block-item")`
inside `#block-list`, which a textarea outside that list cannot match.

## Datastar wiring

Verified against the v1 reference (the pinned bundle is **v1.0.2**), not from
memory, per the house rule.

### The signal is `_jot`, with a leading underscore

Backend actions send **the entire signal store except underscore-prefixed
signals** (default `exclude` is `/(^_|\._).*/`). A normally-named `jot` signal
would therefore ride along on **every** block layout, rename, and delete POST —
up to 100KB attached to every drag.

Naming it `_jot` keeps it out of every other request automatically, and is
**fail-safe**: a future endpoint cannot leak it. Adding
`filterSignals: {exclude: /^jot$/}` to the six existing block actions would be
fail-open — forget one and the blob is back.

### Write

```
data-bind:_jot
data-on:input__debounce.1s="@post('/jot', {filterSignals: {include: /^_jot$/, exclude: /^$/}})"
```

The explicit `exclude: /^$/` replaces the default underscore-stripping pattern
so this one request may carry the signal. It matches only the empty string, and
no signal is named `""`. This is the single cryptic line in the design and
needs a comment saying why.

`data-bind` sets the element's **value property**, which sidesteps the HTML
textarea *dirty value flag* — once a user has typed into a textarea, changing
its child text content no longer updates its value, so an idiomorph element
patch would silently fail to update exactly the devices most likely to be
stale. Property assignment does not have that problem.

**Built:** the initial value still arrives as the textarea's server-rendered
child content, which `data-bind` adopts on init. That runs into a second parser
rule: **a newline immediately after the `<textarea>` start tag is dropped**. So
the render emits a sacrificial one, and a jot that begins with a blank line
keeps it instead of losing that line on every reload. There is a test for it.

### Smoke canary — do this first

Two non-obvious behaviours are load-bearing. Both are cheap to falsify now
rather than mid-build, and `/_smoke` exists for exactly this:

1. The server can patch an **underscore-prefixed signal** and the client stores
   it.
2. `filterSignals: {include: /^_jot$/, exclude: /^$/}` actually **transmits**
   an underscore signal.

If either fails, fall back to the non-underscore name plus six `exclude`
clauses — same design, fail-open instead of fail-safe.

**Result: both hold** on the pinned client bundle (v1.0.2), checked in a real
browser before anything else was built. `/_smoke/events` patches `$_smokesig`,
the page shows the stored value and posts it straight back through that exact
`filterSignals` pair, and `/_smoke/echo` returns it verbatim. The fallback was
never needed, and the underscore name is what shipped.

**The assertions stay in `/_smoke` permanently.** That route is already the
canary for the pinned Datastar version; keeping them turns a future bump from a
silent Jotpad breakage into a visible canary failure — Go tests pin the two
wire halves, and the page itself shows the browser half.

## Build order

1. **`/_smoke` canary** — prove both Datastar assumptions above.
2. **Migration + `internal/jot`** — the `ALTER TABLE`, `Service` with
   `Get`/`Set`, unit tests covering the empty default, the 100k cap, and
   cross-owner isolation.
3. **Markup + CSS** — `.panels`, the `<aside>`, 460px panels, the mobile
   toggle, the nav button, the FAB collision fix.
4. **Write path** — `data-bind:_jot`, 1s debounce, blur/`visibilitychange`/
   `pagehide` flush, `POST /jot` → 204.
5. **`jot.js`** — Enter list continuation, written as a **pure function**
   `continueList(value, selectionStart) → {value, cursor}` so it tests under
   `node --test internal/frontend/jstest/` with no DOM. The wiring around it
   stays untested and trivial.
6. **Docs** — `CONTEXT.md` entry, guide-modal sentence.

Steps 1–4 are already a complete, useful single-device Jotpad.

**All six shipped**, in that order, in one commit on `feat/jotpad`.

### Verification

- `task test` — Go unit tests for `jot.Service`, a frontend handler test for
  `POST /jot` (204, signal read, cap rejection), and the `node --test` case for
  `continueList`.
- The `verify` skill for a headless visual pass at both regimes: side-by-side
  above 52rem (including the FAB clearance) and the mobile toggle below it.

**Done, all green.** `jot.Service` covers the empty default, the round trip
(untrimmed), clearing, the cap at/over the limit counted in *characters* not
bytes, and cross-owner isolation. The handler tests pin 204-with-no-body, the
empty write, and — the one that matters — that an over-cap rejection never
patches the textarea. Render tests pin the single `<main>`, the named `<aside>`
landmark, the write wiring, and the sacrificial newline. `continueList` has 15
cases.

The headless pass confirmed both regimes: two 460px panels sharing a baseline
with the FAB cleared, the mobile toggle swapping panels and label with the
drawer closing behind it, the textarea keeping its value across a switch, list
continuation and the empty-marker clear working against real `execCommand`,
persistence across a reload, and Clear leaving the Jotpad alone. No console
errors at either width.

## Follow-up: live sync (deferred)

Cross-device live sync — your phone and your laptop showing the same text
without a reload. Deliberately **not** in v1.

It is strictly additive: nothing in steps 1–6 needs redoing to add it. And it
is the only part that costs an architectural change, while being the part whose
value is least certain. Use the Jotpad for a week first. If you never hit a
stale panel, the refactor is avoided entirely; if you hit it on day two, you
build it knowing exactly what it is for.

### What it would take

**Generic broker.** `pubsub.Broker` is concretely typed to `block.Event` —
`Publish(block.Event)`, `Events <-chan block.Event`. Make it
`Broker[T Owned]` where `Owned` is `interface{ OwnerID() string }`, and run
**two instances**: one for `block.Event`, one for `jot.Event`.
`EventsHandler` subscribes to both and selects on two channels — its loop
already has three cases, so a fourth reads as an obvious peer. This keeps
compile-time typing end to end: no `any`, no envelope, no type switch.

Rejected alternatives: widening `block.Event` with a `Jot` field (every block
mutation would then have to load the jot text just to publish it, and
`block.Event` stops being about blocks); and an `{Owner, Payload any}` envelope
with a runtime type switch.

ADR 0003 gets an **amending line, not a new ADR** — owner-keyed, in-process,
single-machine is unchanged; only the payload type is parameterised. Costs a
generic-ified `pubsub.go` (~70 lines), `pubsub_test.go` updates, and two
`pubsub.New[...]()` calls in `main.go`. It stays **one** EventSource
connection: two subscriptions, one select loop, one stream.

### Receive-side rule: focus wins

The server patches a **different signal, `_jotin`** — never `_jot` directly —
and the textarea decides whether to accept it:

```
data-on-signal-patch-filter="{include: /^_jotin$/}"
data-on-signal-patch="!document.activeElement?.matches('#jot') && $_jot !== $_jotin && ($_jot = $_jotin)"
```

An incoming update is dropped entirely while the textarea is focused, and
applied only to an unfocused panel. Combined with the flush already specified
in v1, the window in which a remote update can land over local edits is
essentially zero.

**This subsumes self-exclusion.** No tab id, no `Origin` field on `jot.Event`,
no `?tab=` query param on `/events`, no filtering in `EventsHandler` — your own
echo either arrives while you are focused (dropped) or while you are not (text
identical, and the `!==` check makes it a no-op).

### The residual risk, stated plainly

**A stale device that you start typing into overwrites the newer text
wholesale.** Nothing short of a merge strategy fixes that, and a merge strategy
is not worth building for a private single-user scratchpad. Live sync shrinks
the conflict window from "one page load" to "one keystroke's echo"; it does not
eliminate the losing write.

Explicitly rejected: applying remote text unconditionally (it changes under
your cursor), and a "modified elsewhere" staleness marker (a piece of state and
a piece of chrome to explain a situation you will hit rarely — additive later
if real use warrants it).
