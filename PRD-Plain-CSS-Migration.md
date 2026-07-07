# PRD: Revert Tailwind to plain CSS

Working PRD for migrating component styling from Tailwind v4 utilities (ADR 0008)
to a single hand-authored stylesheet built on cascade layers and `@scope`.
Like `PRD-Finish-Tailwind.md` before it, this file is deleted once the
migration lands; the durable record is ADR 0011 (written in the final phase).

## Problem Statement

Component styling currently requires a pinned third-party binary (the Tailwind
standalone compiler), a generated artifact (`output.css`), a watch process in
the dev loop, and utility-class strings that grow unreadable at the hairy end
(the block `<li>` carries ~40 utilities with stacked variants). The maintainer
wants styling expressed as plain, modern CSS that leans on the cascade —
inheritance, layers, scoping — with no compile step, while keeping the Go-only
toolchain and everything the current system does well (live theme/feeling
swap, one cached stylesheet, SSE patches never re-shipping CSS).

## Solution

One committed, hand-authored `app.css`, `go:embed`-served as the single
render-blocking `<link>`. Cascade layers (`reset, tokens, base, layout,
components`) replace specificity management; `@scope` blocks per leaf
component (ADR 0006 revived) replace utility strings; a small shared class
tier (`.btn`, `.field`, `.option-row`, …) fixes the reuse gap that sank ADR
0006 the first time. All existing CSS islands (`baseStyles`, `IconStyles`,
`DialogStyles`, `LoginStyles`) fold into the same file, so `layouts.Layout`
drops its variadic `styles` plumbing. Tailwind and its build wiring are
removed entirely.

Migration is incremental: `app.css` loads *before* `output.css`, pinning its
cascade layers below Tailwind's `utilities` (layer order is fixed by first
appearance), so both systems coexist safely while components migrate one at
a time.

## User Stories

1. As the maintainer, I want component styles in plain CSS files, so that I can read and edit styling without decoding utility strings.
2. As the maintainer, I want zero CSS build step, so that `task dev` runs fewer processes and a clean checkout needs only `templ generate`.
3. As the maintainer, I want the Tailwind standalone binary gone, so that the pinned-download supply-chain concern (docs/backlog/004) disappears.
4. As the maintainer, I want cascade layers to own ordering, so that I never fight specificity or source order.
5. As the maintainer, I want `@scope` blocks named after the templ file they style, so that markup and styles stay findable from either side.
6. As the maintainer, I want a shared class tier for buttons/fields/option rows, so that cross-component chrome is reused instead of re-declared (the ADR 0006 failure mode).
7. As the maintainer, I want the migration done per-component with both systems live, so that each phase is a small, visually verifiable diff.
8. As an app user, I want the app to look and behave pixel-identically after each phase, so that the migration is invisible to me.
9. As an app user, I want theme and feeling swaps to stay live without a reload, so that the token system keeps working as before.
10. As an app user, I want the stylesheet cached as one file, so that page loads stay fast and SSE patches stay markup-only.
11. As a keyboard/AT user, I want focus rings, `.sr-only` text, and the APG resize/drag semantics preserved, so that accessibility does not regress.
12. As a touch user, I want the coarse-pointer affordances (taller slots, always-visible delete/grip) preserved, so that mobile ergonomics do not regress.
13. As a contributor (or agent), I want CLAUDE.md and the ADRs to describe the new styling rules, so that future changes follow one system, not two.
14. As the CI pipeline, I want the Tailwind install/build steps removed, so that builds are faster and have one less network dependency.

## Implementation Decisions

- **One stylesheet.** All CSS lives in a committed `app.css` under the
  embedded static dir, served as today's single cached `<link>`. The head-only
  CSS rule and "SSE patches never re-ship CSS" invariant are unchanged.
- **Layer order** declared once at the top: `@layer reset, tokens, base,
  layout, components;`. Component state styling nests inside its component
  block (`&.dragging { … }`) rather than a separate layer.
- **`reset` replaces Tailwind preflight deliberately.** The markup assumes
  preflight (heading font-size inheritance, zeroed button/dialog chrome,
  `dialog { margin: auto }` for top-layer centering). ~30 intentional lines,
  written in phase 1 so every migrated component lives under it from day one.
- **`tokens` absorbs `baseStyles`**: colorscheme palettes, feeling fonts,
  `@font-face`, plus new type-scale variables (`--text-sm/-base/-lg/-xl`)
  replacing Tailwind's text sizes. Attribute-driven theming
  (`:root[data-colorscheme]`, `[data-feeling]`) is untouched.
- **`components` has two tiers**: a small closed set of shared classes
  (`.btn`, `.btn-secondary`, `.btn-danger`, `.option-row`, `.field`) replacing
  today's Go class-string constants, and one `@scope` block per leaf
  (`.blocks`, `.sidenav`, `.app-dialog`, `.login-main`) using bare element
  selectors inside. Leaf-only scoping per ADR 0006 — never scope `.shell`.
  `@keyframes` and the icon/feeling toggles stay at layer level (global by
  nature). Each block carries a section comment naming its templ file — the
  drift countermeasure, greppable from either side.
- **templ markup collapses to semantic hooks.** The JS hooks (`.block-item`,
  `.grip`, `.slot-add`, `.sidenav`, …) become the styling anchors; class
  constants in Go are deleted; conditional classes use `templ.KV`. No templ
  `css` blocks (flat-only, body-injected — rejected in ADR 0006).
- **`layouts.Layout` simplifies**: `baseStyles()` and the variadic
  `styles ...templ.Component` parameter are removed once all islands fold in;
  routes stop passing style components.
- **Key idiom translations**: `data-[type=…]` variants → `&[data-type="…"]`;
  responsive prefixes → nested `@media (width < 40rem)` range syntax;
  `pointer-coarse:` → `@media (pointer: coarse)`; `group-*`/`peer-*` →
  descendant/sibling selectors (and `:has()` where a parent must react);
  `line-clamp-(--span)` → `-webkit-box` clamp with `var(--span)`
  (unprefixed `line-clamp` is not Baseline yet); `open:flex` → `&[open]`.
  Already-modern pieces (`color-mix()`, `@starting-style`, `::backdrop`)
  carry over verbatim.
- **Coexistence ordering**: during migration `app.css` links **before**
  `output.css`. Layer order is fixed by *first appearance*, so linking first
  pins every app.css layer below Tailwind's `utilities` — unmigrated markup
  keeps its utilities untouched, while migrated components (which carry no
  utility classes) still beat preflight because `components` outranks `base`.
  (Linking after — the original plan — would append `reset`/`tokens`/`layout`
  *above* `utilities`, and the reset's universal margin/padding zeroing would
  clobber every spacing utility in the app.) Bonus: app.css is in its final
  form from day one; teardown just deletes the `output.css` link.
- **The unstyled `.past` hook** (toggled by the now-pill script on block
  labels) is resolved during the column phase: style it or delete the toggle.
- **Teardown removes every Tailwind artifact**: input stylesheet, generated
  output, install/build tasks, `TAILWIND_VERSION`, Dockerfile and CI steps,
  the tailwind arm of `check:versions`, and the gitignore entry.
- **ADR 0011** records the decision: supersedes ADR 0008, updates ADR 0006's
  status note, and names the lesson — scoped leaf CSS *plus* a deliberate
  shared-class tier. CLAUDE.md's styling sections are rewritten to match;
  docs/backlog/004 closes.

## Testing Decisions

- Behavior, not implementation: no CSS unit tests. `task test` (Go +
  `node --test`) must stay green each phase — it covers the JS the styles
  hook into (push cascade, keyboard a11y), not visuals.
- The real gate is a **visual verification matrix per phase**: 3 colorschemes
  × 2 feelings × desktop/mobile, plus the CSS-participating interactions —
  drag lift/shadow, resize cue + label fade, delete/grip reveal (hover and
  coarse pointer), dialog open blur-fade + backdrop, drawer slide, focus
  rings (inset on blocks), OTP active-box ring, reduced-motion off-switches.
- `/_smoke` stays as the Datastar wiring canary; it has no styling stake.
- Lighthouse/devtools spot-check after teardown: one stylesheet request,
  no 404 for `output.css`, no FOUC on a cold load with a dark theme stored.

## Out of Scope

- Any visual redesign — every phase targets pixel parity; improvements come
  after the migration lands.
- Restructuring templ components, routes, Datastar signals, or the JS
  (`drag.js`, `push.js`, `keys.js`, `now-pill.js` keep their class hooks).
- Multiple stylesheets / per-component CSS files (considered; single-file
  chosen), CSS preprocessing of any kind, and unstandardized features
  (custom media queries).
- Touching the token *values* or adding themes/feelings.

## Further Notes

- Rough size: `app.css` lands around 450–600 lines. templ diffs are large but
  mechanical.
- Phase 2 (login) is deliberately the pilot — smallest surface, and it was
  ADR 0006's pilot too, so it proves the revived pattern before the column
  work.
- Idiomorph note: server patches overwrite `class` wholesale, wiping
  JS-toggled state classes — already true today; unchanged so long as page
  and patch keep sharing one component.

---

## Migration Checklist

### Phase 1 — Scaffold (foundation, coexistence)

- [x] Create committed `app.css` in the embedded static dir with the layer
      order declaration and section-comment skeleton
- [x] Write the `reset` layer (preflight replacement: box-sizing, margin
      zeroing, `font: inherit` on controls, button/list/dialog resets)
- [x] Add `base` layer: `.sr-only`, global `:focus-visible` ring (moved from
      `input.css`), body shell rules
- [x] Add `--text-*` type-scale tokens
- [x] Link `app.css` in `Layout` **before** `output.css` (see Coexistence
      ordering — linking after would hoist `reset` above `utilities`)
- [x] Verify: full matrix renders identically with both stylesheets live

### Phase 2 — Login (pilot)

- [x] Port login form, OTP boxes, amoeba layout to `@scope (.login-main)`;
      fold `LoginStyles` keyframes into `app.css`
- [x] Replace `group-focus-within` OTP pattern with `:focus-within` descendant
      selectors
- [x] Strip Tailwind utilities from the login templ files
- [x] Verify: login flow visually + reduced-motion + both feelings

### Phase 3 — Modals

- [x] Add shared tier: `.btn`, `.btn-secondary`, `.btn-danger`, `.field`,
      `.option-row`; delete the Go class-string constants in `modals`
- [x] Port dialog chrome to `@scope (.app-dialog)`; fold `DialogStyles`
      blur-fade island into `app.css` (include `margin: auto` in the scoped
      block — preflight outranks our `reset` until teardown, so the reset's
      `dialog { margin: auto }` won't center dialogs on its own yet)
- [x] Rewrite create-modal radio swatches from `peer-*` to
      `input:checked + …` / `:has()` selectors
- [x] Port theme/hours/clear/create modal bodies; strip utilities
- [x] Verify: all four modals, open transition, backdrop, keyboard focus order

### Phase 4 — Nav

- [x] Port rail↔drawer geometry to `@scope (.sidenav)` with nested
      `@media (width < 40rem)`; delete `sidenavClasses`/`navlinkClasses`
- [x] Port `MenuToggle` + scrim; fold `IconStyles` (icons, hamburger
      animations, feeling toggles) into `app.css`
- [x] Verify: drawer slide, scrim, hamburger morph both feelings,
      disabled Clear state

### Phase 5 — Column & blocks (the big one)

- [ ] Port `BlockColumn` grid, slots, gutter, slot-add, now-pill into
      `@scope (.blocks)`
- [ ] Port `blockItem`: type fills via `&[data-type=…]`, drag/resize states
      via `&.dragging`/`&.resizing`, grip + delete reveal, inset focus ring,
      line-clamp on `var(--span)`, coarse-pointer variants
- [ ] Port `DateHeading` and the `blocks.templ` page shell
- [ ] Resolve the unstyled `.past` hook (style it or remove the JS toggle)
- [ ] Verify: full matrix + drag lift, push preview, resize cue, keyboard
      grab/move/resize, delete reveal on hover and touch

### Phase 6 — Teardown & docs

- [ ] Move token palettes/fonts from `baseStyles()` into the `tokens` layer;
      delete `baseStyles` and the `styles ...templ.Component` param from
      `Layout`; update routes
- [ ] Remove the `output.css` `<link>`; delete `input.css`
- [ ] Taskfile: delete `tailwind:install` + `css` tasks, `TAILWIND_*` vars,
      the watch process in `dev`, the `css` dep in `build`
- [ ] Remove `TAILWIND_VERSION` from `versions.env`; strip the tailwind arm
      of `check:versions`
- [ ] Remove Tailwind steps from the Dockerfile and CI workflow
- [ ] Drop the `output.css` gitignore entry; ensure no stale generated file
      ships
- [ ] Write ADR 0011 (supersedes 0008; status note on 0006); update
      CLAUDE.md styling/commands sections; close docs/backlog/004
- [ ] Final verify: full matrix on a clean checkout + deploy preview;
      cold-load FOUC check with a dark theme stored
- [ ] Delete this PRD file
