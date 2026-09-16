# 006 — Live-reload the browser on `app.css` edits in dev

Status: backlog
Date: 2026-07-11

## Problem

Under `task dev`, `app.css` edits are already served fresh — `TEMPL_DEV_MODE`
serves `static/` from disk, so the new CSS is live on the *next* page load with
no rebuild. What's missing is the reload itself: templ's `--watch` only watches
`.templ`/`.go` (`defaultWatchPattern = (.+\.go$)|(.+\.templ$)`), so a `.css`
save triggers nothing. You have to alt-tab and hit refresh. Astro/Vite gets the
nicer feel by watching CSS and pushing an HMR update over a WebSocket; we want
the cheap 80% of that — auto-reload on save — without adding Node or a build
step (ADR 0011).

## Key findings (the mechanism is already there)

- templ's dev proxy exposes a reload trigger: **`POST
  http://127.0.0.1:7331/_templ/reload/events`**. `NotifyProxy`
  (`proxy.go:377`) is literally that one POST, and the browser page the proxy
  serves is already listening on that SSE stream.
- `templ generate --notify-proxy` **returns before any codegen** — it's the
  first branch of `Generate.Run` (`cmd.go:57`), ahead of the generator. So
  firing it does **not** delete the watch session's literal cache and does
  **not** 500 the running dev server (the usual "never run `templ generate`
  while `task dev` is up" gotcha does not apply to this flag).
- Net: the reload trigger is a stdlib one-liner (`http.Post` to the proxy, or
  shell out to `templ generate --notify-proxy`). All we need to add is the
  "a `.css` file changed" signal.

## Path forward when resumed

1. Add a dev-only watcher `cmd/cssreload/main.go` that watches
   `internal/frontend/static/css` for `*.css` changes (debounce ~80ms — one save
   fans out into several fs events) and POSTs the proxy reload endpoint on
   port 7331. If the proxy port later becomes configurable, pass one shared
   value to both templ's `--proxyport` flag and the watcher.
2. Wire it into the `dev` task: `go run ./cmd/cssreload &` alongside the templ
   watch, with `trap 'kill $! 2>/dev/null' EXIT` so it dies on Ctrl-C.
3. **Dependency decision (the one real call):** fsnotify (`github.com/fsnotify/
   fsnotify`) is already transitive through templ, but importing it would make it
   an explicit dependency of this module for a dev-only command. That still cuts
   against "prefer the stdlib; don't add a dependency until demonstrably
   necessary."
   - **Recommended:** a zero-dep stdlib poll loop over `static/css` — stat CSS
     mtimes every ~300ms and POST on change. Same UX, no `go.mod`
     churn, no ADR-0011 tension. Skip the initial cold-start scan so boot
     doesn't fire a reload.
   - **Alternative:** fsnotify — crisper, event-driven, no 300ms latency, but
     promotes a transitive module to an explicit dependency. Watch the
     *directory*, not each file: editors save
     atomically (rename), which drops a per-file watch.

## Related

- ADR 0011 (plain CSS in one hand-authored stylesheet) — the "no Node / no build
  step" constraint this must respect.
- `Taskfile.yml` `dev` task — where the watcher would be wired in; its comment
  already notes `TEMPL_DEV_MODE` serves `static/` from disk so CSS edits land
  live without a rebuild.
