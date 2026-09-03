# 003 — Optional Vim mode for the CodeMirror 6 Jotpad

Status: research (no production code changed)
Date: 2026-08-24

## Conclusion

This is a **small feature with one non-small integration detail**. The UI and
runtime switch are straightforward: add a labelled checkbox beside “Jotpad”,
put `vim()` in a CodeMirror `Compartment`, and reconfigure that compartment in
place. CodeMirror documents compartments specifically for replacing part of an
editor's extension tree at runtime, including toggling an extension on and off
([CodeMirror configuration example](https://codemirror.net/examples/config/#compartments)).
The editor should **not** be recreated.

A basic, locally persisted implementation is roughly **half a day**. A polished
implementation with accessible mode feedback, theme fixes, dependency-graph
validation, browser/mobile checks, tests, and license housekeeping is roughly
**one to two engineering days**.

The main caveat is dependency identity. `@replit/codemirror-vim@6.4.0` declares
five CodeMirror 6 peer dependencies and imports commands, language, search,
state, and view through them
([package source](https://github.com/replit/codemirror-vim/blob/v6.4.0/packages/codemirror-vim/package.json),
[adapter imports](https://github.com/replit/codemirror-vim/blob/v6.4.0/packages/codemirror-vim/src/cm_adapter.ts#L1-L10)).
This repository deliberately keeps one CodeMirror module graph because duplicate
state/view instances break extension identity
([local import rationale](../../internal/frontend/static/js/jot/cm.js#L1-L9)).
The Vim package therefore cannot be treated as an isolated one-line import: its
resolved peers must match the Jotpad's exact pins.

**Recommendation:** proceed, default off, persist the preference in guarded
`localStorage`, use runtime compartment reconfiguration, show an accessible
Normal/Insert/Visual status, and verify the complete esm.sh graph before merge.

---

## Current implementation constraints

The current Jotpad has a useful narrow seam:

- [`initJotpadCM`](../../internal/frontend/static/js/jot/cm.js#L135-L220) creates
  one `EditorView`, with Markdown/GFM, history, drawn selections, the default
  and history keymaps, task-marker pointer handling, a maximum-length
  transaction filter, and one document-change listener.
- The listener calls the independent sync driver only for document changes;
  remote authoritative text is applied as one unfiltered minimal transaction
  ([`cm.js`](../../internal/frontend/static/js/jot/cm.js#L180-L211),
  [`sync.js`](../../internal/frontend/static/js/jot/sync.js#L25-L45)).
- The save driver owns debounce, CAS acknowledgements, SSE convergence, retry,
  offline state, and teardown beacons
  ([`sync.js`](../../internal/frontend/static/js/jot/sync.js#L73-L224)).
- The editor DOM is intentionally never morphed. The page keeps both panels in
  the DOM on mobile so text, caret, and scroll survive panel switching
  ([`blocks.templ`](../../internal/frontend/routes/blocks.templ#L46-L84)).
- The heading and save status are rendered by
  [`components/jotpad.templ`](../../internal/frontend/components/jotpad.templ),
  and their responsive layout is scoped in the single stylesheet
  ([`app.css`](../../internal/frontend/static/css/app.css#L2427-L2495)).
- CDN versions are exact URLs in `cm.js`; `task check:versions` discovers those
  URLs and compares them with npm
  ([`Taskfile.yml`](../../Taskfile.yml#L53-L87)).

These boundaries mean Vim mode belongs in `cm.js` plus a small Jotpad control.
It does not belong in `sync.js`, the Go `jot.Service`, or the stored Jot text.

## Recommended implementation shape

### Markup

Add a native labelled checkbox immediately after the heading, conceptually:

```html
<h2 id="jot-heading">Jotpad</h2>
<label class="jot-vim-toggle">
  <input id="jot-vim" type="checkbox">
  Vim
</label>
<output id="jot-vim-mode" aria-live="polite" hidden>Normal mode</output>
<output id="jot-status">Saved</output>
```

A native checkbox already supplies keyboard operation and checked semantics;
the explicit label supplies its accessible name
([HTML checkbox reference](https://developer.mozilla.org/en-US/docs/Web/HTML/Reference/Elements/input/checkbox)).
There is no need to recreate switch semantics with `role="switch"`. Keep the
whole labelled target at least 24 by 24 CSS pixels to meet WCAG 2.2's minimum
target criterion
([WCAG 2.2, Target Size (Minimum)](https://www.w3.org/WAI/WCAG22/Understanding/target-size-minimum.html)).

The current header gives the save status `margin-left: auto`; the new compact
control can sit between the heading and that status. Narrow mobile widths need
a real check because the header also reserves space for the fixed hamburger
([current mobile CSS](../../internal/frontend/static/css/app.css#L2435-L2479)).
Do not automatically focus the editor after the user toggles the checkbox—the
control should retain focus and normal Tab order.

### Runtime configuration

Import `Compartment` from the already-pinned `@codemirror/state` module and
`vim` (plus `getCM` if exposing mode feedback) from the pinned Replit package.
Create one compartment per editor:

```js
const vimMode = new Compartment();
const enabled = readVimPreference();

EditorState.create({
  extensions: [
    vimMode.of(enabled ? vim() : []), // before all other keymaps
    // existing extensions unchanged
  ],
});

function setVim(enabled) {
  view.dispatch({
    effects: vimMode.reconfigure(enabled ? vim() : []),
  });
}
```

CodeMirror's official example uses this exact `Compartment.of(...)` plus
`compartment.reconfigure(...)` pattern to enable and disable an extension
without replacing the editor
([dynamic configuration](https://codemirror.net/examples/config/#compartments)).
The Vim package explicitly says `vim()` must appear before other keymaps, and
that subsequent default keymaps remain available in Insert mode
([official README](https://github.com/replit/codemirror-vim/blob/v6.4.0/packages/codemirror-vim/README.md#L12-L30)).
Thus the compartment should precede `markdown(...)` and the current
`keymap.of([...defaultKeymap, ...historyKeymap])`.

Reconfiguration removes and constructs only the Vim extension. Upstream's Vim
plugin enters Vim mode in its constructor and, on destruction, leaves Vim mode,
destroys its block cursor, removes its class, and deletes the adapter from the
view
([plugin lifecycle](https://github.com/replit/codemirror-vim/blob/v6.4.0/packages/codemirror-vim/src/index.ts#L50-L174)).
Turning Vim back on should intentionally restart in Normal mode rather than
trying to persist an ephemeral submode or pending command.

### Why not recreate `EditorView`?

Recreation would force this application to migrate document, selection,
history, scroll, focus, sync closures, `window.__jotRemote`, and teardown
listeners. Those are all tied to the current view or sync instance
([view/sync construction](../../internal/frontend/static/js/jot/cm.js#L140-L220),
[teardown listeners](../../internal/frontend/static/js/jot/sync.js#L226-L242)).
It would also risk duplicate beacons and event listeners because the teardown
wiring exposes no unsubscribe operation. A compartment changes configuration
through an ordinary editor transaction and leaves the existing editor/sync
objects in place
([CodeMirror compartments](https://codemirror.net/examples/config/#compartments)).

## Status display and Vim command panel

`vim({status: true})` adds a permanent bottom panel; `vim()` without that option
adds a panel only while a Vim dialog such as `/` or `:` is active
([extension implementation](https://github.com/replit/codemirror-vim/blob/v6.4.0/packages/codemirror-vim/src/index.ts#L405-L443)).
The permanent upstream status panel shows modes and pending keys, but its mode
control is a clickable `<span>` with no native button semantics or keyboard
handling
([status construction](https://github.com/replit/codemirror-vim/blob/v6.4.0/packages/codemirror-vim/src/index.ts#L88-L166)).
It also consumes editor height.

For this app, prefer `vim()` plus a small app-owned `<output>` for the current
mode. The package exposes its CM5-compatible adapter through `getCM(view)`
([official API instructions](https://github.com/replit/codemirror-vim/blob/v6.4.0/packages/codemirror-vim/README.md#L32-L43)),
and the adapter emits `vim-mode-change`, which upstream itself uses to update
status
([source](https://github.com/replit/codemirror-vim/blob/v6.4.0/packages/codemirror-vim/src/index.ts#L72-L86)).
An app-owned output can be properly labelled/live, hidden when Vim is off, and
styled with existing tokens. Treat that event name as a modest coupling to
upstream source rather than a strongly documented CM6 API, and pin it with a
browser test.

Also add an accessible name to the editable surface while touching the setup,
for example `EditorView.contentAttributes.of({"aria-label": "Jotpad Markdown editor"})`.
CodeMirror documents `contentAttributes` as the facet for attributes on its
editable DOM element
([CodeMirror reference](https://codemirror.net/docs/ref/#view.EditorView^contentAttributes)).

## Persistence options

### Recommended: guarded `localStorage`

Use a namespaced boolean such as `jot-vim` and default to false. `localStorage`
is origin-scoped and survives browser sessions, whereas `sessionStorage` ends
with the page session
([MDN platform documentation](https://developer.mozilla.org/en-US/docs/Web/API/Window/localStorage)).
This also matches the repository's existing device-local persistence for
colorscheme, color mode, and feeling
([`layout.templ`](../../internal/frontend/layouts/layout.templ#L8-L22)).

Wrap reads and writes in `try/catch`. Access can raise `SecurityError` when
browser policy prevents persistence, and private browsing clears local data
when its last private tab closes
([MDN exceptions and behavior](https://developer.mozilla.org/en-US/docs/Web/API/Window/localStorage#exceptions)).
Failure should simply mean “off this load” or “works but is not remembered,” not
prevent the editor from mounting.

This preference does not need Datastar: the server does not care, and JavaScript
must reconfigure CodeMirror anyway. It also does not need the layout's pre-paint
script because neither the editor nor its mode exists before the module mounts.

### Alternatives

- **No persistence:** smallest and safest, but users must re-enable Vim on every
  reload. Reasonable for an experiment, weak as a finished preference.
- **`sessionStorage`:** remembers within a tab session only, per the platform
  behavior above; there is no evident product benefit over either no persistence
  or durable local persistence.
- **Server-side user preference:** gives cross-device consistency, but requires a
  migration, explicit columns/queries, service method, endpoint, and page data.
  That is disproportionate to an editor-only preference and would turn a small
  frontend feature into broader application work.
- **Datastar signal plus `localStorage`:** possible and consistent with theme
  controls, but adds signal plumbing for state the server never consumes. A
  native control wired directly to the editor is the shallower path.

## Interaction with existing behavior

### Saving and live sync

No `sync.js` change should be necessary. Vim edits are CodeMirror document
transactions, and the current update listener reacts to every `docChanged`
transaction regardless of input source
([current listener](../../internal/frontend/static/js/jot/cm.js#L187-L194)).
The existing maximum-length filter, debounce, CAS merge, retry, offline status,
SSE remote application, and teardown beacon therefore remain in force
([current filter and listener](../../internal/frontend/static/js/jot/cm.js#L180-L194),
[`sync.js`](../../internal/frontend/static/js/jot/sync.js)). A compartment-only
transaction does not change the document, so toggling mode does not trigger a
save.

Remote text still maps through one minimal CodeMirror change, preserving the
selection when possible. If remote text lands while Vim is in Visual mode or a
multi-key command is pending, cursor/selection mapping should work at the CM
level, but the pending Vim operator state is package-owned; concurrent-edit
manual tests must cover this edge.

Vim's `:w` command is a no-op with the stock CM6 adapter: Vim core calls
`CM.commands.save` or `cm.save`
([Vim core source](https://github.com/replit/codemirror-vim/blob/v6.4.0/packages/codemirror-vim-core/vim.js#L6394-L6406)),
while the adapter defines undo/redo and editing commands but no save command
([adapter commands](https://github.com/replit/codemirror-vim/blob/v6.4.0/packages/codemirror-vim/src/cm_adapter.ts#L133-L157)).
That is functionally safe because this Jotpad auto-saves, but it may surprise Vim
users. A later polish can define `:w` to call `sync.flush()` using the package's
documented `Vim.defineEx` API
([official example](https://github.com/replit/codemirror-vim/blob/v6.4.0/packages/codemirror-vim/README.md#L45-L49)); it is not required for the first version.

### Key handling

- In Normal/Visual mode Vim receives keys before the Markdown/default keymaps;
  in Insert mode the later keymaps continue to handle normal editing, as the
  package's setup contract specifies
  ([README](https://github.com/replit/codemirror-vim/blob/v6.4.0/packages/codemirror-vim/README.md#L18-L25)).
  Markdown Enter/Backspace continuation should therefore remain available in
  Insert mode.
- Vim undo/redo delegates to `@codemirror/commands`
  ([adapter source](https://github.com/replit/codemirror-vim/blob/v6.4.0/packages/codemirror-vim/src/cm_adapter.ts#L133-L147)),
  so the existing `history()` extension remains the source of history.
- The task-list pointer toggle dispatches an ordinary document change and does
  not depend on keyboard mode
  ([`cm.js`](../../internal/frontend/static/js/jot/cm.js#L115-L133)).
- The page-global `?` shortcut already ignores an active contenteditable
  ([`shortcuts.js`](../../internal/frontend/static/js/shortcuts.js)); CodeMirror's
  content DOM is the active contenteditable, so Vim's `?` search should not open
  the app shortcut dialog.
- The plugin contains special clipboard and composition handling
  ([event handlers](https://github.com/replit/codemirror-vim/blob/v6.4.0/packages/codemirror-vim/src/index.ts#L196-L337)).
  Those paths increase the browser-test surface even though no sync changes are
  needed.

### Keyboard accessibility

Vim mode necessarily captures many printable keys, but it remains opt-in and
has a visible off switch outside the editor. The package handles Escape as part
of Vim operation. Separately, CodeMirror's documented keyboard convention is
that Escape followed by Tab lets focus leave an editor even when Tab is bound
([official Tab-handling guidance](https://codemirror.net/examples/tab/)). Test
that escape hatch with Vim enabled, because keyboard users must not be trapped.
Do not place the toggle inside CodeMirror's key-handling DOM.

## Dependency and import implications

The direct import would be:

```js
import { vim, getCM } from "https://esm.sh/@replit/codemirror-vim@6.4.0";
```

Version 6.4.0 is MIT-licensed and publishes ESM as `dist/index.js`; it has one
runtime dependency, `@replit/codemirror-vim-core`, and peer dependencies on
`@codemirror/commands`, `language`, `search`, `state`, and `view`
([package manifest](https://github.com/replit/codemirror-vim/blob/v6.4.0/packages/codemirror-vim/package.json)).
Search and the Vim core are new to the current Jotpad graph. The Vim package's
own usage example requires `drawSelection`, which the Jotpad already has
([README note](https://github.com/replit/codemirror-vim/blob/v6.4.0/packages/codemirror-vim/README.md#L29-L30)).

At research time, the plain esm.sh wrapper resolved the peers to commands
6.11.0, language 6.12.4, search 6.7.1, state 6.7.1, and view 6.43.9
([live wrapper](https://esm.sh/@replit/codemirror-vim@6.4.0)). The repository
currently pins commands 6.10.4 while the other shared packages match those
resolved versions
([current imports](../../internal/frontend/static/js/jot/cm.js#L11-L38)).
That commands mismatch is material: Vim undo/redo imports commands through its
peer, while `history()` is installed from the app's direct commands module.
Before implementation, align the direct commands pin with the wrapper and
confirm that state/view/language are single instances in the browser module
graph.

There are three viable dependency strategies:

1. **Stay with the repository's plain esm.sh convention (recommended for this
   small feature):** add the exact Vim package URL, align all direct CodeMirror
   pins to its resolved peers, inspect the generated wrapper, and add a graph
   check so a future range resolution cannot silently split identity-sensitive
   modules. This follows the existing rationale that `?deps` combinations are
   operationally unreliable
   ([local comment](../../internal/frontend/static/js/jot/cm.js#L3-L9)).
2. **Use an esm.sh `?deps=` URL:** this can force all peer versions, but directly
   contradicts the repository's recorded experience of on-demand build 408s
   ([same local rationale](../../internal/frontend/static/js/jot/cm.js#L3-L9)).
3. **Vendor the built modules:** strongest reproducibility and fewer CDN graph
   surprises, but much larger scope. The repository already records vendoring
   as its preferred eventual CDN hardening direction
   ([supply-chain backlog](../backlog/004-supply-chain-harden-cdn-and-binaries.md)).

A one-point measurement on 2026-08-24 found approximately 57 KB Brotli transfer
for the Vim adapter, Vim core, and CodeMirror search implementation together,
excluding tiny wrappers and already-shared peers
([adapter artifact](https://esm.sh/@replit/codemirror-vim@6.4.0/es2022/codemirror-vim.mjs),
[core artifact](https://esm.sh/@replit/codemirror-vim-core@0.1.0/es2022/codemirror-vim-core.mjs),
[search artifact](https://esm.sh/@codemirror/search@6.7.1/es2022/search.mjs)).
Treat that as a measurement, not an API guarantee. Because `cm.js` imports Vim
statically, all Jotpad users pay this cost even when the toggle is off. A dynamic
import on first enable avoids that cost but complicates initial persisted-on
mounting and asynchronous error handling; for this app, static import is the
simpler first implementation unless performance measurement says otherwise.

`task check:versions` will discover the exact top-level Vim URL through its
existing regex, but it will not see peer ranges hidden in the generated wrapper
([task source](../../Taskfile.yml#L78-L83)). Extend the check or add a focused
script/assertion for resolved CodeMirror graph agreement. Also add the Vim
package/core MIT notice to [`THIRD_PARTY_LICENSES.md`](../../THIRD_PARTY_LICENSES.md):
the upstream license requires preservation of its copyright and permission
notice in copies or substantial portions
([upstream license](https://github.com/replit/codemirror-vim/blob/v6.4.0/packages/codemirror-vim/LICENSE)).

## Theming concerns

The plugin injects its own base/theme styles. Its command panel uses monospace
and removes input borders/outlines; its search matches use hard-coded light/dark
colors
([Vim style source](https://github.com/replit/codemirror-vim/blob/v6.4.0/packages/codemirror-vim/src/index.ts#L25-L45)).
Its block cursor uses hard-coded `#ff9696` for the focused background and
unfocused outline
([block cursor source](https://github.com/replit/codemirror-vim/blob/v6.4.0/packages/codemirror-vim/src/block-cursor.ts#L125-L145)).
Those conflict with the app's token-only color convention and may have poor
contrast in one of the six theme variants.

Do not rely only on layered `app.css` overrides: this repository already notes
that CodeMirror's injected unlayered styles outrank its CSS layer
([`cm.js`](../../internal/frontend/static/js/jot/cm.js#L149-L162)). Add
Vim-specific overrides through `EditorView.theme` using `--ink`, `--bg`,
`--accent`, and `--rail-border`, including a visible focus style for the `/` and
`:` input. Verify precedence against the plugin's `Prec.highest` cursor theme
([upstream cursor export](https://github.com/replit/codemirror-vim/blob/v6.4.0/packages/codemirror-vim/src/block-cursor.ts#L125-L145)); an editor-theme
rule with deliberate precedence or narrowly scoped `!important` may be needed.

## Mobile UX

Default-off is important on touch devices. Normal-mode commands, Escape,
Control combinations, and mode awareness are designed around a physical
keyboard. The package source does include composition-event handling and an
IME workaround
([input/composition source](https://github.com/replit/codemirror-vim/blob/v6.4.0/packages/codemirror-vim/src/index.ts#L246-L403)),
but its official README makes no mobile-support claim
([README](https://github.com/replit/codemirror-vim/blob/v6.4.0/packages/codemirror-vim/README.md)).
Thus mobile quality is an open test question, not something the dependency
contract guarantees.

Do not hide the option solely on `(pointer: coarse)`: tablets and phones can
have physical keyboards, and pointer capability does not describe keyboard
availability. Keep it optional, compact, and default-off. Test at least iOS
Safari/PWA and Android Chrome for:

- enabling/disabling with the software keyboard open;
- entering Insert mode and returning to Normal mode when no physical Escape key
  exists;
- autocorrect, composition/IME, paste, and selection handles;
- the Vim `/` and `:` panel above the virtual keyboard;
- header wrapping and the mobile panel switch retaining mode and caret.

If those checks expose poor software-keyboard behavior, retain the toggle but
add concise “physical keyboard” helper text, or hide it only after a product
decision—not on an unsupported keyboard-detection heuristic.

## Likely files and tests

### Production files for the eventual implementation

- `internal/frontend/static/js/jot/cm.js` — imports, compartment, persistence,
  toggle wiring, optional mode output, Vim theme overrides.
- `internal/frontend/components/jotpad.templ` — labelled checkbox and mode
  output beside the heading.
- `internal/frontend/static/css/app.css` — compact responsive toggle/output
  styling; no hard-coded colors.
- `internal/frontend/routes/blocks.templ` — only if the existing init call
  explicitly passes the toggle/output elements in `opts` (preferable to hidden
  global lookups).
- `Taskfile.yml` or a focused check script — assert the resolved CodeMirror
  graph stays aligned.
- `THIRD_PARTY_LICENSES.md` — Vim package/core MIT notice.
- Generated `*_templ.go` files locally after `templ generate`; they remain
  git-ignored by repository convention.

No changes are expected in `sync.js`, Go domain/storage code, migrations, SSE,
or the `/jot` handler.

### Automated tests

- Extend [`internal/frontend/jot_test.go`](../../internal/frontend/jot_test.go)
  to assert the checkbox has a stable id, native type, explicit label, default
  off state, and mode-output semantics; keep existing write/remote wiring
  assertions.
- Extract tiny storage parsing/writing helpers into a DOM-free local module if
  useful, and cover default-off, stored-on/off, malformed values, and storage
  exceptions with `node:test`, following
  [`sync.test.js`](../../internal/frontend/static/js/jot/sync.test.js).
- Add a browser-level regression if the repository adopts a browser harness:
  enable Vim, perform `i`, type, Escape, `u`, `Ctrl-r`, `/`, `:`, visual
  selection, toggle off/on, and assert document/history/selection behavior.
  Upstream has extensive Vim browser tests, but this repository still needs one
  integration test for its extension ordering and CDN graph
  ([upstream test suite](https://github.com/replit/codemirror-vim/tree/v6.4.0/packages/codemirror-vim/test)).

### Manual verification

1. Confirm default Markdown mode is unchanged when Vim is off.
2. Turn Vim on without losing text, selection, history, scroll, or focus order.
3. Confirm Insert-mode Markdown list continuation and Backspace cleanup.
4. Confirm `u`/`Ctrl-r`, visual mode, search, command panel, clipboard, task
   marker click, and maximum-length rejection.
5. Type while another session sends an SSE update; test clean apply, dirty
   buffer/merge, and a pending Vim operator.
6. Toggle off from Normal, Insert, Visual, search, and command modes.
7. Reload, private-storage/error fallback, mobile viewport, dark/light schemes,
   every feeling font, keyboard-only navigation, and a screen reader pass.

## Risk summary

| Risk | Level | Mitigation |
| --- | --- | --- |
| Split CodeMirror peer/module identities through esm.sh ranges | **Medium–high** | Align pins, inspect wrapper, automate graph agreement; vendor later if needed. |
| Vim cursor/search/panel hard-coded styles conflict with themes | **Medium** | Override through CodeMirror's theme seam and test every palette/mode. |
| Software-keyboard/mobile behavior | **Medium** | Default off; test iOS/Android composition, Escape, panels, and layout. |
| Inaccessible or unclear current Vim mode | **Medium** | App-owned labelled/live mode output; do not rely on upstream clickable span. |
| Save/sync regression | **Low** | Keep one view and existing listener; test Vim transactions plus SSE merge. |
| Keymap ordering/history mismatch | **Medium** | Compartment first; one commands module version; integration-test undo/redo. |
| Preference unavailable in restricted storage | **Low** | Guard storage and degrade to off/non-persistent. |
| Extra initial download for users who leave Vim off | **Low–medium** | Accept static import initially; consider lazy import only after measurement. |

## Effort estimate

- **Basic (about 0.5 day):** checkbox, exact import, compartment, guarded
  `localStorage`, minimal CSS, templ markup test, manual desktop smoke test.
- **Recommended production quality (about 1–2 days):** basic work plus peer-graph
  guard, mode output, theme overrides, license entry, focused JS/Go tests,
  desktop browser integration, concurrent-sync checks, and iOS/Android UX pass.
- **If server-side preference or vendoring is required (add 1–2 days):** schema
  and service/API work for the former; artifact/update/checksum workflow for the
  latter.

The feature itself is simple. Most of the estimate is confidence work around a
large keyboard plugin, exact CDN module identity, six visual themes, concurrent
editor synchronization, and mobile composition—not business-logic changes.
