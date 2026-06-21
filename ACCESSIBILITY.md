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
| — | Decorative icons exposed to AT | 1.1.1 (A) | `aria-hidden="true"` on the `icon.templ` SVGs and theme feeling-preview SVGs |

---

## Deferred — needs a design decision

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

### 2. Blocks don't expose their time; DOM order ≠ visual order (HIGH)

**SC:** 1.3.1 Info & Relationships (A), 1.3.2 Meaningful Sequence (A)

`BlockColumn` (`components/column.templ`) renders **all** slot `<li>`s first, then
**all** block `<li>`s; visual interleaving is done purely with CSS `grid-row`
derived from `data-slot`. A block's start time is therefore a non-perceivable
grid coordinate — `blockItem` renders only the label, so a screen reader hears
"Deep Work, Delete Deep Work" with **no time** and in an order divorced from the
day (every empty slot first, then a separate run of blocks).

**Recommended direction:**
- Give each block an accessible name that includes its time and duration, e.g. a
  visually-hidden span or `aria-label` like `"9:00–10:30, Deep Work (deep)"`.
  The slot index → clock time conversion already exists (`clockLabel`/`timeLabel`).
- Prefer a source order that matches the schedule (blocks interleaved with /
  sorted by slot) so 1.3.2 reading order tracks the visual order. If keeping the
  slots-then-blocks split for the CSS grid, consider whether the empty slot `<li>`s
  need to be in the AT tree at all (the gutter time is already `aria-hidden`; an
  occupied slot's `<li>` is an empty list item — noise).

### 3. `--ink-muted` fails contrast on the light theme (MEDIUM)

**SC:** 1.4.3 Contrast (Minimum) (AA)

In **Solarized Light**, `--ink-muted: hsl(180, 7%, 60%)` on
`--bg`/`--surface: hsl(44, 86%, 94%)` computes to **≈2.5:1** — below the 4.5:1
floor for normal text (and below 3:1). It is used for real copy, not just hints:

- "We'll email you a one-time code." — `components/login.templ`
- The Clear-modal description — `components/modals/clear.templ`
- The theme section headings ("Colours", "Feeling") — `components/modals/theme.templ`

(Computation: muted L\* ≈ 0.338, bg L\* ≈ 0.922 → (0.922+0.05)/(0.338+0.05) ≈ 2.5.)

**Recommended direction:** darken `--ink-muted` in the `solarized-light` palette
(`layouts/layout.templ`) until it clears 4.5:1 on `--bg`. Verify the two dark
themes (`solarized-osaka`, `catppuccin-mocha`) with an automated checker while
you're in there — they look safer but were not all computed. Also note the email
and create-name inputs lean on browser-default `placeholder` color, which is also
low-contrast; the inputs now have real labels (#3), so the placeholder is hint-only.

---

## Deferred — minor / polish

- **Continuous background motion on login.** The amoeba spin/morph
  (`components/login.templ`) runs >5s; it is `aria-hidden` and honors
  `prefers-reduced-motion`, which is the accepted mitigation, but there is no
  explicit pause control. (2.2.2, currently mitigated)

---

## Already good (no action)

Real `<button>`s with `aria-label` on the icon-only add/delete buttons; native
`<dialog>` + invoker commands; `<fieldset>`/`<legend>` on the type picker; wrapped
`<label>`s on the create/hours inputs; `aria-hidden` on decorative SVGs / gutter /
grip; `lang`, per-page `<title>`, zoomable viewport (no `maximum-scale`); and
`prefers-reduced-motion` honored across every animation (logo, amoeba, hamburger,
dialog fade).
