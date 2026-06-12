# unbusy.day

[![CI/CD](https://github.com/GVPproj/unbusy.day/actions/workflows/ci.yml/badge.svg)](https://github.com/GVPproj/unbusy.day/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/GVPproj/unbusy.day)](https://goreportcard.com/report/github.com/GVPproj/unbusy.day)

Have a structured day. Time-block your schedule, track some progress, no rush.

## Prerequisites

Local toolchain (no login needed):

- `go` ≥ 1.26
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

## Quickstart

Install `go`, `docker`, `psql`, `git`, `curl`, and `jq` via your OS package
manager. The rest is OS-neutral `go install` (binaries land in
`$(go env GOPATH)/bin` — ensure it's on your `PATH`):

```bash
# One-time
go install github.com/go-task/task/v3/cmd/task@latest
go install github.com/air-verse/air@latest
go install github.com/a-h/templ/cmd/templ@latest
go install github.com/evilmartians/lefthook@latest
go install github.com/zricethezav/gitleaks/v8@latest   # note: zricethezav, not gitleaks
lefthook install                  # wires pre-commit gitleaks hook
cp .env.example .env

# Day-to-day
task dev                          # Postgres + templ watch + Go hot reload
curl localhost:8080/healthz       # → 200 OK
```

## Contributing

PRs welcome. Sign commits with `git commit -s` (DCO) — see
[`CONTRIBUTING.md`](./CONTRIBUTING.md). A CI check enforces sign-off.

## License

Licensed under [FSL-1.1-Apache-2.0](./LICENSE.md): free to read, self-host,
and contribute to, but not to offer as a competing commercial service. Each
release converts to Apache-2.0 after two years.
