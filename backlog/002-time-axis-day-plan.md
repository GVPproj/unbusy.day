# 002 — Time-axis Day Plan: slot placement with push semantics

Status: backlog
Labels: ready-for-agent
Date: 2026-06-12

## Problem Statement

Cards today form a dense top-down list (position 0, 1, 2…): they always fill
from the top and can only be reordered relative to each other. A user planning
their day Cal Newport-style can't say *when* anything happens — there is no
time axis, no gaps, and no notion of a day with bounds. The user wants to
place blocks of work at specific times of day, leave slots empty, and resize
blocks in 30-minute increments.

## Solution

Turn the column into a Day Plan: a per-User schedule with a start and end time
on half-hour boundaries, divided into 30-minute Slots. Each Card occupies a
contiguous run of Slots (a starting Slot plus its span) and is anchored to the
clock — it stays at its time no matter how the day's bounds change. Cards can
sit anywhere with empty Slots between them. Dragging or growing a Card onto
occupied Slots pushes the overlapped Cards down by the minimum distance,
consuming empty Slots before displacing further Cards; a push that would force
any Card past the end of the day rejects the whole gesture and the plan snaps
back. The User can edit their day's bounds (within 5:00–18:00 for now); the
day can only shrink into empty Slots. Terminology and invariants are in
CONTEXT.md (Day Plan, Slot, Push); the client/server split is ADR 0005.

## User Stories

1. As a User, I want my cards laid out on a labeled time grid (hours and :30
   marks), so that my plan reads like a paper time-block schedule.
2. As a User, I want each slot to represent 30 minutes of my day, so that
   block sizes correspond to real durations.
3. As a User, I want to drag a card onto any empty slot, so that I can
   schedule work at a specific time rather than only reorder a list.
4. As a User, I want to leave slots empty between cards, so that unscheduled
   time is visible in my plan.
5. As a User, I want a card I place at 11:00 to stay at 11:00, so that my
   schedule is stable unless I change it.
6. As a User, I want dropping a card onto occupied slots to push the
   overlapped cards down by the minimum distance, so that I can insert a block
   without hand-clearing space first.
7. As a User, I want a push to consume empty slots before displacing further
   cards, so that gaps absorb insertions and unrelated cards don't move.
8. As a User, I want a drop that would push any card past the end of my day to
   be rejected whole, so that nothing ever falls off my plan.
9. As a User, I want to grow or shrink a card's span by 30-minute steps with
   the grip, so that block length matches how long the work takes.
10. As a User, I want growing a card to push the cards below it just like a
    drag does, so that resizing feels consistent with moving.
11. As a User, I want to set my day's start and end times on half-hour
    boundaries, so that the plan covers my actual working day.
12. As a User, I want a bounds change that would strand a card outside the day
    to be rejected with the current plan re-shown, so that I never lose a
    placement silently.
13. As a User, I want my day bounds limited to 5:00–18:00 for now, so that the
    grid stays a sane single-day size.
14. As a new User, I want a default 9:00–17:00 day with my starter cards in
    the first slots, so that the plan works before I configure anything.
15. As a User with two browser tabs open, I want a move in one tab to appear
    live in the other, so that my plan is consistent everywhere I look.
16. As a User on a flaky connection, I want my drag to commit visually at
    once and snap back only if the server rejects it, so that the interaction
    stays instant.
17. As a User, I want invalid drops to feel impossible (the ghost snaps to
    valid positions), so that I rarely see a rejection at all.

## Implementation Decisions

- **Time axis is real, not cosmetic** (per CONTEXT.md): a Day Plan has
  user-editable start/end times on half-hour boundaries; slot count is
  derived, never stored. One rolling Day Plan per User; no dates, no history.
  "Board" is retired.
- **Cards are clock-anchored**: storage is a slot index counted from 00:00
  (0–47), not an offset from the day's start. The existing `position` column's
  meaning changes from list rank to clock slot.
- **One mutation replaces Reorder + Resize**: `SetLayout(owner, [{id, slot,
  span}])`. With client-computed push, a move and a grow both produce the same
  payload — the full resulting layout. The server validates invariants only:
  same card set as current, every run `[slot, slot+span)` within bounds, no
  overlaps. Domain rejections stay 200 + re-render of the authoritative
  column, matching the existing convention.
- **Push is client-side, invariants server-side** (ADR 0005): drag.js computes
  the push cascade (always down, minimum distance, gaps consumed first,
  reject-at-bottom aborts the gesture) so the optimistic FLIP commit shows the
  true outcome instantly. Any layout passing the server's invariant check is a
  state reachable by legal drags, so server authority is undiminished.
- **Validator is a pure module**: `Validate(bounds, current, proposed) error`
  with no DB or transport dependencies, called by the service inside the
  `FOR UPDATE` transaction. Typed errors (not-same-cards, out-of-bounds,
  overlap) so adapters re-render truth on rejection.
- **Bounds live on the user row** (`day_start`, `day_end`, stored as slot
  indexes or minutes-from-midnight), default 9:00–17:00. `SetBounds` rejects a
  shrink into occupied slots — same shape as layout rejection. Hard limits for
  now: start ≥ 5:00, end ≤ 18:00, end > start, no midnight wrap.
- **Migration** (timestamped, plain DDL, forward-only per ADR 0004): add
  bounds columns with defaults; repack existing cards from day start by
  current position rank, accounting for spans; drop the deferrable
  `(owner_id, position)` unique; add an `EXCLUDE` constraint on
  `(owner_id, int4range(position, position + span))` as a DB backstop making
  overlap impossible even if app validation regresses. Seeding starter cards
  targets the first slots after day start, span 1 each.
- **Frontend adapter**: one layout mutation endpoint replaces the reorder and
  resize endpoints, plus a bounds-settings endpoint. The SSE read path and the
  shared page/patch component (single source for initial render and every
  patch) are unchanged in shape; events now carry slot-placed cards and the
  render needs the owner's bounds.
- **Grid rendering**: the column renders every slot in the day as a
  first-class element with an hour/:30 gutter (per the Newport-notebook
  reference); empty slots are real drop targets, not collapsed filler. Card
  placement renders from `slot`/`span` so server render and client gesture
  agree and morphs stay idempotent (existing drag.js convention).
- **Bounds editing UI** follows the house style: native HTML first (the theme
  modal is the exemplar), Datastar signals only for state the server cares
  about.

## Testing Decisions

- Good tests assert external behavior — what the service returns, what the
  handler responds, what state the DB holds — never internal call shapes.
- **Layout validator**: pure unit tests, no DB. Cover: identical layout,
  moved-into-gap, exact-fit at day end, overlap (partial and full), span
  growing past end, card set mismatch (missing id, extra id, duplicate id),
  slot before day start, zero/negative span.
- **cards service (SetLayout, SetBounds)**: DB-backed tests in the existing
  cards test style — commit visible via List, rejection leaves state
  untouched, post-commit fan-out only (nil Publisher path included),
  concurrent mutations serialized by FOR UPDATE.
- **Frontend adapter handlers**: HTTP-level tests following the existing
  adapter tests — success responds with an SSE element patch of the committed
  column; a rejected layout/bounds change responds 200 with the authoritative
  re-render; session-gated.
- **Bounds setting**: reject shrink-into-occupied (start side and end side),
  accept shrink into empty slots, enforce 5:00/18:00 limits and half-hour
  boundaries.
- Test with the three existing seeded placeholder cards only.

## Out of Scope

- Card CRUD (create, delete, rename) — placeholder cards only for now.
- Calendar dates, history, or more than one Day Plan per User.
- Templates (named as a future concept in CONTEXT.md).
- The live "current time of day" indicator.
- Midnight-wrapping days or bounds outside 5:00–18:00.
- Cross-instance pub/sub (single machine by design).

## Further Notes

- The interaction feel is mission critical (ADR 0005): if a server-computed
  push could be made to feel as good, the client-side push exception should be
  revisited — but not in this slice.
- The grid changes drag.js's geometry assumptions (currently it derives slot
  pitch from a probe card and balances "consumed" filler slots); the slot-grid
  rendering likely simplifies that math since every slot becomes a real,
  fixed-height element.
- `/_smoke` must keep working if the Datastar/templ pins move (CLAUDE.md).
