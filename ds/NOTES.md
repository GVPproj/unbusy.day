# Adapter B (Datastar + templ) — verified API surface

M2.5a output (PRD §8, F16). Pin-and-verify spike whose only product is this
document plus the `/ds/_smoke` page that proved every claim below by actually
landing a patch in a real browser. The point of the spike is to keep M2.5b from
baking RC-era names that silently no-op (PRD §10).

## Pinned versions

| Layer | Version | Where |
| --- | --- | --- |
| Datastar Go SDK | `v1.2.2` | `go.mod` → `github.com/starfederation/datastar-go/datastar` |
| Datastar JS runtime | `v1.0.2` (latest stable tag on the SF repo at spike time) | `<script type="module" src="https://cdn.jsdelivr.net/gh/starfederation/datastar@v1.0.2/bundles/datastar.js">` in `smoke.templ` |
| templ | `v0.3.1020` (runtime) — CLI `v0.3.1020` | `go.mod`; CLI from `go install github.com/a-h/templ/cmd/templ@latest` |
| SortableJS | `1.15.7` | `<script src="https://cdn.jsdelivr.net/npm/sortablejs@1.15.7/Sortable.min.js">` in `smoke.templ` |

The Go SDK and JS runtime are **versioned independently** — the SDK is much
further ahead in semver. That's expected on this project; the wire contract is
what we verified, not the version alignment.

## Headline F16 finding — `data-on-load` is a silent no-op

The hand-crafted `data-on-load="@get('/ds/_smoke/events')"` opener (which the
[templ.guide datastar page][templguide] still shows in stale-name form
`MergeFragmentTempl` / `data-on:click`) **does not fire on v1.0.2**.

Why: by the time the Datastar engine scans the DOM and attaches a `body.load`
listener (one `setTimeout(0)` tick after module evaluation), the page's `load`
event has already fired — and JS-added `body.addEventListener('load', …)` only
catches `load` events that bubble to body, which the page-load event does not.
There is no error in the console; the listener simply never runs.

**Correct v1.0.2 attribute:** `data-init` (plugin slug: "Runs an expression
when the element is loaded into the DOM"). Replaces `data-on-load` verbatim.

```html
<!-- WRONG — silent no-op on v1.0.2 -->
<body data-on-load="@get('/ds/_smoke/events')">

<!-- RIGHT -->
<body data-init="@get('/ds/_smoke/events')">
```

This is exactly the RC-era-name-rotted bug the PRD §10/§11 warned about.
Adapter B handlers in M2.5b should reach for `data-init` for "open on load"
behaviour; `data-on:{event}` (colon — see the next finding) handles genuine
DOM events (`click`, `submit`, custom events).

## Second headline F16 finding — keyed attributes use a COLON, dash forms silently skipped

Found in M2.5b when the drag→POST chain never fired in a real browser (no
console error, no network request). The v1.0.2 engine parses attribute names
as `data-{plugin}:{key}__{mods}`:

```js
let [t, ...n] = e.split("__"),
    [r, s] = t.split(/:(.+)/),   // plugin : key
```

So `data-on:reorder` and `data-signals:order` are correct, while the dash
forms `data-on-reorder` / `data-signals-order` are looked up as plugin names
(`on-reorder`, `signals-order`), found in no registry, and **silently
skipped** — the same silent-no-op failure class as `data-on-load`. It slipped
through M2.5a because the smoke page only used `data-init` and
`data-json-signals`, which are full plugin names with no key. (The
templ.guide page's `data-on:click` was right after all; its Go-SDK names are
still stale.) `TestColumnUsesVerifiedDatastarKeyedAttributeSyntax` pins the
colon forms and rejects the dash forms.

Two adjacent stream-lifecycle findings from the same debugging session
(verified against bundle source and the headless-browser harness):

- **`retry: 'auto'` (the default) never reconnects after a *clean* stream
  end** — only thrown network errors retry (abrupt kills like an `air`
  rebuild surface as `ERR_INCOMPLETE_CHUNKED_ENCODING` and do retry, with
  backoff up to `retryMaxCount: 10`). A server that ends the response
  normally (handler returns on a render/list error, an intermediary closes
  politely) leaves the tab silently deaf. FE2 now opens the stream with
  `@get('/ds/events', {retry: 'always'})` to match FE1's EventSource
  semantics.
- **GET streams close while the tab is hidden** (`openWhenHidden` defaults to
  false for GET): Datastar aborts the fetch on `visibilitychange` → hidden
  and reopens it on visible. A backgrounded FE2 tab is stale by design and is
  made whole on focus by F13's connect snapshot — and that visibility
  reconnect also resurrects streams that died for any other reason, which is
  what made the original bug look intermittent.

[templguide]: https://templ.guide/server-side-rendering/datastar/

## RC-era rename map (Go SDK side)

The Datastar Go SDK at `v1.2.2` ships the post-rename surface. ctx7 docs and
the [templ.guide][templguide] page disagree about names; this is the verified
truth for `v1.2.2`:

| Verified `v1.2.2` name | Old (RC-era / templ.guide) | Notes |
| --- | --- | --- |
| `sse.PatchElements(html, opts…)` | `MergeFragments` | Emits `event: datastar-patch-elements` |
| `sse.PatchElementTempl(component, opts…)` | `MergeFragmentTempl` | templ-aware wrapper — what `/ds/_smoke/events` uses |
| `sse.PatchElementGostar(el, opts…)` | `MergeFragmentGostar` | GoStar wrapper; we don't use it |
| `sse.PatchElementf(format, args…)` | (new) | `fmt.Sprintf` sugar over `PatchElements` |
| `sse.MarshalAndPatchSignals(v)` | `MarshalAndMergeSignals` | Signals-only update |
| `sse.RemoveElement(selector)` | (similar) | Targeted removal |
| `sse.ExecuteScript(js, opts…)` | (similar) | One-off JS exec |
| `sse.Redirect(path)` | (similar) | Browser nav |

Convenience helpers that return attribute strings (use these instead of
hand-rolling `@get('...')` so a future rename surfaces as a compile error):

```go
datastar.GetSSE("/api/user/%d", id)   // → @get('/api/user/42')
datastar.PostSSE("/api/submit")        // → @post('/api/submit')
datastar.PutSSE / PatchSSE / DeleteSSE  // analogous
```

## Wire format (verified end-to-end by `/ds/_smoke/events`)

`PatchElementTempl(SmokeFragment("patched by datastar"))` emits, on the wire:

```
event: datastar-patch-elements
data: elements <div id="smoke-target">patched by datastar</div>

```

- **`event:`** is `datastar-patch-elements` (the RC-era `datastar-merge-fragments`
  is gone; the v1.0.2 runtime watcher matches this name verbatim, confirmed
  by source inspection of `library/src/plugins/watchers/patchElements.ts`).
- **`data:`** prefix is `elements ` (note the space) — the runtime's onmessage
  parser splits each data line on the first space into `(key, value)` and
  packs into the patch options. Other keys: `selector`, `mode`, `namespace`,
  `useViewTransition`, `viewTransitionSelector`.
- **Default `mode`** is `outer` — the engine finds the patch element by `id`
  and morphs it in place. **No selector header needed** when the fragment has
  a matching `id` already in the DOM. This is the F16 "DOM-ownership
  idempotency" property SortableJS depends on.

## SortableJS ↔ Datastar DOM-ownership (verified by V2)

The PRD §11 bug-list calls out a foreseeable fight: SortableJS mutates the DOM
on drop, Datastar patches the same DOM from SSE, two libraries clobber each
other. M2.5a verifies the safe pattern; M2.5b implements it on the real list.

**Pattern (what M2.5b should use):**

1. **SortableJS owns the drag.** `Sortable.create(list, { animation, onEnd })`
   reorders the `<li>` DOM nodes locally — Datastar does not see this until a
   server round-trip comes back.
2. **On drop, read order via `sortable.toArray()`** — returns the `data-id`s
   of the children in current DOM order. `Sortable.get(el)` retrieves the
   existing instance.
3. **POST that order to the server.** M2.5b will hang this off a Datastar
   action (signal-bound `data-on-end__window` + `@post` is the likely shape;
   confirm wiring when implementing).
4. **Server re-renders the list fragment in the same new order**, ships via
   `PatchElementTempl` with default outer-morph. Because the `id`s on the
   `<li>`s already match the DOM after SortableJS's local reorder, idiomorph
   sees no diff and the patch is a visual no-op. **No fight, no flicker.**

The smoke page demonstrates (1)–(2) only; the round-trip lands in M2.5b
against the real `cards` service.

## What changed in `go.mod` for M2.5a

- `github.com/starfederation/datastar-go v1.2.2` (transitive: `CAFxX/httpcompression`, `klauspost/compress`, `andybalholm/brotli`, `valyala/bytebufferpool`)
- `github.com/a-h/templ v0.3.1020`

## What changed in `package.json` for M2.5a

Nothing. Per the M2.5a deps decision, JS deps are CDN-pinned in `smoke.templ`
(Datastar JS bundle + SortableJS) — no second pnpm toolchain for FE2 yet.
M2.5b revisits this if the round-trip wiring needs anything bundled.

## M2.5b — Adapter B over the shared core (F12–F15)

Adapter B is live at `/ds/` (page, F14), `/ds/events` (patch stream, F13) and
`POST /ds/cards/reorder` (mutation, F12), all over the same `cards.Service`
and pub/sub Broker as Adapter A — one published mutation fans to both
adapters' subscribers (criterion 9's mechanical half, verified by
`TestEventsStreamsPublishedReordersAsPatches` and by curl: a `POST
/api/cards/reorder` lands on `/ds/events` as a templ patch, a `POST
/ds/cards/reorder` lands on `/api/events` as a JSON frame).

### F15 decision — server-driven, no optimistic signal

We stayed with the PRD's default (pure server round-trip) and did **not**
build an optimistic Datastar signal. The reason is a finding, not a shortcut:
**SortableJS already provides the optimistic half for free.** The drop leaves
the DOM in the new order; the server's response patch renders that same
order; idiomorph diffs them as identical and the morph is a visual no-op
(M2.5a's verified idempotency property). So on the success path FE2 *looks*
optimistic — the card never moves back — without any reconciliation machinery.
What's missing vs FE1 is only the failure/latency behavior:

- a **rejected** reorder (F5 analogue) answers `200` + a patch of the
  authoritative column, so the card visibly snaps back — rollback is "the
  server re-asserts truth", zero client code (`TestReorderRejectionPatches…`);
- on a **slow/flaky** network the DOM sits in the un-acked order until the
  response or the next `/ds/events` patch corrects it. There is no held-back
  txid handshake; a foreign reorder arriving mid-flight will clobber the
  in-flight drop. With 3 cards this is a non-issue; on a chatty board it's
  exactly the FE1-reconciliation-rebuilt-server-side cost PRD §9 predicts.

txid is not load-bearing in FE2 (PRD F15): patches are full-state renders, so
ordering is whatever the broker delivers; we keep no flicker-suppression dedup.

### Drop → POST wiring (the F16 confirmation)

`data-on-end__window` turned out unnecessary. SortableJS `onEnd` dispatches a
`CustomEvent('reorder', {detail:{order: toArray()}})` on `#card-list`; the
list's `data-on:reorder="$order = evt.detail.order; @post('/ds/cards/reorder')"`
(colon-keyed — see the second F16 finding above) copies the detail into the
`$order` signal and posts. `@post` ships signals as
the JSON body, so the server reads `{"order":[…]}` via `datastar.ReadSignals`
— the same payload shape as F1. The morph preserves the `<ul>` node itself, so
the Sortable instance and the Datastar listeners survive every patch.

### Component layout (post-M2 refactor)

The UI lives in `ds/components/`, a separate package the `ds` handlers import.
The criterion-9 "63-line templ file" is now four focused files totalling the
same UI; the smoke page stays in `ds` (spike artifact, deliberately bare):

- `layout.templ` — `Layout(title, streamInit)` document shell: pinned CDN
  scripts, inline styles, and the `data-init` stream opener on `<body>`; page
  content fills the `{ children... }` slot.
- `styles.templ` — the inline stylesheet (FE1 palette-swapped to green).
- `column.templ` — `CardColumn` (the #card-list morph anchor), the unexported
  `card` item, and `sortableInit` (the SortableJS → reorder-event bridge).
  Everything coupled to `#card-list` stays in one file so the anchor, the
  data-ids, and the script that reads them evolve together.
- `page.templ` — `CardsPage` composes the above; the only place the
  `/ds/events` URL and `retry: 'always'` option appear.

Exports are exactly what handlers need: `CardsPage` (F14 page render) and
`CardColumn` (F12/F13 patches). One wire-level consequence: `Layout`'s
`streamInit` is a dynamic templ attribute, so it lands in the HTML
entity-escaped (`data-init="@get(&#39;/ds/events&#39;, …)"`) where the old
hardcoded form was raw. Harmless — the parser decodes entities before
`getAttribute`, so Datastar sees the original expression — but in this
codebase's silent-no-op climate it was browser-verified, not assumed: headless
Chromium loaded `/ds/`, a foreign `POST /api/cards/reorder` was issued, and a
CDP `Runtime.evaluate` saw the live DOM converge to the new order (stream
opened + patch applied, end to end).

### Reconnect contract (F13)

`/ds/events` subscribes with no cursor and ships one render of the current
authoritative column on connect — a (re)connecting client is made whole by a
single frame. No ring buffer, no `Last-Event-ID`, no overflow sentinel; FE1's
replay machinery is JSON-delta-shaped and has no FE2 equivalent to need.
Keepalive and buffering hardening are F2's verbatim (25s `:keepalive`,
`no-cache`, `X-Accel-Buffering: no`, no write deadline).

### Criterion 9 comparison (fill measured numbers at M3)

| Dimension | FE1 (React + TanStack DB) | FE2 (Datastar + templ) |
| --- | --- | --- |
| Client code | 365 lines TS/TSX (4 files) + 8 npm deps + Vite build | 63-line templ file (~15 of it client JS) + 2 CDN scripts, no build step beyond `templ generate` |
| DnD integration | dnd-kit (3 packages); optimistic move + txid handshake + rollback in collection adapter | SortableJS + 1 CustomEvent bridge into a Datastar signal; server patch idempotent against the drop |
| Drop → settled | Instant (optimistic; SSE txid match suppresses flicker) | 1 RTT; *visually* instant on success (idempotent morph), snap-back on reject |
| Reconnect | `Last-Event-ID` ring replay, overflow → refetch | one authoritative frame on connect |
| First paint, distant colo | edge-cached hashed assets (measure at M3) | origin-rendered HTML from `sea` (measure at M3) |
| Two-browser sync | ✅ wire-verified (~1s) | ✅ browser-verified: real mouse drag in headless Chromium → second tab converges <1s; survives server kill/restart (`retry: 'always'`) |

## Smoke verification record

- **T1** (unit) — `TestSmokeHandlerRendersTargetAndSSEReference` — page renders with `#smoke-target` + `/ds/_smoke/events` reference. ✅
- **T2** (unit) — `TestSmokeEventsEmitsDatastarPatchElementsFrame` — SSE emits a `datastar-patch-elements` frame whose body morphs `#smoke-target`. ✅
- **V1** (browser) — patch lands: page text flips from "awaiting patch…" to "patched by datastar" within a tick of load. ✅ (after `data-on-load` → `data-init` fix)
- **V2** (browser) — SortableJS co-exists: 3-item list drags freely, local "order:" line updates from `toArray()`. ✅
