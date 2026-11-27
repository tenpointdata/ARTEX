#!/usr/bin/env bash
# Role probe of the console (scripts/e2e/role-probe-e2e.js): signs in as reviewer,
# operator and client_read, clicks every enabled control on every page, and fails on
# any /api 4xx or any request to a route whose permission (server/rbac_policy.go)
# the role lacks. Writes are answered in the browser and never reach the backend.
#
#   scripts/e2e/role-probe.sh
#   ARTEX_E2E_ROLES=client_read ARTEX_E2E_PAGES=/function scripts/e2e/role-probe.sh
#
# Runs on the integrations.sh harness (a throwaway Postgres, the backend with its
# recording proxy on ARTEX_E2E_PROXY_PORT, default 28788, a production web build);
# every ARTEX_E2E_* variable documented there applies. ARTEX_E2E_MAX_CLICKS caps the
# controls clicked per page (60).
set -euo pipefail
export ARTEX_E2E_SCENARIO=role-probe-e2e.js
export ARTEX_E2E_PROXY_PORT="${ARTEX_E2E_PROXY_PORT:-28788}"
exec "$(dirname "$0")/integrations.sh"
