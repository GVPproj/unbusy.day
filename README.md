# hello-cards

A minimal full-stack reference app validating the production architecture for a
Trello-like multi-tenant product with optimistic UI over flaky networks: a Go
API as the only home of business logic, Postgres as source of truth, SSE for
live reads, and **two frontends over one Go core** (Vite + React + TanStack DB,
and Datastar + templ) to prove the "logic exactly once" thesis.

See [`PRD.md`](./PRD.md) for the full spec, milestones, and trade-offs.

## License

This project is **source-available** under
[FSL-1.1-Apache-2.0](./LICENSE.md) — not OSI "open source." You can read,
modify, self-host, and contribute, but you cannot offer it as a commercial
product or service that competes with the licensor's hosted offering. Each
release converts to Apache-2.0 two years after publication.

The hosted service is the commercial offering; the source is here so you can
learn from it, run it yourself, and contribute back.

## Prerequisites

Local toolchain (no login needed):

- `go` ≥ 1.26
- `node` ≥ 20, `pnpm` ≥ 9
- `docker` + `compose`
- `psql` (Postgres client)
- `task` (go-task), `air`, `templ`
- `lefthook`, `gitleaks`
- `git`, `curl`, `jq`

Service CLIs (needed at M3+):

- `flyctl` — `fly auth login` (deploy)
- `neonctl` — `neon auth` (Postgres provisioning)
- `gh` — `gh auth login` (fast-follow CI)
- Cloudflare — no CLI; `CLOUDFLARE_API_TOKEN` env + `curl`

See [`PRD.md` §12](./PRD.md) for install/login commands.

## Quickstart

```bash
# One-time
brew install go-task lefthook gitleaks docker
corepack enable && corepack prepare pnpm@latest --activate
go install github.com/air-verse/air@latest
go install github.com/a-h/templ/cmd/templ@latest
lefthook install                  # wires pre-commit gitleaks hook
cp .env.example .env

# Day-to-day
task dev                          # Postgres + Go hot reload
curl localhost:8080/healthz       # → 200 OK
```

### Linux (Debian / Ubuntu / Mint)

No Homebrew. Install Go-based tools with `go install` (they land in
`$(go env GOPATH)/bin` — ensure it's on your `PATH`), and the system clients
via `apt`. Install `docker` + `node`/`corepack` via your distro's usual method.

```bash
# One-time
sudo apt-get update && sudo apt-get install -y postgresql-client jq curl git
go install github.com/go-task/task/v3/cmd/task@latest
go install github.com/air-verse/air@latest
go install github.com/a-h/templ/cmd/templ@latest
go install github.com/evilmartians/lefthook@latest
go install github.com/zricethezav/gitleaks/v8@latest   # note: zricethezav, not gitleaks
corepack enable && corepack prepare pnpm@latest --activate
lefthook install                  # wires pre-commit gitleaks hook
cp .env.example .env
```

## Contributing

PRs welcome. Sign commits with `git commit -s` (DCO) — see
[`CONTRIBUTING.md`](./CONTRIBUTING.md). A CI check enforces sign-off.
