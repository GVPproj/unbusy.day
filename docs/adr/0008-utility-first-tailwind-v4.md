# Utility-first CSS via the Tailwind v4 standalone binary

> **Superseded by ADR 0011.** Component styling migrated back to plain CSS —
> one hand-authored `app.css` on cascade layers and `@scope` with a shared
> class tier — and Tailwind (binary, input/output CSS, build wiring) was
> removed on 2026-07-07.

Component styling moves from co-located `@scope` CSS blocks (ADR 0006) to
Tailwind v4 utility classes written directly in the templ markup. The compiler
is the official **standalone binary** — no Node, no npm, preserving the
Go-only toolchain. Its version is pinned in `versions.env` (shell-sourceable,
read by the Taskfile, Dockerfile, and CI). This supersedes ADR 0006 for
component styling.

Written retroactively (the decision originally lived in a working PRD,
`PRD-Finish-Tailwind.md`, deleted after the migration landed on 2026-06-18);
several docs cite this ADR as the no-Node / markup-first decision record.

## Considered Options

- **Keep per-component `@scope` CSS (ADR 0006)** — rejected: the predicted
  revisit-trigger ("want reusable cross-component class names") came true, and
  styles kept drifting from the markup they styled.
- **Tailwind via Node/npm** — rejected: reintroduces the JS toolchain the
  architecture deliberately excludes.
- **Tailwind v4 standalone binary** — chosen: one pinned executable, utilities
  live in the markup they style.

## Consequences

- Style one-off elements with inline utilities; don't invent single-use class
  names. State classes JS toggles (`.open`, `.dragging`, `.active`) stay as
  hooks, styled via arbitrary variants (`[&.open]:…`).
- Design tokens are bridged via `@theme inline` in `frontend/input.css`, so
  `bg-surface` resolves to `var(--surface)` and the live theme swap keeps
  working. The content scan covers `**/*.templ` only.
- Generated `output.css` is git-ignored, `go:embed`-served as one cached
  `<link>`; SSE patches never re-ship CSS. `task dev` runs `tailwindcss
  --watch`.
- CSS survives only for foundational, non-component things — design tokens and
  theme palettes, `@font-face`, `@keyframes`/animation drivers, the rare
  `::backdrop`/SVG filter — each a small co-located `<style>`/`@scope` island
  with a one-line comment saying why it's CSS.
- A pinned downloaded binary joins the supply-chain surface — see
  docs/backlog/004.
