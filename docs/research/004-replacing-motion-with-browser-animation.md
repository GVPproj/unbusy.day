# 004 — Replacing Motion with browser/CSS animation

Status: implemented by UNB-49  
Date: 2026-09-11  
Scope: focused audit of every app-owned file under `internal/frontend/static/js/`
and `internal/frontend/static/css/app.css`, with extra attention to production
block pointer gestures and the Guide demo.

## Implementation outcome

UNB-49 followed this recommendation. Motion and its transitive runtime graph
were removed rather than vendored; CSS now interpolates planner and Guide
transform/height targets, with JavaScript observing the resulting transitions
for completion.

## Revised conclusion

The earlier recommendation to replace Motion with an app-owned spring/rAF engine
is technically sound, but it does not best serve the explicit goal of **reducing
JavaScript**.

The smallest robust production approach is instead:

1. keep JavaScript in charge of pointer capture, measurements, push/compression
   rules, auto-scroll, optimistic layout, persistence, accessibility, and SSE
   arbitration;
2. let CSS transitions interpolate the transient `transform` and `height`
   values that JavaScript already computes;
3. use a small JavaScript completion barrier over the resulting CSS transitions;
   then perform the existing transform-to-grid FLIP atomically; and
4. accept that the current physical spring becomes a short CSS ease. Preserving
   exact spring velocity across repeated retargets requires JavaScript (Motion or
   a local spring) and conflicts with the reduction goal.

This removes the remote Motion runtime and most Motion-specific value/control
plumbing without inventing a replacement animation library. It will not remove
`pointer.js`, and it should not be presented as a large source-LOC reduction:
the main win is deleting a broad shipped dependency and moving interpolation to
the browser.

## Three distinct outcomes

| Bucket | Opportunities | What remains in JavaScript |
| --- | --- | --- |
| **1. CSS replaces animation/interpolation only** | Production sibling push/compression, held-block resize preview, drag release, resize cancel/release, pickup opacity/scale (already CSS), Guide block movement/height (already CSS), Guide programmatic smooth scrolling | Gesture recognition, target calculation, `pushLayout`, direct pointer tracking, auto-scroll, state, commit, and cancellation |
| **2. CSS eliminates actual JavaScript** | Delete Motion's imported runtime plus `motionValue`/`styleEffect`/animation-control bookkeeping; CSS `scroll-behavior` can remove the Guide's JS-selected `smooth` behavior; existing `@starting-style`, dialog, nav, menu, hover, and entry effects already avoid animation JS | A small completion barrier is still required for gesture FLIP. No remaining app-owned animation-only module can be deleted wholesale. |
| **3. JavaScript cannot reasonably be removed** | Production pointer/keyboard gestures, Guide push demo, Guide step↔scroll synchronization, wall-clock “now”, Jotpad convergence, Habit receipts/recovery, tab ARIA/focus, dialog compatibility, rename focus recovery, shortcuts, and service-worker lifecycle | These read input/time/network/DOM state, mutate semantic state, or invoke business behavior; CSS animation does none of those things. |

The distinction matters: replacing seven Motion animation call sites with CSS
removes substantial shipped JavaScript, but CSS is only the interpolation engine.
It is not a replacement for the gesture module or the client-computed layout
rule accepted by ADR 0005.

## Dependency at audit time and fallback options

At the time of the audit there was one production Motion import, in `pointer.js`, using only
`motionValue`, `styleEffect`, and scalar `animate`. The exact-looking
`motion@13.1.0` jsDelivr URL is not a complete content lock: the package exports
through a ranged `framer-motion@^13.1.0` dependency, which in turn has ranged
Motion DOM/utility dependencies. On the audit date the URL resolved part of the
graph to 13.2.0, while `task check:versions` reported only the top-level 13.1.0
text ([official npm metadata](https://registry.npmjs.org/motion/13.1.0),
[deployed wrapper](https://cdn.jsdelivr.net/npm/motion@13.1.0/+esm)).

A point-in-time fetch of that resolved graph measured about **148 KB raw / 53
KB gzip**. Vendoring must therefore resolve and rewrite the whole graph locally;
copying the small top-level wrapper would retain runtime CDN imports.

| Option | Likely effort | JavaScript result | Main trade-off |
| --- | ---: | --- | --- |
| CSS transitions + small completion barrier | **1–3 days** | Removes Motion and roughly 20–70 app-owned lines | Changes the physical spring to a short CSS ease |
| App-owned rAF spring | **2–4 days** | Removes Motion but adds roughly 100–160 animation lines | Preserves spring-like feel; app owns physics/scheduling |
| Fully vendor Motion | **0.5–1.5 days** | Keeps roughly 148 KB of general-purpose JS | Lowest behavior risk; keeps graph/update burden |

Vendoring remains the right emergency hardening option if changing interaction
feel is unacceptable or the CDN must disappear before the CSS work can be
scheduled. It is not the best long-term JS-reduction option.

## Static frontend audit

### Production blocks

Before UNB-49, [`js/blocks/pointer.js`](../../internal/frontend/static/js/blocks/pointer.js)
was the only Motion consumer. Motion interpolated:

- each displaced sibling's y offset and explicit height;
- the held block's resize height;
- the held block's x/y/rotation on release; and
- cancel/revert targets.

CSS can interpolate all of those properties. During a drag, however, the held
block must continue to track x/y directly with no transition. During resize, JS
must still quantize pointer distance to spans, run `pushLayout`, and assign the
resulting target height. Edge auto-scroll must remain an rAF loop because a
stationary captured pointer can keep moving the scroll container.

The rest of `js/blocks/` is not animation code:

- `push.js` and `keyboard-reducer.js` own domain decisions;
- `grid.js` measures and writes semantic layout;
- `commit.js` persists and restores accessible mirrors;
- `keyboard.js`, `rename.js`, and `gestures.js` own input, focus, and
  arbitration.

CSS cannot replace any of those modules. Removing the HTTPS Motion import would,
however, allow `pointer.js` to stop being dynamically imported solely for Node's
benefit and would make pointer-adjacent tests easier to arrange.

### Guide

[`js/guide/demo.js`](../../internal/frontend/static/js/guide/demo.js) is already
the proof that the visual model works with CSS: JS writes `translate`/`height`,
`.gc-block` transitions them, and JS swaps the settled pixels for `grid-row` in
a FLIP.

Its weak point is not CSS interpolation but completion:

- `SETTLE_MS = 180` duplicates the CSS duration;
- the timer assumes every relevant property started and ended together;
- hiding/closing the dialog or removing a transition can cancel it; and
- the demo lacks a reduced-motion override for `.gc-block` movement, even
  though Guide pane fades honor reduced motion.

Replace the fixed timer with the same completion-barrier pattern as production,
then keep the no-transition FLIP phase. Most of the demo's 157 lines still
remain because it is a real pointer gesture and runs the real push cascade.
Deleting that JS would mean making the demo non-interactive, not making it CSS.

[`js/guide/swipe.js`](../../internal/frontend/static/js/guide/swipe.js) already
uses the right native primitive—scroll snap—for user swipes. CSS
`scroll-behavior: smooth` (and `auto` under reduced motion) can own interpolation
for Back/Next programmatic scrolling and remove the top-level JS `matchMedia`
choice. JS must still synchronize the settled scroll position with
`$_guidestep`, observe Datastar's `.showing` state, and jump correctly when a
closed dialog opens. The `scrollend` event could replace the 120 ms debounce on
the project's supported browser floor, but that is a browser-event refactor,
not a pure-CSS deletion.

Scroll-driven animations are not useful here. They map an animation timeline to
scroll or view progress; they do not set a Datastar signal, make panes inert,
or choose a semantic step. Adding one would create decoration, not remove the
synchronization code ([CSS Scroll-driven Animations Level 1](https://drafts.csswg.org/scroll-animations-1/),
[MDN overview](https://developer.mozilla.org/en-US/docs/Web/CSS/CSS_scroll-driven_animations)).

### Everything else

- `now.js` uses rAF only to coalesce mutation reactions; its one-second clock,
  active/past classification, countdown, and measured fractional slot position
  cannot be driven by CSS.
- `companion.js`, Habit files, Jotpad/CodeMirror files, `save-status.js`, and
  `rename.js` manage semantic, focus, network, or convergence state. Their
  timers are not animation timers.
- `invoker-fallback.js`, `shortcuts.js`, and `sw.js` are compatibility/input/PWA
  behavior, not visual interpolation.
- The 20 vendored CodeMirror `.mjs` modules were also searched. Their only rAF
  use is in `@codemirror/view` for Android-input flushing, DOM measurement, and
  render scheduling—not app animation. It is dependency-owned editor machinery,
  not a pure-CSS replacement candidate.
- At audit time, test files contained no shipped animation path and pointer
  motion lacked automated coverage.
- `app.css` already owns all purely decorative motion: button feedback, login
  spin, dialog/nav entry, menu icon animation, Guide pane fades, block pickup,
  SSE block entry, and check-in feedback. No matching JS animation can be
  deleted.
- `@starting-style` is already used appropriately for login children, dialogs,
  the nav scrim, and SSE-inserted blocks. It supplies a starting style when an
  element lacked a before-change style; it does not animate between two
  measured grid layouts or provide a completion/commit hook
  ([CSS Transitions Level 2](https://drafts.csswg.org/css-transitions-2/#defining-before-change-style),
  [MDN](https://developer.mozilla.org/en-US/docs/Web/CSS/Reference/At-rules/@starting-style)).
  The two-frame `.hydrated` gate remains necessary if initial blocks must stay
  static while later SSE insertions animate—pure CSS cannot distinguish those
  two insertions here.

## Smallest robust production design

### 1. Keep targets and direct manipulation in JS

At gesture start, prime explicit heights and transient transforms. While moving:

- write the held drag block's complete
  `transform: translateX(...) translateY(...) rotate(...)` directly with its
  transform transition disabled;
- write sibling transform/height targets after each accepted `pushLayout`;
- write held resize height targets; and
- keep CSS pickup opacity/scale as-is.

Keep the combined `transform` string. Before replacement, production composed
Motion's transform property with the individual CSS `scale` pickup. Switching x/y/tilt
to individual `translate` and `rotate` properties changes transform composition
order and should not happen accidentally.

### 2. Let CSS transitions handle interruption

A transition retarget starts from the current computed value, so rapid slot
crossings remain position-continuous without JS sampling an in-flight value.
CSS also defines special reversing behavior through a reversing-adjusted start
value and shortening factor. That behavior is robust but is **not** Motion's
velocity-preserving damped spring
([CSS Transitions Level 1, starting transitions and faster reversing](https://drafts.csswg.org/css-transitions-1/#starting)).

Use one short ease and approve the changed feel explicitly. A sampled
`linear()` spring-shaped curve can mimic one uninterrupted spring, but on
retarget CSS still follows transition interruption/reversal rules rather than a
physical oscillator's carried velocity. It adds tuning complexity without
restoring the contract, so it is not the smallest option.

The former ownerless Motion Values also did not receive Motion's WAAPI
acceleration; Motion ran them through its JavaScript frame loop before
`styleEffect` writes styles
([single-value dispatch](https://github.com/motiondivision/motion/blob/e871ba7f175d0609cef84f416f984e8e84be8333/packages/framer-motion/src/animation/animate/subject.ts#L32-L110),
[WAAPI eligibility](https://github.com/motiondivision/motion/blob/e871ba7f175d0609cef84f416f984e8e84be8333/packages/motion-dom/src/animation/waapi/supports/waapi.ts#L26-L78)).
Moving interpolation to CSS is therefore an improvement in ownership and
shipped code, not a retreat from a compositor path the app currently uses.

Raw `Element.animate()` is not a smaller middle ground. WAAPI has no physical
spring primitive, repeatedly retargeting a combined transform still needs
presentation-value/velocity bookkeeping, and canceling an animation rejects
its `finished` promise. It moves the lifecycle complexity without eliminating
it. If CSS easing is rejected on feel, use vendored Motion for minimum risk or
the focused local spring described in the options table—not a WAAPI wrapper.

### 3. Use a real completion barrier

A bare `setTimeout` or single `transitionend` listener is not sufficient:

- events are per element and per transitioned longhand;
- unchanged values and zero durations produce no transition events;
- `transitioncancel` replaces `transitionend` when a transition is canceled;
- removing `transition-property`, setting `display: none`, or detaching a node
  can cancel running work; and
- a release can cancel preview transitions while starting final ones.

These are specified event states, not edge browser behavior
([CSS Transitions events](https://drafts.csswg.org/css-transitions-1/#transition-events),
[MDN `transitionend`](https://developer.mozilla.org/en-US/docs/Web/API/Element/transitionend_event),
[MDN `transitioncancel`](https://developer.mozilla.org/en-US/docs/Web/API/Element/transitioncancel_event)).

The smallest reliable barrier is to set final CSS targets, collect the owned
CSS transitions with `Element.getAnimations()`, filter to the gesture's
`transform`/`height` transitions, and await every `animation.finished` with
`Promise.allSettled`. `getAnimations()` includes CSS transitions; canceled Web
Animations reject their current finished promise, hence `allSettled`
([MDN `getAnimations`](https://developer.mozilla.org/en-US/docs/Web/API/Element/getAnimations),
[MDN `Animation.finished`](https://developer.mozilla.org/en-US/docs/Web/API/Animation/finished),
[MDN `Animation.cancel`](https://developer.mozilla.org/en-US/docs/Web/API/Animation/cancel)).
No-op and zero-duration cases yield no owned running transition and complete
immediately.

If transition events are preferred, the helper must listen to both `transitionend`
and `transitioncancel`, key by element + `propertyName`, distinguish the final
transition generation from canceled previews (for example via `transitionrun`),
and explicitly resolve properties that never start. That is more bookkeeping
than observing the browser's animation objects.

### 4. Preserve the FLIP and make cancellation authoritative

After the barrier, in one synchronous phase:

1. suppress gesture transitions with a class;
2. write `data-slot`, `data-span`, `--span`, and `grid-row`;
3. clear transient transform/height;
4. force/await a style boundary before removing suppression; and
5. dispatch the existing commit only if authoritative DOM state still matches
   the gesture's start state.

This retains the current FLIP: old grid placement plus transform and new grid
placement without transform describe the same pixels. CSS cannot perform the
semantic attribute/grid write, so it cannot eliminate this JS.

SSE needs an explicit stale-generation guard. The CSS Transitions specification
requires running transitions on a detached element to be canceled, and an
animation-promise barrier can absorb that cancellation. A keyed idiomorph may,
however, preserve the held element while changing its datasets/grid placement.
A `parentElement === list` check alone does not prove that the gesture still
owns the layout. Before writing, compare the live block layout/block set with
the gesture-start snapshot (or cancel from a relevant morph observer); if they
differ, clear only transient gesture styles and let the server win. Cover both
replacement and identity-preserving SSE morphs.

### 5. Avoid transition shorthand collisions

`transition` is a shorthand for property, duration, timing function, delay, and
behavior, so every shorthand declaration resets the whole transition list
([MDN `transition`](https://developer.mozilla.org/en-US/docs/Web/CSS/Reference/Properties/transition)).
This stylesheet already has overlapping shorthands for:

- `.block-item.dragging` pickup opacity/scale;
- `.blocks.hydrated .block-item` SSE entry opacity/scale;
- the hydrated dragging override; and
- Guide normal/dragging/resizing states.

Do not add a broad `transition: all`, and do not toggle
`element.style.transition = "none"` in production. Define explicit gesture
state classes after the entry rules, with complete property/duration/easing
lists for each state. The held drag state must omit `transform` while retaining
opacity/scale; siblings and resize preview include only `transform` and/or
`height`; the FLIP suppression class temporarily sets `transition-property:
none`. In the Guide, replace the inline shorthand suppression with the same
class pattern so future transitions are not silently erased.

### 6. Reduced motion

Keep direct pointer tracking and scrolling under user control, but remove the
settle/push easing and decorative tilt. Put the gesture duration in one CSS
custom property and set it to `0s` under `prefers-reduced-motion: reduce`; the
completion barrier must treat the resulting absence of transitions as normal.
Add the same override to `.gc-demo .gc-block`. For Guide Back/Next scrolling,
use `scroll-behavior: auto` under reduced motion
([MDN `prefers-reduced-motion`](https://developer.mozilla.org/en-US/docs/Web/CSS/Reference/At-rules/@media/prefers-reduced-motion),
[MDN `scroll-behavior`](https://developer.mozilla.org/en-US/docs/Web/CSS/Reference/Properties/scroll-behavior)).

## Realistic implementation and test impact

Anticipated production source change:

- `pointer.js`: roughly **40–80 fewer lines** after deleting Motion Values,
  effects, and stop/control bookkeeping, then adding direct style targets and
  barrier calls;
- a narrow transition-completion helper: roughly **20–40 lines** if shared by
  production and Guide;
- `app.css`: roughly **20–40 added/changed lines** for explicit gesture states,
  FLIP suppression, one duration token, and reduced motion;
- `guide/demo.js` and `guide/swipe.js`: approximately neutral to **10–20 fewer
  lines**—the timer/behavior plumbing leaves, but lifecycle state remains.

Net app-owned JavaScript is therefore likely only **20–70 lines smaller**. The
larger reduction is the removed Motion graph (the prior audit measured the then
resolved graph at about 148 KB raw / 53 KB gzip); no local spring module replaces
it.

Tests should grow, not shrink:

- **40–80 lines** of deterministic barrier tests: zero transitions, multiple
  elements/properties, cancellation rejection, filtering unrelated pickup/entry
  transitions, and stale generations;
- **120–220 lines** of browser pointer tests: drag/resize, rapid retarget,
  cancel, reduced motion, auto-scroll, cleanup, and keyboard arbitration;
- **50–100 lines** for Guide completion/close/reduced-motion and SSE replacement
  versus identity-preserving morph cases.

Expect roughly **1–3 engineering days** if changing spring feel is approved.
Keeping exact spring feel still points to vendored Motion (lowest behavioral
risk) or the earlier 100–160-line app-owned spring plus substantially more unit
testing; neither is the smallest JavaScript-reduction path.

## Revised recommendation

Use CSS transitions for production block interpolation and keep JavaScript as
the gesture/state controller. Base production on the Guide's target-writing
model, but do **not** copy its fixed timer or inline transition shorthand.
Introduce one small animation-observation barrier, preserve the current FLIP,
make SSE/morph cancellation authoritative, and add reduced-motion coverage to
the Guide.

Do not build a local spring unless preserving the exact Motion feel is an
explicit product requirement after comparing a short CSS ease on real pointer
hardware. Do not use scroll-driven animation for Guide state synchronization,
and do not try to move push/commit logic into CSS.

This is the best fit for the stated goal: it removes Motion and animation-loop
JavaScript while leaving business and input state in the only layer that can
reasonably own it.

## Primary sources consulted

- [CSS Transitions Level 1](https://drafts.csswg.org/css-transitions-1/):
  transition generation, interruption/reversal, cancellation, and events.
- [CSS Transitions Level 2](https://drafts.csswg.org/css-transitions-2/):
  `@starting-style`, discrete transitions, and transition generations.
- MDN:
  [`transition`](https://developer.mozilla.org/en-US/docs/Web/CSS/Reference/Properties/transition),
  [`@starting-style`](https://developer.mozilla.org/en-US/docs/Web/CSS/Reference/At-rules/@starting-style),
  [`transitionend`](https://developer.mozilla.org/en-US/docs/Web/API/Element/transitionend_event),
  [`transitioncancel`](https://developer.mozilla.org/en-US/docs/Web/API/Element/transitioncancel_event),
  [`Element.getAnimations()`](https://developer.mozilla.org/en-US/docs/Web/API/Element/getAnimations),
  [`Animation.finished`](https://developer.mozilla.org/en-US/docs/Web/API/Animation/finished),
  [`Animation.cancel()`](https://developer.mozilla.org/en-US/docs/Web/API/Animation/cancel),
  [`prefers-reduced-motion`](https://developer.mozilla.org/en-US/docs/Web/CSS/Reference/At-rules/@media/prefers-reduced-motion),
  and [`scroll-behavior`](https://developer.mozilla.org/en-US/docs/Web/CSS/Reference/Properties/scroll-behavior).
- [CSS Scroll-driven Animations Level 1](https://drafts.csswg.org/scroll-animations-1/)
  and [MDN's overview](https://developer.mozilla.org/en-US/docs/Web/CSS/CSS_scroll-driven_animations).
- Motion official docs for the behavior being traded away:
  [`motionValue`](https://motion.dev/docs/motion-value),
  [`styleEffect`](https://motion.dev/docs/style-effect), and
  [`animate`](https://motion.dev/docs/animate).
