# HANDOFF.md — ARTEX

**Last updated:** 2026-11-24 13:03 MST, by claude-code: E3 (c-d25ff9, now in review): built the RSI.md Loop C benchmark gate and Loop B lesson graduation, with ARTEX_RSI_LOOP off by default. Earlier hand-offs are under Deltas and in `ej handoff log`.

> Single entry point for whoever picks this up next (any agent or human), with no access to the
> originating conversation. Read this first, then the sections of the code it points at.

## Handoff policy (propagated — keep this section intact)

Any repository you work in must stay in a state where another agent (Claude, Codex, Gemini,
Antigravity, or a human) can pick up the work cold. Two things make that true, and you maintain both:

1. **`HANDOFF.md` at the repository root** — this file. It holds: what the project is and where its
   pieces live; the current state under a dated stamp; how to set up/run/test/deploy; the open work,
   each item linked to its GitHub issue; a **triage** list (lanes: agents, a human, blocked); known
   issues and gotchas; recent decisions and why; and this policy, in full, so it survives the handoff.
2. **GitHub issues for every open piece of work** — anything unfinished, deferred, newly discovered,
   or decided against for now, with enough context to act on without the originating conversation.
   Close issues when the work lands and reference them from the commit/PR. The open-work list points
   at issues; it never carries work that has no issue.

**When to update:** at the end of any session/turn that changed code, state, config, or plans, before
the final commit/push. Refresh the stamp, add/close issues, record decisions.

**Fork and merge:** keep this file in the standard form (sections below). On start, after `orient`,
`fork_handoff`; on finish, rewrite Current state and `merge_handoff` (summary, next steps, decisions,
issues opened/closed); `discard_handoff` if nothing to hand on. Without those tools:
`uv run --project ~/Documents/Projects/composite ej handoff check|init|fork|merge|discard`.

**Triage:** take the top unclaimed agents'-lane item unless given a task, and claim it. On finish,
re-rank; keep it short — the next handful, not the whole backlog.

**Propagation:** every `HANDOFF.md`/`AGENTS.md` carries this policy; `CLAUDE.md` imports `@AGENTS.md`.

## What this is

ARTEX is a self-hosted, LLM-driven **autonomous penetration-testing platform for authorized security
testing**. A planner decomposes a goal into intents; workers run real tools; results are written back
into two linked PostgreSQL graphs — an **asset graph** (*what exists*, shared across tasks) and a
per-task **exploration graph** (*what was tried, with what result and why*), joined by anchors.
Findings carry the captured request/response that prove them (recording MITM proxy + browser
extension). Every autonomous action passes through scope rules, an approval gate, and budgets.

English-translation continuation of `Autumn-27/ARTEX`, **AGPL-3.0**, hosted at `tenpointdata/ARTEX`.
Current released version: **v0.7.0** (2026-11-21); `main` is ahead with an Unreleased section.

## Where things live

See `docs/ARCHITECTURE.md` for the code-level map. Summary:

- `cmd/artex` — entry point. `agent/` — LLM roles (`goals`/`planner`/`worker`/`mainagent`/`retester`)
  + tool catalog + prompts + untrusted-output boundary. `server/` — HTTP API (~270 routes) +
  orchestration (`Manager`/`Engine`), JWT auth, SSE (~246 files). `db/` — persistence, `schema.sql` +
  numbered migrations, AES-256-GCM secret columns. `traffic/` — recording MITM proxy, replay, rewrite
  rules, tamper hooks, SOCKS5/transparent, body archive. `evidence/`, `intercept/` (approval gate),
  `guard/`, `egress/` (noise governor), `llmpool`/`llmrec`, `mcphttp`, `notify`, `report`,
  `sidequestion`, `skills/`. `web/` — Next.js 16 / React 19, embedded via `go:embed` (`-tags embedui`).
- `docs/` — `ARCHITECTURE.md`, `OPS.md`, traffic docs, `RSI.md` (self-improvement governance contract).

## Current state

**2026-11-24 (c-f4115c, E5 lifecycle, branch `ej/20261124-133353-5aceb1`)** — The six E5 lifecycle gaps are
closed on a branch that merges the E5 + client_read branch (`ej/20261124-110647-182fa9`, PR #16), so it
lands after PR #16 and c-5b5421. (1) Task archives snapshot `engagement_authorizations`; archiving keeps
the rows with the stub; restore re-creates missing versions (audit actor `archive-restore`) and keeps a
differing DB copy with a warning. (2) An artifact requires the target gate: `AddAuthorization` turns it on
in the same transaction, `PATCH target_gate=false` answers 409 (`db.ErrTargetGateRequired`), and spawned
children under an artifact are gated. (3) The scheduler pauses tasks whose artifact is revoked or past
`not_after` (`server/authorization_expiry.go`, cause `authorization_ended`); a revocation pauses at once;
resume answers 409 until a version that has not ended is stored. (4) `POST /api/auth/change-password`
serves console users (compare-and-swap on the session's user version, 409 on a race); the sidebar
dialog signs out after a change. (5) SSE in-stream re-checks use `db.CachedUser`; (6) the gate reads
`db.CachedAuthorization`. Both are 2 s caches on the new generic `memo/` package, invalidated by every
write through `db`. Docs: `docs/GOVERNANCE.md`. Verified: PG tests in `db/authorization_lifecycle_test.go`,
`db/task_archives_authorization_pg_test.go`, `server/authorization_lifecycle_pg_test.go`,
`server/user_password_pg_test.go` (each key assertion mutation-checked); `go test ./db ./intercept ./memo
./agent ./server` with PG passes except the two known environmental failures; `scripts/verify.sh` with
web steps passes. Not checked: a live task actually being paused by the scheduler at expiry (covered by
calling the sweep directly). Legacy artifacts stored before this change with the gate off are not
backfilled (only unreleased dev data). Follow-ups: `c-5018f6`, `c-01377e`.

**2026-11-24 (c-d25ff9, E3)** — The `RSI.md` Loop C benchmark gate and Loop B lesson graduation are built,
flag off (`ARTEX_RSI_LOOP`, default off, read per request). Pure core `rsi/` (search-space allowlist:
`prompt_body` for goals/planner/worker and `lesson`; immutable and git-reviewed surfaces refused;
annealed budget 4,2,1,1…; `Evaluate` recomputes per-gate and paired results from raw E8 arm records,
binds them to the exact body sha / lesson set, refuses proposed-from targets, net regressions and
gains on fewer than 2 targets). API `/api/rsi/*` (`server/rsi_loop.go`); migration **0020**
(`rsi_proposals`, `rsi_proposal_evidence`, append-only `rsi_adoption_log`). Adoption = owner
password + newest passing evidence id + evidence baseline equal to the live prompt; it appends an
`agent_prompts` version; rollback is a pointer change (refused if an owner edited since, open
with the flag off). `ARTEX_RSI_LESSONS` gains `adopted` (only graduated lessons) and `bench` (a
pinned set, `PUT /api/rsi/bench-lessons`). This branch merges the unmerged E8 suite branch
(`ej/20261124-084655-ccdab2`); harness gains lesson arms and `scripts/benchmark/rsi_submit.py`.
An adversarial review (opus) found real-upload breakage (string task ids) and replay/forgery
paths; fixed here: task ledger across proposals, `suite_run` invariant, 3 uploads per proposal,
crashed candidate runs count as losses, rendered + normalized marker checks, ≤4 open proposals
with forward-only rounds, split agent lock, lesson block sha and unedited-role shas re-checked at
adoption. Remaining findings: `c-0d277f`. Verified: `go test ./rsi`, the RSI DB and server tests
against Postgres 16, 65 benchmark Python tests, `scripts/verify.sh`. Not verified: any live suite run through the gate (`c-772190`); no UI
(`c-936308`).

**2026-11-24 (c-75c593)** — The benchmark subject instance (`docker-compose.bench.yml`) now builds from a
clean clone: production `Dockerfile`, a sibling Postgres, named volumes, committed `bench/.env.example`
with `ARTEX_RSI_LESSONS=off`. Verified on Docker Desktop arm64: `up --build` brings both containers up,
isolation self-test passes, egress filter installed, UI 200, `ARTEX_RSI_LESSONS=off` in the container.
No benchmark run was made against it.

**2026-11-24 — E5 engagement governance (card `c-669406`)** is on branch `ej/20261124-055929-bd2253`. It
is tested against Postgres 17 on macOS; see `docs/GOVERNANCE.md`.
- **Roles.** `admin`, `operator`, `reviewer` and `client_read` work over the existing JWT. The users
  are in `artex_users` and managed at `/api/users`. The policy table is `server/rbac_policy.go`,
  enforced in `requireAuth`. `ARTEX` stays the break-glass admin.
- **Authorization artifact.** Each task can have a versioned, immutable artifact covering scope, time
  window and technique classes (`/api/tasks/{id}/authorization`). It is enforced only inside the
  existing target gate (`intercept/authorization.go`, called from `CheckTargets`/`run`,
  `CheckUngated`/`ungated`, and the dial and redirect paths). A spawned child task inherits it.
- **Audit chain.** `audit_chain` is hash-chained and written by row triggers on `intercept_pending`,
  `artex_users` and `engagement_authorizations`. Logins and refusals by role are appended too.
  `GET /api/intercept/history/chain` lists it and `/chain/verify` checks it.
- **Migration numbering.** Migration **0016** holds the schema, because 0015 is taken by the E2
  integrations branch. If E2 has not merged when this branch lands, renumber whichever merges second.
  The loader fails closed on a pending version older than one already applied.
- **Tests.** With `ARTEX_PG_DSN`, all of `go test ./...` passes except three tests:
  - `TestGateOffTaskIsUnchanged`, which also fails on base `1abe24f` (card `c-3157b4`);
  - `TestConnectWithoutPasswordRealPG`, because the local Postgres uses trust auth;
  - the SOCKS5 UDP test, because macOS cannot bind 127.0.0.2.
- **Head-lock contention fixed (card `c-ba9f64`).** Migration **0018** recreates the three audit row
  triggers as `DEFERRABLE INITIALLY DEFERRED` constraint triggers, so `audit_chain_head` is locked
  only at commit. A task archive no longer stalls gate decisions or logins, and the
  write-then-lock deadlock is gone. 0018 rather than 0017 because the E1 branch
  (`ej/20261124-083744-02fa3c`) takes 0017. Tests: `db/audit_chain_concurrency_test.go` (both
  concurrency tests fail without 0018). This branch merged the E5 branch in, so it carries E5 too.
- **Not done.** There is no web UI for E5 yet (card `c-869050`). The login page still assumes `ARTEX`.

**2026-11-24 — client_read bound to companies (card `c-5b5421`, in review)** is on branch
`ej/20261124-110647-182fa9`. That branch merged the E5 branch (PR #16) in, so it carries E5 too.
- A `client_read` user sees only the companies in `artex_user_companies` (migration **0019**, audited at
  commit). Admins set them with `GET/PUT /api/users/{id}/companies`. An unbound client gets 403.
- `server/client_scope.go`: every route open to `client_read` has a binder that runs after the role
  check. Findings routes go through the #425 company filter with `StrictCompany`, which ignores
  agent-added `task_scope` rows. Tasks and ids outside the binding answer 404. A route without a binder
  is refused, and `TestClientBindersCoverClientRoutes` enforces that. The test found
  `findings/{id}/traffic` and its body route open to clients without a binder, so both are now bound.
- A client's report counts only its own companies' assets, and its `/api/stats` drops the global
  traffic and asset totals. `findings/retests/active` now needs `read`.
- Tests: `server/client_scope_pg_test.go`, checked by mutation. `go test ./db ./server` with PG passes
  except `TestConnectWithoutPasswordRealPG` (trust auth) and the SOCKS5 UDP test (macOS). verify.sh passes.
- **Migration order:** 0016 (E5), 0018, 0019 (this branch), with E1 holding 0017. The loader refuses a
  pending version older than an applied one, so renumber E1 above 0019 if it lands later.

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

Operator-facing changes:
- Native (non-Linux-root) starts now refuse unless `ARTEX_ALLOW_UNSANDBOXED=1` is set; `dev.sh` sets it.
- A non-loopback `-addr` without TLS is refused. Use `-tls-cert`/`-tls-key` (or
  `ARTEX_TLS_CERT`/`ARTEX_TLS_KEY`), or set `ARTEX_ALLOW_INSECURE_BIND=1`; the Dockerfile sets that env.
- The session is now an HttpOnly, SameSite=Strict cookie (`__Host-` prefixed over TLS), so users log in
  again once.
- The capture proxy verifies upstream TLS; `ARTEX_CAPTURE_INSECURE_UPSTREAM=1` is the opt-out for
  self-signed targets. A non-loopback `-proxy` needs `ARTEX_PROXY_ALLOW_NONLOOPBACK=1`.
- Docker: the artex container has NET_ADMIN, and `egress.sh` denies the executor uid access to metadata,
  the host gateway and the API port (`ARTEX_EGRESS_ALLOW`/`_DENY`/`_FILTER`). GHCR compose needs
  `ARTEX_IMAGE_TAG`.
- Cloud-metadata and link-local addresses are always denied for platform dials, redirects and WebFetch.
  Since 2026-11-23 (branch `ej/20261123-152404-f61d73`, card `c-485de0`) this includes the capture
  proxy's own upstream dials: go-mitmproxy's upstream is a loopback gate (`traffic/upstream_gate.go`)
  that dials with `Interceptor.DialSOCKS5`.
- **2026-11-24** (card `c-5a6f96`, commit `75184c3`): task agents' proxy URLs now carry a task scope token,
  so capture-proxy upstream dials (and SOCKS5 with the token as password) also apply that task's
  block rules and, with its gate on, its pins/approvals; an ended task's token is refused.
  **2026-11-24** (card `c-eb32cc`, commit `ac7c35e`, branch `ej/20261124-075745-5a4987`, which also
  carries the not-yet-merged `c-5a6f96` branch): chat/conversation agents and `use_recording_proxy`
  HTTP tools now get their run's scoped URL (`Manager.ScopedProxyAddr`, `server/proxy_scope.go`);
  conversations have their own token, forgotten on delete; an unloaded task's call gets a refused
  URL. The transparent listener is global-only by decision. **2026-11-24** (card `c-15b39d`, branch
  `ej/20261124-110649-dbd854`, stacked on PR #13's branch): stdio MCP servers, the Playwright browser
  included, are spawned per run with their capture-proxy `--proxy-server`/proxy env rewritten to a
  per-run loopback **scope relay** (`traffic/scope_relay.go`, `Manager.scopeMCPProxy`), so browser
  dials carry the run's scope; listing tools from the UI (no run) still uses the global rules. **2026-11-24**
  (card `c-a1c0fc`, branch `ej/20261124-114610-cae888`, which carries c-15b39d): a relay serves at most 256
  connections at once; a prepareMCP closer is released at once when its MCP fails to start; tests cover
  the hook (PG), two concurrent scopes, the keep-alive host change, wss, an in-flight request across the
  relay's close (tombstone), the cap, and a real headless Chrome (opt-in `ARTEX_TEST_CHROME`). This is on
  main (landed with #22). Re-verified 2026-11-24 12:30: the PG hook test ran against a throwaway Postgres
  (not skipped) and passed, the Chrome test passed, relay tests pass with -race. A full live task is `c-517c2d`. Operators' own clients keep the global rules. go-mitmproxy's keep-alive host pinning is worked around
  (`unpinHostChange`; upstream report `c-9b48b7`). The global-egress-proxy bypass is accepted and
  documented (`docs/OPS.md` §6). `scripts/verify.sh` passes.
  Block rules apply even with the gate off. Fuzzy allow rules match on name and address boundaries.
- Main-agent writes carry origin `mainagent` and are framed as agent-authored. Norma compaction
  summaries are framed as untrusted data.
- Per-run executor HOME; MCP servers run as uid 10002 (`ARTEX_EXEC_MCP_UID`). The workspace file
  manager uses `os.Root`.
- **2026-11-24 — per-task executor uids (card `c-13f293`, opt-in).** `-exec-uid-range LO-HI` (e.g.
  `20000-29999`, passed to `start.sh`/the container command) gives each task and chat session its own
  uid while it is live. Its run dirs are `<uid>:<uid>` 2770. Processes take the identity of the run dir
  they start in: the bash shim, custom tools and file tools all read it from the dir's owner. The uid is
  released (dirs closed, processes reaped, files back to root) on a terminal status, a task delete or a
  conversation delete. `egress.sh` filters the range. Code: `server/execuid.go`,
  `server/execuid_unix.go`. Single-uid mode is still the default.
  Verified: Linux-root tests (`server/execuid_root_linux_test.go`, where task A gets EACCES on
  `tasks/B/i1` and `.artex-home`), the full server and agent suites as root in `golang:1.26.11-bookworm`,
  and egress rules with iptables in a NET_ADMIN container.
  Spill dirs follow the key too (card `c-dd160a`, `server/execuid_spill_unix.go`). A live key's
  `<data>/spill/{tasks,sessions}/<id>` is `root:<uid>` 0750 with files `root:<uid>` 0640, so the
  shared exec gid no longer reads another task's spills. It is regrouped on acquire, closed to
  `root:root` 0700 at the start of release, and closed at start-up when the key holds no uid. Verified
  by `TestExecUIDRangeSpillIsolationAsRoot`, where task A's shell and file tool get EACCES on B's spill,
  plus the full server suite as root in Docker.
  Root tests in Docker: build the test binary into a 0755 dir (`go test -c -o /t/server.test`). The
  default 0700 go-build dir makes the isolation self-tests fail on `main` too.

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

## Triage (next handful)

**Agents' lane**
1. *(unclaimed)* — **SEC: per-task uid follow-ups** (S each): `c-12df03` (single-uid start-up fails on a
   socket; MCP uid not egress-filtered), `c-e70e5c` (release ended tasks at start-up; per-key lock). The "Done when" is on each card.
2. *(unclaimed)* — **E5: web UI for users / authorization / chain / client bindings** (M), card `c-869050`.
3. *(unclaimed)* — **SEC: gorilla/websocket bump, GO-2026-6278** (XS), card `c-c10f2c`; needs network.
   Done = govulncheck clean.
4. *(unclaimed)* — **SEC: upstream-gate residuals** (M), card `c-5a6f96`: task-scoped rules on proxy
   dials, go-mitmproxy keep-alive host pinning, and the global-egress-proxy bypass. Done = a test or a decision for each.
5. *(unclaimed)* — **SEC: logout revocation** (S), card `c-a0af33`. Done = a token replayed after
   logout or a password change gets 401, by a test.
6. *(unclaimed)* — **SEC: Docker Desktop named volumes** (S), card `c-775dc2`. Done = `docker compose up`
   starts on a Mac with the isolation self-test passing; Linux bind-mount path documented.
7. *(unclaimed)* — **E1 Client-grade reporting & deliverables** (M). Turn `report/` into
   CVSS/EPSS-scored, remediation-bearing, standards-mapped deliverables (OWASP ASVS/WSTG, MITRE
   ATT&CK). Done = a report export with per-finding CVSS vector + remediation + framework tags,
   covered by tests. Card `c-8f9cca`.
8. *(unclaimed)* — **E4 Methodology-driven coverage** (M), card `c-c0bb07`. Extend the coverage ledger to score
   coverage against a codified methodology checklist, not only asset touch. Done = planner can
   read a methodology and the coverage graph shows checklist completion.

- *(unclaimed)* — **RSI gate hardening** (M), card `c-0d277f`: server-side per-task arm ledger, suite
  pre-registration, adopted-lesson retire sync. Done = Postgres tests for each.
- *(unclaimed)* — **RSI gate web UI** (M), card `c-936308`. Done = adopt/rollback from a proposals table in ≤2 clicks + password.

**Human lane**
- **First live run through the RSI gate** (card `c-772190`, after `c-f1518c`): propose → held-out
  suite → evidence → adopt (password) → rollback on the bench instance with `ARTEX_RSI_LOOP=on`.
- **E8 first live held-out run** (card `c-f1518c`): needs a bench instance, an LLM, a judge key and a
  drost-benchmarks checkout. Done = a `report.md` with per-gate passes/n for baseline vs builtin.
- Revisit tracking: E1–E10 are on the private Composite board (decided 2026-11-23, repo is public).
  File GitHub issues only if the repo goes private or you want them public.
- Decide engagement-governance direction (E5): how formal the rules-of-engagement/authorization
  artifact should be for your use.
- **SEC: live Playwright scope-relay check** (S), card `c-517c2d`: a real task with capture on and the
  seeded Playwright MCP. Done = the browser's dials show the task scope and an out-of-scope host is refused.
- E5 shipped a lightweight artifact (reference, signatory, scope, window, technique classes); the gate
  is now forced on for tasks with one (c-f4115c). Decide whether the extra admin-only routes in `server/rbac_policy.go` (tools, MCP, agents, LLM config,
  notify) suit how operators work.

- *(unclaimed)* — **E5 UI for the gate requirement and authorization_ended pauses** (S), card `c-01377e`,
  after `c-869050`. Done = no console click hits those 409s unexplained.
- *(unclaimed)* — **Change-password 401 logs the user out** (XS), card `c-5018f6`. Done = a wrong current
  password shows an error and keeps the session.

**Blocked**
- Nothing blocked.

## Set up / run / test / deploy

- **Toolchain:** Go (see `go.mod`), Node (web), Docker + Compose, PostgreSQL. Verified present
  locally: Go 1.27.1, Docker 29.4, Compose v5.1, Node v24.14.
- **Build backend:** `go build ./...` (verified: exit 0).
- **Web deps:** `cd web && npm ci`, then `npx tsc --noEmit` (clean) and `npm run lint` (84 known errors, `c-834a5a`). **Embedded-UI build:** `./build.sh`.
- **Run (Docker):** `cp .env.example .env` (set `POSTGRES_PASSWORD`), `docker compose up -d --build`,
  open http://localhost:8787, setup token via `docker compose logs artex | grep "Setup token"`.
- **Verify (reproduces CI):** `scripts/verify.sh` (fast set, no Docker); `--full` adds `go test -race`
  + web static build; `--security` runs `govulncheck ./...`.
- **Benchmark harness:** `scripts/benchmark` against the subject instance `docker-compose.bench.yml`
  (`cp bench/.env.example bench/.env`, set `ARTEX_PG_PASSWORD` + an LLM key, then
  `docker compose -f docker-compose.bench.yml up --build -d`; `ARTEX_BENCH_PORT=18787` when a production
  instance holds 8787). Builds from the production `Dockerfile`, own Postgres and named volumes,
  `ARTEX_RSI_LESSONS=off`. Details in `scripts/benchmark/README.md` ("Subject instance").
- **Container egress/login check:** after `docker compose up`, run
  `scripts/docker-egress-probe.sh <artex container> [host port] [LAN url]`, then set the admin
  password and run `scripts/docker-login-probe.js` inside the container (usage in its header).
  On Docker Desktop, use named volumes for data/keys/skills (card `c-775dc2`); a separate
  `docker compose -p <name>` project with its own ports keeps a verification stack apart from a
  running one.

## Open work

Expansion items E1–E10 from the plan (see Decisions for the full document). Tracked privately on the
Composite ARTEX board, not as GitHub issues, because the repo is public (decision 2026-11-23).
Card ids in brackets:

- **E1** [`c-8f9cca`] Client-grade reporting & deliverables (scoring, remediation, framework mappings, branded export).
- **E2** [`c-6def05`] Finding/ticketing integrations out (Jira, GitHub issues, DefectDojo) and more ingest in (Nuclei, Nmap, ZAP/Burp, Semgrep).
- **E3** [`c-d25ff9`] — gate built (in review). Follow-ups: hardening `c-0d277f`, UI `c-936308`, first live run `c-772190`, proposer/critic `c-c89f5b`.
- **E4** [`c-c0bb07`] Methodology-driven coverage (OWASP WSTG / API Top 10 as structured checklists).
- **E5** [`c-669406`] Engagement governance. The backend is done on `ej/20261124-055929-bd2253`. Follow-ups:
  `c-5b5421` (client binding), `c-869050` (UI); `c-f4115c` (lifecycle gaps) is in review, follow-ups `c-5018f6`, `c-01377e`.
- **TEST** [`c-3157b4`] `TestGateOffTaskIsUnchanged` fails on main with Postgres (also fails before E5).
- **E6** [`c-cb0093`] Cost/observability dashboard + OpenTelemetry export, building on existing context telemetry.
- **E7** [`c-d60149`] Distributed/remote workers (queue-backed intents) for larger engagements; Helm chart.
- **E8** [`c-5487e8`] Planner/worker evaluation suite (regression targets, prompt-version A/B via the benchmark gate).
- **E9** [`c-73af97`] Frontend/UX pass (tables over cards, fewer-clicks triage workflow, live engagement dashboard).
- **SEC** [`c-517c2d`] Live Playwright task check of the scope relay (human/bench: needs an LLM key and an authorized target).
- **SEC** [`c-3b9252`] Capture-proxy upstream connection reuse across scopes (rules checked only at dial time).
- **SEC** [`c-9b48b7`] Report go-mitmproxy keep-alive host pinning upstream; drop `unpinHostChange` once fixed.
- **SEC** [`c-dadc14`] Verify the security branch on Linux root, in Docker and against Postgres, plus the web typecheck.
- **SEC** [`c-a0af33`] Logout does not revoke the session token (valid until the 7-day JWT TTL).
- **SEC** [`c-775dc2`] Docker Desktop bind mounts defeat executor isolation; stock compose will not start on a Mac.
- [`c-834a5a`] web: 84 pre-existing biome lint errors; `npm run lint` cannot gate CI yet.
- **SEC** [`c-8d54d5`] Capture proxy: insecure upstream only for in-scope targets.
- **SEC** [`c-c10f2c`] gorilla/websocket bump (GO-2026-6278).
- **SEC** [`c-bd9a6d`] Residual gaps from the High/Medium fixes.
- **SEC** [`c-12df03`] Single-uid start-up fails on a socket in a run dir; MCP uid is not egress-filtered.
- **SEC** [`c-e70e5c`] Per-task uids: release ended tasks at start-up; per-key lock.
- **SEC** [`c-cc25b6`] Low-severity audit items.
- **E8 follow-ups** [`c-f1518c`] first live run, [`c-75c593`] bench
  image from a clean clone, [`c-d73308`] optional provider sampling seed per LLM profile.
- **E10** [`c-72dd9d`] Code-security testing lane (SAST/dependency ingest) feeding the same dual-graph + evidence model.

## Known issues and gotchas

- **Migration 0020 (RSI) vs other branches' 0015–0019:** the runner refuses a pending migration older
  than the newest applied one, so merge 0015–0019 (E2, E5, E1, c-ba9f64, c-5b5421) before this branch
  reaches a database, or renumber 0020 forward at merge. `db/migrate_test.go` lists 0020 explicitly.
- The RSI gate trusts the judge verdicts inside uploaded arm records (it checks shape, binding, task
  freshness and uniqueness, not the grading); the owner reads `report.md` before adopting. The generic
  prompt editor stays an owner path outside the gate and is what the bench harness uses to switch arms.

- go-mitmproxy v1.9.2 has no upstream dial hook. Every capture-proxy upstream dial goes through its
  upstream-proxy URL, which is our loopback gate (random credential, `traffic/upstream_gate.go`). Do not
  set `SetUpstreamProxy` back to returning nil: nil means a bare `net.Dialer` as root, with no checks (`c-485de0`).
- A task's proxy scope rides on the proxy URL userinfo (`egress/proxycred.go`). Code that rewrites a
  proxy URL must keep it (`MarkProxyURL` does); prompts and logs show it stripped
  (`egress.WithoutProxyCredential`). The client-connection binding is keyed by remote address, the only
  key go-mitmproxy hands `upstreamFor` (`traffic/scope.go`). A client that cannot send a credential
  (Chromium's `--proxy-server`) gets a scope relay instead (`traffic/scope_relay.go`): the binding is
  the relay's outbound address, stored before any byte is forwarded, and it wins over a credential.
- The scope is checked when the proxy dials upstream, not per request. So `ForgetScope` also closes
  every proxy client connection bound to the token, by credential (`sink.scoped`) or by relay
  (`sink.relayed`), through `sink.clients` (`traffic/scope.go` `closeScope`). That closes the
  connection's open upstream too. A path that forgets a token without `ForgetScope` would bring back the
  pinned-upstream reuse. Test: `traffic/scope_forget_test.go`.
- `server` tests on macOS: `TestManagerSOCKS5UDPOptInAndAssetBlockRuleDropsDatagram` fails because
  127.0.0.2 is not configured on the host (`bind: can't assign requested address`). That's environmental.

- **Guardrails are invariants.** Scope/target gate, interception gate, budgets, noise governor,
  untrusted-output boundary, audit trail — never weaken/bypass/route around them. `RSI.md`
  "immutable surfaces" must never enter an automated search space.
- **Migrations fix forward** — a published `db/migrations/*.sql` is never edited.
- **Audit chain is append-only:** `UPDATE`, `DELETE` and `TRUNCATE` on `audit_chain` fail, and artifact
  rows reject `UPDATE`. A test database that gets polluted has to be recreated, not cleaned. Verify only
  catches a full consistent rewrite by a DB superuser if `head_hash` is compared with a copy kept
  outside the database.
- **Embedded UI** requires `-tags embedui`; a plain `go build` omits the web assets.
- **Root tests on Linux:** run them as root in a container, e.g. `docker run --rm --cap-add NET_ADMIN
  -v "$PWD":/src -w /src -e ARTEX_PG_DSN=postgres://… golang:1.26.11-bookworm go test ./...`. Without
  root, the `*AsRoot` tests skip; without `ARTEX_PG_DSN`, the PG-backed tests skip (macOS runs miss both).
- **Docker Desktop (Mac):** bind-mounted `data/`, `keys/` and `skills/` are writable by any uid in the
  container, so the isolation self-test refuses to start. Use named volumes (`c-775dc2`).
- **Egress probes:** as the executor, `/app/artex __artex-exec-drop 10001 10001 -- curl …`; rule hits
  show up in `iptables -L ARTEX_EGRESS -v -n -x`.
- **Secrets** are AES-256-GCM sealed with per-column AAD; losing the key (`keys/secret.key` or
  `ARTEX_SECRET_KEY`) loses stored secrets.

## Decisions

- **2026-11-24** — E5 lifecycle (c-f4115c): an artifact *forces* the target gate (refuse turning it off,
  409) rather than recommending it, because with the gate off the scope sees only declared targets and
  platform dials. Expiry/revocation pauses the task (unconditionally, no setting) and blocks resume until
  a new version: pausing is reversible and the gate already refuses every target call. A window that
  has not started does not pause. Password self-service extends the existing `/api/auth/change-password`
  instead of adding `/api/users/me/password`: the UI already calls it and it is already rate-limited.
  Archives keep the artifact rows with the stub (gate still applies on restore; no DELETE audit noise)
  and carry them in the package; restore re-creates missing versions under its own audit actor using
  `SET CONSTRAINTS trg_engagement_authorizations_audit IMMEDIATE` for that one insert (0018 reads
  `artex.actor` at commit, so a transaction-wide actor would relabel the restored intercept rows).
  Caches: one generic `memo` package (TTL + race-safe invalidation), 2 s, invalidated by every write
  through `db`; the first auth check of a request stays uncached.
- **2026-11-24** — E3 (c-d25ff9): "one explicit owner action" is an owner-password step-up on adopt,
  because on single-user ARTEX a bearer token minted from `jwt.key` (what the loop holds) is the same
  identity as the owner. The server recomputes every verdict from raw records rather than accepting the
  harness's comparison (no self-approval). Evidence is bound to the live prompt at adoption (baseline
  sha) so a stale comparison cannot be adopted. Rollback and reject stay open with the flag off; only
  propose/evidence/adopt need it. Skills and tool descriptions stay git-PR-only. Lesson graduation uses
  the same gate via a scope-free benchmark pin (`ARTEX_RSI_LESSONS=bench`), and the new `adopted` mode
  injects only graduated lessons; `inject` is unchanged. Budget 4·0.6^round (floor 1), rounds forward-only
  one step at a time, ≤4 open proposals. Reject and rollback need only a session (they return toward the
  owner-approved prior state; open question in `c-0d277f`). Failing uploads also consume their tasks in
  the ledger, so a run never backs two uploads.

- **2026-11-24** — c-75c593: the bench subject instance builds from the production `Dockerfile` instead
  of a separate `Dockerfile.bench`, so the benchmark measures the shipped image and there is one image
  definition. Postgres is a sibling service (not baked in), data/keys are named volumes (Docker Desktop
  bind mounts fail the isolation self-test), and it is a bridge network with `127.0.0.1` ports instead of
  the old `network_mode: host` (no port clashes, works on Docker Desktop). Host-published corpora are
  reached as `host.docker.internal`; the egress filter still denies that to agents until the operator
  sets `ARTEX_EGRESS_ALLOW` in `bench/.env` — documented, not defaulted, since it opens host services.
- **2026-11-23** — c-dadc14: the security branch's always-on metadata check resolves each name the
  platform dials (gate on or off); `TestGateOffTaskIsUnchanged` now asserts one lookup per platform
  connection instead of none, since this is the intended behaviour, not a regression. The isolation
  probe binary became a test hook (`isolationProbeBinary`) rather than being changed in production.
  Logout revocation and the Docker Desktop bind-mount problem were filed as cards, not fixed here: both
  predate the branch and need a design choice.
- **2026-11-24** — DVGA ground truth bumped to v2 after reading the 2.2.0 source (card `c-085c80`). Every entry
  now has a `source` field naming its resolver, helper, middleware or template (the judge prompt ignores it;
  it only changes the content hash). Changes: GT-009 is *signature not verified* (`helpers.get_identity`,
  `verify_signature: False`), not alg=none in particular. GT-011 is now the GraphiQL `env` cookie bypass,
  because the deny-list, operation-name and introspection middlewares only run in Expert mode (moved to
  notes). Added GT-012, stack traces on `/graphiql` (`format_custom_error`). The notes now say that
  difficulty is server-wide DB state, switched by `X-DVGA-MODE` or `/difficulty/<level>`, not chosen per request.

- **2026-11-23** — Stood ARTEX up locally and registered it as a Composite project rather than
  forking/renaming; it stays an AGPL-3.0 fork under `tenpointdata`. Backend verified building; web
  install and Docker run deferred (not needed to produce the setup + plan deliverable).
- **2026-11-23** — Expansion plan produced as a Composite Office doc (link recorded by
  `merge_handoff` / in the Composite project). All proposed work is guardrail-preserving; no item
  touches the `RSI.md` immutable surfaces, and Loop-C work (E3) is gated behind that contract.
- **2026-11-23** — E1–E10 tracked on the private Composite ARTEX board instead of GitHub issues,
  because `tenpointdata/ARTEX` is public and the roadmap should not be published yet.

- **2026-11-23** — Security fixes fail closed with explicit env opt-outs (`ARTEX_ALLOW_UNSANDBOXED`,
  `ARTEX_ALLOW_INSECURE_BIND`, `ARTEX_CAPTURE_INSECURE_UPSTREAM`, `ARTEX_PROXY_ALLOW_NONLOOPBACK`) rather
  than silent degradation. The metadata deny is not applied to Bash command text, because SSRF payloads
  legitimately name 169.254.169.254; it is enforced at the dial level instead. Block rules keep substring
  matching, since matching more is the safe direction; only allow rules became strict. Security
  follow-ups are private board cards, not GitHub issues, because the repo is public.
- **2026-11-23** — The capture proxy keeps verifying upstream TLS by default. Engagements against
  self-signed targets set `ARTEX_CAPTURE_INSECURE_UPSTREAM=1` for that run only, until per-target trust
  (card `c-8d54d5`) exists. Reason: the goproxy setting is global, so turning it on also leaves package
  downloads through the proxy open to tampering.

- **2026-11-24** — Per-task uids derive a process's identity from the **owner of its working run
  directory**, not from a channel between the server and the shim. Norma always starts bash in the
  server-chosen `tc.WorkingDir`, and only root sets that owner. A task's gid is the same number as its
  uid, and the exec gid stays a supplementary group (script dirs; spill dirs are grouped to the task
  uid instead, `c-dd160a`). As a result MCP servers no
  longer write into run dirs when the range is on. Release kills the uid's processes and gives its files
  back to root, so a uid returned to the pool owns nothing. The mode stays opt-in until it has run in a
  real deployment.

- **2026-11-24** — Capture-proxy task scope (`c-5a6f96`): carried as a per-task token in the proxy URL
  userinfo, because a proxy client can only identify itself through its credential and every tool
  already reads `HTTP_PROXY`. The token only *adds* a task's rules, so it is not treated as a secret
  and a missing token falls back to the global rules (the floor), while an unknown token fails closed.
  Proxy dials never wait for approval (an unpinned host opens a request and is refused). The
  global-egress-proxy path is **accepted**, not routed through the gate: the operator's proxy resolves
  and reaches the target, so the gate could only make a name-only check. OPS.md tells operators not to
  run that proxy where it can reach metadata. Keep-alive host pinning is fixed locally with
  go-mitmproxy's separate client rather than closing the client connection, so clients see no resets.

- **2026-11-24** — Remaining proxy clients (`c-eb32cc`): shared clients (the per-profile chat agent,
  the http custom tool) resolve the scoped URL per call from the run's `intercept.TargetScope` on the
  context, rather than being rebuilt per run. A task scope whose task is not loaded gets a URL with
  an unregistered token (refused), never the global-only URL, and `TaskProxyAddr` no longer mints
  tokens for unloaded tasks. The transparent TCP listener stays global-only: it has no credential,
  and per-run uid/cgroup mapping is operator netfilter setup; documented in `docs/OPS.md` §6.

- **2026-11-24** — Browser MCP scope (`c-15b39d`): the card assumed one shared browser process; in
  fact `wireAgentAugment` spawns every stdio MCP per run (worker run, planner round, main-agent and
  chat turn) on a context that already carries the run's `TargetScope`. So the fix rewrites the spawn,
  not the process model: any stdio MCP's `--proxy-server` or `HTTP(S)_PROXY`/`ALL_PROXY` naming the
  recording proxy is replaced by a per-run loopback relay. A relay, not the userinfo, because
  Chromium drops it from `--proxy-server` and only answers a 407 the proxy never sends. A relay
  that cannot start skips the server for the run (fail closed) rather than spawning on the plain
  URL. UI tool discovery (no run) keeps the stored URL, global rules: it lists tools and browses
  nothing. This branch merges PR #13's branch (`ej/20261124-075745-5a4987`), which this work needs.
- **2026-11-24** — E8: the A/B suite runs one task at a time on a dedicated instance and refuses a busy one,
  because the server reads the active prompt live and a swap would reach a concurrent task. `--seed` fixes the
  schedule, not the LLM, because ARTEX has no sampling-seed control; replicates control variance instead
  (`c-d73308`). Gate rates are counted over ground-truth entries, not over detected findings, so arms with
  different detection counts stay comparable. A failed run is counted but left out of the denominators. A judge
  failure aborts the suite. Arms may vary only the editable goals/planner/worker body; the `RSI.md` immutable
  surfaces stay outside the prompt API by construction.

- **2026-11-24** — E5: the engagement is the task. The artifact is enforced only by the existing target
  gate, so there is one enforcement path, and refusals are decided intercept rows. Names are never
  resolved for the scope match, so a name must be listed by name; this fails closed. The technique
  classifier is a declared check driven by a data table, not a sandbox; unclassified calls are not
  limited. The audit chain is fed by DB row triggers (atomic with every writer, present and future),
  and the hashed payload is stored as text so the bytes are hashed exactly. Users' roles are read from
  the DB on every request, not from the token, so revocation is immediate.

- **2026-11-24** — Audit head-lock contention (`c-ba9f64`): fixed with deferred constraint triggers
  (migration 0018) rather than reordering the archive's deletes (which fixes one caller only) or an
  unchained staging table plus a sequencer (which leaves a window where committed writes are not yet
  chained, and adds a worker). Entries stay atomic with their writes; the head becomes the last lock a
  transaction takes. Accepted costs: chain order is commit order, and the actor is `artex.actor` at commit.

- **2026-11-24** — Client binding (`c-5b5421`): enforced in one middleware step with a declarative
  per-route binder table that fails closed, rather than in scattered handler checks, so a new client
  route cannot leak by omission. Findings reuse the #425 company filter. Task ownership counts only
  operator-made company scope rows, because agent output is untrusted. A multi-company task is fully
  visible to each bound client (accepted). Bindings apply to `client_read` only.

## Deltas

<!-- composite:deltas · newest first; Composite rewrites this block on every merge (ej handoff merge / merge_handoff) -->
- **2026-11-24 11:57 MST** · claude-code · `ej/20261124-114610-cae888` `11d040e`→`b6c05ad` (1 commit) · `h-c36681` — c-a1c0fc (now in review; board cards opened: c-5d033e, c-517c2d): scope-relay tests and limits, on a branch that merges c-15b39d's unmerged branch (ej/20261124-110649-dbd854). wireAgentAugment now releases a prepareMCP closer at once when its MCP fails to connect or list tools; before, it was held until the run ended. A PG test of the hook uses the test binary as a fake stdio MCP. It checks that a hook error skips the server, that the spawned server is the prepared one, and that cleanup releases its closer. ScopeRelay caps client connections at 256 at once. New relay tests cover two concurrent scopes, the keep-alive host change, wss, a deterministic tombstone (a request is held after it is read while the relay closes), and the cap. An opt-in real headless Chrome test (ARTEX_TEST_CHROME) passes: through the relay, dials run on the scope, and a forgotten scope is refused. The tombstone, cap and early-close tests were mutation-checked. verify.sh passes. traffic passes with -race. In server, only TestManagerSOCKS5UDPOptIn… fails, because macOS has no 127.0.0.2. **Next:** c-5d033e: ForgetScope should close the scope's client connections; today an already-open upstream stays usable (dial-time check). c-517c2d (human/bench): a full live task with the Playwright MCP and capture on. Land PR #13, then c-15b39d, then this branch (this branch carries both). **Decided:** An over-cap relay connection is closed, not queued: it fails closed and Chromium retries. 256 matches Chromium's per-profile socket limit. When an MCP's tools/list fails, its client and relay are closed at once. The live check was done with a real Chrome against a relay in a test, not as a full task. A full task needs an image build, an LLM key and an authorized target, so it is on card c-517c2d. The forgotten-scope keep-alive reuse was not fixed here: it predates this work, is not specific to the relay (relays close their connections at run end), and is on card c-5d033e.
- **2026-11-24 11:53 MST** · claude-code · `ej/20261124-113533-f16c00` `382e26f`→`bea8d58` (2 commits) · `h-356fb9` — c-b73542 (now in review): agent processes are refused cloud metadata and link-local addresses at the dial level on any Linux root start, not only in Docker. The server installs its own iptables chain ARTEX_AGENT_FLOOR covering the exec uid, the MCP uid and -exec-uid-range. It refuses intercept.MetadataPrefixes by -d and by conntrack --ctorigdst; DNS and ARTEX_EGRESS_ALLOW pass. A new product-neutral ownerfw package builds the rules. The isolation self-test dials the metadata endpoints as the executor, and a connection stops the start unless ARTEX_ALLOW_METADATA_EGRESS=1. Verified as root with NET_ADMIN in golang:1.26.11-bookworm. Against a real listener on 169.254.169.254, curl as uid 10001 succeeds before the floor and fails after it, the MCP uid fails too and root succeeds; a DNAT of 169.254.170.2 to loopback is refused. Real-binary starts behave the same way: installed natively, refused with the filter off while metadata answers, and allowed with the opt-out. scripts/verify.sh passes. **Next:** c-02e554: re-check the floor while running, rebuild it atomically, remove stale jumps. Also run the full Docker image with egress.sh plus the floor (scripts/docker-egress-probe.sh), including a no-NET_ADMIN run, to confirm the new refuse-to-start message. **Decided:** The floor is a separate chain and egress.sh was left untouched, so its Docker-only gateway and API-port rules are never lost. The floor is checked by a dial from the executor rather than by reading the rules. A start is refused only when a metadata address answers. DNS passes because native resolvers can be link-local. A bad ARTEX_EGRESS_ALLOW entry is skipped with a warning instead of dropping the floor. Work is tracked on Composite cards rather than GitHub issues (the decision of 2026-11-23).
- **2026-11-24 11:51 MST** · claude-code · `ej/20261124-114608-2455e2` at `c09f6e4`, no commits · `h-7774a6` — c-12df03 (now in review): the single-uid start-up share walk (shareWithExecGroup) now logs and skips an entry it cannot open, such as a stale ssh ControlMaster socket, instead of failing start-up. It never chowns by name in share mode. New test TestShareWithExecGroupSkipsSocket fails without the fix. egress.sh adds an OUTPUT jump for the MCP uid (ARTEX_EXEC_MCP_UID, default exec uid + 1, read the way mcpUID() reads it). Checked with iptables -S in a NET_ADMIN container: default, env + range, idempotent rerun, and a bad value that fails closed. scripts/verify.sh passes. Commit c09f6e4. **Next:** When branch ej/20261124-092031-1d05de (5be8c4b, c-bd9a6d) merges, keep main's egress.sh: that commit adds a looser MCP-uid rule. Not run: the Linux root suite in Docker for this change (the new test runs without root). h-356fb9 (c-b73542) works on related agent-uid dial denies; its uid set should include the MCP uid. **Decided:** Share mode skips an unopenable entry rather than chowning it by name. A by-name group change could be redirected by a swap to a root-only file, and in single-uid mode the entry keeping its executor owner is harmless. An unreadable ARTEX_EXEC_MCP_UID makes egress.sh fail closed, like an unreadable range.
- **2026-11-24 11:39 MST** · claude-code · `ej/20261124-113524-71e978` `382e26f`→`7a51f4f` (1 commit) · `h-d512ac` — c-75c593 (now in review): docker-compose.bench.yml builds from a clean clone. It uses the production Dockerfile instead of the gitignored Dockerfile.bench, runs its own Postgres service and named volumes (compose project artex-bench), and publishes 127.0.0.1 ports that ARTEX_BENCH_PORT / ARTEX_BENCH_PROXY_PORT can override. bench/.env.example is committed: no secrets, ARTEX_RSI_LESSONS=off, one ARTEX_PG_PASSWORD for both services, and a commented ARTEX_EGRESS_ALLOW for corpora published on the host. Verified on Docker Desktop arm64: up --build is healthy, the isolation self-test passes, the egress filter is installed, the UI returns 200, and jwt.key copies out. scripts/benchmark/README.md has a new Subject instance section. **Next:** c-f1518c (h-9652aa) can use this instance for the first live held-out run. With a corpus on the host, use --target-host host.docker.internal and set ARTEX_EGRESS_ALLOW to the IP it resolves to inside the container. The -proxy listener logs 127.0.0.1:8788 inside the container, so the published 8788 port is likely unreachable from the host. The same is true of the production compose; it was not investigated. **Decided:** Bench builds the production Dockerfile (measure the shipped image, one image definition). It uses a sibling Postgres instead of a baked-in one. It uses a bridge network and 127.0.0.1 ports instead of network_mode: host, which avoids port clashes and works on Docker Desktop. Named volumes replace bind mounts because of c-775dc2. The host-gateway egress exception is documented but not defaulted, because it opens host services to agents.
- _Older hand-offs, a line each; `ej handoff log` has them whole:_
- **2026-11-24 11:23 MST** · claude-code · `h-6a42d5` — c-5b5421 (now in review): client_read users are bound to companies. **Next:** Land PR #16 (E5) first, then this branch.
- **2026-11-24 11:22 MST** · claude-code · `h-d87101` — c-15b39d (board card, now in review; follow-up card c-a1c0fc opened): the Playwright browser MCP, and any stdio MCP whose --proxy-server or HTTP(S)_PROXY / ALL_PROXY / PLAYWRIGHT_MCP_PROXY_SERVER names the recording proxy, now dials under the calling run's sc… **Next:** c-a1c0fc: PG-backed test of the prepareMCP hook (skip on error, closer released), concurrent/keep-alive/WebSocket relay tests, a per-relay connection cap, and a live Playwright check with capture on.
- **2026-11-24 11:13 MST** · claude-code · `h-8521a8` — c-3b9252 closed by a test and a decision, with no runtime code change: go-mitmproxy v1.9.2 never shares upstream connections across client connections in ARTEX. **Next:** c-5a6f96: when adding task-scoped rules on proxy dials, return a distinct gate URL/credential per scope from upstreamFor (identifies the scope at the gate and keys go-mitmproxy's separate-client pool…
- **2026-11-24 11:03 MST** · claude-code · `h-721d29` — c-a6891b (card, now in review): built the full Docker image (bbot, chromium, Playwright) from 1abe24f with the committed Dockerfile on Docker Desktop arm64 in 34 min. **Next:** Re-run both probe scripts after c-a0af33 (logout revocation) merges: the old-cookie replay should become 401.
- **2026-11-24 09:41 MST** · claude-code · `h-691201` — c-ba9f64 done (board card): migration 0018 recreates the intercept_pending, artex_users and engagement_authorizations audit triggers as DEFERRABLE INITIALLY DEFERRED constraint triggers, so audit_chain_head is locked only at COMMIT. **Next:** Merge order: E5 (0016), then this branch (0018), then E1 (0017 on ej/20261124-083744-02fa3c).
- **2026-11-24 09:33 MST** · claude-code · `h-e8d3e5` — Card c-bd9a6d: every residual gap is now either fixed or recorded as accepted in SECURITY.md (commit 5be8c4b).
- **2026-11-24 08:48 MST** · claude-code · `h-2396f3` — E8 (c-5487e8, now in review): added the planner/worker evaluation suite in scripts/benchmark. **Next:** c-f1518c (human): first live held-out run on a bench instance with ARTEX_RSI_LESSONS=off; check that DVGA is reachable through the port mapping.
- **2026-11-24 08:07 MST** · claude-code · `h-c42a5d` — c-eb32cc done (board cards: closed c-eb32cc; opened c-15b39d, c-3b9252). **Next:** c-3b9252: check whether go-mitmproxy reuses upstream connections across scopes.
- **2026-11-24 07:59 MST** · claude-code · `h-6bba55` — Board card c-f89815 is fixed. **Next:** Reconcile TestGateOffTaskIsUnchanged with the c-3157b4 fix when it lands: the http tool now makes one dial lookup with the gate off.
- **2026-11-24 06:49 MST** · claude-code · `h-7dc82c` — E5 engagement governance backend. **Next:** c-ba9f64 (audit head-lock contention), c-5b5421 (bind client_read to a client), c-869050 (web UI incl. **Issues:** opened #c-3157b4, #c-5b5421, #c-869050, #c-ba9f64, #c-f4115c.
- **2026-11-24 06:21 MST** · claude-code · `h-9348ac` — E2 backend landed (card c-6def05 → review): package ticketing (GitHub/Jira/DefectDojo over notify.Do with masking + SSRF guard + stable dedup key), two-way finding↔ticket status sync, package scanimport (Nmap, Nuclei, ZAP, Burp, SARIF) and an import API that… **Next:** c-597eb6: web UI for trackers and imports.
- **2026-11-24 06:17 MST** · claude-code · `h-537f2b` — c-8d54d5 done: the capture proxy verifies upstream TLS per host. **Next:** Run `ARTEX_PG_DSN=… go test ./db -run ActiveTaskScopeRulesPG`.
- **2026-11-24 05:18 MST** · claude-code · `h-5a3839` — c-5a6f96 is done; follow-up cards c-eb32cc and c-9b48b7 are open. **Next:** c-eb32cc: scope the chat and conversation agents, the custom HTTP tools and the transparent listener.
- **2026-11-23 21:08 MST** · claude-code · `h-43cc9e` — c-775dc2: docker-compose.yml and docker-compose.ghcr.yml now keep /app/data, /app/keys and /app/skills in named volumes, so the stock `docker compose up` starts on Docker Desktop with the isolation self-test passing. **Next:** c-965b0e: Docker's `restart: unless-stopped` still restarts the container after an exit 78.
- **2026-11-23 21:07 MST** · claude-code · `h-ee4b2b` — c-a0af33: logout now revokes the session server-side. **Next:** Re-run `ARTEX_PG_DSN=… go test ./server` once Docker is healthy. **Issues:** opened #c-8dea4d.
<!-- /composite:deltas -->

_(written by `merge_handoff`)_

## Updating this document

Keep the sections above in the standard form. Refresh **Current state** (dated stamp) and **Triage**
at the end of any session that changed code/state/config/plans; add or close GitHub issues and record
**Decisions** with their reason. `merge_handoff` writes the Deltas block and the one-line stamp — do
not hand-maintain a history chain in Current state.
