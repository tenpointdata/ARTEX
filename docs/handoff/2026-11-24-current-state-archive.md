# ARTEX — archived Current state blocks (moved from HANDOFF.md on 2026-11-24)

Older, mostly landed status blocks, moved out to keep HANDOFF.md under its size limit. HANDOFF.md's Current state holds what is in flight; `ej handoff log` has the full history.

**2026-11-24 — E5 engagement governance (card `c-669406`)** is on `main` (#31); see `docs/GOVERNANCE.md`.
Roles `admin`/`operator`/`reviewer`/`client_read` over the JWT (`artex_users`, `/api/users`, policy in
`server/rbac_policy.go`, enforced in `requireAuth`; `ARTEX` is the break-glass admin). The per-task
authorization artifact is enforced only inside the target gate (`intercept/authorization.go`). `audit_chain`
is hash-chained by deferred row triggers (migrations 0016, 0018); `/api/intercept/history/chain[/verify]`.
With `ARTEX_PG_DSN`, `go test ./...` fails only `TestGateOffTaskIsUnchanged` (`c-3157b4`),
`TestConnectWithoutPasswordRealPG` (trust auth) and the macOS SOCKS5 UDP test.

**2026-11-24 — client_read bound to companies (card `c-5b5421`, in review).** Detail (branch, verification, gaps) moved to `docs/handoff/2026-11-24-client-read-company-bindings.md`.

**2026-11-23** — Fixes for the High and Medium findings from the 2026-11-23 security audit are on
`main` (PR #1, branch `ej/20261123-145205-6a3af6`); the Linux/Docker verification fixes come from
`ej/20261123-152406-fccff3`. It covers audit findings #1–#16 except as listed under Open work.
Verified 2026-11-23 (card `c-dadc14`, done):
- `go vet ./...` + `go test ./...` **as root on Linux** (golang:1.26.11-bookworm, arm64) with
  `ARTEX_PG_DSN` set: all packages ok, including the `*AsRoot` executor/isolation tests,
  `TestExecHomePerRunDirAsRoot` and the MCP uid-10002 split. Two test fixes were needed (commit
  `fc8c3f9`): the isolation probe could not exec the `go test` binary from its 0700 build dir (also
  broken on main), and `TestGateOffTaskIsUnchanged` still expected zero lookups with the gate off.
- Docker egress: through the real `docker-compose.yml` (NET_ADMIN) on a slim image of the same
  runtime base, `[artex] egress: installed` is logged, `-m owner`/`-m addrtype` load on bookworm's
  nf_tables iptables 1.8.9, and the executor is refused for 169.254.169.254, 100.100.100.200, the
  gateway, host.docker.internal (root reaches the host service) and :8787 on every local address,
  while a LAN router, another container and the internet stay reachable. The full image gives the
  same results (2026-11-24, `c-a6891b`, below).
- Web: `npm ci` and `npx tsc --noEmit` clean. `npm run lint` fails with 84 errors, all already on main
  and none in the changed files (`c-834a5a`). Headless Chromium: setup, login, Sign out over the
  HttpOnly SameSite=Strict cookie work and `document.cookie` is empty, **but a token copied before
  logout stays valid for 7 days** (`c-a0af33`).
- The stock compose cannot start on Docker Desktop for Mac: bind mounts ignore permissions, so the
  isolation self-test fails closed (also on main; `c-775dc2`). Use named volumes there.

Operator-facing changes from that audit (unsandboxed-start refusal, TLS on non-loopback binds, and more): `docs/handoff/2026-11-23-security-audit-operator-changes.md`.

**2026-11-24: full Docker image verified (card `c-a6891b`).** The complete image (bbot, chromium,
Playwright) built from `1abe24f` with the committed Dockerfile on Docker Desktop arm64 in 34 min. The
bbot layer's own dry-run grep passed: trufflehog installed cleanly this time, with no ERRR. Run through
`docker-compose.yml` with named volumes, it logs `[artex] egress: installed for uid 10001 (ipv4)…(ipv6)`
and the isolation self-test passes. Checked with `scripts/docker-egress-probe.sh` (new): the executor
gets REJECT on 169.254.169.254, 127.0.0.1:8787, the container IP's :8787 and host.docker.internal; root
reaches all of them except metadata, which Docker Desktop doesn't serve (timeout). Both reach the LAN
router and example.com. `scripts/docker-login-probe.js` (new, uses the image's Playwright) passes
login, HttpOnly + SameSite=Strict cookie, token hidden from scripts, Sign out → /login and 401. Replaying
the pre-logout cookie still gets 200 because revocation (`c-a0af33`) is not on main yet. Not re-run on
the newer main (`-exec-uid-range`, #18).

**2026-11-24: E8 evaluation suite** (card `c-5487e8`) is in `scripts/benchmark`. `eval_suite.py run` runs a
baseline and a candidate prompt arm over a held-out suite. Both arms share one stamp: model chain, schedule
seed, per-task budget, judge, and content-hashed ground truth. Runs go one at a time and the arms alternate in
an order drawn from the seed. The output is per-gate passes/n for `detected`, `component`, `class`, `keywords`,
`proof` and `flag_captured`, with Wilson and Newcombe intervals, a paired gained/lost table and cost per arm.
`compare` refuses two results whose stamps differ. Arms change only the editable `goals`/`planner`/`worker`
body, through the versioned prompt API, and the starting bodies are restored afterwards. Suites:
`suites/heldout-v1.json` (Juice Shop v20.4.0, DVGA 2.2.0, DrostBench 005…095) and a disjoint
`suites/dev-v1.json`. New ground truth: `ground_truth/dvga.json` (v2, hand-verified 2026-11-24 against DVGA tag 2.2.0 commit `a961308`; card `c-085c80` done, see Decisions). `run_bench.py` now exposes
`build_parser`/`prepare`/`run` and stamps `/api/tasks/{id}/outcome`. Unit-tested (15 new tests,
`scripts/verify.sh` passes); never yet run live (card `c-f1518c`).
