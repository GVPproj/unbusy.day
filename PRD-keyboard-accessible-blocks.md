# PRD — Keyboard-accessible block grid (move / resize / rename)

Resolves ACCESSIBILITY.md issue #1 ("Block grid is pointer-only — no keyboard path").

**Status:** ready-for-agent
**SC targeted:** 2.1.1 Keyboard (A), 2.5.7 Dragging Movements (AA); supported by 4.1.2 / 4.1.3 via live-region announcements.

---

## Problem Statement

A keyboard-only or screen-reader user can open the day plan, add a block to a free
slot, and delete a block — but they **cannot move, resize, or rename** a block.
Every one of those three operations is wired exclusively through pointer gestures
(`pointerdown/move/up` on `#block-list` in `static/drag.js`): blocks aren't
focusable, the resize grip is `aria-hidden`, and rename only fires on a pointer
tap. There is no single-pointer / keyboard alternative to dragging, which 2.5.7
(WCAG 2.2) additionally requires. The schedule is therefore unusable for arranging
a day without a mouse or trackpad.

## Solution

From the user's perspective, every block becomes operable from the keyboard using
the interaction patterns assistive-technology users already know — no bespoke
command language to learn:

- **Move** — Tab focuses a block; **Space/Enter** "grabs" it; **Up/Down arrows**
  move it one 30-minute slot at a time (other blocks slide out of the way exactly
  as they do on drag); **Space/Enter** drops it; **Escape** cancels and snaps it
  back. This is the widely-copied "grab → move → drop" convention from
  react-beautiful-dnd / dnd-kit / Trello.
- **Resize** — the grip becomes a focusable handle following the APG **Window
  Splitter** pattern. Tab reaches it after the block body; **Up/Down arrows**
  grow/shrink the span; **Home/End** jump to the smallest/largest legal span;
  **Enter** (or moving focus away) commits; **Escape** reverts.
- **Rename** — **F2** on a focused block enters the existing inline editor; Enter
  commits, Escape reverts (unchanged from today's pointer-tap behaviour).

Every state change is spoken through a polite-but-assertive live region
("Deep Work grabbed, 9:00. Use arrow keys to move…", "moved to 9:30 to 11:00",
"dropped at 9:30", "move cancelled"), so the operation is perceivable without
sighted feedback. Sighted pointer users see and do exactly what they do today —
the keyboard path is purely additive and commits through the same server endpoints.

## User Stories

1. As a keyboard-only user, I want to move focus onto a schedule block by pressing
   Tab, so that I can act on a specific block without a pointer.
2. As a screen-reader user, when a block receives focus I want to hear its label,
   its clock range, and that it is an operable "schedule block", so that I know
   what it is and that I can rearrange it.
3. As a screen-reader user, when a block receives focus I want to hear brief usage
   instructions (how to grab, move, resize, rename), so that I can discover the
   controls without external documentation.
4. As a keyboard user, I want to press Space or Enter to "grab" the focused block,
   so that I enter a mode where the arrow keys move it instead of scrolling the page.
5. As a screen-reader user, when I grab a block I want to hear that it is grabbed
   and at what time, so that I have a clear starting reference.
6. As a keyboard user, I want Up/Down arrows to move a grabbed block one slot
   earlier/later, so that I can place it precisely.
7. As a keyboard user, I want the blocks below/above to slide out of the way as I
   move (the same push cascade as dragging), so that the result matches what a
   mouse drag would produce.
8. As a screen-reader user, I want each arrow move announced as the block's new
   clock range, so that I can track where it is without seeing the grid.
9. As a keyboard user, I want a grabbed block to stop at the start/end of the day
   rather than vanish, so that I cannot move it out of bounds.
10. As a screen-reader user, when a move is blocked (edge of day, or no legal
    placement), I want to hear that it's blocked rather than silence, so that I
    know nothing happened.
11. As a keyboard user, I want to press Space/Enter to drop the grabbed block, so
    that I commit the new placement.
12. As a screen-reader user, I want the drop announced with the final clock range,
    so that I have confirmation the change was saved.
13. As a keyboard user, I want to press Escape while grabbed to cancel, so that the
    block returns to where it started with no change saved.
14. As a keyboard user, I want to Tab from a block to its resize handle, so that I
    can resize without grabbing/moving.
15. As a screen-reader user, when the resize handle is focused I want to hear it's
    a separator that resizes the block and hear the current range, so that I know
    what arrows will do.
16. As a keyboard user, I want Up/Down arrows on the handle to grow/shrink the
    block by one slot, so that I can change its duration.
17. As a keyboard user, I want a grow that runs into the blocks below to compress
    them closest-first (the same behaviour as a pointer resize), so that resizing
    behaves identically across input methods.
18. As a keyboard user, I want Home/End on the handle to jump to the minimum
    (one slot) and maximum legal span, so that I can resize quickly.
19. As a keyboard user, I want resize changes to commit when I press Enter or move
    focus off the handle, so that I control when the change is saved.
20. As a keyboard user, I want Escape on the handle to revert an in-progress
    resize, so that I can back out cleanly.
21. As a screen-reader user, I want each resize step and the final commit
    announced as the block's new clock range, so that I can perceive the change.
22. As a keyboard user, I want to press F2 on a focused block to rename it, so that
    I can edit its label without a pointer tap.
23. As a keyboard user renaming a block, I want Enter to save and Escape to revert,
    so that editing behaves like every other inline editor.
24. As a keyboard user, I want the page to never scroll out from under me when I
    use arrows on a grabbed block or focused handle, so that the gesture stays
    in control of the arrows.
25. As a keyboard user, after a move/resize/rename commits and the server
    re-renders, I want focus to return to the block I was acting on, so that I can
    keep operating without re-finding it.
26. As a sighted pointer user, I want drag, stretch, and tap-to-rename to keep
    working exactly as before, so that the keyboard work doesn't regress my
    experience.
27. As a screen-reader user, I want empty "now" markers and occupied gutter rows to
    stay out of the reading order, so that the keyboard path only stops on real,
    operable items (unchanged from issue #2b).
28. As a developer, I want the keyboard path to commit through the same
    `layout` / `rename` events and server endpoints as dragging, so that there is
    no second mutation path to keep in sync.
29. As a developer, I want the key-to-layout decision logic to be a pure,
    unit-testable function, so that I can prove the movement/clamping/cascade
    behaviour without a browser.

## Implementation Decisions

### Interaction model (decided)

- **Move = grab → move → drop.** A focused block is grabbed with Space/Enter,
  moved one slot per Up/Down arrow, dropped with Space/Enter, cancelled with
  Escape. This is the react-beautiful-dnd / dnd-kit convention. The old
  WAI-ARIA `aria-grabbed` / `aria-dropeffect` attributes are **deprecated and
  deliberately not used**; the convention is carried by behaviour + live region.
- **Resize = APG Window Splitter.** The grip is exposed as `role="separator"` with
  `aria-valuemin` / `aria-valuemax` / `aria-valuenow` (span in slots),
  `aria-valuetext` (the clock range), and `aria-controls` referencing the block.
  Up/Down resize, Home/End jump to min/max span, Enter / blur commit, Escape
  reverts. (Chosen over a moded resize-on-the-block; user picked the canonical
  splitter, accepting a second tab stop per block.)
- **Rename = F2.** F2 is the established rename-in-place key. Enter is **not** used
  to enter rename because Enter already toggles grab/drop; keeping them distinct
  avoids an ambiguous keystroke.

### Focus model: natural tab order, not roving

- Each block body is a normal `tabindex="0"` tab stop; arrow keys are **inert
  until a block is grabbed (or a handle is focused)**. This is the
  react-beautiful-dnd approach and it is what removes the
  arrows-navigate-vs-arrows-move conflict — arrows are never used to move *focus*,
  only to move/resize the *grabbed thing*. (Explicitly chosen over a roving
  tabindex composite, which would need arrows for inter-item focus.)
- Resulting per-block tab order: block body → delete button → resize handle. Free
  slots keep their existing "Add block at …" button as a tab stop. Occupied slot
  gutters and the now-pill stay `aria-hidden` (unchanged), so Tab only lands on
  real, operable items.

### Modules built / modified

- **New pure module — keyboard decision reducer** (sibling of `push.js`, e.g.
  `static/keys.js`). A DOM-free function:
  `keyboardLayout(bounds, current, grabbed, key) → { layout, kind } | null`,
  where it maps a key to a target slot/span, clamps to the day, delegates the
  cascade to the existing `pushLayout` (move) / `pushLayout(…, {compress:true})`
  (resize-grow), and reports which announcement to make. Returns `null` (or the
  unchanged layout with `kind: "blocked"`) when the move is illegal. This is the
  single new piece of business logic and the highest test seam.
- **Modified — `static/drag.js`** (DOM glue). Adds one delegated `keydown`
  listener on `#block-list` (morph-stable, beside the existing pointer
  listeners), a grab-state object mirroring the existing `drag`/`resize` shapes,
  and reuses the module's existing private helpers (`writeLayout`, `slotPitch`,
  `boundsNow`, `layoutIn`, `blocksIn`, the `dispatchLayout` pattern). During a
  grab, arrow moves are applied **optimistically to the DOM only** (via the same
  `writeLayout` placement) with **no per-key server round-trip**; a single
  `layout` event is dispatched **on drop** (Escape dispatches nothing and snaps
  back). Resize commits one `layout` event on Enter/blur. Rename dispatches the
  existing `rename` event. The glue writes announcement text into the live region.
- **Modified — `components/column_block.templ`** ✅ **DONE (markup + Go render test).**
  - block `<li>`: added `tabindex="0"`, `role` left as listitem semantics with
    `aria-roledescription="schedule block"`, `aria-describedby="dnd-instructions"`,
    and an `id={c.ID}` so the grip's `aria-controls` resolves to it (the block had
    only `data-id` before; `drag.js` keys off `data-id`/`.grip`, never element `id`,
    so the new `id` is inert to the glue and to idiomorph's data-id keying). The
    `sr-only` clock-range span (issue #2a) already supplies name-from-content, so
    the accessible name needs no change.
  - grip `<span>`: removed `aria-hidden="true"`; added `role="separator"`,
    `tabindex="0"`, `aria-controls` (the block id), `aria-label` (e.g. "Resize
    Deep Work"), and `aria-valuemin="1"` / `aria-valuemax` / `aria-valuenow` /
    `aria-valuetext`. `aria-valuemax` is the day-end ceiling (`dayEnd - position`,
    threaded into `blockItem`); the true compressible max is probed live by the
    End-key reducer. The pointer-resize behaviour on the grip is unchanged.
- **Modified — `routes/blocks.templ` (BlocksPage)** ✅ **DONE (markup + Go render test).**
  Added two stable, visually-hidden (`sr-only`) nodes **outside `#block-list`** (so
  they are not part of the SSE patch target and survive every morph): an
  `aria-live="assertive"` announcement region (`#sr-announce`) and a static
  instructions node (`#dnd-instructions`, referenced by each block's
  `aria-describedby`).

### Server / endpoints: unchanged

- No new endpoints, signals, handlers, SQL, or migrations. A keyboard move and a
  keyboard resize both produce the **same `layout` event / `POST /blocks/layout`**
  the drag path already uses (a resize is a `Placement` with a changed `span`);
  rename reuses the existing `rename` event / `POST /blocks/rename`. The
  server's 200-and-re-render rollback convention is reused as the snap-back for
  both Escape-cancel and any domain rejection.

### Announcements

- A single visually-hidden `aria-live="assertive"` region carries action feedback
  (grab / each move step / drop / cancel / each resize step / resize commit /
  blocked). Assertive is chosen because the feedback is a direct response to a
  deliberate keystroke and should not be queued behind other output.
- Announcement strings reuse the existing `blockTimeRange` / `timeLabel` server
  helpers' notion of clock ranges (mirrored client-side from the block's
  `data-slot` / `data-span` and the day bounds) so spoken times match the visible
  gutter.

### Focus restoration across morphs

- During a grab the moves are DOM-only, so there is no morph mid-gesture and focus
  is stable. The single commit (drop / resize-commit / rename) triggers one
  `#block-list` morph; the glue **restores focus** to the acted-on block (or its
  handle) by `data-id` after the patch lands, since idiomorph keys the list anchor
  by `id` and the inner blocks by `data-id`. This mirrors how `drag.js` already
  re-reads persisted `data-*` state after a morph.

## Testing Decisions

A good test here asserts **external behaviour** (the layout a sequence of keys
produces, and the semantics the server renders), never the internal shape of the
glue.

- **Pure reducer — `node --test`** (new `static/keys.test.js`). Prior art:
  `internal/frontend/jstest/push.test.js`, run by `task test`
  (`node --test internal/frontend/jstest/*.test.js`). Cover:
  - grab + ArrowDown moves the block one slot and cascades displaced blocks
    identically to `pushLayout`;
  - ArrowUp/Down clamped at the day's first/last legal slot returns
    blocked/unchanged (no out-of-bounds);
  - resize +1 span via the compress path matches `pushLayout(…, {compress:true})`,
    and a grow that even one-slot floors can't absorb returns blocked;
  - Home/End map to the minimum (span 1) and maximum legal span;
  - the returned `kind` selects the correct announcement (grabbed / moved /
    dropped / blocked / resized).
- **Server-rendered semantics — Go render tests.** ✅ **DONE** —
  `internal/frontend/accessibility_test.go` (new; prior art:
  `internal/frontend/adapter_test.go` and `smoke_test.go`, which assert on rendered
  HTML substrings). Asserts that a rendered block carries `tabindex="0"`,
  `aria-roledescription`, and `aria-describedby` (`TestBlockIsFocusableAndDescribed`);
  that the grip renders `role="separator"` with `aria-controls` + `aria-valuemin/max/now`
  + `aria-valuetext` and is **no longer** `aria-hidden`, and that the block carries a
  matching `id` (`TestGripIsResizeSeparator`); and that `BlocksPage` renders the
  `#sr-announce` live region and `#dnd-instructions` node **outside `#block-list`**
  (`TestPageRendersLiveRegionAndInstructionsOutsidePatchTarget`).
- **DOM glue, focus management, and live-region writes are not unit-tested** —
  there is no DOM test harness in the repo and `drag.js` itself is verified the
  same way. These are verified manually with a screen reader (VoiceOver, and NVDA
  if available) per the steps below. This gap is intentional and called out so it
  isn't mistaken for coverage.

## Implementation Steps

Ordered as thin, independently-verifiable slices.

1. **Live-region + instructions scaffolding.** ✅ **DONE.** Added `#sr-announce`
   (visually hidden `sr-only`, `aria-live="assertive"` + `aria-atomic`) and
   `#dnd-instructions` to `BlocksPage`, outside `#block-list`. Go render test
   (`TestPageRendersLiveRegionAndInstructionsOutsidePatchTarget`) asserts both exist
   and live outside the patch target. No behaviour yet (glue writes the region in
   step 4).
2. **Make blocks focusable & described.** ◑ **Markup + Go render test DONE** —
   `column_block.templ` block `<li>` carries `tabindex="0"`,
   `aria-roledescription="schedule block"`, `aria-describedby="dnd-instructions"`
   (test `TestBlockIsFocusableAndDescribed`). **Remaining:** manual screen-reader
   verification that focus announces label + clock range + role + instructions
   (folded into the step 8 pass).
3. **Pure reducer + tests.** ✅ **DONE.** New `internal/frontend/static/keys.js`
   exporting `keyboardLayout(bounds, current, grabbed, key) → { layout, kind } | null`,
   delegating the cascade to `pushLayout`; `internal/frontend/jstest/keys.test.js`
   (19 tests) covers move/clamp/cascade **and** the resize logic below. Runs via
   the existing `task test` glob (`node --test internal/frontend/jstest/*.test.js`).
   Reducer scope is layout-only: `kind` ∈ `moved`/`resized`/`blocked`; grab/drop/
   cancel stay glue-owned. Move returns the running `slot`, resize the running
   `span`, so the glue threads a cursor and the reducer always recomputes the
   cascade from the immutable grab-start layout (symmetric across both modes). No DOM yet.
4. **Wire keyboard move.** ◑ **Glue DONE (no automated coverage — see Testing).**
   `drag.js` now has a delegated `keydown` on `#block-list` (move tab stop = the
   block `<li>` only; grip/delete/rename own their keys), a `grab` state object
   beside `drag`/`resize`, Space/Enter grab+drop, Up/Down optimistic move via the
   reducer + `writeLayout` (DOM-only, no spring, no per-key post), Escape cancel,
   `#sr-announce` writes (grabbed / range per step / dropped / blocked / cancelled),
   one `layout` event on drop, and `restoreFocusAfterMorph` (refocus by `id` only
   if the morph dropped focus to `<body>`). Arrows/Space/Enter/Escape are
   `preventDefault`ed only while grabbed (inert otherwise, so they don't fight
   focus nav / page scroll); a `focusout` on the grabbed block auto-cancels;
   `pointerdown` supersedes an active grab. **Visual grab state reuses the existing
   `.dragging` lift** — `.active` could not be the grab hook because `now-pill.js`
   already owns it (the "now" accent rail, toggled on a timer + every morph). A
   client-side `timeLabel`/`timeRange` mirrors the server helpers so spoken times
   match the gutter. **Remaining:** manual VoiceOver pass of a full move + cancel
   (folded into step 8).
5. **Grip → separator (resize).** ◑ **Reducer + markup + glue DONE (no automated
   coverage of the glue — see Testing).** `keys.js` resize mode (Up/Down grow/shrink
   with the compress path, Home → span 1, End → max legal span probed via `pushLayout`)
   is implemented and tested in step 3's `keys.test.js`, including the shrink-after-grow
   case that proves each step recomputes from the grab-start layout (so a grow's
   compression is undone, not stranded). The `column_block.templ` grip is
   `role="separator"` with the aria-value* set, `aria-controls`, focusable, no longer
   `aria-hidden` (+ Go render test `TestGripIsResizeSeparator`; block `<li>` gained a
   matching `id`). `drag.js` now wires the grip `keydown` (`kresize` state beside
   `grab`): Up/Down/Home/End optimistic + DOM-only via the reducer + `writeLayout`,
   live `aria-valuenow`/`aria-valuetext` updates, one `layout` event on Enter **or
   blur** (the splitter "blur commits" convention; Enter steers focus back across the
   morph, blur leaves it where the user Tabbed), Escape reverts, `#sr-announce` writes
   each step + commit + blocked (`Minimum length.`/`Maximum length.`), `pointerdown`
   supersedes an active resize. The blocked move announcement was also made
   direction-truthful (`Can't move earlier.`/`Can't move later.`). **Remaining:**
   manual VoiceOver pass of grow/shrink/Home/End/commit/cancel (folded into step 8).
6. **Rename via F2.** ◑ **Glue DONE (no automated coverage — see Testing).** F2 on a
   focused block (not while grabbed; Enter is taken by grab/drop) calls the existing
   `enterEdit()`. `enterEdit`'s caret point is now optional: a pointer tap still passes
   `(x, y)` and leaves focus to the platform (tap-to-rename unchanged), while keyboard
   entry passes none — the caret goes to the end (`caretToEnd`) and focus is steered
   back to the block (`restoreFocusAfterMorph` on commit, direct refocus on revert/no-op),
   since with no pointer there's nothing to land focus on after the morph. Enter/Escape
   inside the editor are unchanged. The `#dnd-instructions` text already advertised F2,
   so it is now truthful. **Remaining:** manual VoiceOver pass of F2 → edit → Enter /
   Escape (folded into step 8).
7. **Focus-restoration polish & docs.** Confirm focus returns correctly after each
   of the three commit paths' morphs. Mark ACCESSIBILITY.md #1 DONE; add a short
   note to CLAUDE.md's "Frontend gotchas" describing the keyboard model and the
   live-region-outside-the-patch-target rule.
8. **Full screen-reader pass.** VoiceOver (and NVDA if available): focus, grab,
   move, drop, cancel, resize (incl. Home/End), rename. Update the audit.

## Out of Scope

- **Any server/data change** — no new endpoints, signals, schema, or migrations.
- **Pointer behaviour changes** — drag, stretch, and tap-to-rename are untouched;
  keyboard is additive and shares their commit path.
- **Block-type change via keyboard**, and the separate 1.4.1 Use-of-Colour concern
  that block *type* is conveyed by fill colour only (tracked elsewhere, not here).
- **A command palette / Linear-style single-key shortcut language** — explicitly
  rejected as the accessibility baseline; conventional patterns are the path. A
  power-user shortcut layer could be added later but is not part of this work.
- **Add / delete keyboard access** — already keyboard-operable (per-slot add
  button, per-block delete button); unchanged.
- **The now-pill and occupied gutter rows** — purely visual, remain `aria-hidden`.
- **Multi-day / multi-column reordering** — the app is a single day column.

## Further Notes

- **Standards basis:**
  - Resize → [APG Window Splitter pattern](https://www.w3.org/WAI/ARIA/apg/patterns/windowsplitter/)
    (`role="separator"`, `aria-valuenow/min/max`, arrow + Home/End).
  - Move → the react-beautiful-dnd / dnd-kit "grab → move → drop" convention with
    live-region announcements
    ([keyboard sensor](https://github.com/atlassian/react-beautiful-dnd/blob/master/docs/sensors/keyboard.md),
    [screen-reader guide](https://github.com/atlassian/react-beautiful-dnd/blob/master/docs/guides/screen-reader.md),
    [dnd-kit accessibility](https://docs.dndkit.com/guides/accessibility)).
  - Focus & tab order → [APG Developing a Keyboard Interface](https://www.w3.org/WAI/ARIA/apg/practices/keyboard-interface/).
  - Natural tab stops (not roving) follow react-beautiful-dnd's deliberate choice
    so arrows never compete with focus navigation.
- The WAI-ARIA drag-and-drop attributes (`aria-grabbed`/`aria-dropeffect`) are
  **deprecated** — there is no sanctioned APG drag pattern, which is why the move
  interaction relies on the de-facto convention plus a live region rather than
  those attributes.
- This satisfies **2.5.7 Dragging Movements (AA)** by providing a non-dragging
  single-pointer-independent (keyboard) alternative for both move and resize, and
  **2.1.1 Keyboard (A)** for all three operations.
