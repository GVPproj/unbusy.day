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
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN templ generate
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/unbusy ./cmd/unbusy

# --- Runtime: scratch (no shell, no libc) ---
FROM scratch
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /out/unbusy /unbusy
EXPOSE 8080
ENTRYPOINT ["/unbusy"]
