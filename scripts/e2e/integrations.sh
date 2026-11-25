#!/usr/bin/env bash
# Browser e2e of the E2 integrations UI (scripts/e2e/integrations-e2e.js) against a
# real backend, a fake GitHub API on loopback, and a database thrown away afterwards.
#
#   scripts/e2e/integrations.sh            # everything local: initdb'd Postgres, build, run
#   ARTEX_E2E_PG_DSN=postgres://u:p@host/db scripts/e2e/integrations.sh
#                                          # use an existing EMPTY database (CI service)
#
# Needs Go, Node, and (without ARTEX_E2E_PG_DSN) initdb/pg_ctl on PATH. Playwright is
# taken from NODE_PATH when it resolves there, otherwise installed into the work dir
# (with its Chromium, which `playwright install` skips when cached).
#
# Env: ARTEX_E2E_API_PORT (28787), ARTEX_E2E_UI_PORT (25173), ARTEX_E2E_PG_PORT (56432),
# ARTEX_E2E_KEEP=1 keeps the work dir (logs, screenshots) for inspection.
#
# The backend runs with ARTEX_ALLOW_UNSANDBOXED=1 (agents are never started here: no
# task runs) and ARTEX_NOTIFY_ALLOW_LOCAL=1 so the tracker client may reach the
# loopback fake. Both are test-only; never set them on a real instance.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
API_PORT="${ARTEX_E2E_API_PORT:-28787}"
UI_PORT="${ARTEX_E2E_UI_PORT:-25173}"
PG_PORT="${ARTEX_E2E_PG_PORT:-56432}"
PASSWORD="e2e-correct-horse-battery-9"
WORK="$(mktemp -d "${TMPDIR:-/tmp}/artex-e2e.XXXXXX")"
mkdir -p "$WORK/shots"
PIDS=()

log() { printf '[e2e] %s\n' "$*"; }
cleanup() {
  local code=$?
  for pid in "${PIDS[@]:-}"; do [ -n "$pid" ] && kill "$pid" 2>/dev/null || true; done
  if [ -d "$WORK/pg" ]; then pg_ctl -D "$WORK/pg" -m immediate stop >/dev/null 2>&1 || true; fi
  if [ "$code" -ne 0 ]; then
    log "failed; last backend log lines:"
    tail -n 25 "$WORK/backend.log" 2>/dev/null || true
  fi
  if [ "${ARTEX_E2E_KEEP:-0}" = "1" ] || [ "$code" -ne 0 ]; then
    log "work dir kept: $WORK"
  else
    rm -rf "$WORK"
  fi
  exit "$code"
}
trap cleanup EXIT INT TERM

wait_for() { # url, seconds
  local i
  for i in $(seq 1 "$2"); do
    curl -fsS -o /dev/null "$1" 2>/dev/null && return 0
    sleep 1
  done
  log "timed out waiting for $1"
  return 1
}

# ---- database ----
if [ -n "${ARTEX_E2E_PG_DSN:-}" ]; then
  DSN="$ARTEX_E2E_PG_DSN"
else
  log "starting a throwaway Postgres on 127.0.0.1:$PG_PORT"
  initdb -D "$WORK/pg" -U postgres -A trust >"$WORK/initdb.log" 2>&1
  LC_ALL="${LC_ALL:-en_US.UTF-8}" pg_ctl -D "$WORK/pg" -o "-p $PG_PORT -k $WORK -c listen_addresses=127.0.0.1" \
    -l "$WORK/pg.log" -w start >/dev/null
  createdb -h 127.0.0.1 -p "$PG_PORT" -U postgres artex
  DSN="postgres://postgres@127.0.0.1:$PG_PORT/artex?sslmode=disable"
fi

# ---- backend (config, keys and data live next to the binary, in the work dir) ----
log "building the backend"
(cd "$ROOT" && go build -o "$WORK/artex" ./cmd/artex)
ARTEX_PG_DSN="$DSN" ARTEX_ALLOW_UNSANDBOXED=1 ARTEX_NOTIFY_ALLOW_LOCAL=1 \
  "$WORK/artex" -addr "127.0.0.1:$API_PORT" -proxy "" -data "$WORK/data" >"$WORK/backend.log" 2>&1 &
PIDS+=("$!")
wait_for "http://127.0.0.1:$API_PORT/api/auth/status" 90
TOKEN=""
for _ in $(seq 1 30); do
  TOKEN="$(sed -n 's/.*Setup token for the \/setup page: \([0-9a-f]*\).*/\1/p' "$WORK/backend.log" | tail -n 1)"
  [ -n "$TOKEN" ] && break
  sleep 1
done
[ -n "$TOKEN" ] || { log "no setup token in the backend log (is the database empty?)"; exit 1; }
curl -fsS -X POST "http://127.0.0.1:$API_PORT/api/auth/init" -H 'Content-Type: application/json' \
  -d "{\"password\":\"$PASSWORD\",\"setup_token\":\"$TOKEN\"}" >/dev/null
log "backend up on :$API_PORT, admin password set"

# ---- web UI: a production Next build whose /api rewrite points at the backend ----
log "building the web UI (next build)"
(cd "$ROOT/web" && { [ -d node_modules ] || npm ci --no-audit --no-fund; } &&
  AUTOPENTEST_API="http://127.0.0.1:$API_PORT" ./node_modules/.bin/next build >"$WORK/next-build.log" 2>&1) ||
  { tail -n 30 "$WORK/next-build.log"; exit 1; }
# exec, so the recorded pid is next itself and cleanup stops it.
(cd "$ROOT/web" && AUTOPENTEST_API="http://127.0.0.1:$API_PORT" exec ./node_modules/.bin/next start \
  -H 127.0.0.1 -p "$UI_PORT" >"$WORK/next.log" 2>&1) &
PIDS+=("$!")
wait_for "http://127.0.0.1:$UI_PORT/login" 120

# ---- Playwright ----
if ! node -e 'require("playwright")' >/dev/null 2>&1; then
  log "installing Playwright into the work dir"
  npm install --prefix "$WORK/pw" --no-audit --no-fund --silent playwright@1 >/dev/null
  export NODE_PATH="$WORK/pw/node_modules${NODE_PATH:+:$NODE_PATH}"
  (cd "$WORK/pw" && npx playwright install chromium >/dev/null)
fi

log "running the scenario"
ARTEX_E2E_URL="http://127.0.0.1:$UI_PORT" ARTEX_E2E_PASSWORD="$PASSWORD" ARTEX_E2E_PG_DSN="$DSN" ARTEX_E2E_SHOTS="$WORK/shots" \
  node "$ROOT/scripts/e2e/integrations-e2e.js"
