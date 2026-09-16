# 004 — Replacing Motion with browser/CSS animation

Status: implemented by UNB-49
Date: 2026-09-11

## Outcome

Motion and its transitive runtime graph were removed rather than vendored. CSS
now interpolates planner and Guide transform/height targets; JavaScript still
owns gestures, layout, state, cancellation, and transition completion.

The shipped design:

- `js/blocks/gestures.js` statically imports `pointer.js`; the old HTTPS Motion
  import and dynamic-import workaround are gone.
- Pointer and Guide code write transient `transform`/`translate` and `height`
  targets directly; CSS owns the transition declarations and shared
  `--gesture-duration`. The existing transform-to-grid FLIP remains.
- Both surfaces use `waitForTransitions` from `js/transitions.js` rather than
  duplicate fixed-duration timers.
- Cancellation is authoritative: transition cancellation, SSE replacement, and
  detached nodes cannot strand completion or commit stale state.
- Planner and Guide set gesture duration to zero under reduced motion; pointer
  tilt is suppressed too.
- Guide Back/Next interpolation uses CSS `scroll-behavior: smooth`, with `auto`
  under reduced motion. JavaScript requests an instant jump only when reopening
  the dialog requires one.

The physical spring became a short CSS ease. That was the deliberate product
trade: preserving spring velocity across repeated retargets requires Motion or a
local JavaScript spring and conflicts with the goal of reducing shipped
JavaScript.

## Why CSS did not replace the gesture modules

Motion was only the interpolation engine. The remaining JavaScript reads input,
time, network, and DOM state or applies business behavior:

- pointer capture, direct manipulation, auto-scroll, keyboard gestures, and
  rename/focus arbitration;
- push/compression target calculation and full-layout commit;
- SSE/morph cancellation and identity checks;
- Guide pointer/swipe state and step synchronization;
- the wall clock, Jotpad convergence, Habit receipts/recovery, shortcuts, and
  service-worker lifecycle.

CSS cannot perform those jobs. The useful boundary is:

1. JavaScript computes semantic state and target geometry.
2. CSS interpolates visual properties.
3. A small JavaScript barrier observes completion or cancellation.
4. JavaScript settles the semantic layout atomically.

That boundary preserves ADR 0005's client-computed Push while removing a broad
animation dependency.

## Audit findings that drove the decision

At audit time, `pointer.js` was the only production Motion consumer. It used
`motionValue`, `styleEffect`, and scalar `animate`; no component animation API
or spring orchestration justified the rest of the package graph.

The exact-looking `motion@13.1.0` jsDelivr URL was not a complete content lock:
its ranged transitive dependencies resolved part of the graph to newer versions.
A point-in-time fetch measured roughly **148 KB raw / 53 KB gzip**. Copying the
small top-level wrapper would therefore have retained runtime CDN imports;
proper vendoring would have required resolving and rewriting the complete graph.

Three options were considered:

| Option | Result | Trade-off |
| --- | --- | --- |
| CSS transitions + completion barrier | **Chosen** | Smallest shipped JS; spring becomes an ease. |
| App-owned `requestAnimationFrame` spring | Rejected | Preserves spring feel but adds roughly 100–160 lines of scheduling/physics code. |
| Vendor Motion | Rejected | Lowest behavior change but keeps the large general-purpose graph. |

The Guide already proved the target-writing model, but its old `SETTLE_MS` timer
was not robust: it duplicated CSS duration and could be invalidated by canceled
or absent transitions. Sharing one completion barrier fixed that weakness while
keeping the interactive demo and real Push cascade.

## Completion-barrier contract

The shared helper observes only the elements and properties owned by the
current gesture. It must:

- complete immediately when there is no transition or duration is zero;
- wait for every relevant property across every relevant element;
- treat `transitioncancel` as completion for that property;
- ignore unrelated decorative transitions;
- settle if nodes detach or browser events are missing; and
- let the caller's operation/identity guards decide whether a commit is still
  authoritative.

The helper observes animation; it does not own gesture state or persistence.

## Validation

The implementation added deterministic unit coverage in
`js/transitions.test.js` for multiple properties/elements, zero-duration and
no-op targets, cancellation, and unrelated transitions. Existing pointer and
Guide Playwright coverage exercises drag, resize, close/reopen, SSE/morph
interaction, and reduced motion.

The lasting constraints are:

- keep transition property lists explicit;
- keep target calculation in JavaScript and interpolation in CSS;
- preserve cancellation/identity checks around every asynchronous settle;
- keep reduced motion at zero duration rather than adding a parallel code path;
- do not introduce a local spring unless exact spring feel becomes an explicit
  product requirement.

## Primary sources consulted

- [CSS Transitions Level 2](https://drafts.csswg.org/css-transitions-2/)
- MDN: [`transitionend`](https://developer.mozilla.org/en-US/docs/Web/API/Element/transitionend_event),
  [`transitioncancel`](https://developer.mozilla.org/en-US/docs/Web/API/Element/transitioncancel_event),
  [`getAnimations()`](https://developer.mozilla.org/en-US/docs/Web/API/Element/getAnimations),
  [`prefers-reduced-motion`](https://developer.mozilla.org/en-US/docs/Web/CSS/Reference/At-rules/@media/prefers-reduced-motion),
  and [`scroll-behavior`](https://developer.mozilla.org/en-US/docs/Web/CSS/Reference/Properties/scroll-behavior)
- Motion docs: [`motionValue`](https://motion.dev/docs/motion-value),
  [`styleEffect`](https://motion.dev/docs/style-effect), and
  [`animate`](https://motion.dev/docs/animate)
