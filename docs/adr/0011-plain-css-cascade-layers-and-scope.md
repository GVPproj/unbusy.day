# Plain CSS via cascade layers and @scope in one hand-authored stylesheet

Component styling moves from Tailwind v4 utilities in the markup (ADR 0008)
back to plain CSS: a single committed `internal/frontend/static/app.css`,
`go:embed`-served as the one render-blocking `<link>`. This supersedes ADR
0008 and removes the CSS build step entirely — a clean checkout needs only
`templ generate`. The decision originally lived in a working PRD
(`PRD-Plain-CSS-Migration.md`, deleted after the migration landed on
2026-07-07).

## Context

Tailwind delivered markup-first styling but at a recurring cost: a pinned
third-party binary (the standalone compiler — a supply-chain surface,
docs/backlog/004), a generated artifact (`output.css`), a watch process in
`task dev`, version pins in three places, and utility strings that grew
unreadable at the hairy end (the block `<li>` carried ~40 utilities with
stacked variants). Meanwhile the CSS the markup actually needed became
expressible in plain, Baseline CSS: cascade layers, `@scope`, nesting,
`color-mix()`, `@starting-style`.

## Considered Options

- **Keep Tailwind v4 standalone** — rejected: the binary, the generated
  artifact, and the watch process are permanent toolchain weight for styling
  that no longer needs a compiler.
- **Per-component `@scope` islands in templ files (ADR 0006 as-was)** —
  rejected: that design failed once on the reuse gap (no shared tier meant
  cross-component chrome was re-declared per component) and on head-only
  ordering pressure.
- **One hand-authored stylesheet: layers + `@scope` + a shared class tier**
  — chosen. ADR 0006's scoping idea revived, plus the missing piece.

## The design

- **One file.** All CSS lives in `app.css`; SSE patches never re-ship CSS
  (unchanged invariant). `task dev` serves it from disk (TEMPL_DEV_MODE), so
  edits land on reload with no build.
- **Cascade layers own ordering**: `@layer reset, tokens, base, layout,
  components;`. `reset` is a deliberate ~30-line preflight replacement the
  markup assumes; `tokens` holds `@font-face`, feeling fonts, colorscheme
  palettes, and the type scale (`--text-*`, same metrics as Tailwind's so
  the migration didn't move a pixel); `base` has the body shell, default
  focus ring, `.sr-only`; `layout` the page shell; `components` everything
  else.
- **`components` has two tiers**: a deliberately small closed set of shared
  classes (`.btn`, `.btn-secondary`, `.btn-danger`, `.field`, `.option-row`)
  — the ADR 0006 countermeasure — and one `@scope` block per leaf component
  (`.login-main`, `.app-dialog`, `.sidenav`, `.blocks`), bare element
  selectors inside, each section comment naming the templ file it styles.
  Leaf-only scoping; never scope the page wrapper.
- **Markup carries semantic hooks only.** The JS hook classes (`.block-item`,
  `.grip`, `.slot-add`, `.open`, `.dragging`, …) double as styling anchors;
  conditional classes use `templ.KV`. No templ `css` blocks (flat-only,
  body-injected — rejected in ADR 0006).
- **Key idiom translations** from the utility era: `data-[type=…]` →
  `&[data-type="…"]`; responsive prefixes → nested `@media (width < 40rem)`
  range syntax; `pointer-coarse:` → `@media (pointer: coarse)`;
  `group-*`/`peer-*` → descendant/sibling selectors and `:has()`;
  `line-clamp-(--span)` → `-webkit-box` clamp with `var(--span)` (unprefixed
  `line-clamp` is not Baseline yet).

## Consequences

- Zero CSS build step: Tailwind's binary, `input.css`, `output.css`,
  `versions.env`, the Taskfile/Dockerfile/CI download steps, and the
  `check:versions` tailwind arm are all gone. The binary leg of
  docs/backlog/004's supply-chain exposure closes with it.
- `layouts.Layout` lost its variadic `styles` parameter — there are no
  per-page style islands left to pass.
- New styles must use the design tokens and land in the right layer; a new
  leaf component gets a new `@scope` block named after its templ file.
  Cross-component chrome goes in the shared tier — growing that tier is a
  deliberate act, not a default (the ADR 0006 lesson: scoped leaf CSS works
  *only* alongside an explicit reuse tier).
- Theme/feeling swap, one-cached-stylesheet, and head-only CSS all carry
  over unchanged. The migration was verified pixel-identical per phase
  (the one deliberate change: `.past` blocks now render struck through,
  matching now-pill.js's documented intent).
- ADR 0006's status updates to "revived by ADR 0011 in amended form";
  ADR 0008 is superseded.
