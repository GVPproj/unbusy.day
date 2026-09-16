# 003 — Optional Vim mode for the CodeMirror 6 Jotpad

Status: research (no production code changed)
Date: 2026-08-24
Updated for the vendored CodeMirror graph: 2026-09-16

## Conclusion

Vim mode remains a small, viable editor feature with one important integration
rule: it must join the repository's single, locally vendored CodeMirror graph.
Do not import `@replit/codemirror-vim` from a CDN in browser code.

Recommended shape:

- a native, labelled checkbox in the Companion Panel header;
- default off, persisted in guarded `localStorage`;
- one CodeMirror `Compartment`, reconfigured without recreating the editor;
- `vim()` before Markdown and other keymaps;
- an app-owned, accessible Normal/Insert/Visual status;
- Vim and its transitive graph added through `vendorcodemirror` and imported
  only from `/static/vendor/codemirror/`.

A basic implementation is about half a day. Production-quality dependency
review, theming, accessibility, concurrent-sync checks, and browser/mobile tests
make it roughly one to two engineering days.

## Current boundaries

- `internal/frontend/static/js/jot/cm.js` creates the one persistent
  `EditorView`. It owns editor extensions, keymaps, document-change forwarding,
  remote text application, and responsive panel-return behavior.
- `internal/frontend/static/js/jot/sync.js` independently owns debounce, CAS
  acknowledgements, SSE convergence, retries, save state, and teardown beacons.
  Vim edits are ordinary CodeMirror transactions, so this module should not
  change.
- `internal/frontend/components/companion.templ` owns the Jotpad tab and shared
  save status. The Vim control and mode output belong there;
  `components/jotpad.templ` should remain only the persistent editor section.
- The editor DOM is never morphed or recreated. Preserving one view keeps text,
  history, selection, scroll, focus, sync closures, and teardown wiring intact.
- CodeMirror is served entirely from
  `internal/frontend/static/vendor/codemirror/`. Direct roots live in
  `internal/frontend/vendorcodemirror/main.go`; the generated manifest is the
  complete deployed lock state. See `docs/agents/codemirror.md`.

## Dependency workflow

Choose and inspect an exact `@replit/codemirror-vim` release at implementation
time. The 2026-08-24 audit used 6.4.0; treat its API and dependency details as a
historical baseline, not the required version.

1. Add the exact browser entry module to `roots` in
   `internal/frontend/vendorcodemirror/main.go`.
2. Run the intentional refresh path:

   ```sh
   go run ./internal/frontend/vendorcodemirror -refresh
   ```

   The generator resolves Vim core, CodeMirror search, and all peer imports into
   the one local graph; rewrites imports; fetches license text; and records
   source and served-file hashes.
3. Review every root, version, module, license, and hash change. Confirm each
   shared `@codemirror/*` package appears at one version.
4. Import `vim`, `getCM`, and any other selected API from the generated local
   module path. Add `Compartment` to the existing local state-module import.
5. Update `docs/agents/codemirror.md` because the direct-root count changes,
   and add Vim/Vim-core packages to the CodeMirror inventory in
   `THIRD_PARTY_LICENSES.md`. Per-module notices remain generated and hash-locked.
6. Verify the lock and application:

   ```sh
   task vendor:codemirror
   go test ./internal/frontend/vendorcodemirror ./internal/frontend
   task test:browser
   task check:versions
   ```

The default vendor task must produce no diff. Generated served modules already
carry the upstream notices locked by their final hashes; do not add a separate
manual license copy unless repository policy changes.

A static local import is the simplest first implementation, though every Jotpad
user then downloads Vim even while it is off. Consider lazy loading only after
measurement: it complicates persisted-on startup, error handling, and the
vendor graph's prohibition on unverified dynamic imports.

## Runtime configuration

Create one compartment per editor and place it before every other keymap:

```js
const vimMode = new Compartment();
const enabled = readVimPreference();

EditorState.create({
  extensions: [
    vimMode.of(enabled ? vim() : []),
    // Existing extensions; Vim must precede Markdown/default keymaps.
  ],
});

function setVim(enabled) {
  view.dispatch({
    effects: vimMode.reconfigure(enabled ? vim() : []),
  });
}
```

CodeMirror documents compartments specifically for runtime extension changes
([configuration example](https://codemirror.net/examples/config/#compartments)).
The Vim package requires its extension before other keymaps so Insert mode can
fall through to normal editing behavior
([upstream README](https://github.com/replit/codemirror-vim/tree/v6.4.0/packages/codemirror-vim)).
Turning Vim back on should restart in Normal mode rather than persist an
unfinished operator or submode.

Do not recreate `EditorView`: doing so would require transferring document,
history, selection, scroll, focus, responsive-panel state, remote callbacks, and
sync teardown ownership for no benefit.

## Control and status UX

Use a native checkbox rather than recreating switch semantics. Keep its labelled
target at least 24×24 CSS pixels, leave focus on the checkbox after toggling,
and pass the control/output elements through `initJotpadCM`'s existing options
seam instead of hidden global lookups.

The Companion header is shared with Habits. Decide explicitly whether the Vim
preference remains visible while Habits is selected or hides with other
Jot-specific state, and test narrow mobile widths.

Prefer `vim()` plus an app-owned `<output aria-live="polite">` over the
package's permanent status panel. In the audited release, `getCM(view)` exposed
a CM5-compatible adapter that emitted `vim-mode-change`; revalidate that seam in
the selected version and pin it with a browser test. Also add an accessible name
to the editable surface via `EditorView.contentAttributes`.

Persist a namespaced boolean such as `jot-vim` in `localStorage`, wrapping reads
and writes in `try/catch`. Restricted storage should degrade to off or
non-persistent behavior, never prevent editor mounting. The server does not care
about this device-local preference, so it needs no Datastar signal or database
column.

## Existing behavior and edge cases

Vim document transactions naturally pass through the current maximum-length
filter and document-change listener. Compartment-only transactions do not save.
No changes are expected in `sync.js`, the Go Jotpad service, migrations, SSE, or
`POST /jot`.

Test remote authoritative text while Vim is in Visual mode and while an operator
is pending. CodeMirror should map selection through the edit, but pending
operator state belongs to the plugin and needs integration coverage.

In the audited release, `:w` had no adapter save command. That is safe because
the Jotpad auto-saves, though it may surprise users. A later polish can map `:w`
to the sync driver's flush operation through the package's `Vim.defineEx` API.

Vim must not trap keyboard focus. Verify CodeMirror's Escape-then-Tab escape
hatch with Vim enabled. The page-global `?` shortcut already ignores an active
contenteditable, so Vim search should remain inside the editor.

## Theming and mobile

Re-audit the selected release's injected styles. The audited version used
hard-coded cursor and search colors that conflict with the app's token-only
convention. Override editor-internal Vim styles through `EditorView.theme`, not
only layered `app.css`, because CodeMirror's unlayered runtime styles outrank the
component layer. Style the Companion checkbox/output in `@scope (.companion)`.

Keep Vim default-off on touch devices, but do not hide it based only on
`pointer: coarse`; phones and tablets can have physical keyboards. Check iOS
Safari/PWA and Android Chrome for software-keyboard toggling, composition/IME,
paste, selection handles, `/` and `:` panels, Escape alternatives, header
wrapping, and panel switches preserving mode, caret, and scroll.

## Expected files and tests

Production changes:

- `internal/frontend/vendorcodemirror/main.go`
- generated `static/vendor/codemirror/manifest.json` and module files
- `docs/agents/codemirror.md`
- `THIRD_PARTY_LICENSES.md` package inventory
- `internal/frontend/static/js/jot/cm.js`
- `internal/frontend/components/companion.templ`
- `internal/frontend/static/css/app.css`
- `internal/frontend/routes/blocks.templ` if it passes control elements in opts

Automated coverage:

- Go markup tests for native labelling, default-off state, and mode-output
  semantics without reintroducing a Jotpad heading;
- small `node:test` coverage for guarded preference parsing if helpers are
  extracted;
- Playwright coverage for enable/disable, Insert/Normal/Visual modes, undo/redo,
  search/command panels, save/reload, panel switching, remote text, focus escape,
  and the existing no-external-CodeMirror-request assertion;
- vendor graph tests for root agreement, hashes, license-bearing served bytes,
  local links, one version per package, and no unlisted files.

## Risk summary

| Risk | Mitigation |
| --- | --- |
| Split CodeMirror identities | Add Vim as a vendor root and review one resolved manifest. |
| Keymap/history mismatch | Put the compartment first and integration-test undo/redo. |
| Theme contrast regressions | Re-audit plugin CSS and override through `EditorView.theme`. |
| Mobile/software-keyboard behavior | Default off and test composition, Escape, panels, and layout. |
| Save, remote-text, or panel-return regression | Keep one view and run existing plus Vim-specific Playwright coverage. |
| Restricted storage | Guard access and degrade cleanly. |
| Extra initial transfer | Measure the refreshed graph before considering lazy loading. |
