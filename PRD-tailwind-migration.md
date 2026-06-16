# PRD — Migrate CSS to utility-first Tailwind v4 (standalone binary)

> Decided in a grilling session; see `docs/adr/0008-utility-first-css-tailwind-v4.md`
> (and the amendment note on ADR 0006). This PRD is the rollout plan.

## Problem Statement

Styling the app means inventing single-use class names (`.slot-add`,
`.block-delete`, `.grip`) for one-off elements and maintaining the per-leaf
`@scope` machinery from ADR 0006. For routine spacing/layout/color work this is
tedious and adds cognitive overhead: the style lives in a separate `*Styles`
block from the markup it styles, the names carry no value, and there is no
reusable cross-component vocabulary. The developer wants styling to live *on* the
element (the Tailwind/Chakra model) and the `@scope` complexity abstracted away
where it isn't earning its keep.

## Solution

Adopt **utility-first CSS via Tailwind v4** (CSS-first, no JS config) using the
**standalone Tailwind binary** — a single pinned executable, no Node/npm, no
`node_modules`. Utilities handle the tedious 80% (spacing, layout, color,
typography on one-off elements) inline in the markup. The existing design tokens
and runtime theming stay exactly as they are, bridged into Tailwind via
`@theme inline` so `bg-surface` resolves to `var(--surface)` and the live
`data-colorscheme` swap keeps working. The genuinely structural/stateful CSS (day
grid geometry, drag states) stays as co-located `@scope` blocks. The migration is
incremental: a half-converted tree always works because utilities and `@scope`
coexist.

## User Stories

1. As the developer, I want to style a one-off element by adding utility classes
   inline, so that I no longer invent and maintain a single-use class name.
2. As the developer, I want the styling to live on the element in the markup, so
   that I don't jump between a `*Styles` block and the template to understand one
   element.
3. As the developer, I want to keep my three colorschemes and two font
   "feelings", so that the migration changes how I write styles, not how the app
   looks or themes.
4. As the developer, I want `data-colorscheme`/`data-feeling` swaps to remain
   instant and rebuild-free, so that live theming is exactly as responsive as
   before.
5. As the developer, I want no Node/npm toolchain introduced, so that I avoid
   dependency churn and lockfile maintenance.
6. As the developer, I want the Tailwind binary version pinned like the templ
   CLI, so that it bumps deliberately and never drifts across environments.
7. As the developer, I want `task dev` to regenerate CSS on `.templ` change and
   reload the browser, so that the dev loop feels the same as today.
8. As the developer, I want the CSS build to not trigger a Go rebuild, so that
   templ's text-only fast path is preserved.
9. As the developer, I want the generated CSS embedded and served as one cached
   `<link>`, so that SSE patches never re-ship CSS.
10. As the developer, I want tokens bridged via `@theme inline`, so that utilities
    reference live custom properties rather than frozen values.
11. As the developer, I want the day-grid geometry and `data-slot` rules to stay
    as named CSS, so that geometry-as-markup stays readable.
12. As the developer, I want the `.grip::after`, `.dragging`, and `.resizing`
    hooks to keep their class/attribute names, so that `static/drag.js` keeps
    working unchanged.
13. As the developer, I want to pilot the toolchain on `login` first, so that I
    prove the whole pipeline on a small, drag-free, theming-exercising component.
14. As the developer, I want to convert `column.templ` last, so that the riskiest
    hybrid file is touched only once the toolchain is trusted.
15. As the developer, I want the CSS build to scan `**/*.templ` only, so that the
    build depends on source, not generated `*_templ.go`.
16. As a user, I want the app to look and theme identically after the migration,
    so that the change is invisible to me.
17. As a user, I want no flash of unstyled/wrong-theme content, so that the
    pre-paint theme script and inline tokens must remain in `<head>`.
18. As the developer, I want the `/_smoke` canary to keep passing, so that the
    pinned Datastar + templ wiring stays verified through the change.

## Implementation Decisions

- **Tailwind v4, CSS-first, standalone binary.** No `tailwind.config.js`; config
  is a small `input.css` (`@import "tailwindcss";` + `@theme inline { … }`). No
  Node, no `node_modules`.
- **Version pinning mirrors the templ CLI convention:** pinned in the
  **Dockerfile** (download in build stage), the **Taskfile** (local install for
  `task dev`), and **CI** — bumped together. go.mod cannot hold a non-Go binary,
  so the Taskfile is the third anchor in place of go.mod.
- **Tokens unchanged.** `baseStyles`' `:root[data-colorscheme]` /
  `:root[data-feeling]` blocks, `@font-face`, and the pre-paint theme script stay
  verbatim and stay inline in `<head>`.
- **Token bridge uses `@theme inline`** (load-bearing keyword) so utilities
  resolve to `var(--…)` directly and runtime theming survives. One mapping line
  per token; `--font-sans: var(--font-family)` bridges the font feeling.
- **Hybrid end-state.** `@scope` survives for: the day grid
  (`grid-template-columns`, `grid-auto-rows`, per-slot `grid-row`), `data-slot`
  attribute selectors (dashed half-hour lines), pseudo-elements (`.grip::after`,
  ghost-block states), and drag state classes (`.dragging`, `.resizing`,
  `.block-delete` hooks) that `drag.js` reads. Utilities replace only one-off
  spacing/layout/color/typography.
- **Delivery.** One `go:embed`'d `output.css` served under `/static/` via a single
  `<link>` in `Layout`'s `<head>`. The variadic `styles ...templ.Component` param
  stays for the surviving `@scope` blocks.
- **Content scan: `**/*.templ` only** — never `.go`/`*_templ.go`. No utility class
  names are composed in Go string concatenation (confirmed).
- **Dev loop.** `task dev` gains a Tailwind `--watch` process (`-i input.css -o
  output.css --watch`) alongside `templ generate --watch --proxy`; browser reload
  on CSS change via templ's `--notify-proxy`. CSS regen does not trigger a Go
  rebuild (fast path intact).
- **Rollout order:** (1) toolchain wiring + token bridge + `<link>`/embed, app
  unchanged visually; (2) `login` pilot; (3) remaining components; (4)
  `column.templ` last.

## Testing Decisions

- A CSS migration has few automated unit seams; good verification here is
  behavioral and visual, not implementation-coupled.
- **`task build` stays green** (`templ generate` + `go build`) at every step — the
  primary structural guard.
- **`/_smoke` + `/_smoke/events` keep passing** — the existing wiring canary for
  the pinned Datastar SDK + templ versions; do not regress it.
- **Manual theme-swap verification** after the token bridge lands: cycle all three
  colorschemes and both feelings, confirm instant swap with no rebuild and no
  FOUC on hard reload (pre-paint script + inline tokens).
- **Drag/stretch manual check** after `column.templ`: confirm `drag.js` still
  reads `.dragging`/`.resizing`/`data-slot`/`.grip` and the layout/bounds round
  trips intact, since those hooks are deliberately kept as named CSS.
- No new Go test seams are introduced; existing `internal/block` tests are
  unaffected (no server logic changes).

## Out of Scope

- Any change to design tokens, palettes, fonts, or the theming model itself.
- Any change to server-side logic, the broker, SSE paths, or `block.Service`.
- Rewriting the day-grid or drag interaction; their CSS is deliberately retained.
- Introducing a Node/npm toolchain, a JS bundler, or a `tailwind.config.js`.
- A component library (templUI etc.) — utilities only.
- Removing `@scope` entirely — it is retained for structural CSS by design.

## Further Notes

- Reverses the literal "no build step" line in CLAUDE.md, scoped to CSS only; the
  no-Node / no-SPA / server-side-logic intent is preserved. CLAUDE.md's "Frontend
  gotchas" and "Theming & HTML practices" sections will need an edit to describe
  the utility-first + `@theme inline` model and the new watcher in `task dev`.
- ADR 0006 is partially superseded (see its amendment note); its leaf `@scope`
  rule still governs the retained structural CSS.
- The templ-pinned trio note in CLAUDE.md ("pinned identically in three places")
  gains a parallel for the Tailwind binary.
</content>
</invoke>
