# 004 — Harden the supply-chain surface (CDN scripts + downloaded binaries)

Status: backlog
Date: 2026-06-16

## Why this is worth doing

The stack is already well-positioned against the npm class of supply-chain
attack: Go has **no install-time script execution** (no `postinstall`), the
dependency set is deliberately tiny (templ, modernc.org/sqlite, goose,
datastar-go), every Go module is hash-pinned in `go.sum` against the
transparency-logged checksum DB, and there is **no Node build step** — Tailwind
is a standalone binary (ADR 0008). MVS means a freshly-published malicious
version isn't auto-pulled the way npm's `^`/`~` ranges grab new releases.

That removes the three legs the recent npm attacks stand on. The **residual**
exposure is a short, enumerable list of things fetched over the network that sit
*outside* `go.sum`'s protection: runtime CDN scripts and downloaded build
binaries. None are hash-verified today. This doc captures hardening them.

## Exposure 1 — runtime CDN `<script>`s with no SRI (highest priority)

Every production page load pulls executable JS from jsdelivr with **no
Subresource Integrity hash**. A jsdelivr compromise, or a compromise of the
upstream package/tag, injects arbitrary JS into every authenticated session —
full DOM/cookie/keystroke access on the live app.

- `internal/frontend/layouts/layout.templ:46` — Datastar SDK
  `cdn.jsdelivr.net/gh/starfederation/datastar@v1.0.2/bundles/datastar.js`.
  **Production, every page.**
- `internal/frontend/static/drag.js:6` — Motion
  `cdn.jsdelivr.net/npm/motion@12.40.0/+esm`. **Production, every board page.**
- `internal/frontend/smoke.templ:14-15` — Datastar + `sortablejs@1.15.7` from
  jsdelivr. Lower stakes (wiring canary, not on the auth path) but same gap.

Note `@v1.0.2` / `@12.40.0` are *version* pins, not *content* pins — a retagged
or compromised artifact at that version is served transparently.

### Options (rough priority)

1. **Self-host under `internal/frontend/static/`** (the existing `go:embed`
   folder) and serve from `/static/` like `drag.js` already is. This brings the
   bytes into the repo, under review and immutable per deploy — the strongest
   option, and it matches the "no runtime third-party" posture. The `+esm`
   Motion bundle and the Datastar bundle would be vendored as pinned files.
   Tradeoff: manual bump step, and Motion's `+esm` form needs a resolved bundle.
2. **Add SRI** — `integrity="sha384-…" crossorigin="anonymous"` on each
   `<script>`. Cheaper than vendoring, keeps the CDN, and makes a swapped
   artifact fail closed. Doesn't help the ESM `import` in `drag.js` (no SRI for
   bare module imports) — that one effectively *needs* vendoring (option 1).
3. Either way, do `smoke.templ` too so the canary doesn't model the unsafe
   pattern.

Recommendation: **vendor Motion (drag.js can't use SRI anyway) and the Datastar
bundle into `static/`**; it's the only option that fully closes the ESM-import
gap and it fits the embed-everything architecture.

## Exposure 2 — Tailwind binary downloaded without checksum verification

The Tailwind v4 standalone binary is version-pinned in three places
(Taskfile.yml `TAILWIND_VERSION`, Dockerfile `ARG TAILWIND_VERSION`, CI) but
each just `curl`/`wget`s the release artifact and `chmod +x` — **no SHA256
verification**. A compromised release asset would execute in the dev machine,
the Docker build, and CI.

- `Taskfile.yml` `tailwind:install` — `curl -sL` then `chmod +x`.
- `Dockerfile` — `wget -qO` then `chmod +x`.
- `.github/workflows/ci.yml` "Generate CSS (Tailwind)" — `curl -sL` then
  `chmod +x`.

### Path forward

Pin the **SHA256 per (os, arch)** alongside `TAILWIND_VERSION` and verify after
download (`sha256sum -c` / `shasum -a 256 -c`), failing the build on mismatch.
Keep the checksums next to the version pin so the "bump all three together"
rule (CLAUDE.md / ADR 0008) extends to "bump version **and** checksums". Pull
the published checksums from the Tailwind release page when bumping.

## Not a gap (for the record)

- `templ` is installed via `go install …/templ@v0.3.1020` in the Dockerfile/CI
  — `go install pkg@version` resolves through the module checksum DB, so it's
  already content-verified like any other Go dependency. No action.
- Go module deps are covered by `go.sum`. Keep upgrades deliberate (never a
  blanket `go get -u ./...`) to preserve the MVS benefit.

## Related

- ADR 0008 — Tailwind standalone binary (the no-Node decision).
- CLAUDE.md "Conventions & deploy" — the three-places pin rule these checksums
  would extend.
- `internal/frontend/layouts/layout.templ`, `internal/frontend/static/drag.js`,
  `internal/frontend/smoke.templ` — the CDN call sites.
