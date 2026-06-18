# Component CSS scoped per leaf via native @scope

> **Superseded by ADR 0008 for component styling.** The "want reusable
> cross-component class names" revisit-trigger in the last sentence came true: we
> adopted utility-first Tailwind v4 and then migrated **all** component styling to
> the markup level (PRD-Finish-Tailwind), including the file this ADR piloted
> (`login.templ`) and the hardest one (`column.templ` — its ~290-line
> `ColumnStyles()` `@scope` block is gone, day grid and drag states and all). The
> leaf-only `@scope` rule below no longer governs any component styling. CSS now
> survives **only** for the foundational, non-component kind: design tokens and
> theme palettes (`baseStyles`), `@font-face`, `@keyframes` and their
> `animation:`/`transition:` drivers (the hamburger/logo/amoeba animations, the
> dialog blur-fade), and the rare `::backdrop`/SVG-filter the Tailwind variants
> don't cover cleanly — each kept in a small co-located `<style>`/`@scope` island
> with a one-line comment stating why. The few `@scope` blocks that remain
> (`login.templ`, `modals/dialog.templ`) hold only those islands, not component
> styling.

Component styles are co-located in each component's `*Styles` block but render
into one shared `<head>`, so class names share a global namespace — a `.status`
in login silently collided with a `.status` meant for another page. The fixes
considered: a naming convention (prefix every class, e.g. `.login-status`),
templ's built-in `css name() {…}` (hashed, truly isolated, but flat declarations
only — no nesting, pseudo-classes, media queries, or keyframes, and it injects
`<style>` inline in the body, violating our head-only CSS rule), or native CSS
`@scope`. We chose `@scope`, applied **per leaf component, never per page
wrapper**: `@scope (.login-main) { … }` confines the component's selectors to its
subtree, letting it drop class names in favor of bare element selectors (`p`,
`form`, `button`, `input[type="email"]`) for the things that are unambiguous
inside that subtree. Login is the pilot. The leaf-only rule is the load-bearing
constraint: `@scope` with no lower boundary reaches the entire subtree, so
scoping a wrapper that hosts other components (e.g. `.shell`, which contains the
nav and modals) would bleed bare-tag rules into them — scope the block list or a
modal body, not the page shell. Design tokens (`:root`), `@font-face`, and
`@keyframes` stay global by nature and live outside any scope. Cost: `@scope` is
Baseline 2024 (Chrome 118, Safari 17.4, Firefox 128), and scoped bare-tag
selectors are low specificity, so `baseStyles` must stay free of competing
element rules. Revisit if we ever want genuinely reusable cross-component class
names (`.card`, `.badge`), which `@scope` does not provide — that is when a
naming convention or templ's `css` blocks would earn their place alongside it.
