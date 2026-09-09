# PR #7 — Habit Tracking Fixes and Maintainability Plan

Status: implemented and verified under UNB-46; two-axis review complete.

Review baseline: PR [#7](https://github.com/GVPproj/unbusy.day/pull/7), head
`8a80bd6`, compared with `main` at `f41d184`.

## Problem Statement

Habit tracking largely meets its requirements, but three remaining behaviors
undermine trust in an otherwise server-authoritative interface:

- A Check-in can commit and appear checked, yet a failed follow-up read invites
  the User to retry by clicking the same date. That click can uncheck the date
  instead of reconciling the successful write.
- An untouched creation form defaults to yesterday if the page remains open
  overnight, silently backdating a new Habit.
- A live matrix mutation can reset horizontal scrolling before the latest scroll
  position has been captured.

The review also identified four cleanup needs: matrix styles leak into nested
Habit dialogs; mutations compute snapshots that their callers discard; calendar
checks and initial loading perform unnecessary storage reads; and Habit-form
behavior is scattered across tab-navigation and Check-in modules.

These are focused fixes, not grounds for redesigning the application.

## Solution

Make retry behavior safe, keep untouched creation defaults current, and preserve
intentional scrolling while live updates arrive. Keep pending, failed, and
confirmed states truthful without introducing optimistic Check-ins or an offline
write queue.

Remove unused database work and clarify ownership of form behavior and styling.
Preserve the existing Go domain modules, SQLite transactions, authenticated
adapters, post-commit publication, and server-rendered monthly views.

## User Stories

1. As a User, I want retrying a failed confirmation to preserve my original
   Check-in intent, so that retry cannot undo a successful save.
2. As a User, I want a committed Check-in to reconcile through an authoritative
   read, so that uncertainty about loading is not confused with a failed write.
3. As a User, I want an uncertain write to remain visibly retryable, so that I
   can recover without guessing whether my action succeeded.
4. As a User on multiple devices, I want reconciliation to show the latest
   committed state, so that retrying a read does not overwrite another device.
5. As a User checking several dates, I want each pending action tracked
   independently, so that one successful response cannot hide another failure.
6. As a User browsing history, I want delayed responses to respect my selected
   month, so that an old request cannot replace the month I am viewing.
7. As a User returning after midnight, I want an untouched creation form to
   default to browser-local today, so that a new Habit is not silently backdated.
8. As a User intentionally backdating a Habit, I want my chosen date preserved,
   so that refreshing defaults does not overwrite my draft.
9. As a User drafting across midnight, I want my entered name and date retained,
   so that the calendar rollover does not destroy unfinished work.
10. As a User reopening a creation dialog, I want an existing intentional draft
    preserved, so that dismissing the dialog is not equivalent to resetting it.
11. As a User scrolling a monthly matrix, I want live updates to preserve my
    current position, so that the date I am inspecting does not disappear.
12. As a keyboard User, I want focused controls and keyboard-induced scrolling
    preserved during live updates, so that I can continue without relocating them.
13. As a User editing or deleting a Habit, I want consistent, readable dialog
    feedback, so that matrix styling does not interfere with the form.
14. As a User switching Companion Panel tabs, I want Jotpad text, selection,
    scroll, and saving preserved, so that Habit fixes do not disturb my notes.
15. As a User, I want Habit changes to leave my Day Plan untouched, so that these
    independent features remain safe to use together.
16. As a User with a long Habit history, I want creation and editing to avoid
    loading unrelated history, so that accumulated data does not add unnecessary
    work to every write.
17. As an operator, I want idle live connections to check calendar rollover
    without querying Habit storage, so that heartbeat cost does not grow with
    stored Check-ins.
18. As a maintainer, I want Habit-form behavior owned in one place, so that form
    changes do not require editing unrelated tab and Check-in logic.
19. As a maintainer, I want mutation interfaces to return only useful results,
    so that callers and tests do not depend on unnecessary snapshots.
20. As a maintainer, I want deterministic regressions for the failing
    interleavings, so that passing broad smoke tests does not conceal them again.

## Implementation Decisions

### 1. Separate write recovery from read reconciliation

- Preserve explicit desired checked/unchecked writes. Do not introduce blind
  toggles on the server or derive retry intent from newly rendered toggle markup.
- After a committed receipt, recovery retries an authoritative month read, not
  another mutation. This allows another device's later committed state to win.
- Before a committed receipt is available, preserve the original desired state
  for any explicit write retry. Do not interpret a retry as a fresh toggle.
- Distinguish an unconfirmed write from a committed write awaiting a successful
  read in user-facing feedback. Do not advertise an ordinary inverse toggle as
  a retry of the preceding operation.
- Preserve per-attempt correlation and the existing month, refresh, view, and
  read-generation guards. A failed, empty, canceled, or delayed read must neither
  falsely confirm an attempt nor leave its recovery permanently inaccessible.
- Keep attempts independent across dates. Preserve rejection feedback,
  reconnect recovery, and stale-response protection.
- Keep the current transport contracts unless a minimal additive change is
  demonstrably necessary. No new endpoint or generic synchronization framework
  is planned.

### 2. Refresh creation defaults without overwriting drafts

- Recalculate browser-local today when opening an untouched creation form.
- Treat default ownership explicitly: once the User has begun an intentional
  draft, reopening or midnight must not rewrite its name or date. An explicitly
  chosen date remains intentional even if it equals the previous default.
- Do not reset an open draft at midnight. Server validation continues to use
  the browser timezone and the current server clock.
- Preserve initial date seeding before Datastar binds the input. Moving form
  behavior must not change that initialization ordering.
- Keep generation-guarded success and rejection responses, including protection
  for drafts opened after a previous submission.

### 3. Preserve current scroll intent

- Capture observed horizontal scroll positions synchronously rather than reading
  them for the first time in a later animation frame.
- Do not restore an old position on every matrix attribute or text mutation.
  Restore only where an authoritative DOM update actually requires preservation.
- Ensure restoration cannot overwrite a newer deliberate scroll or cause a
  feedback loop that records the restored position as fresh User intent.
- Preserve keyboard focus and the existing fallback when a focused Habit or
  Check-in control disappears. Do not steal focus from an open dialog.

### 4. Limit matrix styling to matrix content

- Prevent matrix paragraph and feedback rules from reaching nested edit/delete
  dialogs. Dialog chrome and feedback retain deliberate, independent ownership.
- Keep styles in the single hand-authored stylesheet, using existing tokens,
  cascade layers, shared classes, and leaf scopes, as required by ADR 0011.
- Do not compensate for leakage with increasingly specific dialog overrides.

### 5. Remove discarded mutation snapshots

- Change Habit mutation interfaces so they no longer return whole lists or month
  snapshots that production callers discard. Return an error or a minimal result
  only where a caller genuinely needs it.
- Remove snapshot-only queries from write transactions. Keep every query needed
  for ownership, uniqueness, eligibility, durable identity allocation, and
  existing-Check-in validation.
- Retain atomic writes and publication only after successful commit. SQLite's
  immediate writer serialization remains unchanged under ADR 0007.
- Update tests to inspect committed state through existing read operations,
  rather than retaining snapshot returns solely for test convenience.

### 6. Separate calendar calculation from storage reads

- Provide current-calendar information through the Habit module using its
  existing injected clock and timezone validation, without querying SQLite.
- Use that operation for live-connection initialization, Habit invalidation
  calendar checks, and heartbeat rollover detection.
- Render the initial loading shell without fetching Habit history that cannot
  yet be displayed before the browser supplies its timezone.
- Keep authenticated, owner-scoped month reads as the authoritative grid read
  path. Preserve subscribe-before-read ordering wherever state is read for
  reconnect recovery.
- Remove now-unused interface members and pass-through data parameters rather
  than preserving compatibility layers with no production consumer.

### 7. Improve locality without adding architecture

- Keep Companion Panel navigation responsible for accessible tab selection and
  the editor hide/show lifecycle.
- Gather Habit-form defaulting, validation presentation, and form save lifecycle
  behavior into a Habit-form module. Leave Check-in correlation in its existing
  module and retain the shared save-status aggregation seam.
- Give create, edit, and delete dialog definitions consistent feature-grouped
  ownership while keeping matrix composition straightforward.
- Group browser scenarios by behavior when modifying them; retain shared session
  setup and avoid building a generic page-object framework for this cleanup.
- Keep the existing package structure, typed publication channels, and native
  dialogs. No new dependencies, database schema changes, migrations, or frontend
  build steps are required.

## Testing Decisions

### Preferred seams

Use existing seams, with the browser as the primary acceptance surface:

1. **Authenticated browser interactions and HTTP request interception** for
   retries, delayed responses, dialogs, scrolling, timezone defaults, focus,
   and cross-feature regressions.
2. **Existing Check-in driver events** for exhaustive deterministic attempt and
   read lifecycle permutations that would be expensive to exercise only through
   the browser.
3. **Habit service methods over real SQLite, the existing clock option, and the
   publication seam** for persistence, invariants, and post-commit behavior.
4. **Existing HTTP handler tests** for authenticated reads, acknowledgements,
   rejection feedback, and stale-response guards.

No new externally exposed test seam is proposed. The calendar operation is a
production interface simplification, not a test-only hook. These seam choices
were confirmed by the User before implementation.

### What makes a good regression

- Assert externally meaningful results: persisted checked state, emitted request
  intent, authoritative rendered state, feedback, selected month, draft contents,
  focus, and scroll position.
- Arrange the exact failing interleaving deterministically. Avoid arbitrary
  sleeps, assertions about private maps, or tests that merely mirror helper code.
- Keep implementation-detail observations limited to arranging browser-specific
  races where the existing regression suite already uses that technique.
- Demonstrate each behavioral regression failing before its fix, then passing
  without weakening existing assertions.

### Required coverage

- **Retry after commit:** render the checked state from the owner stream before
  the receipt; fail or truncate the correlated read; activate recovery; assert
  no inverse write occurs and a successful read settles the attempt.
- **Superseding device:** repeat recovery after another device changes the same
  date; the last committed state wins without being rewritten by read retry.
- **Unknown outcome:** lose the receipt, vary whether authoritative HTML has
  arrived, and ensure any explicit write retry retains the original desired
  state. Cover concurrent cells and navigation away from the pending month.
- **Read lifecycle:** retain existing failure, empty-stream, reconnect, canceled
  older read, duplicate receipt, and out-of-order response scenarios.
- **Untouched overnight default:** load before browser-local midnight and first
  open creation afterward; today is selected without manually filling the date.
- **Draft preservation:** cross midnight with edited fields, an explicitly chosen
  date, and a dismissed/reopened draft; none is overwritten. Cover a timezone
  whose local date differs from UTC and a month/year boundary.
- **Scroll race:** scroll horizontally, deliver a mutation before the next
  animation frame, and assert the new position survives. Exercise attribute
  changes and authoritative grid morphs, keyboard navigation, and narrow layout.
- **Dialog styling:** verify create/edit/delete feedback and warnings retain
  intentional dialog spacing across themes and desktop/mobile layouts, without
  matrix paragraph or output rules leaking into them.
- **Service simplification:** retain owner isolation, concurrent uniqueness,
  eligibility, edit-history protection, permanent deletion, non-reused IDs, and
  post-commit publication tests against real SQLite.
- **Calendar independence:** exercise the clock-backed calendar operation and
  heartbeat rollover without relying on Habit storage availability. Confirm by
  review that the loading shell and calendar-only path do not issue Habit reads;
  avoid exact SQL-count assertions or new instrumentation solely for this plan.
- **Existing features:** preserve Day Plan behavior, Jotpad text/selection/scroll,
  pending saves, native dialog focus recovery, and per-view month selection.

Prior art includes the current correlated-Check-in JS tests, delayed creation
and edit acknowledgement browser tests, month-following regressions, native
Jotpad refocus scenarios, and real-SQLite service concurrency tests.

## Out of Scope

- New Habit features: archive, schedules, quantities, streaks, charts, targets,
  offline queues, or integration with Blocks.
- Changing last-committed-write semantics or introducing optimistic Check-ins.
- Replacing Datastar, templ, SQLite, CodeMirror, or the publication mechanism.
- Generic repository layers, event frameworks, new package hierarchies, broad
  handler rewrites, or abstractions introduced only for hypothetical reuse.
- Splitting the stylesheet, adding a CSS build step, or redesigning the theme.
- Rewriting the Jotpad native-focus workaround or claiming real Android OS
  verification from Chromium JavaScript-branch emulation.
- Deployment topology, migration history, backup strategy, or authentication
  policy changes.
- Automatic issue publication or application-code changes as part of preparing
  this local plan.

## Further Notes

### Delivery order

1. Add failing regressions and fix safe Check-in recovery.
2. Add failing regressions and fix overnight defaults and the scroll race.
3. Fix CSS scope leakage and verify dialog presentation.
4. Remove discarded mutation snapshots and calendar-only storage reads, updating
   consumers and tests together.
5. Consolidate form behavior and dialog ownership without changing the now-tested
   behavior. Keep mechanical moves separate from behavioral fixes where practical.
6. Run the complete verification suite and review the final diff for unnecessary
   interfaces, duplicated ownership, and accidental scope expansion.

Steps 1–3 are acceptance fixes. Steps 4–5 address the reviewed maintainability
problems; they must simplify the implementation rather than layer over it.

### Completion checklist

- [x] Advertised Check-in recovery cannot reverse an already committed action.
- [x] Untouched creation defaults advance to browser-local today on opening.
- [x] Intentional drafts survive midnight and reopening.
- [x] Live updates preserve the latest horizontal scroll and keyboard focus.
- [x] Matrix styles no longer reach Habit dialog warnings or feedback.
- [x] Mutation transactions no longer calculate discarded snapshots.
- [x] Calendar-only operations and the initial loading shell avoid Habit reads.
- [x] Habit-form behavior has one clear owner with correct initialization order.
- [x] Existing response guards, owner isolation, and post-commit publication remain.
- [x] Go race tests, JS unit tests, browser tests, vet, build, and diff checks pass.
- [x] Relevant tracker work exists before implementation and commits are signed off.

At the reviewed baseline, Go race tests, all 121 JavaScript unit tests, all 61
Chromium browser tests, vet, build, and diff checks passed. Current-head GitHub
test and sign-off checks also passed during review. These results do not cover
the newly identified gaps; the retry lifecycle and scroll reset were reproduced
separately. Android coverage remains branch emulation, not a real-device claim.

Related requirements: UNB-34 (creation and Companion Panel), UNB-35 (Check-ins and
live synchronization), UNB-37 (historical months), UNB-38 (editing), UNB-39
(deletion), UNB-40 (Jotpad teardown), UNB-42 (layout and themed actions), UNB-43
(reconciliation and generation guards), and UNB-44 (Jotpad refocus preservation).

Before changing Datastar or templ behavior, verify the current official docs.
Generate templ output before build/test on a clean checkout, but never run
one-shot generation while the development watcher is active.

### Implementation verification

- `go test -race ./...`, `go vet ./...`, `task build`, and diff checks pass.
- All 129 JavaScript unit tests and all 86 Chromium browser tests pass.
- Deterministic regressions demonstrated red → green for recovery intent,
  overnight defaults, the scroll race, dialog padding leakage, and
  calendar/loading independence from Habit storage.
- Desktop/mobile dialog screenshots were checked across all three current
  color families in light/dark mode; no clipping or spacing issues were found.
- The existing live-scroll scenario retains its focus, convergence, and scroll
  assertions, but activates the focused control with Enter so Playwright does
  not introduce a new scroll-to-click position during the preservation test.
- Standards review: no documented violations; a minor duplication of status
  assertions in independent test fixtures is accepted. Spec review: no findings.
- Android coverage remains Chromium JavaScript-branch emulation, not OS/device
  verification.
