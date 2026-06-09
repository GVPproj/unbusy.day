# syntax=docker/dockerfile:1.7
# Multi-stage build: node (FE1) → go + templ (FE2) → scratch.
# M0 ships the Go-only path; node/templ stages activate as M2a / M2.5b land
# (see PRD §8). Keeping the shape explicit now so later milestones are additive.

# --- 1. Frontend (Vite + React + TanStack DB) — wired in M2a ---
# Builds frontend/dist/ which the Go binary embeds via go:embed (PRD F4).
# FROM node:20-alpine AS frontend
# WORKDIR /app/frontend
# COPY frontend/package.json frontend/pnpm-lock.yaml ./
# RUN corepack enable && pnpm install --frozen-lockfile
# COPY frontend/ ./
# RUN pnpm build

# --- 2. Go build (+ templ generate for FE2 — wired in M2.5b) ---
FROM golang:1.22-alpine AS build
WORKDIR /src
RUN apk add --no-cache git
# Uncomment once ds/ has templ files (M2.5b):
# RUN go install github.com/a-h/templ/cmd/templ@latest
COPY go.mod ./
# COPY go.sum ./
RUN go mod download
COPY . .
# COPY --from=frontend /app/frontend/dist ./frontend/dist
# RUN templ generate
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/hello-cards .

# --- 3. Runtime: scratch (no shell, no libc) ---
FROM scratch
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /out/hello-cards /hello-cards
EXPOSE 8080
ENTRYPOINT ["/hello-cards"]
