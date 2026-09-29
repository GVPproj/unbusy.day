# 004 — Harden the supply-chain surface

Status: backlog (runtime scripts and build/CI provenance)
Date: 2026-06-16

## Why this is worth doing

The application is already well-positioned against the npm install-script class
of attack: production code is built with Go, direct dependencies are few, and Go
modules are hash-pinned in `go.sum` against the checksum database. There is no
Node or CSS production build step; Node is used for JavaScript and browser
tests.

The remaining inputs outside `go.sum` are a short, reviewable list: runtime
third-party scripts, the Docker base/tool packages, and CI actions. The old
Tailwind-binary and Motion exposures were resolved by removing them.

## Exposure 1 — runtime third-party scripts

Motion and its transitive graph were removed by UNB-49. Datastar is now
self-hosted by UNB-74: the app and wiring canary share one local script component,
with reviewed bundle bytes, source map, license, and SHA-256 checksums committed
under `internal/frontend/static/vendor/datastar/`.

Production login still loads Cloudflare Turnstile from
`internal/frontend/components/login.templ` when a site key is configured.
Treat it as a deliberate auth-provider dependency rather than an app library
to vendor; keep it isolated to the unauthenticated login surface.

## Exposure 2 — mutable build and CI inputs

The application binary is content-pinned at the module layer, but the environment
that builds and deploys it is not fully immutable:

- `Dockerfile` uses the mutable `golang:1.26-alpine` tag and installs unversioned
  `git` from the current Alpine repository. A compromised or changed build image
  can alter the resulting binary even though the final runtime image is
  `scratch`.
- GitHub Actions uses moving major tags such as `actions/checkout@v4` and
  `actions/setup-go@v5`; Fly setup is broader still at
  `superfly/flyctl-actions/setup-flyctl@master`.
- `# syntax=docker/dockerfile:1.7` selects a version tag rather than an immutable
  frontend digest.

### Path forward

1. Pin the Docker build image and Dockerfile frontend by digest, with a documented
   update command so security patches remain deliberate rather than forgotten.
2. Pin third-party GitHub Actions to reviewed commit SHAs, especially the Fly
   action currently tracking `master`; keep the human-readable release tag in a
   comment or dependency-update configuration.
3. Decide whether to pin Alpine package repository/snapshot inputs or replace
   the build-time `git` requirement. Record any intentionally mutable package
   channel as an accepted patch-ingestion trade-off.
4. Keep Playwright/npm integrity under the committed lockfile and continue using
   `npm ci --ignore-scripts`.

## Resolved exposures

- **Tailwind standalone binary:** removed by ADR 0011 along with all download and
  CSS-build wiring. ADR 0008 preserves the historical decision.
- **Motion runtime CDN graph:** removed by UNB-49; see research 004.
- **Datastar CDN bundle:** vendored by UNB-74; upgrade instructions and integrity
  records live in `internal/frontend/static/vendor/datastar/`.
- **CodeMirror CDN graph:** replaced by the local content-locked vendor manifest
  and reproducible `vendorcodemirror` workflow.
- **templ CLI:** installed in Docker/CI at the version selected from `go.mod` via
  `go install pkg@version`, covered by the Go checksum database.

## Related

- ADR 0011 — removal of the Tailwind toolchain.
- `docs/research/004-replacing-motion-with-browser-animation.md` — Motion
  removal outcome.
- `docs/agents/codemirror.md` — current browser-module vendoring workflow.
- `AGENTS.md` "Conventions & deploy" — version-source conventions.
