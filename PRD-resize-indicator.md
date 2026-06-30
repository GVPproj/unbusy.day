# PRD — Resize-state indicator (variant D)

## Problem

A block being **moved** shows a lift shadow (`[&.dragging]:[box-shadow:var(--shadow-lift)]`).
A block being **resized** has no distinct cue — `[&.resizing]` only bumps `z-2` and
reveals the grip handle, which is easy to miss. Users can't tell at a glance that a
drag is changing duration vs. position.

## Decision

Adopt **variant D** from the `/_prototype/resize` exploration: while a block is being
resized, overlay a vertical double-headed resize arrow (↕) that **stretches with the
block height**, frame the block in a **dashed accent outline**, and **dim the label**
behind the arrow. Direct, affordance-y — it reads as "drag up/down to change length".

Chosen over variant B (dashed outline + span-count pill) — the arrow is more
immediately legible as a resize affordance and doesn't introduce a numeric readout we'd
then have to localize/format as a clock range.

Prototype artifacts (to delete on completion): `internal/frontend/routes/prototype_resize.templ`,
`internal/frontend/routes/PROTOTYPE_resize_NOTES.md`, `internal/frontend/prototype_resize.go`,
and the `/_prototype/resize` route in `cmd/unbusy/main.go`.

## Scope

Single file: **`internal/frontend/components/column_block.templ`** (`blockItem`).
No Go logic, no drag.js, no server changes. The cue is pure CSS keyed off the existing
`.resizing` class that **both** resize paths already toggle on the block `<li>`:

- pointer drag — `drag.js:484` (`el.classList.add("resizing")`)
- keyboard splitter — `drag.js:664`

So the overlay works for mouse, touch, and arrow-key resize with no JS wiring.

## Implementation

1. **Block outline + label dim while resizing.** Extend the `[&.resizing]` utilities on
   the `<li>` (currently just `[&.resizing]:z-2`):
   ```
   [&.resizing]:outline-2 [&.resizing]:outline-dashed [&.resizing]:outline-offset-2 [&.resizing]:outline-accent
   [&.resizing_.block-label]:opacity-40
   ```
   Keep `overflow-hidden` (already present) so the arrow + dashed outline clip cleanly.

2. **Arrow overlay.** Add a sibling overlay span inside the `<li>` (after the grip),
   shown only while resizing — a flex column of: fixed up-chevron, a `flex-1`
   dashed shaft, fixed down-chevron, so the arrow grows/shrinks with the block:
   ```html
   <span class="hidden group-[.resizing]:flex absolute inset-0 z-10 flex-col items-center justify-center py-2 pointer-events-none">
     <svg width="22" height="11" viewBox="0 0 22 11" fill="none" stroke="var(--accent)" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="drop-shadow shrink-0">
       <path d="M5 9 L11 3 L17 9"></path>
     </svg>
     <span class="w-0.5 flex-1 min-h-1 bg-[repeating-linear-gradient(to_bottom,var(--accent)_0_6px,transparent_6px_10px)]"></span>
     <svg width="22" height="11" viewBox="0 0 22 11" fill="none" stroke="var(--accent)" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="drop-shadow shrink-0">
       <path d="M5 2 L11 8 L17 2"></path>
     </svg>
   </span>
   ```
   `aria-hidden="true"` on the overlay — it's decorative; the grip's `role="separator"`
   + `aria-value*` already convey state to AT.

### Notes / rationale

- **Token-only / colorscheme-aware.** Stroke and shaft are `var(--accent)`; the dashed
  outline is `outline-accent`. Live theme swaps just work — no hardcoded color.
- **Uniform dashes.** The shaft is a `repeating-linear-gradient` (6px dash / 4px gap),
  *not* a CSS dashed border, so dash length stays constant as the block resizes
  (a border would stretch dashes to fit). Tuned to match the dashed outline's rhythm.
- **`pointer-events-none`** so the overlay never intercepts the grip/drag gesture.
- **`z-10`** sits above the label; the block itself is `z-2` while resizing so the
  now-pill (`z-3`) still wins — unchanged.

## Acceptance criteria

- [ ] Resizing a block (mouse drag, touch drag, **and** arrow-key on the grip) shows the
      ↕ arrow + dashed accent outline; the label dims; the cue clears on release/blur.
- [ ] The arrow spans the block's full height at every span (1 slot … day-end), with
      uniform-length dashes between the chevrons.
- [ ] Cue tracks the active colorscheme (verify against all three: Solarized Light,
      Solarized Osaka, Catppuccin Mocha).
- [ ] Moving (dragging) still shows only the lift shadow — the two states stay visually
      distinct.
- [ ] `task build` clean; no AT regression (grip still announced as a splitter).
- [ ] Prototype route + files removed.
