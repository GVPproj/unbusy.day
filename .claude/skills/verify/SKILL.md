---
name: verify
description: Build, run, and visually drive unbusy.day headlessly — mint an OTP session over HTTP and screenshot with Playwright.
---

# Verifying unbusy.day changes at the browser surface

## Build & run

```bash
task build                       # templ + go build → tmp/unbusy
DATABASE_URL="file:$SCRATCH/verify.db?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_txlock=immediate" \
  PORT=8199 ./tmp/unbusy > $SCRATCH/server.log 2>&1 &
curl -s http://localhost:8199/healthz   # 200 = up; DB migrates on boot, starter blocks seed on first login
```

Never run `task templ` while `task dev` is up (check `pgrep -f templ` first).

## Mint a session (auth is OTP; dev LogMailer prints codes to stdout)

```bash
curl -s -X POST :8199/login/code -H 'Content-Type: application/json' -d '{"email":"verify@example.com","code":""}'
grep "login code" $SCRATCH/server.log        # → 6-digit code
curl -si -X POST :8199/login/verify -H 'Content-Type: application/json' -d '{"email":"...","code":"NNNNNN"}' | grep -i set-cookie
```

Endpoints read Datastar signals as a JSON body, not form fields.

## Drive with Playwright

- Browsers are cached at `~/Library/Caches/ms-playwright` but no global package: `npm install playwright@<ver>` in a scratch dir, matching the cached chromium revision (check `node_modules/playwright-core/browsers.json` — e.g. revision 1208 → playwright@1.58).
- Inject the session cookie via `context.addCookies`; set theme/feeling pre-paint via `addInitScript` writing `localStorage` keys `colorscheme` (`solarized-light|solarized-osaka|catppuccin-mocha`) and `feeling` (`cozy|pixel`).
- **`waitUntil: 'networkidle'` never fires** — the `/events` SSE stream stays open. Use `'load'` + a short timeout.
- Mobile = viewport width < 640px (40rem breakpoint). Drawer/hamburger hooks: `.menu-toggle`, `#sidenav`, `.nav-scrim`; `.open` class + `aria-expanded` confirm state.

## Gotchas

- The pixel-feeling nav logo (UBLogo at pixelScale 0.35) renders blank in headless chromium — pre-existing SVG-filter quirk, not a regression (login's 0.6-scale logo renders fine).
- Clearing the day via the Clear modal disables the nav Clear button (`$firstOccupiedSlot >= $lastOccupiedEnd`) — handy for probing disabled state, but it mutates the scratch DB; re-login with a fresh email to reseed starter blocks.
- For a pre-change baseline: `git stash && task build && cp tmp/unbusy <scratch>/baseline && git stash pop && task build` (templ output is generated from markup, so a plain `go build` without `task build` can embed stale renders).
- Pixel parity: run baseline + current on separate ports with FRESH DBs and separate sessions, freeze the client clock in both (`page.clock.install({time})` — otherwise the now-pill/past/active states drift between shots), then diff with `pixelmatch` + `pngjs` (npm-install in scratch; no imagemagick on this box).
- Locator identity is unstable across SSE morphs: `.block-item.first()` can resolve to a *different* block after a move/rename swap. Assert on `data-id`, or read the final screenshot, not a re-queried locator.
- Keyboard block move: focus the block `<li>`, Space → ArrowDown → Space (keys.js). Resize headlessly: mouse-drag the `.grip` (mid-gesture shows `.resizing`, faded label, `.resize-cue`).
