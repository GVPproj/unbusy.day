# 001 — Cal Newport time-blocking (sourced) and the unbusy.day column as a time-blocking tool

Status: research (input for a "Guide" modal — no code committed)
Date: 2026-07-10

Research for a Guide modal that (1) explains the why/how of time-block planning
grounded in Cal Newport's own writing, and (2) teaches the app's day-planner
column as a concrete time-blocking tool. Part A is sourced against
calnewport.com and his books; Part B is drawn from the codebase. Every
substantive Part A claim carries its source inline; full URL list at the end.

---

## Part A: Cal Newport's time-block method (sourced)

### Definition and the core rationale

Newport defines time blocking as partitioning the workday into blocks and
assigning specific work to each: "dedicate ten to twenty minutes every evening
to building a schedule for the next day … divide the hours of your workday into
blocks and assign work to them"
([Deep Habits: The Importance of Planning Every Minute of Your Work Day](https://calnewport.com/deep-habits-the-importance-of-planning-every-minute-of-your-work-day/)).

His headline argument for why it is worth the trouble:

- "A 40 hour time-blocked work week, I estimate, produces the same amount of
  output as a 60+ hour work week pursued without structure"
  ([Planning Every Minute](https://calnewport.com/deep-habits-the-importance-of-planning-every-minute-of-your-work-day/)).
- Framed later as: "time blockers accomplish roughly twice as much work per week
  as compared to those who use more reactive methods," plus "a much clearer
  separation between work and non-work time"
  ([The Time Blocking Revolution Begins…](https://calnewport.com/the-time-blocking-revolution-begins/)).

The cost he is contrasting against is **unstructured, reactive time** — the
default of filling the gaps between meetings by reacting to the inbox and
occasionally pulling from a task list. The point of blocking is to treat "time
like the best investors view their capital, as a resource to wield for maximum
returns" — proactive allocation instead of reactive drift
([Planning Every Minute](https://calnewport.com/deep-habits-the-importance-of-planning-every-minute-of-your-work-day/)).

### The mechanics he prescribes

- **Block every minute of the workday.** The whole workday is partitioned; there
  is no un-assigned time. Newport dedicates "two lines to each hour of the day and
  then divide[s] that time into blocks labeled with specific assignments"
  ([Planning Every Minute](https://calnewport.com/deep-habits-the-importance-of-planning-every-minute-of-your-work-day/)).
- **Granularity.** Roughly hour-scale blocks, subdivided; blocks carry a specific
  assignment (a named job), not a vague category.
- **Two-column layout / revised plans.** Left column holds the hourly time blocks;
  the right side holds notes and, crucially, **rewrites**. "The columns growing to
  the right side are rewrites that I made throughout the day as my plan changed"
  ([Deep Habits: Three Recent Daily Plans](https://calnewport.com/deep-habits-three-recent-daily-plans/)).
  He deliberately leaves "extra room next to my time blocks" so he can correct the
  plan when "the day unfolds in an unexpected way"
  ([Planning Every Minute](https://calnewport.com/deep-habits-the-importance-of-planning-every-minute-of-your-work-day/)).
- **Correct the plan when it breaks — don't abandon it.** When an interruption
  arrives, he re-blocks the *remaining* time rather than reverting to reactivity. A
  visitor to his office "shifted the length of my 1:30 task block," which required
  "yet another rewrite of the plan"
  ([Three Recent Daily Plans](https://calnewport.com/deep-habits-three-recent-daily-plans/)).
  The habit is: when reality diverges, redraw the blocks for what's left.
- **Overflow / conditional blocks.** Unpredictable, reactive obligations are
  themselves blocked: "periods of open-ended reactivity can be blocked off like any
  other type of obligation." He also gives blocks a conditional secondary purpose —
  e.g. "process client requests; if I have downtime during this block, work on
  project X"
  ([Planning Every Minute](https://calnewport.com/deep-habits-the-importance-of-planning-every-minute-of-your-work-day/)).
- **You don't have to plan first thing.** "On many days, I like to dive right into
  a deep task for an hour or so before taking the time to make a plan for the rest
  of the day"
  ([Three Recent Daily Plans](https://calnewport.com/deep-habits-three-recent-daily-plans/)).

### Deep work vs shallow work (the block types)

The deep/shallow distinction is canonical from the book *Deep Work* (2016): deep
work is "professional activities performed in a state of distraction-free
concentration that push your cognitive capabilities to their limit"; shallow work
is "noncognitively demanding, logistical-style tasks, often performed while
distracted" that "tend not to create much new value and are easy to replicate"
(*Deep Work*, Cal Newport, 2016 — book, definitions; discussed across
[Some Notes on Deep Working](https://calnewport.com/some-notes-on-deep-working/)).
Time blocking is the mechanism that *protects* deep work: by assigning named
blocks you can defend multi-hour stretches for depth instead of letting shallow
reactivity fragment the day. Weekly planning explicitly reserves depth — e.g.
"three hours a day for deep work"
([Deep Habits: Plan Your Week in Advance](https://calnewport.com/deep-habits-plan-your-week-in-advance/)).
The app's third type, **break**, is not a Newport term of art but sits naturally
alongside: rest is a legitimate thing to block, and Newport writes about
deliberate breaks (e.g. [On Deep Breaks](https://calnewport.com/on-deep-breaks/)).

### Related rituals he ties in

- **Fixed-schedule productivity.** Fix a hard end to the workday first, then make
  the work fit: "Choose a schedule of work hours that you think provides the ideal
  balance of effort and relaxation" and "do whatever it takes to avoid violating
  this schedule" — "Fix the schedule you want. Then make everything else fit around
  your needs"
  ([Fixed-Schedule Productivity](https://calnewport.com/fixed-schedule-productivity-how-i-accomplish-a-large-amount-of-work-in-a-small-number-of-work-hours/)).
  This is the direct analogue of the app's **configurable day bounds** (a start and
  end): you set the container, then blocks compete for the finite space inside it.
- **Shutdown ritual.** A structured end-of-day routine: capture loose tasks into
  master lists, review lists and the calendar for the next two weeks, review the
  weekly plan, then speak a termination phrase — "schedule shutdown, complete." The
  effect: "I've basically eliminated stressful work-related thoughts from my
  evenings and weekends"
  ([Drastically Reduce Stress with a Work Shutdown Ritual](https://calnewport.com/drastically-reduce-stress-with-a-work-shutdown-ritual/)).
  It gives the workday a clean lower boundary — the counterpart to the day's end
  bound.
- **Weekly planning.** "On Monday mornings I plan the upcoming work week. I capture
  this plan in an e-mail and send it to myself." The weekly plan sets each day's
  objectives by matching work types to available time; "to visualize your whole
  week at once allows you to spread out, batch, and prioritize work." He stresses
  "There is no *best* format for creating a weekly plan"
  ([Plan Your Week in Advance](https://calnewport.com/deep-habits-plan-your-week-in-advance/)).
- **Multi-scale planning (quarterly → weekly → daily).** Newport's system nests
  three horizons: a seasonal/quarterly plan sets big goals, the weekly plan
  allocates the coming week against the calendar, and the daily time-block plan
  executes. Time blocking lives at the *daily* layer, fed by the weekly plan; the
  "Quarterly → Weekly → Daily" framing is associated with his Time-Block Planner
  ([The Time Blocking Revolution Begins…](https://calnewport.com/the-time-blocking-revolution-begins/);
  weekly↔daily link in [Plan Your Week in Advance](https://calnewport.com/deep-habits-plan-your-week-in-advance/)).
  *Flagged:* the explicit "Quarterly → Weekly → Daily" label surfaced in the
  planner announcement / comments rather than a dedicated methodology essay; the
  daily and weekly layers are directly sourced, the quarterly layer is the least
  documented in his free blog writing.

### His explicit caveats

Newport is emphatic that a plan is a tool for intentionality, not a cage:

- "My goal, of course, is not to make a rigid plan I must follow no matter what"
  ([Three Recent Daily Plans](https://calnewport.com/deep-habits-three-recent-daily-plans/)).
  The aim is to avoid "reactive, inbox-driven busyness" and "mindless surfing," not
  to obey the schedule mechanically.
- Plans *will* break — that is expected, and the response is to re-block, which is
  why he builds rewrite space into the layout
  ([Planning Every Minute](https://calnewport.com/deep-habits-the-importance-of-planning-every-minute-of-your-work-day/)).
- The value is **control over your time**: deciding on purpose where each stretch
  of the day goes, and adjusting deliberately when reality intervenes.

**Sourcing note.** The "40h ≈ 60h", every-minute mechanics, two-column rewrite,
overflow/conditional blocks, and the intentionality caveats are all from
calnewport.com blog posts (primary). The "twice as much" figure and the
Quarterly→Weekly→Daily framing come from the Time-Block Planner announcement post
(primary, but marketing-adjacent). Deep/shallow work definitions are from the book
*Deep Work* (primary, book). Nothing substantive here rests on a third-party
summary.

---

## Part B: unbusy.day column as a time-blocking tool

Sourced from the code: `internal/block/{block.go,layout.go}`, `CONTEXT.md`,
`internal/frontend/components/{column.templ,column_block.templ,nav.templ}`,
`internal/frontend/components/modals/{create,clear,hours}.templ`,
`internal/frontend/static/drag.js`, and ADRs 0005 / 0011.

### The surface: one rolling day of 30-minute slots

- The Day Plan is a single, private, **rolling "today"** — not a calendar date and
  with no history of past days (`CONTEXT.md`, "Day Plan").
- The day is a vertical grid of **30-minute Slots** between a configurable start and
  end. Bounds are slot indexes counted from 00:00; the day covers `[Start, End)`
  (`internal/block/layout.go`, `Bounds`). Defaults are 9:00–17:00; the hard legal
  range is 4:00–18:00 (`MinDayStart = 8`, `MaxDayEnd = 36`, `DefaultDayStart = 18`,
  `DefaultDayEnd = 34`). No midnight wrap.
- A **Slot is either empty or covered by exactly one Block — blocks never overlap**
  (`CONTEXT.md`, "Slot"). This no-overlap rule is the app's spine.
- The current time of day is shown live on the plan (the "now pill," revealed
  client-side; the server can't know the viewer's clock — `column.templ`,
  `now-pill.js`).

### A Block: a named job pinned to a stretch of time

- A Block has a `Label`, a `Position` (its start slot), a `Span` (height in slots,
  ≥ 1), and a `Type` (`internal/block/block.go`, `Block` struct).
- A block is **anchored to the clock, not to the top of the plan** — changing the
  day's bounds never moves a block's time (`CONTEXT.md`, "Block").
- **Block Type** is chosen at creation and is a flat three-way tag: **deep**
  (demanding, focused work), **shallow** (low-cognitive-demand work), or **break**
  (rest). Every block has exactly one; it's color-coded on the plan and immutable
  after creation (`CONTEXT.md`, "Block Type"; `block.go`, `BlockType`).

### The user-facing actions and what each maps to

| App action | Mechanics (code) | Newport concept it serves |
|---|---|---|
| **Set Hours** (bounds) | `SetBounds` sets day start/end on half-hour boundaries; a shrink that would strand a block is rejected whole (`block.go`, `SetBounds`; nav "Set Hours" → `HoursModal`). Options that would strand the first/last block disable reactively (`hours.templ`). | **Fixed-schedule productivity** — fix the container first; the day can only shrink into empty slots, so time is finite by construction. |
| **Add a block** (the "+") | Every *free* slot renders a "+" that opens the New Block modal, pre-filling that slot; `Create` inserts a span-1 block, rejecting a blank label, an out-of-bounds slot, or an occupied slot (`column.templ` `slot`; `create.templ`; `block.go`, `Create`). | **Assign a named job to a stretch of time** — the atomic act of time blocking. Naming is required (no anonymous blocks), like Newport's "specific assignments." |
| **Choose a type** | Radio swatch: shallow (default) / deep / break (`create.templ`). | **Deep vs shallow work** (plus break) — makes the depth of each block explicit and defendable. |
| **Drag to move** | Pointer/keyboard move re-slots the block; the **push cascade** slides overlapped blocks toward the vacated slot, consuming gaps first (`drag.js`; `CONTEXT.md`, "Push"; ADR 0005). | **Re-block when the day changes** — rearranging the plan is cheap and expected. |
| **Stretch to resize** | The grip grows/shrinks the span; growing pushes/compresses neighbors, clamped to day end (`drag.js` resize; `pushLayout … {compress:true}`). | **Right-sizing a block** — giving a job the time it actually needs. |
| **Rename** | Tap/F2 inline edit → `Rename` (`drag.js` `enterEdit`; `block.go`, `Rename`). | Sharpening a block's assignment as the plan is revised (Newport's right-column elaboration). |
| **Delete** (the "×") | Per-block delete → `Delete` (`column_block.templ`; `block.go`, `Delete`). | Dropping a job that no longer belongs. |
| **Clear** | Nav "Clear" → confirm → `Clear` wipes all blocks, bounds untouched; disabled on an already-empty day (`clear.templ`; `nav.templ`; `block.go`, `Clear`). | Starting the next day's plan fresh. |

### The push cascade and "time is finite"

Moving or growing a block onto occupied slots **pushes** each overlapped block
*down* (later) by the minimum distance that clears the overlap, consuming empty
slots before displacing further blocks (`CONTEXT.md`, "Push"). A placement whose
push would force any block past the end of the day is **rejected whole — nothing
moves** (`CONTEXT.md`; `layout.go`, `ValidateLayout`).

Architecturally the client computes the push for instant feel and the server
enforces the invariants exactly once — **same block set, in bounds, no overlaps,
span ≥ 1** (`layout.go`, `ValidateLayout`; ADR 0005). This is the single overlap
guard.

This is the app's most important teaching moment: the plan physically cannot hold
more than the day contains. When a move is illegal the optimistic change **snaps
back** to the last legal layout — the app's error convention is 200 + a re-render
of the authoritative column, so a rejected change visibly reverts (CLAUDE.md,
"Error convention"). That snap-back is the system *enforcing that time is a finite
resource* — the exact discipline Newport's fixed-schedule and every-minute
blocking are meant to impose.

### Notes for the guide author

- There is **no task list on the side** and **no weekly/quarterly layer** in the
  app today — it implements only Newport's **daily** time-block layer. The guide can
  mention the weekly/quarterly context as the surrounding practice, but shouldn't
  imply the app does it. (A reusable "Template" Day Plan is named as future work in
  `CONTEXT.md` but is not built.)
- The day is a **rolling today** with no history — so "start fresh" = Clear, not a
  new dated page.
- Bounds are limited to 4:00–18:00 for now (`layout.go`), so the guide's examples
  should stay inside a daytime window.

---

## Part C: suggested content outline for the Guide modal

A short modal, ideally two panes: **the idea** then **using the app**. Each app
feature ties back to a Newport concept.

**1. Why time-block your day (the idea)**
- One line: assign every stretch of your day to a specific job, instead of
  reacting to whatever lands in front of you.
- The payoff, in Newport's words: a time-blocked ~40-hour week does the work of a
  60+ hour unstructured one — control your time like capital
  ([Planning Every Minute](https://calnewport.com/deep-habits-the-importance-of-planning-every-minute-of-your-work-day/)).
- The honest caveat up front: the plan *will* break — that's fine. The win is
  intentionality, not rigid obedience; when the day shifts, you re-block
  ([Three Recent Daily Plans](https://calnewport.com/deep-habits-three-recent-daily-plans/)).

**2. Set your hours** → fixed-schedule productivity
- Fix the container first (Set Hours). Everything else has to fit inside it — the
  day can only shrink into empty time.

**3. Add blocks for your work** → assign named jobs; deep vs shallow
- Tap "+" on any free slot, name the block, pick a type:
  - **Deep** = distraction-free, cognitively demanding work you want to defend.
  - **Shallow** = logistical/admin work.
  - **Break** = deliberate rest.
- Naming is required — a block is a *specific* assignment, not a category.

**4. Shape the day** → right-size and rearrange
- **Drag** to move a block; **stretch** the grip to resize. Neighbors push out of
  the way (the push cascade). Rename inline; delete with "×".

**5. When the day changes, re-block** → correct the plan, don't abandon it
- Drag things around as reality intervenes; the plan is meant to be rewritten.
  This is Newport's right-column rewrite, made physical.

**6. Time is finite (why it snaps back)**
- If a move would overflow the day, the whole move is rejected and snaps back —
  the app won't let you pretend you have more hours than you do. That constraint is
  the point.

**7. Start fresh** → Clear
- One rolling "today." Clear wipes the plan when you want to lay out a new day.

*(Optional footer)* This app covers the **daily** layer of Newport's system;
he also plans by week and season, and ends the day with a shutdown ritual
([shutdown ritual](https://calnewport.com/drastically-reduce-stress-with-a-work-shutdown-ritual/),
[weekly planning](https://calnewport.com/deep-habits-plan-your-week-in-advance/)).

---

## Sources

Primary — calnewport.com blog:
- [Deep Habits: The Importance of Planning Every Minute of Your Work Day](https://calnewport.com/deep-habits-the-importance-of-planning-every-minute-of-your-work-day/) — definition; "40h ≈ 60+h"; every-minute blocking; two-line-per-hour granularity; leave rewrite room; overflow/conditional reactive blocks; correct-when-it-breaks.
- [Deep Habits: Three Recent Daily Plans](https://calnewport.com/deep-habits-three-recent-daily-plans/) — two-column rewrites through the day; "not a rigid plan I must follow"; re-blocking after interruptions; don't have to plan first thing.
- [Fixed-Schedule Productivity](https://calnewport.com/fixed-schedule-productivity-how-i-accomplish-a-large-amount-of-work-in-a-small-number-of-work-hours/) — fix the schedule first, make work fit; maps to the app's day bounds.
- [Drastically Reduce Stress with a Work Shutdown Ritual](https://calnewport.com/drastically-reduce-stress-with-a-work-shutdown-ritual/) — end-of-day shutdown ritual and the "schedule shutdown, complete" phrase; bounding the workday.
- [Deep Habits: Plan Your Week in Advance](https://calnewport.com/deep-habits-plan-your-week-in-advance/) — weekly planning; weekly→daily link; "three hours a day for deep work"; no single best format.
- [On Simple Productivity Systems and Complex Plans](https://calnewport.com/on-simple-productivity-systems-and-complex-plans/) — daily vs weekly plan artifacts; complex lives justify complex plans.
- [Some Notes on Deep Working](https://calnewport.com/some-notes-on-deep-working/) — deep vs shallow work discussion (definitions canonically from the book).
- [On Deep Breaks](https://calnewport.com/on-deep-breaks/) — deliberate breaks (context for the "break" block type).

Primary — Time-Block Planner announcement (marketing-adjacent):
- [The Time Blocking Revolution Begins…](https://calnewport.com/the-time-blocking-revolution-begins/) — "twice as much work per week"; clearer work/non-work separation; Quarterly→Weekly→Daily framing (flagged as least-documented).

Primary — book:
- *Deep Work: Rules for Focused Success in a Distracted World*, Cal Newport, 2016 — canonical deep-work / shallow-work definitions.

Codebase (Part B):
- `internal/block/block.go`, `internal/block/layout.go` — Service (Create/SetLayout/SetBounds/Delete/Clear/Rename), Bounds, slot/span semantics, ValidateLayout.
- `CONTEXT.md` — domain glossary (Block, Block Type, Day Plan, Slot, Push).
- `internal/frontend/components/column.templ`, `column_block.templ`, `nav.templ` — the grid, block item, and nav controls.
- `internal/frontend/components/modals/create.templ`, `clear.templ`, `hours.templ` — add / clear / set-hours modals.
- `internal/frontend/static/drag.js` — drag-to-move, stretch-to-resize, push-cascade preview.
- `docs/adr/0005-client-computed-push-server-enforced-invariants.md`, `docs/adr/0011-plain-css-cascade-layers-and-scope.md`.
