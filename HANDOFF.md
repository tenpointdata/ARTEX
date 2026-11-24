# HANDOFF.md — ARTEX

**Last updated:** 2026-11-24 14:52 MST, by claude-code: c-13e86c fixed: the activity stream 401 after a console login predates E5. Earlier hand-offs are under Deltas and in `ej handoff log`.

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

**2026-11-24 — E7 distributed workers + Helm (`c-d60149`, in review)** on `ej/20261124-150525-1a95b3`; design
and limits in `docs/DISTRIBUTED.md`. `-role all|control|worker` (`server/node.go`): worker nodes are the same
binary and `server.New` wiring (guard, intercept rules, LLM reviewer, target gate, budgets, isolation,
egress filter), run only intent worker loops (`server/worker_node.go`), and serve only `GET /healthz`. They
claim through the existing fenced lease. Control state moved to Postgres (migration **0023**: `worker_nodes`,
`work_signals`, `tasks.dispatch_closed`, `proxy_scope_tokens`): pause, queued, terminal, deadline, the
settle/delete fence and target gate are read per claim or call (fail closed); budgets are published by the
control node and spend is synced every 2 s; intercept rules follow a DB revision and ask approvals poll
their row; kill/pause/cancel/steer go through `work_signals`; the noise governor is split across live nodes.
Worker traffic goes through the control node's recording proxy with DB-resolved per-task tokens (token-less
remote clients refused; CA cert published). `GET /api/settings/nodes` lists nodes. Helm chart
`deploy/helm/artex` (control + worker Deployment with hard host anti-affinity) and
`docker-compose.distributed.yml`. Verified against Postgres 17: two worker nodes ran one task's intents, each
blocked by a rule added on the control node after they started and seeing the control's gate switch
(`TestTwoWorkerNodesShareOneTaskWithIdenticalGating`); kill/pause/no-budget/no-control
(`TestWorkerNodeFollowsControlState`); proxy path recorded on the control node, forged and deleted tokens
refused (`TestWorkerNodeUsesControlRecordingProxy`); `helm lint`/`template` clean. An opus adversarial
review found 12 issues; H1–M9, L10, L11 fixed (`c0210dd5`). **Not verified:** a real install on two hosts
(`c-8cfbfe`). **Gaps:** worker findings cannot bind captured traffic (`c-3b781a`), transcripts are per host
(`c-29ff19`), smaller items `c-18f7a8`.

**2026-11-24 — SSE 401 after console login (`c-13e86c`, fixed; pre-E5).** Streams are same-origin only
(`web/src/lib/sse.ts`), dev gzip off, `openSSE` writes `: open`. Test `server/activity_stream_auth_pg_test.go`;
browser-checked on the E5 stack.

**2026-11-24 (c-ca2bd0, E4 follow-up)** — An operator can choose a task's methodologies. Migration
**0022** adds `tasks.methodologies TEXT[]` (empty `{}` = automatic, as before). The create request
(`methodologies`, ids matched loosely, stored canonical, unknown id = 400) and the create form
(`methodology-picker.tsx`, a table fed by the new `GET /api/methodologies`) set it; `db.TaskMethodologies`
uses it when set, so the coverage API and the planner block show only the chosen checklists. It is
carried through archive/restore and echoed on the task DTO. This branch merges the unmerged E4 branch
(`ej/20261124-123222-d3dc8a`). Verified against Postgres 16: `TestTaskMethodologiesOperatorChoice`,
the archive round-trip, `TestCreateTaskMethodologies` (server), the full `go test -p 1 ./...` (only the
known macOS 127.0.0.2 SOCKS5 test fails), web `tsc` and i18n. Not checked: the form in a browser.
Gaps carded as `c-aed5fd`: no "none" choice, and the choice is fixed at creation.

**2026-11-24 — client_read console (`c-7c4715`, in review)** on `ej/20261124-144554-67afc9`. New
`GET /api/me/companies` and `GET /api/me/tasks?company=` (`server/client_console.go`, binders
`bindBound`/`bindCompany`), both `client_read`. Web: `ClientCompanyGate` (`web/src/app/(main)/_components/`)
loads the bound companies and holds pages until one is active; the sidebar switcher in bound mode offers
only those; new **Reports** page `/function/reports` (client landing; hidden for `read` holders via the
nav item's new `hideWith`) lists the company's engagements in a table with the selected one's outcome
and Markdown report. Verified: `TestClientReadBoundToCompanyPG` (now also a client bound to two
companies reading each one's tasks, findings, report, stats and outcome with no refusal), the RBAC route
tests, web `tsc`, governance unit tests, i18n and `scripts/verify.sh` (only the two known env failures).
Browser-checked in headless Chromium against a real backend + Postgres 17: a client bound to two
companies lands on Reports, sees only Reports + Findings and a switcher with just its two companies,
switches between them and reads each one's findings and report; all 51 `/api` responses were 200.

**2026-11-24 — E5 web UI (`c-869050`, in review)** on `ej/20261124-132958-94f4bf` (carries E5 + client_read).
`/system/users`, task **Authorization** tab, Approval Records **Chain** tab with Verify; sidebar filtered by
`useCan` (`web/src/hooks/use-session.tsx`); 403 never logs out (`web/src/lib/api-errors.ts`); login takes any
username. Browser-tested as admin, reviewer and client_read against a real backend. Open: `c-92bb52`,
`c-7c4715`, `c-601ec6`.

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

**2026-11-24 (c-c0bb07, E4)** — Methodology-driven coverage: OWASP WSTG 4.2 (97 tests) and the OWASP
API Security Top 10 2023 are typed checklists in `methodology/` (data in `methodology/data/*.json`,
embedded and validated). A task's applicable checklists come from its goal and description (no
column). `db/coverage_methodology.go` scores them from the coverage ledger, findings (vulnclass)
and intents citing an item (`payload.methodology_item`). The planner gets a `[Methodology coverage]`
block each round, and `add_intent` takes `methodology_item`, which is validated and stored as the
canonical item id. `GET /api/tasks/{id}/coverage` returns `methodologies`, and the task overview's
coverage card shows a filterable table of the items. Verified by the methodology, agent and
db unit tests, a Postgres integration test (`TestMethodologyCoverage`), `tsc` and `scripts/verify.sh`.
The UI was not checked in a browser.

**2026-11-24 (c-75c593)** — The benchmark subject instance (`docker-compose.bench.yml`) now builds from a
clean clone: production `Dockerfile`, a sibling Postgres, named volumes, committed `bench/.env.example`
with `ARTEX_RSI_LESSONS=off`. Verified on Docker Desktop arm64: `up --build` brings both containers up,
isolation self-test passes, egress filter installed, UI 200, `ARTEX_RSI_LESSONS=off` in the container.
No benchmark run was made against it.

**2026-11-24 — E5 engagement governance (card `c-669406`)** is on `main` (#31); see `docs/GOVERNANCE.md`.
Roles `admin`/`operator`/`reviewer`/`client_read` over the JWT (`artex_users`, `/api/users`, policy in
`server/rbac_policy.go`, enforced in `requireAuth`; `ARTEX` is the break-glass admin). The per-task
authorization artifact is enforced only inside the target gate (`intercept/authorization.go`). `audit_chain`
is hash-chained by deferred row triggers (migrations 0016, 0018); `/api/intercept/history/chain[/verify]`.
With `ARTEX_PG_DSN`, `go test ./...` fails only `TestGateOffTaskIsUnchanged` (`c-3157b4`),
`TestConnectWithoutPasswordRealPG` (trust auth) and the macOS SOCKS5 UDP test.

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

## Triage (next handful)

**Agents' lane**
1. *(unclaimed)* — **SEC: per-task uid follow-ups** (S each): `c-12df03` (single-uid start-up fails on a
   socket; MCP uid not egress-filtered), `c-e70e5c` (release ended tasks at start-up; per-key lock). The "Done when" is on each card.
2. *(unclaimed)* — **E5 UI: hide role-lacking actions on existing pages** (M), card `c-92bb52`. Done = as
   reviewer and operator, no visible control answers 403. Then `c-601ec6` (chain newest-first paging, S).
   (`c-7c4715`, the client_read console, is built and in review on `ej/20261124-144554-67afc9`.)
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

- *(unclaimed)* — **RSI gate hardening** (M), card `c-0d277f`: server-side per-task arm ledger, suite
  pre-registration, adopted-lesson retire sync. Done = Postgres tests for each.
- *(unclaimed)* — **RSI gate web UI** (M), card `c-936308`. Done = adopt/rollback from a proposals table in ≤2 clicks + password.

- *(unclaimed)* — **E7: evidence and traffic tools on worker nodes** (M), card `c-3b781a`. Done = a
  finding recorded on a worker node binds its captured exchange, by a two-manager test.

**Human lane**
- **E7 live install on two hosts** (card `c-8cfbfe`): `helm install` on a ≥2-node cluster, a task whose
  intents run on both worker pods, capability set confirmed, worker egress deny of Postgres/control API.
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
- E5 shipped a lightweight artifact (reference, signatory, scope, window, technique classes). Decide
  whether tasks with an artifact must have the target gate forced on (card `c-f4115c` item 2), and
  whether the extra admin-only routes in `server/rbac_policy.go` (tools, MCP, agents, LLM config,
  notify) suit how operators work.

**Blocked**
- Nothing blocked.

## Set up / run / test / deploy

- **Toolchain:** Go (see `go.mod`), Node (web), Docker + Compose, PostgreSQL. Verified present
  locally: Go 1.27.1, Docker 29.4, Compose v5.1, Node v24.14.
- **Build backend:** `go build ./...` (verified: exit 0).
- **Web deps:** `cd web && npm ci`, then `npx tsc --noEmit` (clean) and `npm run lint` (84 known errors, `c-834a5a`). **Embedded-UI build:** `./build.sh`.
- **Run (Docker):** `cp .env.example .env` (set `POSTGRES_PASSWORD`), `docker compose up -d --build`,
  open http://localhost:8787, setup token via `docker compose logs artex | grep "Setup token"`.
- **Distributed (E7):** one `-role control` node plus `-role worker` nodes sharing Postgres and
  `ARTEX_SECRET_KEY`; workers need `ARTEX_CONTROL_PROXY`. Helm: `deploy/helm/artex` (lint with
  `docker run --rm -v "$PWD/deploy/helm/artex:/chart" alpine/helm:3 lint /chart`); local: `docker compose -f
  docker-compose.yml -f docker-compose.distributed.yml up -d --build`. See `docs/DISTRIBUTED.md`.
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
- **E4** [`c-c0bb07`] Methodology-driven coverage: landed 2026-11-24 (see Current state). Follow-ups:
  [`c-ca2bd0`] an operator-chosen methodology per task (landed, in review; gaps `c-aed5fd`); [`c-ed937d`] checklist completion in the report export (after E1).
- **E5** [`c-669406`] Engagement governance. The backend is done on `ej/20261124-055929-bd2253`. Follow-ups:
  `c-5b5421` (client binding), `c-869050` (UI, in review), `c-f4115c` (lifecycle gaps), `c-92bb52`
  (role-aware controls on existing pages), `c-7c4715` (client_read console, in review), `c-601ec6` (chain paging).
- **TEST** [`c-3157b4`] `TestGateOffTaskIsUnchanged` fails on main with Postgres (also fails before E5).
- **E6** [`c-cb0093`] Cost/observability dashboard + OpenTelemetry export, building on existing context telemetry.
- **E7** [`c-d60149`] Distributed workers + Helm: built, in review (see Current state). Follow-ups:
  `c-3b781a` (evidence on workers), `c-29ff19` (transcripts across hosts), `c-8cfbfe` (live two-host
  install), `c-18f7a8` (browser MCP relay, finding wake-ups, proxy TLS, smaller gaps).
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
- **SEC** [`c-d689e2`] First GitHub run of the c-cc25b6 SHA-pinned workflows — **blocked until branch
  `ej/20261124-140001-18dcf1` is merged to main and pushed** (it is in neither as of 2026-11-24 14:30).
  Pre-checked offline 2026-11-24: all 15 pinned SHAs resolve to their tagged commits (gh api). Before the
  `release.yml` workflow_dispatch dry run, gate `actions/attest-build-provenance` on
  `github.event_name == 'push'`: the repo is public, so a dry run would otherwise write a repo attestation
  and a public Sigstore log entry. Dependabot `github-actions` PRs start only once the new `dependabot.yml`
  is on main (origin/main's has no `github-actions` entry).
- **E8 follow-ups** [`c-f1518c`] first live run, [`c-75c593`] bench
  image from a clean clone, [`c-d73308`] optional provider sampling seed per LLM profile.
- **E10** [`c-72dd9d`] Code-security testing lane (SAST/dependency ingest) feeding the same dual-graph + evidence model.

## Known issues and gotchas

- **`next dev` writes `web/AGENTS.md`** (Next 16 agent rules) on start. Delete it before committing; it
  is not this repo's file.
- **SSE in dev goes through the Next rewrite** (`/api/*` → `AUTOPENTEST_API`, default :8787). Keep
  `compress: false` in the dev branch of `web/next.config.mjs` and the `: open` comment in `openSSE`, or
  streams buffer or stay unopened in the browser.

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

- **2026-11-24** — E7 (`c-d60149`): the queue is the existing `exploration_nodes` lease, not a broker.
  It already had atomic claims, fencing tokens and lease takeover, so a broker would have added a
  second source of truth. Worker nodes are the same binary and wiring, so the gating is identical by
  construction; every control decision a single process kept in memory is now read from Postgres, and
  a read failure blocks the claim. Worker traffic is recorded by the control node's proxy (tokens
  stored only as SHA-256), not by per-host stores. A worker never falls back to its own budget env,
  and claims nothing without a live control node or while a `role=all` node shares the database.
  Migration **0023** — renumber if another branch lands 0023 first.

- **2026-11-24** — c-13e86c: SSE streams are same-origin only, with no cross-origin override. The cookie is
  the only credential a script-free EventSource can present, and the backend's CORS answers `*`, which a
  credentialed request may not use. Credentialed CORS for dev origins was rejected: cookies are not
  port-isolated, so it would let a page on another port of the same host (a lab target) read the streams.

- **2026-11-24** — c-ca2bd0: the operator's methodology choice is a list column on `tasks`, with an empty
  list meaning automatic, so existing tasks and every other create path (spawn, templates) keep the
  goal-word selection. A non-empty list replaces the automatic set rather than adding to it, which also
  covers excluding a checklist. An unknown id is refused at creation, so a typo never falls back to auto
  without notice. A stored id whose checklist has since been removed is skipped. The migration is
  numbered 0022 because 0015–0021 are taken on open branches, and the migrate test refuses an
  out-of-order version, so 0022 must land after them.
- **2026-11-24** — c-7c4715: the client's pickers are new `/api/me/*` routes rather than opening
  `GET /api/companies` / `GET /api/tasks` to `client_read`: those return operator data (scope rules,
  goal text, tokens, task-wide finding counts that span other clients), and a narrow DTO keeps that out
  by construction. The client task list uses the operator-made company scope rows only (same rule as
  `TaskCompaniesIn`). The Reports page, not a client Dashboard, is the client landing: it covers report
  and outcome per bound task with routes already open to clients.
- **2026-11-24** — c-869050: the E5 UI branch merges the E5 + client_read branch rather than waiting
  for PR #16, so the UI is built and browser-tested against the real API; merging it lands E5 too. The
  governance client lives in `web/src/lib/governance-api.ts` (not `api.ts`, already over 600 lines) and
  reuses its `http()`. The UI hides by permission only as a convenience; the server's policy stays the
  enforcement. The Dashboard is hidden for `client_read` because its widgets call operator-wide routes.
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

- **2026-11-24** — c-c0bb07 (E4): methodologies are leaf-package data (`methodology/`), not DB rows,
  so another project can reuse them and a new checklist is one JSON file. Applicability is derived from
  the task goal and description (`applies_when` words), so it needs no migration. An explicit per-task
  choice is a follow-up. An item closes only by a closing observation: a ledger entry whose risk_area is
  the item id or an alias (a no_issue_found needs evidence, a ruled_out or not_applicable needs a
  reason), or a non-false-positive finding of that class. A citing intent only makes the item
  in_progress. Within one methodology an alias names one item (Validate), so one "xss" entry never
  ticks three items. The scores are not added to the report: c-8f9cca (E1) owns `report/` right now.
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
- **2026-11-24 14:52 MST** · claude-code · `ej/20261124-142817-7e3c57` `b89fab2`→`dc77cdc` (6 commits) · `h-6ea377` — c-13e86c fixed: the activity stream 401 after a console login predates E5. Since the cookie move, getToken() is null, but sseUrl sent non-production builds cross-origin to :8787, where the EventSource carried no credential. Now web/src/lib/sse.ts keeps every stream same-origin (NEXT_PUBLIC_SSE_BASE removed), next.config turns dev gzip off (it buffered proxied SSE), and server openSSE writes ': open' when a stream opens (the dev rewrite held the headers until the first body byte, the 20 s ping). PG test server/activity_stream_auth_pg_test.go covers legacy ARTEX and a console user with the cookie only, and no session → 401. Also web/src/lib/sse.test.mjs. Browser-checked on the E5 stack (61d777 + this change, next dev + PG): both logins connect, no Reconnecting. HANDOFF: the stale E5 entry is condensed (it is on main as #31), and the 2026-11-23 audit operator list moved to docs/handoff/ so Current state fits its limit. **Next:** Land with or after the E5 stack branches (c-f4115c, c-01377e). The patch applied cleanly on ej/20261124-140004-61d777, where the test passes. **Decided:** SSE is same-origin only. Credentialed CORS for dev origins was rejected because cookies are not port-isolated, so a lab target on another port of the same host could read the streams.
- **2026-11-24 14:24 MST** · claude-code · `ej/20261124-141028-5cc420` `5c56fe9`→`d42cd76` (1 commit) · `h-8a2866` — c-ca2bd0 (E4 follow-up, now in review; board card c-aed5fd opened): an operator can choose a task's methodologies. Migration 0022 adds tasks.methodologies TEXT[] (empty = automatic, unchanged). The create request and create form (methodology-picker.tsx, a table fed by the new GET /api/methodologies) set it. Ids are matched loosely and stored canonical, and an unknown id returns 400. db.TaskMethodologies uses the choice when set, so the coverage API and the planner block show only the chosen checklists. The choice is carried through archive/restore and returned on the task DTO. This branch merges the unmerged E4 branch ej/20261124-123222-d3dc8a. Verified on Postgres 16: TestTaskMethodologiesOperatorChoice, the archive round-trip, TestCreateTaskMethodologies, the full go test ./... (only the known macOS 127.0.0.2 SOCKS5 test fails), web tsc and i18n. The form was not checked in a browser. **Next:** Land the E4 branch (ej/20261124-123222-d3dc8a) and this branch together. Migration 0022 must land after the 0015–0021 branches. c-aed5fd: a "none" choice and editing the choice after creation. Check the create form's picker in a browser. **Decided:** An empty list means automatic, so other create paths and existing tasks keep the goal-word selection. A non-empty list replaces the automatic set rather than adding to it, which also covers excluding a checklist. An unknown id is refused at creation, so a typo never silently falls back to automatic. Stored ids whose checklist was later removed are skipped. The migration is numbered 0022 because 0015–0021 are taken on open branches.
- **2026-11-24 13:51 MST** · claude-code · `ej/20261124-132955-7aef3f` `4dd0330`→`68d14af` (3 commits) · `h-ed4135` — c-597eb6 (now in review; follow-up card c-2d91eb opened): built the E2 web UI, and merged the unmerged E2 backend branch ej/20261124-055326-6bcfa9 so both land together. System → Integrations has a trackers table with an inline enable switch, Sync, edit and delete (with a confirm). Its sheet shows masked credential inputs driven by secret_keys plus an include_evidence switch; the ConfigField/lib/config-form.ts behind them is now shared with Notifications. Scanner imports detects the format from the file and lists past imports. The findings bulk bar has a Push to tracker select, batched at 200. The finding detail has a Tickets card with ticket links, remote state, import provenance and a single push. Types are in lib/types.ts, the client is in lib/integrations-api.ts, and the pure helpers are in lib/integrations.ts with 9 node tests (npm run test:integrations, in verify.sh). 89 i18n keys were added in tr and zh-CN. Verified: tsc, i18n check, and scripts/verify.sh after merging main (migrate_test now lists 0015 and 0020). A Playwright run in real Chromium against the real backend, with a fake GitHub on loopback, passed: add, masked edit, the destination-change refusal, nuclei import, push of 3 (a re-push updated), the detail panel, sync pulling a closed issue to fixed, and delete. **Next:** c-2d91eb: an import_id filter on the findings list so each import links to its findings; commit the browser e2e scenario as a script; pagination for the imports list. The E2 backend follow-ups c-da9198 and c-ea60a0 are unchanged. **Decided:** Merged the unmerged E2 backend branch into the UI branch, because the UI is useless without the routes. Tracker field layout lives in the web and only secret_keys comes from the server, the same split as Notifications, so the masked-field component is shared. Push has no confirmation because it is idempotent; delete keeps one. Integrations and Scanner imports are System sidebar entries. A GitHub tracker is the default type. Duplicate React keys on the finding detail page (retest and traffic panels) were fixed in passing.
- **2026-11-24 13:50 MST** · claude-code · `ej/20261124-133353-5aceb1` `bc44469`→`9830ac9` (2 commits) · `h-2d8bcf` — c-f4115c (now in review): closed the six E5 artifact lifecycle gaps on a branch that merges the E5 + client_read branch (ej/20261124-110647-182fa9, PR #16). Archives carry engagement_authorizations and restore re-creates missing versions; an artifact forces the target gate on (PATCH gate-off → 409); the scheduler pauses tasks whose artifact expired or was revoked, and resume is refused until a new version; console users change their own password through /api/auth/change-password (version compare-and-swap, sessions end); SSE re-checks and the gate's artifact read go through 2 s caches on a new generic memo/ package, invalidated by every db write. PG tests for each, mutation-checked; verify.sh (with web steps) passes. Board cards opened: c-5018f6, c-01377e. **Next:** Land PR #16 (E5) and c-5b5421's branch first, then this branch. c-01377e: UI for the gate requirement and authorization_ended pauses (after c-869050). c-5018f6: wrong current password returns 401, which the web client treats as logout. Not checked: a live task paused by the scheduler at not_after (the sweep is tested directly). **Decided:** An artifact forces the target gate (refuse turning it off) instead of recommending it. Expiry/revocation pauses unconditionally and blocks resume; a not-yet-started window does not pause. Password self-service extends the existing /api/auth/change-password rather than adding /api/users/me/password (UI already calls it; already rate-limited). Archive keeps artifact rows with the stub and carries them in the package; restore uses SET CONSTRAINTS IMMEDIATE on the one artifact audit trigger so re-created rows get actor archive-restore without relabelling restored intercept rows. One generic memo cache (TTL + race-safe invalidation), 2 s; a request's first auth check stays uncached.
- _Older hand-offs, a line each; `ej handoff log` has them whole:_
- **2026-11-24 13:46 MST** · claude-code · `h-7f6f23` — c-869050 (now in review): web UI for E5. **Next:** c-92bb52: gate mutating controls on existing pages by role (reviewer currently sees Interception Rules edit controls). **Issues:** opened #c-601ec6, #c-7c4715, #c-92bb52.
- **2026-11-24 13:36 MST** · claude-code · `h-59bfe6` — c-9b48b7: confirmed go-mitmproxy keep-alive host pinning is still unfixed through v1.9.5 (= upstream main 07e0ca5), and no upstream report exists. **Next:** Ian: approve filing (human lane).
- **2026-11-24 13:26 MST** · claude-code · `h-4efcbc` — c-965b0e done and c-775dc2 landed: merged the unmerged branch ej/20261123-192351-fab3f4 (named volumes, exit 78, start.sh idle hold e1d7a27) and added an image HEALTHCHECK that is unhealthy while start.sh holds on an isolation refusal (root-only marker /run/a… **Next:** Run docker-compose.bind.yml on a real Linux host (human lane).
- **2026-11-24 13:25 MST** · me · `h-7df71f` — c-7e0087 (now in review): each ARTEX server's agent egress floor has its own chain, ARTEX_AGENT_FLOOR_<exec uid>, with jumps tagged artex-floor-<exec uid>. **Next:** Land c-b73542 (ej/20261124-113533-f16c00), then c-02e554 (ej/20261124-122709-d136a7), then this branch.
- **2026-11-24 13:13 MST** · claude-code · `h-7925fc` — c-9ca57a built and verified on branch ej/c-9ca57a-llm-cost-rbac (pushed, commit 7d6b4cf, no PR), stacked on the client_read branch ej/20261124-110647-182fa9 (carries E5 / PR #16) and E6 (PR #28), because neither has landed on main. **Next:** After PR #16, the c-5b5421 branch and PR #28 land: merge main into ej/c-9ca57a-llm-cost-rbac (only HANDOFF.md should conflict; the stacked branch kept stale HANDOFF copies), run scripts/verify.sh and…
- **2026-11-24 13:03 MST** · claude-code · `h-ce7d36` — E3 (c-d25ff9, now in review): built the RSI.md Loop C benchmark gate and Loop B lesson graduation, with ARTEX_RSI_LOOP off by default. **Next:** c-0d277f (hardening: a server-side per-task arm ledger, suite pre-registration, sync for retired adopted lessons). **Issues:** opened #c-0d277f, #c-772190, #c-936308, #c-c89f5b.
- **2026-11-24 12:56 MST** · claude-code · `h-ef9b28` — c-c0bb07 (E4, now in review): the OWASP WSTG 4.2 (97 tests) and the OWASP API Top 10 2023 are typed, embedded, validated checklists in the new leaf package methodology/. **Next:** c-ca2bd0: let the operator choose the methodology per task (migration plus create form).
- **2026-11-24 12:46 MST** · claude-code · `h-7f77bb` — c-02e554 (now in review): the agent egress floor is kept in force while the server runs. **Next:** Land c-b73542's branch (ej/20261124-113533-f16c00) first, then this branch. **Issues:** opened #c-7e0087.
- **2026-11-24 12:46 MST** · claude-code · `h-e4f846` — c-cb0093 E6 (now in review): new Function → Cost & efficiency page and GET /api/llm/cost (by task/profile/worker/model, days) showing EUR spend priced per profile, tokens, cache hit rate, out/in ratio, EUR/1M, LLM time, wall time, avg latency and errors. **Next:** c-4c8fd6: look at the page in a browser against a running instance with real LLM calls, and see one task's trace in a collector.
- **2026-11-24 12:33 MST** · claude-code · `h-b47a3b` — c-517c2d (still open, needs only an LLM key): added scripts/live-scope-check/run.sh plus a compose overlay. **Next:** c-517c2d: set ANTHROPIC_API_KEY (or OPENAI_API_KEY) and ARTEX_PG_PASSWORD in bench/.env, run `docker build -t artex:verify .`, then scripts/live-scope-check/run.sh.
- **2026-11-24 11:57 MST** · claude-code · `h-c36681` — c-a1c0fc (now in review; board cards opened: c-5d033e, c-517c2d): scope-relay tests and limits, on a branch that merges c-15b39d's unmerged branch (ej/20261124-110649-dbd854). **Next:** c-5d033e: ForgetScope should close the scope's client connections; today an already-open upstream stays usable (dial-time check).
<!-- /composite:deltas -->

_(written by `merge_handoff`)_

## Updating this document

Keep the sections above in the standard form. Refresh **Current state** (dated stamp) and **Triage**
at the end of any session that changed code/state/config/plans; add or close GitHub issues and record
**Decisions** with their reason. `merge_handoff` writes the Deltas block and the one-line stamp — do
not hand-maintain a history chain in Current state.
