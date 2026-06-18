# PRD — Finish the Tailwind migration (markup-first)

Status: complete
Date: 2026-06-18

## Progress

- ✅ **1. `modals/theme.templ`** — `ThemeStyles()` deleted; `.active` ring moved
  to `[&.active]:border-accent [&.active]:[outline:1px_solid_var(--accent)]` on
  `optionClasses`. Dropped from the `Layout` call.
- ✅ **2. `nav.templ`** — `NavStyles()` deleted. `.shell` → `flex min-h-screen`
  on the shell div; `.shell .column` → `w-full` on `<main>`; the `@scope
  (.sidenav)` rail↔drawer geometry → `sidenavClasses` (desktop rail +
  `max-sm:` off-canvas drawer, translate/transition guarded `max-sm:` so the
  desktop sticky rail never transforms). Verified the `.open` override
  (specificity 0,2,0, later in source) wins over base `translate-x-full` inside
  the `max-width:40rem` media block.
- ✅ **3. `modals/create.templ`** — `CreateStyles()` deleted; migrated to the
  canonical `peer` radio pattern (`peer sr-only` radio + `peer-checked:` /
  `peer-focus-visible:` on the swatch, per-type fills via `data-[type=…]:`
  variants). Dropped from the `Layout` call.

- ✅ **4. `login.templ`** — the amoeba blob's positioning/sizing/color moved to
  utilities (`absolute left-1/2 top-1/2 -translate-x-1/2 -translate-y-1/2
  w-[min(1000px,max(95vmin,150vw))] … text-column-bg`; v4's `translate`-property
  utilities compose with the keyframe's `transform: rotate`). The OTP
  focus-within active rule → `.otp` became `group`, the box carries
  `group-focus-within:[&.active]:border-accent` (the `.active` data-class hook
  stays). `LoginStyles()` trimmed to just the `@keyframes` + its
  `animation:`/reduced-motion driver (kept by the non-goals rule).
- ✅ **5. `modals/dialog.templ`** — the shared chrome (box geometry, surface,
  border, `rounded-lg`, `[box-shadow:var(--shadow-lift)]`, `open:flex` layout,
  `h2`, `footer`, the two button styles) → reusable `modals` package consts
  (`dialogClass`/`dialogH2Class`/`dialogFooterClass`/`dialogButton*Class`)
  applied across all three dialogs (theme/create/hours). `DialogStyles()` trimmed
  to the blur-fade open transition + `::backdrop` scrim only — kept as the small
  CSS island per the non-goals (a transition driver). Dead leaf classes
  (`theme-modal`/`create-modal`/`bounds-modal`) dropped.
- ✅ **6/7 `icon.templ`/`logo.templ`** — documented as intentionally-CSS
  (keyframes/feeling-toggle/filter); each surviving `<style>` now carries a
  one-line "why it's CSS" note.

All steps complete. Each was `go build` + `go vet` + `go test -race` green with
the new utilities confirmed emitted in `output.css`. **Manual dev-server eyeball
(`:7331`) of the drawer/modals/OTP-comb states is still recommended before
merge.** ADR 0006 banner, CLAUDE.md "Theming & HTML practices" updated to the
markup-first end state.

## Context

ADR 0008 adopted utility-first Tailwind v4 for the "one-off spacing/color 80%"
and left structural/stateful CSS in co-located `@scope` blocks (ADR 0006). In
practice the split has become the main source of styling friction: to change a
component you hop between its markup and a faraway `*Styles()` block and
reverse-engineer which selector fires.

`column.templ` was migrated as a proof of concept and **its entire ~290-line
`ColumnStyles()` `@scope` block is now gone** — day-grid geometry, the
drag/resize state machine, the grip `::after` affordance, touch tuning, the
label line-clamp, and the mobile flex layout all moved to markup with no CSS
left behind (−324 / +34 lines). It was the hardest file in the tree, and it
migrated cleanly. This PRD finishes the job on the remaining components.

## Goal

Move all **component styling** to the markup level so restyling means editing
one `class="…"` attribute, not a separate stylesheet. After this, the only CSS
in the codebase is the foundational, non-component kind: design tokens, theme
palettes, `@font-face`, and `@keyframes` animations.

## Non-goals (CSS that stays, by design)

These are **not** component styling and stay as CSS — forcing them into
utilities makes them *less* readable, not more:

- **`baseStyles()`** (`layouts/layout.templ`): `:root` design tokens, the three
  `data-colorscheme` theme palettes, `@font-face`, the reset. Foundational.
- **`@keyframes` and their drivers**: the hamburger animations (`icon.templ`),
  the login amoeba spin (`login.templ`), the UB logo sway (`logo.templ`), the
  dialog blur-fade transition (`modals/dialog.templ`). Keyframes + the rules
  that set `animation:` / `transition:` on them stay co-located CSS.
- **`@property`-style SVG filters / `image-rendering`** in `logo.templ` and the
  `.icon-pixel` pixel-rendering hook.

## The playbook (techniques proven on `column.templ`)

Every remaining pattern maps to a Tailwind v4 feature. The migration is
mechanical once the mapping is known:

| CSS pattern | Markup-level equivalent |
|---|---|
| `.el.open { … }` (JS/Datastar-toggled class) | `[&.open]:…` arbitrary variant — class stays as the hook |
| `.parent:hover .child::after` | `group` on parent + `group-hover:after:…` on child |
| `:root[data-feeling="pixel"] .x` | `@custom-variant pixel (…)` in `input.css` → `pixel:hidden` |
| `[data-type="deep"]` | `data-[type=deep]:…` variant |
| `input:checked ~ .swatch` | `peer` on input + `peer-checked:…` on sibling |
| `:focus-within .active` | `group`/`peer` + `group-focus-within:…` / `peer-focus-within:…` |
| `::after { content:"" }` / `::backdrop` | `after:content-['']` / `backdrop:…` variant |
| `@starting-style { … }` | `starting:…` variant |
| `@media (pointer: coarse)` | `pointer-coarse:…` variant |
| `@media (width < 40rem)` | `max-sm:…` (exact rem complement of `sm:`) |
| `-webkit-line-clamp: var(--x)` | `line-clamp-(--x)` (expands the full `-webkit-box`) |
| render-time conditional (`s%2==1`) | compute in templ: `templ.KV("border-dashed", cond)` |
| token not bridged in `@theme` (`--gutter-ink`) | arbitrary property `text-(--gutter-ink)` |
| box-shadow token | `[box-shadow:var(--shadow-lift)]` (explicit, avoids the ring-composition utility) |

**Two load-bearing rules learned from the POC:**

1. **JS hooks stay.** Class names `drag.js` (or Datastar `data-class:*`) reads —
   `.block`, `.slot`, `.grip`, `.dragging`, `.open`, `.active` — must remain on
   the element. We remove the *CSS rules*, not the *class names*; utilities ride
   alongside the hook.
2. **Cascade is by source order + specificity, not `@scope`.** `@scope` gave
   structural rules an isolation boundary; utilities don't have one. `<head>`
   order is `baseStyles → output.css → page *Styles`, so the only
   order-sensitive conflicts are *within* `output.css`, which Tailwind orders
   deterministically (base before responsive/media variants; specificity does
   the rest). Verify the order-sensitive pairs in the generated CSS (see
   Verification) rather than assuming.

## Work breakdown — one PR per file, easiest first

Ordered so each PR is small and the riskier interaction-heavy files come after
the pattern is well-worn. Line counts are the current `@scope`/`<style>` size.

### 1. `modals/theme.templ` — `ThemeStyles()` (~16 lines) · trivial · ✅ done
The `.active` selection ring on theme buttons, toggled by Datastar
`data-class:active`. → `[&.active]:ring-2 [&.active]:ring-accent …` on the
button; delete `ThemeStyles()` and drop it from the `Layout` call in
`routes/blocks.templ`. Smallest possible warm-up.

### 2. `nav.templ` — `NavStyles()` (~53 lines) · low · ✅ done
- `.shell` (global flex wrapper) and `.shell .column { width:100% }` →
  utilities on the `.shell` div and the `column` `<main>`. Keep the `column`
  class (the `<main>` already carries it).
- `@scope (.sidenav)` rail geometry → utilities on `<nav class="sidenav">`.
- Mobile off-canvas drawer: base `translate-x-full` + `[&.open]:translate-x-0`
  (the `.open` class is Datastar-toggled via `data-class:open`), `max-sm:fixed
  max-sm:inset-y-0 max-sm:right-0 max-sm:w-[200px] max-sm:z-30
  transition-transform duration-200`.

### 3. `modals/create.templ` — `CreateStyles()` (~113 lines) · medium · ✅ done
Block-type swatches built on visually-hidden native radios. → the canonical
Tailwind **`peer`** pattern: `peer sr-only` on each `<input type="radio">`,
`peer-checked:ring-2 peer-checked:ring-accent peer-focus-visible:…` on the
adjacent swatch `<label>`. This is the textbook radio-card migration; most of
the 113 lines is repetition across three types that collapses to one shared
class string.

### 4. `login.templ` — `LoginStyles()` (~51 lines) · medium · ✅ done
- Keep the `@keyframes login-amoeba-spin` and the rule that applies it
  (animation + `prefers-reduced-motion` → `motion-reduce:animate-none`); the
  blob's absolute positioning migrates to utilities.
- `.otp:focus-within .otp-box.active` → `group`/`peer` focus-within: the real
  (invisible) input is the `peer`, the boxes use `peer-focus-within:[&.active]:…`
  (or make `.otp` a `group` and use `group-focus-within:`). The `.active` class
  stays as the per-box hook.

### 5. `modals/dialog.templ` — `DialogStyles()` (~96 lines) · medium-high · ✅ done
- Dialog chrome (padding, surface bg, border, `rounded-lg`, max-width) →
  straightforward utilities.
- `::backdrop` → **`backdrop:`** variant; the open transition `@starting-style`
  → **`starting:`** variant (both exist in v4). The blur-fade *keyframe/transition*
  itself is the judgment call — keep it as the small CSS island if the
  `starting:`/`backdrop:` utilities read worse than the named rule. Document
  whichever way it lands.

### 6. `icon.templ` — `IconStyles()` (~97 lines) · keep CSS (mostly)
**Recommendation: leave as-is.** It is ~90% `@keyframes` (jelly-wobble,
tetris-drop, X-morph) plus a 4-line feeling-toggle (`.icon`,
`:root[data-feeling="pixel"] .icon-solar { display:none }`). The animations stay
by the non-goals rule, and a `@custom-variant pixel` to move four
display-toggle lines to markup is low value and arguably worse. If we migrate
anything here, it's only the `.icon { width/height }` sizing, and only if it
stops being shared. Note this explicitly so the file isn't mistaken for
unfinished work.

### 7. `logo.templ` (~57 lines) · keep CSS
Sway `@keyframes` + pixel mosaic SVG filter. Non-component animation/filter —
stays. Note it as intentionally-CSS.

## Risks & gotchas

- **Specificity drop vs `@scope`.** A scoped bare-tag rule (`p`, `button`) had
  an isolation boundary; the equivalent utility is a flat single-class selector.
  Where a global rule (e.g. `.shell .column`, the global `.icon`) targets the
  same element with higher specificity, it still wins — confirm the global isn't
  silently overriding a new utility (the POC checked `.shell .column { width }`
  and the global `.icon { 24px }` this way).
- **`@custom-variant` for the feeling axis.** If we do migrate any
  `data-feeling` toggle, the variant must be added once to `input.css` and is a
  shared surface — treat it like a token, not a one-off.
- **Datastar `data-class:*` still toggles the hook class**, so arbitrary
  variants (`[&.open]:`, `[&.active]:`) keep working unchanged. Don't replace the
  signal wiring; only the CSS rule moves.
- **`@source "./**/*.templ"` already covers every file** — no Tailwind config
  change needed as classes move into markup.

## Verification (per PR)

1. `go build ./... && go vet ./internal/frontend/...` — compiles the regenerated
   templ.
2. `go test -race ./...` — and **fix brittle test assertions** that match exact
   `class="x"` literals (the POC changed `class="slot"` →
   `class="slot ` prefix + `data-slot=` matches in `adapter_test.go`). Grep the
   test for `class="` before starting.
3. **Inspect the generated `output.css`** for any order-sensitive pair the file
   introduces (state-vs-base, base-vs-`pointer-coarse`, shorthand-vs-longhand
   padding/margin): confirm source order + specificity resolve as intended.
   Tailwind silently drops classes it can't parse, so also confirm each new
   arbitrary utility actually emitted.
4. **Eyeball in the running dev server** (`task dev`, `:7331`) the
   interaction/viewport states automated checks can't cover — open/close the
   drawer and modals, focus the OTP comb, check a checked swatch, narrow below
   40rem. Do **not** run one-shot `task templ` while `task dev` is up.

## Documentation

Amend ADR 0006 (and the ADR 0008 cross-reference) to record the final state:
component styling is markup-first; CSS is reserved for tokens, theme palettes,
`@font-face`, and `@keyframes` (+ their `animation:`/`transition:` drivers) and
the rare `::backdrop`/filter the variants don't cover cleanly. Update CLAUDE.md's
"Theming & HTML practices" section, which currently says "`column.templ` stays
`@scope`-heavy" — that is no longer true.

## Acceptance criteria

- No `@scope` blocks remain in the tree except where a non-goal justifies CSS;
  each surviving `<style>` block has a one-line comment stating why it's CSS
  (keyframe / token / filter), so none reads as unfinished.
- Every removed `*Styles()` component is also removed from its route's `Layout`
  call.
- `go test -race ./...` green; `output.css` cascade spot-checked per file.
- ADR 0006/0008 and CLAUDE.md updated to the markup-first end state.

## Rollout

Six small PRs in the order above (theme → nav → create → login → dialog), with
icon/logo handled as a documentation-only "intentionally CSS" note. Each PR is
independently shippable and visually verifiable; no shared migration branch.
