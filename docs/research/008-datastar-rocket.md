# 008 — Datastar Rocket and the existing JavaScript boundaries

Status: research (no application code changed)
Date: 2026-10-01

## Conclusion

**Rocket is now free, but it is not a replacement for this app's JavaScript.** It is a beta JavaScript custom-element API with reactive props, instance-local signals, rendering and lifecycle hooks. The useful opportunity is encapsulating browser-owned widgets, not moving server-rendered Blocks/Habits into client templates.

Do not switch bundles merely to reduce JS. Most existing JS implements gestures, editor behavior or asynchronous correctness that Rocket would still need to call. The strongest conditional candidate is a lifecycle wrapper around the Jotpad; the safest small experiment is a Guide demo. Neither currently demonstrates enough payoff to justify a production migration.

This assessment uses the working tree at `c954ab13465bed1ad602fe65a7c2e9626454b7e5`, `AGENTS.md`, all 21 non-test modules under `internal/frontend/static/js/` (2,580 lines), `static/sw.js`, templ integration sites, vendor instructions and existing research notes. Counts exclude vendor code, inline templ scripts and tests. Recommendations below are engineering judgments, not upstream performance promises.

## Verified availability, version, licensing and cost

Evidence retrieved with `curl` on **2026-10-01**:

| Question | Finding and primary evidence |
| --- | --- |
| Is Rocket actually free now? | Yes. The official [v1.0.4 release](https://github.com/starfederation/datastar/releases/tag/v1.0.4), published 2026-09-21, explicitly says Rocket was taken out of Datastar Pro and is available as a free bundle. |
| Which version? | The [Rocket reference][rocket] and [Getting Started][start] link `v1.0.4/bundles/datastar-rocket.js`. GitHub's [latest-release API](https://api.github.com/repos/starfederation/datastar/releases/latest) also returned `v1.0.4` at retrieval. This is a dated observation, not a floating version recommendation. |
| Is the whole Pro product free? | No. [Datastar Pro][pro] remains commercial, one-time/lifetime: the page displays Solo $349 (struck-through $399) and Team $1,299 (struck-through $1,499); Enterprise is contact-sales. These prices are not a Rocket fee. No checkout was performed. |
| May this public repo vendor Rocket? | The public v1.0.4 [LICENSE.md][license] grants MIT permissions, including modification/distribution, with copyright/permission notice retention. Rocket source and artifacts are in that public release tree. Conversely, the Pro FAQ prohibits public-repo/open-source redistribution of Pro software. Use the public release artifact, **not the documentation site's Pro demo runtime**. |
| Is it in our existing bundle? | No. Our pin is already 1.0.4, but it loads `datastar.js`, not `datastar-rocket.js`. The [standard bundle source][standard] has no Rocket export; the [Rocket bundle source][bundle] includes the same plugins plus Rocket exports. This is a bundle-flavor change at the same version, not necessarily a version upgrade. |
| Must we add a build tool? | No. Official docs provide an importable browser ES module and matching `.d.ts`. Existing Go-only production build and static vendoring can remain. TypeScript declarations are optional editor tooling, not a required compiler. |
| Is the API stable? | No. The reference explicitly labels Rocket **beta, API subject to change** even though the containing Datastar release is 1.0.4. |

Downloaded the two unmodified public v1.0.4 bundles from `raw.githubusercontent.com`, then measured file bytes and Python `gzip.compress(..., mtime=0)` (default level 9):

| Artifact | Raw bytes | gzip bytes | SHA-256 |
| --- | ---: | ---: | --- |
| `datastar.js` | 33,553 | 13,413 | `727844adfc825ee651fb93c544a2a739986f9a21820a94524b35f0cac470cf91` |
| `datastar-rocket.js` | 65,470 | 23,662 | `c723fb309157a555609df273e4693dcc4e193e68e6d7477f3285b955288e1e7b` |

Rocket adds **31,917 raw / 10,249 gzip bytes** before any app component code. These are local artifact measurements, not actual HTTP transfer sizes or CPU benchmarks. The published sources are [standard JS](https://raw.githubusercontent.com/starfederation/datastar/v1.0.4/bundles/datastar.js) and [Rocket JS](https://raw.githubusercontent.com/starfederation/datastar/v1.0.4/bundles/datastar-rocket.js). Removing a few listener registrations would not offset that cost.

## What the verified API does

The [reference][rocket], [declarations][types] and tagged [runtime][runtime] agree on the relevant interface:

- `rocket('some-widget', definition)` registers a custom element. Public `props` use codecs; camelCase prop names map to kebab-case attributes. Codecs normalize invalid/missing values rather than supplying server-side domain validation.
- `setup({ host, props, $, $$, effect, observeProps, action, actions, cleanup, emit, ... })` establishes instance behavior. `$` accesses page signals; `$$` accesses instance-local signals. `action(name, fn)` exposes component actions via `@dispatchRocket(...)`; `actions` calls existing Datastar actions imperatively.
- `onFirstRender` runs after initial render and Datastar application, with `refs` available. It is the appropriate DOM/widget initialization seam, not a promise that hidden content is measurable.
- `render({ html, svg, props, ... })` returns DOM; prop changes queue/coalesce rerenders by default. `renderOnPropChange: false` and `observeProps` support imperative widgets. **Omitting `render` is supported**: setup, scoping and actions still work, without Rocket morphing a rendered subtree.
- `mode` defaults to **open shadow DOM**; `light` uses the host itself. For this repo, explicit `mode: 'light'` is usually the viable starting point, so document selectors, delegated events and `app.css` still reach the content. CSS custom properties crossing a shadow boundary does not make the existing stylesheet selectors cross it.
- `cleanup(fn)` registers disconnect cleanup; effects created through the context are tracked. Arbitrary listeners, timers, animation frames, observers, CodeMirror views and native `fetch` continuations are **not automatically made safe** just because their creation occurs in setup.
- Local expression names are rewritten under `$._rocket`, using a normalized host ID or generated instance ID. Scope rewriting also affects authored children. `__root` is an escape for selected signal-name attributes (`data-bind`, `data-computed`, `data-indicator`, `data-ref`), not a universal no-scoping flag. Wrapping existing templ markup therefore needs a binding audit, not just a new outer tag.

The official [Copy Button example](https://data-star.dev/examples/rocket_copy_button) illustrates the intended small reusable widget shape. Its [served component source](https://data-star.dev/static/rocket/copy-button-963ebbef9aa9e3f1eb79c237ec20479e8a2502ec1de917f853dd7c6efa95fb75.js) uses local signals, clipboard behavior and timeout cleanup. That is a much narrower problem than this app's replicated editor or authoritative layout commits.

## Actual integrations and per-module fit

Paths below are repository-primary evidence; all JS paths are relative to [`internal/frontend/static/js/`](../../internal/frontend/static/js/). These rows cover production modules, not just files that import Datastar directly.

| Module(s) | Current responsibility and integration | Rocket fit / limitation |
| --- | --- | --- |
| `blocks/gestures.js` | Initializes pointer/keyboard arbitration once on stable `#block-list`; `routes/blocks.templ` supplies announcer. | A behavior-only light-DOM wrapper could own initialization/disposal. Today the stable delegated container already avoids per-block rebinding; wrapping alone removes little. |
| `blocks/pointer.js` | Pointer capture, auto-scroll, Push previews, CSS target geometry, FLIP settle, pre-morph cancellation and snapshot ownership. | Keep. Rocket supplies neither drag mechanics nor authoritative layout arbitration. Module-level instance state would have to become per-instance before advertising reusable components. |
| `blocks/keyboard.js`, `keyboard-reducer.js` | Keyboard move/resize/delete/rename, pointer arbitration, accessible announcements; pure reducer uses the same Push rules. | Keep reducer and behavior. Rocket cannot infer keyboard semantics. `keyboard.js` also has module-level state requiring refactoring for multiple instances. |
| `blocks/push.js`, `grid.js`, `commit.js` | Pure cascade/compression; DOM-layout conversion; shared commit guards, accessible mirrors and `layout` CustomEvent. | Do not replace. These are explicit ADR 0005 logic and contracts, not rendering boilerplate. |
| `blocks/rename.js` | Plaintext editing, caret placement, blur/Enter/Escape and bounded post-morph focus restoration. | Could attach cleanup to a future owner, but edit/focus rules remain. A custom element per block adds lifecycle churn to a server-owned list. |
| `jot/cm.js` | Persistent CodeMirror view, Markdown/task decorations, transaction limits, remote minimal edits, Android/panel-return focus/scroll protections; assigns `window.__jotRemote`. | Best potential encapsulation payoff: a host API can replace the window singleton and own editor disposal. It does not replace CodeMirror or these fixes. Keep the editor subtree imperatively owned, never Rocket-render text into it. |
| `jot/sync.js` | CAS JSON POST/ack, remote-version buffering, debounce/backoff, save state, online/offline, keepalive and teardown beacon. | Keep as an independent driver. It currently returns no disposer; Rocket cleanup cannot invent one. A wrapper first needs explicit listener/timer/in-flight ownership and a carefully defined flush/dispose contract. |
| `guide/demo.js` | Multiple `.gc-demo` instances, real Push simulation, CSS preview and generation-guarded async settle; dialog close abort. | Safest Rocket trial: real repeated widgets, isolated from persistence. Prefer behavior-only light DOM around templ-authored examples; retain Push, transitions and close/generation guards. Initialization already uses per-instance closures, so savings are modest. |
| `guide/swipe.js` | Scroll-settle debounce, class/open observers and `guidestep` event bridge to `$_guidestep`. | Some timer/observer lifecycle benefit if Guide instances become dynamic. Still needs feedback-loop suppression, closed-dialog geometry handling and close/reopen behavior. |
| `now.js` | Local-clock date/now/countdown, block classes, geometry measurement, mutation observer suspended during own writes, interval. | A plausible lifecycle owner for timer/observer, not a replacement for time/geometry computation. One clock affects several separate DOM surfaces; putting each pill in its own component risks duplicated timers/scans. |
| `habits/checkins.js` | Document-scoped attempt/source maps, receipt correlation, original-intent retry, read-generation fences, aggregate save state, scroll/focus restoration after grid morph. | Do not scope attempts to disposable cell elements. They intentionally survive grid replacement and navigation. Rocket local state disappearing on disconnect would lose unknown-write intent. |
| `habits/reorder.js` | Pointer/keyboard row order, capture on stable matrix, server-patch cancellation, dialog cancellation, post-morph focus and `reorder` event. | At most an outer lifecycle wrapper. Keep table structure, authoritative grid ownership and whole-order payload; component-per-row is especially unattractive for reparenting/capture. |
| `habits/forms.js` | Classic-script date seeding **before Datastar binding**, draft ownership, native validation feedback, fetch-status aggregation. | Little payoff; module/custom-element initialization can change ordering and overwrite seeded dates or drafts. Preserve view-keyed responses in templ and native form/dialog behavior. |
| `companion.js` | 39-line native tab keyboard/ARIA behavior, persistent hidden panels, synchronous `companion-hide` before hiding and `companion-show` after showing. | Could be reusable tabs, but little net reduction for one instance. Conditional removal instead of `hidden` breaks the persistent editor contract. |
| `save-status.js` | 23-line per-document/per-writer status priority reducer and output update. | Keep shared aggregation. Local component status must not hide another writer's pending/failed state. |
| `transitions.js` | 11-line `getAnimations()`/`Promise.allSettled()` completion barrier filtered to owned CSS properties. | Keep. Rocket is not an animation/gesture completion engine; CSS already replaced Motion (research 004). |
| `shortcuts.js`, `invoker-fallback.js` | Focus-guarded `?` shortcut; feature-detected native dialog invocation/light-dismiss fallbacks. | Keep native HTML and small browser fallbacks. Custom elements do not add missing browser dialog support. |
| `static/sw.js` | Passthrough PWA service-worker lifecycle. | No fit: separate worker environment, not DOM components. |

### Templ glue is already doing the declarative work

- [`layouts/layout.templ`](../../internal/frontend/layouts/layout.templ) owns pre-paint theme migration/storage, root theme signals/effects, service-worker registration and body `/events` initialization. Rocket's deferred module lifecycle cannot replace pre-paint work.
- [`routes/blocks.templ`](../../internal/frontend/routes/blocks.templ) mounts gestures/CodeMirror and routes `_jotv`/`_jott` patches through `data-on-signal-patch`. Both panels remain present. The Jotpad is not an SSE element-patch target.
- [`components/column.templ`](../../internal/frontend/components/column.templ) is the shared initial/SSE renderer. It translates `layout`, `rename`, `delete` CustomEvents into signals and `@post`; its inline date script handles first paint. Do not duplicate this renderer as Rocket `html` or `data-for`.
- [`components/habits.templ`](../../internal/frontend/components/habits.templ) owns week/following/view/refresh/read signals, guarded week requests, reorder POST and check-in payloads. Create/edit/delete templ files use view-keyed acknowledgements and explicit cancellation/retry policy. These are server-protocol concerns, not missing component reactivity.
- [`components/companion.templ`](../../internal/frontend/components/companion.templ), `jotpad.templ`, nav and modal components already combine server HTML, native controls and small declarative attributes. Guide uses `_guidestep` plus `guidestep` events. Theme uses native dialog/radio markup.
- [`components/login.templ`](../../internal/frontend/components/login.templ) bridges Turnstile callbacks through window events, posts native forms, sanitizes OTP input and focuses after SSE replacement with `data-init`. Keep third-party callback and authentication contracts; there is no repeated client widget here needing Rocket.
- [`layouts/datastar.templ`](../../internal/frontend/layouts/datastar.templ) is shared by the app and [`smoke.templ`](../../internal/frontend/smoke.templ), so a bundle switch affects login and the wiring canary too.

## Lifecycle, morphing and asynchronous correctness

These are adoption gates, not reasons to assume Rocket is incompatible with SSE.

1. **Cleanup is connection-scoped, not “once for the page.”** Tagged runtime `connectedCallback()` invokes setup and initial render; `disconnectedCallback()` clears the instance signal path, actions, refs, prop observers and registered cleanups, then allows reconnection. A retained host whose children morph does not thereby receive disconnect cleanup. Existing child observers/gesture cancellation still matter. [Runtime][runtime]
2. **Moves require testing, not an identity assumption.** Datastar's morph implementation uses `moveBefore` where available and falls back to `insertBefore`. Rocket has immediate disconnect teardown in the inspected release. Test reorder, subtree replacement and reconnect across supported browsers; stable IDs help matching but do not promise uninterrupted component state. [Morph source][morph]
3. **One writer per subtree.** Rocket rendering uses an inner morph. The patcher supports post-morph Rocket child scoping, but this is not arbitration between server DOM, Rocket render and CodeMirror. For existing surfaces choose templ ownership plus a behavior-only wrapper; for CodeMirror keep a persistent mount and no competing render. `renderOnPropChange: false` stops prop-triggered rerenders, not the initial render or arbitrary explicit `render()` calls. [Runtime][runtime], [reference][rocket]
4. **Async work remains app-owned.** Official docs' native-fetch example itself uses a `cancelled` flag registered with `cleanup`. Use per-operation generations/identity checks after every await and explicit cancellation where appropriate. A response from an old connection/prop snapshot must not update a new instance. Observe-props callbacks run synchronously for each change even when renders coalesce; a multi-attribute server patch can expose intermediate combinations to imperative callbacks. Do not start coupled requests from each prop without batching/snapshot discipline. [Reference: render/props][rocket], [runtime][runtime]
5. **Network cancellation is not transaction rollback.** Datastar fetch actions offer default `auto`, `cleanup`, `disabled`, or an `AbortController`; cleanup-bound cancellation applies to those actions, not arbitrary native fetches. v1.0.4 fixed cleanup/supersession races, which our existing pin already includes. Neither cancellation nor `finished` establishes that a mutation did/did not commit. Keep Habit receipt/read fences, unknown-intent recovery and deliberate `requestCancellation: 'disabled'`; keep Jot CAS/keepalive/beacon behavior. [Actions][actions], [release][release], local `checkins.js`/`sync.js`
6. **Do not delete correctness guards to make a component smaller.** Pointer settle must still yield to an incoming column patch even when the element identity/placement looks unchanged. Guide must still invalidate an awaited settle after close/reopen. Jot must preserve selection/history and buffered remote convergence. Habits must reject stale week/view reads and distinguish acknowledged write from confirmed authoritative refresh.
7. **Setup is not an automatic disposer retrofit.** Current gesture initializers expose arbitration, not removal of all listeners; `initBlockGestures` returns nothing. Jot attaches page listeners and installs a singleton bridge; its driver has no destroy API. A sound Rocket wrapper requires those seams to gain explicit ownership first. That improvement can be implemented without Rocket if dynamic mounting ever becomes necessary.
8. **Attribute initialization is a different lifecycle.** Ordinary `data-init` can run on initial load, insertion or attribute modification, per the [attribute reference][attributes]. Replacing top-level initialization with `data-init` alone also needs idempotency and cleanup; it is not evidence that Rocket is necessary or that repeated initialization is safe.

## Bundle/vendoring implications if a trial succeeds

Follow [`static/vendor/datastar/README.md`](../../internal/frontend/static/vendor/datastar/README.md), with explicit changes for the bundle flavor:

1. Vendor public tag artifacts `bundles/datastar-rocket.js`, `bundles/datastar-rocket.js.map` and `LICENSE.md`; optionally the matching declarations. Preserve the upstream source-map basename and review the license/hashes. No runtime CDN and no Pro demo asset.
2. **Replace, do not load alongside, `datastar.js`.** All component imports and the shell must resolve to the same local Rocket module URL. The combined bundle already includes the engine/plugins; loading a second standalone runtime risks duplicate stores/watchers. This replacement rule is explicit in the [official bundle instructions][start].
3. Keep one version source in `layouts/datastar.templ`. A clear filename such as `datastar-rocket-1.0.4.js` requires updating `task check:versions`: [`Taskfile.yml`](../../Taskfile.yml) currently extracts only `datastar-([0-9.]+).js` and checks that filename. Alternatively retain the current versioned naming scheme but document the changed flavor unmistakably. Do not silently break version checking.
4. Update the vendor README's upstream paths, SHA256SUMS, and [`THIRD_PARTY_LICENSES.md`](../../THIRD_PARTY_LICENSES.md) inventory description. The Go SDK pin (`datastar-go v1.2.2`) need not change just to choose Rocket; verify the existing wire contract rather than assuming matching client/server version numbers.
5. Run offline hash verification, `task check:versions`, `task test`, `task test:browser:smoke` and full `task test:browser`. Add focused tests for duplicate mount, detach/reconnect, server morph during interaction, stale async completion and preserved signal payloads. Jot trials additionally need slow/offline concurrent edits, beacon deduplication and Android focus/scroll coverage.
6. Measure total compressed JS, startup cost and actual removed complexity. An API wrapper with unchanged module logic is an encapsulation change, not a bundle-size optimization.

No bundle switch or browser prototype was performed in this research. Source-level compatibility is not browser validation.

## Recommendations ranked by actual payoff

1. **Highest payoff now: retain existing module boundaries and current standard bundle.** No fee blocks adoption, but no large replaceable JS layer was found. Keep server-rendered truth, native controls, pure reducers and the tested async drivers. This avoids a beta dependency surface and roughly 10 KB additional gzip for little demonstrated code removal.
2. **Highest potential Rocket payoff, conditional: Jotpad lifecycle/API encapsulation.** Revisit if editors become dynamically mounted or multiple instances are needed. A light-DOM host API could retire `window.__jotRemote` and consolidate editor lifetime. First design a real driver disposer and persistence policy; keep CodeMirror, sync and the persistent subtree. This is medium/high-risk work, not a quick JS deletion.
3. **Best low-risk experiment: one Guide demo behavior component.** Existing repeated instances make local state/cleanup tangible. Retain templ markup and call existing logic; prove detach/reconnect and close-during-settle tests. Reject the migration if it merely moves an initializer into `setup` and adds wrappers/imports.
4. **Low payoff: clock or reusable tabs only after a reuse requirement appears.** These could demonstrate lifecycle APIs but one page-global clock and a 39-line tabs module do not justify changing every page's runtime. Ordinary init/dispose functions remain a simpler alternative.
5. **Do not pursue:** Rocket-rendered Blocks/Habit grids; per-cell persistence components; replacing Push, reducers, commit guards, Jot sync, Habit recovery, CSS transitions, native dialogs, authentication glue, pre-paint scripts or the service worker. These solve problems outside Rocket's abstraction, or their lifetime must outlast disposable DOM.

Reconsider after beta API stabilization or an actual component reuse/dynamic-lifecycle requirement. The adoption criterion should be fewer ownership bugs and a smaller comprehensible interface—not fewer `.js` filenames or transferring the same code into template strings.

## Primary sources

All web sources accessed 2026-10-01. Tagged source/artifacts are preferred over mutable reference pages when interpreting v1.0.4 behavior. Official announcement videos were linked by the release but were not used as evidence; the release text and published source establish the relevant claims.

[rocket]: https://data-star.dev/reference/rocket
[start]: https://data-star.dev/guide/getting_started
[pro]: https://data-star.dev/pro
[release]: https://github.com/starfederation/datastar/releases/tag/v1.0.4
[license]: https://github.com/starfederation/datastar/blob/v1.0.4/LICENSE.md
[standard]: https://github.com/starfederation/datastar/blob/v1.0.4/library/src/bundles/datastar.ts
[bundle]: https://github.com/starfederation/datastar/blob/v1.0.4/library/src/bundles/datastar-rocket.ts
[runtime]: https://github.com/starfederation/datastar/blob/v1.0.4/library/src/rocket/runtime.ts
[types]: https://github.com/starfederation/datastar/blob/v1.0.4/bundles/datastar-rocket.d.ts
[morph]: https://github.com/starfederation/datastar/blob/v1.0.4/library/src/plugins/watchers/patchElements.ts
[actions]: https://data-star.dev/reference/actions
[attributes]: https://data-star.dev/reference/attributes#data-init
