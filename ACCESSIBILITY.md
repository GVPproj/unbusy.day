# Accessibility (WCAG) audit

Audit of the server-rendered templ markup and the JS that injects semantics
(`drag.js`, `now-pill.js`). Date: 2026-06-20.

This file tracks the findings that were **not** auto-fixed because they carry an
interaction-design or visual-design tradeoff. The mechanical/safe fixes below
have already landed.

---

## Already fixed (safe batch)

| # | Issue | SC | Fix |
|---|-------|----|-----|
| 3 | Email input had no accessible name | 1.3.1 / 3.3.2 / 4.1.2 (A) | `aria-label="Email address"` on `login.templ` |
| 4 | OTP code input had no accessible name | 1.3.1 / 3.3.2 / 4.1.2 (A) | `aria-label="One-time code"` on `login.templ` |
| 5 | `<dialog>`s had no accessible name | 4.1.2 (A) | `aria-labelledby` → each modal's `<h2>` (theme/hours/create/clear) |
| 6 | Type radios had no shared `name` | 1.3.1 / 4.1.2 / 2.1.1 (A) | `name="addtype"` on the radios in `create.templ` |
| 7 | No `<h1>`; date was a `<p>` | 1.3.1 / 2.4.6 (AA) | Date → `<h1>` (`column.templ`); visually-hidden `<h1>` on the login page |
| 8 | Login error not announced | 4.1.3 (AA) | `role="alert"` on the error `<p>` (`login.templ`) |
| 9 | Logo accessible name was cryptic (`"ub.d"`) | 1.1.1 (A) | `role="img"` + `aria-label="unbusy.day"` on `UBLogo` (`logo.templ`) |
| 10 | Hamburger had no `aria-controls` | 4.1.2 (A, advisory) | `aria-controls="sidenav"` on `MenuToggle` → `id="sidenav"` on the `<nav>` (`nav.templ`) |
| 11 | Delete button focus == hover (no distinct focus indicator) | 2.4.7 (AA) | Replaced `focus-visible:outline-none` with an offset accent outline on `block-delete` (`column_block.templ`) |
| 12 | `--ink-muted` failed contrast (Solarized Light 2.5:1; Osaka 2.8:1 on surface) | 1.4.3 (AA) | Repointed `--ink-muted` to a canonical palette tone that clears 4.5:1 in each scheme (`layouts/layout.templ`) — see #3 below |
| — | Decorative icons exposed to AT | 1.1.1 (A) | `aria-hidden="true"` on the `icon.templ` SVGs and theme feeling-preview SVGs |

---

## Remaining ISSUES

### 1. Block grid is pointer-only — no keyboard path (HIGH)

**SC:** 2.1.1 Keyboard (A), 2.5.7 Dragging Movements (AA)

`static/drag.js` wires move-by-drag, resize-by-grip, and rename-by-tap entirely
through `pointerdown/move/up` on `#block-list` (drag.js:69–103, 249). Blocks
(`components/column_block.templ`) have no `tabindex`, the resize grip is
`aria-hidden`, and rename only fires on a pointer tap (drag.js:196, 260).

Keyboard users **can** add (per-slot button) and delete (delete button), but
**cannot** move, resize, or rename a block. 2.5.7 (WCAG 2.2) additionally
requires a non-dragging single-pointer alternative for move/resize.

**Recommended direction:**
- Make `.block-item` focusable (`tabindex="0"`, `role` as appropriate).
- Arrow keys move the focused block by one slot; Shift+Arrow (or `[`/`]`) changes
  span. Reuse the existing `push.js` layout maths and dispatch the same `layout`
  event `drag.js` already emits, so the server path is unchanged.
- Enter (or F2) on a focused block enters rename — `enterEdit()` already exists;
  it just needs a keyboard entry point instead of only `pointerup`.
- Announce the operable affordances (e.g. `aria-roledescription`, or instructions
  in the block's accessible name).

This is the largest piece of work and the one most worth doing.

### 2. DOM order ≠ visual order (HIGH)

**SC:** 1.3.2 Meaningful Sequence (A)  ·  ~~1.3.1 Info & Relationships (A)~~ *(time now exposed — see 2a)*

`BlockColumn` (`components/column.templ`) renders **all** slot `<li>`s first, then
**all** block `<li>`s; visual interleaving is done purely with CSS `grid-row`
derived from `data-slot`. The reading order is therefore divorced from the day
(every empty slot first, then a separate run of blocks).

**2a — block time exposed (DONE):** `blockItem` now leads with a visually-hidden
`sr-only` span carrying the clock span (`blockTimeRange` → e.g. `"9:00 to 10:30, "`)
before the label, so a screen reader hears `"9:00 to 10:30, Deep Work"` instead of a
bare `"Deep Work"`. Reuses `timeLabel` (end = slot past the block's last occupied
slot). This is name-from-content, so it also feeds the accessible name once #1
makes blocks focusable. (Block *type* is still conveyed by fill colour only — a
separate 1.4.1 Use of Colour concern, not tracked here.)

**2b — reading order (REMAINING):** Prefer a source order that matches the
schedule (blocks interleaved with / sorted by slot) so 1.3.2 reading order tracks
the visual order. If keeping the slots-then-blocks split for the CSS grid, consider
whether the empty slot `<li>`s need to be in the AT tree at all (the gutter time is
already `aria-hidden`; an occupied slot's `<li>` is an empty list item — noise).
Verify the `layout` wire payload is order-independent before reshuffling the DOM
(`drag.js` reads `#block-list` children; layout is keyed by `data-id`/`data-slot`).

