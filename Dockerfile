# syntax=docker/dockerfile:1.7
# Single-stage Go build (+ templ generate) → scratch. 

# --- Go build (+ templ generate) ---
FROM golang:1.26-alpine AS build
WORKDIR /src
# git for go tooling.
# hadolint ignore=DL3018
RUN apk add --no-cache git
# go.mod is the single source of truth for the templ version.
COPY go.mod go.sum ./
RUN go mod download \
  && go install "github.com/a-h/templ/cmd/templ@$(go list -m github.com/a-h/templ | awk '{print $2}')"
COPY . .
# Generate templ before the Go build embeds static/.
RUN templ generate \
  && CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/unbusy ./cmd/unbusy

# --- Runtime: scratch (no shell, no libc) ---
FROM scratch
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /out/unbusy /unbusy
EXPOSE 8080
ENTRYPOINT ["/unbusy"]
