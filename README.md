<p align="center">
  <img src=".github/ub-logo.svg" width="220" alt="ub.d — the unbusy.day wordmark">
</p>

# unbusy.day

[![CI/CD](https://github.com/GVPproj/unbusy.day/actions/workflows/ci.yml/badge.svg)](https://github.com/GVPproj/unbusy.day/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/GVPproj/unbusy.day)](https://goreportcard.com/report/github.com/GVPproj/unbusy.day)

Have a structured day. Time-block your schedule, track some progress, no rush.

## Quickstart

Requires `go` ≥ 1.26, `docker` + `compose`, and a few Go tools:

```bash
# One-time
go install github.com/go-task/task/v3/cmd/task@latest
go install github.com/a-h/templ/cmd/templ@latest
cp .env.example .env

# Day-to-day
task dev                          # Postgres + templ watch + Go hot reload
```

## Contributing

PRs welcome. Sign commits with `git commit -s` (DCO) — see
[`CONTRIBUTING.md`](./CONTRIBUTING.md). A CI check enforces sign-off.

## License

Licensed under [FSL-1.1-Apache-2.0](./LICENSE.md): free to read, self-host,
and contribute to, but not to offer as a competing commercial service. Each
release converts to Apache-2.0 after two years.
