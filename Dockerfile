# syntax=docker/dockerfile:1.7
# Single-stage Go build (+ templ generate) → scratch. The frontend is
# server-rendered Datastar + templ, so there's no node/Vite build to embed.

# --- Go build (+ templ generate) ---
FROM golang:1.26-alpine AS build
WORKDIR /src
RUN apk add --no-cache git
# Pinned to the templ runtime version in go.mod — a CLI/runtime mismatch fails
# generation rather than producing skewed output.
RUN go install github.com/a-h/templ/cmd/templ@v0.3.1020
# Tailwind v4 standalone binary — pinned identically in the Taskfile and CI
# (bump all three together). musl build for alpine; mapped from the buildx
# TARGETARCH so amd64/arm64 builders both work.
ARG TAILWIND_VERSION=4.3.1
ARG TARGETARCH
RUN case "${TARGETARCH}" in arm64) twarch=arm64 ;; *) twarch=x64 ;; esac; \
    wget -qO /usr/local/bin/tailwindcss \
      "https://github.com/tailwindlabs/tailwindcss/releases/download/v${TAILWIND_VERSION}/tailwindcss-linux-${twarch}-musl" \
    && chmod +x /usr/local/bin/tailwindcss
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN templ generate
# Generate the embedded utility stylesheet before the Go build embeds static/.
RUN tailwindcss -i internal/frontend/input.css -o internal/frontend/static/output.css --minify
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/unbusy ./cmd/unbusy

# --- Runtime: scratch (no shell, no libc) ---
FROM scratch
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /out/unbusy /unbusy
EXPOSE 8080
ENTRYPOINT ["/unbusy"]
