# Throwaway landing study — UNB-77

**Question:** can the login become a light, scroll-down introduction without burying sign-in or adding a wall of marketing copy?

```sh
task prototype:landing
```

Stop any existing `task dev` first. Open <http://localhost:7331/login?variant=A>.
The floating arrows (or ← / → outside inputs) switch directions:

- **A — Quiet reveal:** familiar centered login; alternating scenes below the fold.
- **B — Editorial:** README-style headline and tilted planner; type ribbon and broad workbench.
- **C — Field guide:** playable plan up front; indexed chapters for planning, Jotpad, and habits.

Uses the existing theme, block tokens, logo, guide miniature and drag/stretch driver.
Login is a visual stub: it sends no email. Notes, habits, and demo placements are in memory only.
The console prints the relevant state on load and demo edits. The scratch database is
`tmp/PROTOTYPE-wipe-me-landing.db`; no real app data is needed.

Normal `/login` is unchanged. Variants require both `LANDING_PROTOTYPE=1` and
`TEMPL_DEV_MODE`; the task enables them. No production route or switcher is enabled.

**Verdict: pending visual review.** No direction is validated yet. Keep this primary
source on `prototype/UNB-77-login-landing`, not main. Once a direction wins, implement
that decision properly and leave the alternatives on this branch.
