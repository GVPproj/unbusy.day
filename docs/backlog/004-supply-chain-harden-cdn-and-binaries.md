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

Motion and its transitive graph were removed by UNB-49. Datastar remains on
every page, and production login loads Cloudflare Turnstile when the presence
gate is configured.

- `internal/frontend/layouts/layout.templ` — Datastar SDK
  `cdn.jsdelivr.net/gh/starfederation/datastar@v1.0.2/bundles/datastar.js`.
- `internal/frontend/smoke.templ` — the same Datastar bundle in the wiring
  canary.
- `internal/frontend/components/login.templ` — Cloudflare Turnstile's runtime
  when a site key is configured.

The Datastar version tag is a version pin, not a content pin. A CDN or
upstream-tag compromise could inject JavaScript into an authenticated page,
read its DOM and keystrokes, and perform same-origin actions as the User. The
session cookie remains unreadable because it is `HttpOnly`.

### Path forward

1. **Vendor Datastar under `internal/frontend/static/`** and serve it locally,
   matching the content-locked CodeMirror precedent. This is preferred over SRI
   because the reviewed bytes become part of the repository and deploy.
2. Alternatively, add `integrity="sha384-…" crossorigin="anonymous"` so swapped
   Datastar bytes fail closed while retaining the CDN.
3. Update `smoke.templ` with the same loading pattern.
4. Treat Turnstile as a deliberate auth-provider dependency. Its hosted client
   is part of the provider integration rather than an app library to vendor;
   keep it isolated to the unauthenticated login surface.

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
