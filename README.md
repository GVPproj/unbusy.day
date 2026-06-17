<p align="center">
  <img src=".github/ub-logo.svg" width="220" alt="ub.d — the unbusy.day wordmark">
</p>

# unbusy.day

[![CI/CD](https://github.com/GVPproj/unbusy.day/actions/workflows/ci.yml/badge.svg)](https://github.com/GVPproj/unbusy.day/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/GVPproj/unbusy.day)](https://goreportcard.com/report/github.com/GVPproj/unbusy.day)
[![Go](https://img.shields.io/github/go-mod/go-version/GVPproj/unbusy.day?logo=go&logoColor=white)](https://go.dev)
Have a structured day. Time-block your schedule, track some progress, no rush.

## Quickstart

Requires `go` ≥ 1.26 and a few Go tools (no Docker — the database is a local
SQLite file):

```bash
# One-time
go install github.com/go-task/task/v3/cmd/task@latest
go install github.com/a-h/templ/cmd/templ@latest
cp .env.example .env

# Day-to-day
task dev                          # SQLite + templ watch + Go hot reload
```

