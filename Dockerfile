# syntax=docker/dockerfile:1.7
# Single-stage Go build (+ templ generate) → scratch. 

# --- Go build (+ templ generate) ---
FROM golang:1.26-alpine AS build
WORKDIR /src
# git for go tooling; libstdc++/libgcc for the Tailwind musl binary.
# hadolint ignore=DL3018
RUN apk add --no-cache git libstdc++ libgcc
# go.mod is the single source of truth for the templ version; versions.env for
# Tailwind (shared with the Taskfile and CI). Copy both before installing.
COPY go.mod go.sum versions.env ./
RUN go mod download \
  && go install "github.com/a-h/templ/cmd/templ@$(go list -m github.com/a-h/templ | awk '{print $2}')"
# Tailwind v4 standalone (musl), version from versions.env, arch from buildx.
ARG TARGETARCH
RUN . ./versions.env; case "${TARGETARCH}" in arm64) twarch=arm64 ;; *) twarch=x64 ;; esac; \
  wget -qO /usr/local/bin/tailwindcss \
  "https://github.com/tailwindlabs/tailwindcss/releases/download/v${TAILWIND_VERSION}/tailwindcss-linux-${twarch}-musl" \
  && chmod +x /usr/local/bin/tailwindcss
COPY . .
# Generate templ + the embedded stylesheet before the Go build embeds static/.
RUN templ generate \
  && tailwindcss -i internal/frontend/input.css -o internal/frontend/static/output.css --minify \
  && CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/unbusy ./cmd/unbusy

# --- Runtime: scratch (no shell, no libc) ---
FROM scratch
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /out/unbusy /unbusy
EXPOSE 8080
ENTRYPOINT ["/unbusy"]
