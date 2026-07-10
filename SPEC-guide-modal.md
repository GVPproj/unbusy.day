# Spec — Guide Modal

Status: ready-for-agent
Date: 2026-07-10
Branch: `feat/guideModal`
Research input: `docs/research/001-cal-newport-time-blocking-and-app-mapping.md`

---

## Problem Statement

A visitor landing on unbusy.day — either on the login page or newly inside the
app — has no in-product explanation of *why* they'd want to time-block their day
or *how* this particular column works. The concept (Cal Newport's time-blocking)
and the app's mechanics (bounded day, named blocks, deep/shallow/break types,
push cascade, snap-back) are non-obvious, and there is nowhere in the product
that teaches them. The research exists but lives only in a repo doc.

## Solution

A short, self-contained **Guide** modal — a four-step walkthrough of the why and
how, at a basic level — reachable on demand from two places:

- the app's **nav** (a "Guide" item between Theme and Log Out), and
- the **login page** (an outlined "Why?" button under "Send code").

It is a native `<dialog>` in the established modal style (`commandfor`/`command`
invokers, `closedby="any"`), stepped by a client-only Datastar signal, with
progress dots and Back/Next→Done controls. Two of the four panes carry a small
non-interactive figure built from the app's own theme tokens. Content is
paraphrased in our own voice and attributed to Cal Newport without outbound
links. No server state, no persistence — purely presentational.

## User Stories

1. As a logged-out visitor on the login page, I want a "Why?" button under "Send
   code", so that I can learn what the app is for before committing my email.
2. As a logged-out visitor, I want the "Why?" button to be visually distinct
   from and subordinate to "Send code" (outlined, same footprint), so that the
   primary action stays obvious.
3. As a logged-out visitor, I want opening the Guide to not submit the email
   form, so that I can read first and sign up after.
4. As a logged-out visitor, I want to dismiss the Guide and land back on the
   login form unchanged, so that my flow isn't disrupted.
5. As a logged-in user, I want a "Guide" item in the nav between Theme and Log
   Out, so that I can revisit the explanation any time.
6. As a logged-in user on mobile, I want the nav Guide button to close the drawer
   when tapped, so that the modal isn't hidden behind the open drawer.
7. As any user, I want the Guide to open on **step 1 (Why time-block)**, so that
   I always start at the beginning regardless of where I left off previously.
8. As any user, I want step 1 to explain that time-blocking means assigning every
   stretch of the day to a specific job instead of reacting, so that I grasp the
   core idea.
9. As any user, I want step 1 to state the payoff (a planned ~40-hour week does
   the work of an unstructured 60), so that I see why it's worth the effort.
10. As any user, I want step 1 to set the honest expectation that the plan will
    break and that's fine — you re-block — so that I don't treat it as a rigid
    cage.
11. As any user, I want step 2 to explain fixing my hours first and then adding
    named blocks, so that I understand the container-then-fill workflow.
12. As any user, I want step 2 to explain the three block types (deep, shallow,
    break) with their color swatches, so that I know what each tag means and can
    recognize them on the plan.
13. As any user, I want step 3 to explain dragging to move, stretching to resize,
    and that neighbors push out of the way, so that I know how to shape the day.
14. As any user, I want step 3 to frame rearranging as re-blocking when reality
    changes, so that I understand the plan is meant to be rewritten.
15. As any user, I want step 4 to explain that an overflowing move snaps back
    because the day is finite, so that the snap-back reads as intentional, not a
    bug.
16. As any user, I want step 4 to mention Clear for starting a fresh day, so that
    I know how to reset.
17. As any user, I want a closing attribution line crediting Cal Newport's
    time-blocking method, so that the ideas' provenance is clear.
18. As any user, I want progress dots showing which of the four steps I'm on, so
    that I know how far along I am.
19. As any user, I want a Back button that's unavailable on step 1, so that I
    can't step before the start.
20. As any user, I want a Next button that advances, and that becomes "Done" on
    the last step to close the modal, so that finishing is a single clear action.
21. As any user, I want to dismiss the Guide with Escape or a click outside
    (light dismiss), so that it behaves like the app's other modals.
22. As any user, I want the Guide to reset to step 1 whenever it closes, so that
    my next open starts fresh.
23. As a user of any theme (three colorschemes, two feelings), I want the Guide
    and its figures to render correctly in my current theme, so that nothing
    looks out of place.
24. As a keyboard or screen-reader user, I want the Guide to be a labelled dialog
    with focusable controls, so that I can read and navigate it without a mouse.
25. As a Safari user on a version without native invoker commands, I want the
    "Why?" and "Guide" buttons to still open the modal, so that the feature works
    on the login page too (the invoker fallback must be present there).

## Implementation Decisions

**New shared modal component.** A new `GuideModal()` templ component under
`internal/frontend/components/modals/` (its own `guide.templ`, per the
one-file-per-feature templ convention). It is self-contained: it must **not**
reference app-only signals (e.g. `$firstOccupiedSlot`) because it also renders on
the login page.

**Two mount points.**
- App: added to `routes/blocks.templ` alongside the existing modals.
- Login: added to `routes/login.templ` at the `.login-main` level, **outside**
  `#login-form` so the email→code SSE morph never touches the dialog.
- `DialogInit()` (the `invoker-fallback.js` loader) must also be mounted on the
  login page, which does not currently include it. The app page already has it.

**Native `<dialog>`, established chrome.** `<dialog id="guide-modal" class=
"app-dialog guide-dialog" closedby="any" aria-labelledby="guide-modal-title">`.
Opened by invoker buttons (`commandfor="guide-modal" command="show-modal"`),
consistent with Theme/Hours/Clear/Create.

**Step mechanism — client-only signal.** A single Datastar client-only signal
`$_guidestep` (underscore = never shipped to the server), declared on the dialog
as `data-signals:_guidestep="1"`, 1-indexed. This matches the existing
client-only UI signals (`$_navopen`, `$_colorscheme`, `$_feeling`). There is no
native multi-step primitive, so a client signal is the honest minimum; no server
round-trip, no persistence.

**Four panes**, each shown via `data-show="$_guidestep === N"`:
1. **Why time-block** — assign every stretch to a job vs. reacting; the ~40h≈60h
   payoff; the "plan will break — you re-block" caveat.
2. **Set hours & add blocks** — fix the container, then drop named blocks;
   deep / shallow / break. *(Figure: the three type swatches.)*
3. **Shape & re-block** — drag to move, stretch to resize, neighbors push;
   rewrite as the day changes.
4. **Time is finite** — overflow snaps back (that's the point); Clear for a fresh
   day; closing attribution line "Based on Cal Newport's time-blocking method"
   (no links). *(Figure: snap-back / overflow motif.)*

**Content.** Paraphrased in our own voice; attributed to Cal Newport / *Deep
Work* by name; **no quotes, no outbound links**. Basic level, terse panes.

**Figures — bespoke, non-interactive, token-driven.** Only panes 2 and 4 carry a
figure. Figures are small, purpose-built, `aria-hidden` decorative HTML styled
from existing custom properties (`--type-deep/-shallow/-break` and chrome
tokens). They **do not** reuse `BlockColumn`/`column_block` (which would drag in
slot math, drag wiring, and a colliding second `#block-list`) and are **not**
screenshots (which would freeze one theme and add binary assets).

**Footer controls.** Four progress dots (`data-class:active` from `$_guidestep`,
indicator-only, not clickable); a **Back** button (secondary, hidden on step 1
via `data-show`); a **Next** button (primary) that shows **Done** on step 4. Next
does `$_guidestep++`; Back does `$_guidestep--`; Done closes the dialog. Reuse the
existing `.btn` / `.btn-secondary` classes.

**Reset on close.** The dialog carries `data-on:close="$_guidestep = 1"` — a
single source of truth so every fresh open starts at step 1, regardless of which
invoker opened it or how it was dismissed.

**Nav wiring.** A new "Guide" `<button>` in `components/nav.templ`, placed
**between Theme and Log Out**, with `commandfor="guide-modal" command="show-modal"`
and `data-on:click="$_navopen = false"` (close the mobile drawer, matching the
other nav modal buttons). Label "Guide".

**New icon.** A new `IconGuide()` (question-mark-in-circle) in
`components/icon.templ`, matching the existing icon conventions (24×24 viewBox,
`currentColor`, `aria-hidden`).

**Login "Why?" button.** Inside `LoginEmailForm` (in `components/login.templ`),
below "Send code": an **outlined** button, **same footprint as "Send code"**,
`type="button"` (must not submit the form), invoker
`commandfor="guide-modal" command="show-modal"`, label "Why?".

**CSS (single `app.css`, tokens only, existing layers/scopes).**
- Extend `@scope (.app-dialog)`: a `.guide-dialog` width modifier
  `min(440px, calc(100vw - 2rem))`; `.guide-dots` styling; a `.guide-figure`
  sub-tree for the two figures.
- Extend `@scope (.login-main)`: an outlined button variant for "Why?" sized to
  match "Send code".
- No hardcoded colors; reuse `--type-*`, `--accent`, `--ink`, `--surface`, etc.

**Codegen & build.** Run `task templ` (one-shot, **not** while `task dev` is
running) after editing templ files, then `go build` / `go test`. `*_templ.go` is
generated and git-ignored.

## Testing Decisions

**What a good test is here.** This feature is presentational and declarative —
its step logic is Datastar attributes, not custom Go or JS — so tests pin
**server-rendered structure and wiring**, i.e. external contract, not internal
mechanics. The step-through interaction (dots advancing, panes toggling) is
Datastar's own behavior and is verified manually / with `/verify`, exactly as
`drag.js` keyboard glue is ("verified manually" per `accessibility_test.go`).

**Seam — one, and the highest available.** Reuse the existing render-and-assert
seam in `internal/frontend/*_test.go` (render a `routes.*Page(...)` component to
a string and assert on the HTML). No new seam is introduced. Prior art:
`accessibility_test.go` (`renderPage` helper), `login_test.go`, `smoke_test.go`.

**Modules tested (render assertions).**
- `routes.BlocksPage` renders `id="guide-modal"` once, with the four pane markers
  and the nav "Guide" invoker button (`commandfor="guide-modal"`).
- `routes.LoginPage` renders `id="guide-modal"` and the "Why?" invoker button,
  the invoker-fallback script (`DialogInit`) is present, and the dialog sits
  **outside** `#login-form` (assert the dialog markup is not within the
  `#login-form` element — mirror the `blockListElement` outside-the-patch-target
  technique in `accessibility_test.go`).
- The "Why?" button is `type="button"` (does not submit the form).

**Manual / `/verify` coverage** (not unit-tested): dots advance, Back disabled on
step 1, Next→Done closes, reset-to-1 on reopen, light dismiss, and correct
rendering across all three colorschemes × two feelings.

## Out of Scope

- **First-run / onboarding auto-open.** No "has seen guide" server state, no
  migration, no auto-showing for new users. On-demand only. (Possible follow-up.)
- **Clickable progress dots**, a "Skip" affordance, or a step counter — dots +
  Back/Next only.
- ~~**Interactive / live demo** figures (draggable mock blocks). Figures are
  static.~~ *(Revised 2026-07-10: pane 3's column is now a live FE-only demo —
  drag/stretch via `static/guide-demo.js` reusing `push.js`, committed nowhere.
  Pane 4's overflow motif stays static.)*
- **Quotes or outbound links** to calnewport.com from the modal.
- **Reusing `BlockColumn`** or any slot-indexed grid markup inside the modal.
- **Weekly / quarterly / shutdown-ritual** content — the app is the daily layer
  only; the guide teaches only what the app does.
- Content differing between the login and app mounts — one shared component,
  identical content in both places.

## Further Notes

- The research doc (`docs/research/001-...`) is the content source of truth; Part
  C's seven beats are folded into the four panes agreed here.
- Bounds examples in copy should stay inside a daytime window (the app's legal
  range is 4:00–18:00).
- Keep templ comments succinct and only where they state a constraint or a
  browser quirk (per repo code style).
- Commit with `git commit -s` (DCO enforced).

---

## Working Checklist

### 1. Content & component
- [ ] Draft the four panes' copy (terse, own-voice, Newport-attributed, no links)
      from the research doc's Part C.
- [ ] Create `internal/frontend/components/modals/guide.templ` with
      `templ GuideModal()`: `<dialog id="guide-modal" class="app-dialog
      guide-dialog" closedby="any" aria-labelledby="guide-modal-title"
      data-signals:_guidestep="1" data-on:close="$_guidestep = 1">`.
- [ ] Add the four panes, each `data-show="$_guidestep === N"`, with
      `guide-modal-title` on the dialog heading.
- [ ] Build the bespoke `aria-hidden` `.guide-figure` for pane 2 (three
      `--type-*` swatches) and pane 4 (snap-back / overflow motif).
- [ ] Add the footer: progress dots (`data-class:active`), Back
      (`data-show="$_guidestep > 1"`, `$_guidestep--`), Next/Done
      (`$_guidestep++` until step 4, then `command="close"`).

### 2. Wiring — app
- [ ] Add `IconGuide()` (question-mark-circle) to `components/icon.templ`.
- [ ] Add the "Guide" button to `components/nav.templ` between Theme and Log Out
      (`commandfor`/`command` + `data-on:click="$_navopen = false"`).
- [ ] Mount `@modals.GuideModal()` in `routes/blocks.templ`.

### 3. Wiring — login
- [ ] Add the outlined `type="button"` "Why?" button under "Send code" in
      `components/login.templ` (`LoginEmailForm`), invoking `guide-modal`.
- [ ] Mount `@modals.GuideModal()` and `@modals.DialogInit()` in
      `routes/login.templ`, outside `#login-form`.

### 4. CSS (`internal/frontend/static/app.css`, tokens only)
- [ ] `@scope (.app-dialog)`: `.guide-dialog` width modifier
      (`min(440px, calc(100vw - 2rem))`), `.guide-dots`, `.guide-figure` sub-tree.
- [ ] `@scope (.login-main)`: outlined "Why?" button variant sized to match
      "Send code".

### 5. Codegen, tests, verify
- [ ] `task templ` (ensure `task dev` is **not** running), then `go build ./...`.
- [ ] Add render tests in `internal/frontend/`: guide-modal + nav invoker in
      `BlocksPage`; guide-modal + "Why?" invoker + `DialogInit` + dialog-outside-
      `#login-form` + `type="button"` in `LoginPage`.
- [ ] `task test` (Go + node) green.
- [ ] `/verify` manually: dots advance, Back hidden on step 1, Next→Done closes,
      reset-to-1 on reopen, light dismiss, all three colorschemes × two feelings,
      login "Why?" doesn't submit the form.
- [ ] `git commit -s`.
