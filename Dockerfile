# syntax=docker/dockerfile:1.7
# Multi-stage build: node (FE1) → go + templ (FE2) → scratch.
# M0 ships the Go-only path; node/templ stages activate as M2a / M2.5b land
# (see PRD §8). Keeping the shape explicit now so later milestones are additive.

# --- 1. Frontend (Vite + React + TanStack DB) ---
# Builds frontend/dist/ which the Go binary embeds via go:embed (PRD F4).
# node:22 — current pnpm (11.x, resolved by corepack) requires node:sqlite,
# which only exists on Node 22+. PRD §12 floor is node >= 20; we pin 22 here.
FROM node:22-alpine AS frontend
WORKDIR /app/frontend
COPY frontend/package.json frontend/pnpm-lock.yaml ./
RUN corepack enable && pnpm install --frozen-lockfile
COPY frontend/ ./
RUN pnpm build

# --- 2. Go build (+ templ generate for FE2 — M2.5b) ---
FROM golang:1.26-alpine AS build
WORKDIR /src
RUN apk add --no-cache git
# Pinned to the templ runtime version in go.mod (ds/NOTES.md) — a CLI/runtime
# mismatch fails generation rather than producing skewed output.
RUN go install github.com/a-h/templ/cmd/templ@v0.3.1020
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Vite build from stage 1 — embedded via go:embed in the -tags embedassets
# build below (PRD F4). Lands after `COPY . .` so it isn't clobbered.
COPY --from=frontend /app/frontend/dist ./frontend/dist
RUN templ generate
RUN CGO_ENABLED=0 GOOS=linux go build -tags embedassets -ldflags="-s -w" -o /out/hello-cards .

# --- 3. Runtime: scratch (no shell, no libc) ---
FROM scratch
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /out/hello-cards /hello-cards
EXPOSE 8080
ENTRYPOINT ["/hello-cards"]
