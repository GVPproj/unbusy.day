#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
if [ ! -x "$root/node_modules/.bin/playwright" ]; then
  echo "browser smoke: run npm ci and npx playwright install chromium first" >&2
  exit 1
fi

scratch=$(mktemp -d)
port=${BROWSER_SMOKE_PORT:-18199}
server_pid=
cleanup() {
  if [ -n "$server_pid" ]; then kill "$server_pid" 2>/dev/null || true; fi
  rm -rf "$scratch"
}
trap cleanup EXIT INT TERM

DATABASE_URL="file:$scratch/smoke.db?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_txlock=immediate" \
PORT="$port" "$root/tmp/unbusy" >"$scratch/server.log" 2>&1 &
server_pid=$!

i=0
until curl -fsS "http://127.0.0.1:$port/healthz" >/dev/null 2>&1; do
  i=$((i + 1))
  if [ "$i" -ge 100 ]; then
    echo "browser smoke: server did not start" >&2
    cat "$scratch/server.log" >&2
    exit 1
  fi
  sleep 0.1
done
if ! kill -0 "$server_pid" 2>/dev/null; then
  echo "browser smoke: server exited during startup" >&2
  cat "$scratch/server.log" >&2
  exit 1
fi

cd "$root"
# Real OTP sends share one rate budget; parallel scenarios can starve each other's login.
BROWSER_SMOKE_URL="http://127.0.0.1:$port" BROWSER_SMOKE_LOG="$scratch/server.log" npm run test:browser -- --workers=1 "$@"
