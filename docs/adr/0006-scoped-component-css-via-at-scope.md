# Component CSS scoped per leaf via native @scope

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
