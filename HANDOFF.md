# HANDOFF.md — ARTEX

**Last updated:** 2026-11-24 17:39 MST, by claude-code: c-81193b (now in review): the finding detail hides controls the role is refused. Earlier hand-offs are under Deltas and in `ej handoff log`.

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

**2026-11-24 — Finding detail hides controls the role is refused (`c-81193b`; in review).** Triage pickers
need `review`; Start retest, Link traffic and retest session links need `operate`; the retest and traffic panels
gate themselves with `useCan`. The branch merges c-119206's (`ej/20261124-171041-463f80`), so landing it lands
both. `scripts/e2e/integrations.sh` passes 51 checks.

**2026-11-24 — Stale `worker_nodes` rows pruned (`c-597b06`, E7; in review).** Under Kubernetes the
node id is the pod name, so each rollout left dead rows. `db.NodeRetention` (20 × NodeTTL = 15 min):
`ListNodes` hides older rows and every node heartbeat runs `PruneNodes` (`db/worker_nodes.go`,
`server/node.go`). Test `db/worker_nodes_test.go` passes on Postgres 18; documented in
`docs/DISTRIBUTED.md` ("Node registry"). Not re-run on a kind cluster.

**2026-11-24 — TLS between worker nodes and the control node by default (`c-548e8b`, E7; in review).**
Worker nodes reach the control API (node RPC) over **TLS with the CA pinned**: `ARTEX_CONTROL_CA`
(`server/node_tls.go`) makes the worker accept only `https://` and trust only that CA, never the system
roots; without it an `http://` API still works with a start-up WARNING. The control listener rereads a
renewed `ARTEX_TLS_CERT`/`KEY` within 30 s (`cmd/artex/certreload.go`). Helm: `controlTLS.enabled=true`
by default, `source` = `generate` (chart-made CA via `lookup`) | `secret` | `certManager`; HTTPS probes,
`appProtocol: https`, nginx backend-protocol annotation; worker pods mount only the CA key. Compose
overlay: one-shot `artex-tls-init` runs the new `artex node-tls` subcommand (CA + pair, CA key
discarded) into separate volumes. Docs: `docs/DISTRIBUTED.md` "Transport", chart README.
**This branch merges the unmerged c-3b781a branch (`ej/20261124-155351-f5bfb9`), which carries E7, so
landing it lands E7 + c-3b781a.** Verified: `TestWorkerNodeFindingBindsControlCapture` now runs its
control API over pinned TLS (Postgres 16, passes); `TestControlRPCPinnedCA` (wrong CA and system roots
refused), cert reload and `node-tls` tests; `helm lint` and `helm template` in all three sources plus
disabled and the error cases; a chart-generated cert verified by the pinned Go client;
`docker compose config` of the overlay; `scripts/verify.sh` passed. Full `go test ./server` with
Postgres: only the known macOS SOCKS5 test and the order-dependent
`TestFindingWorkflowAutoHintToPlannerAndSetting` (passes alone) failed. **Not checked:** a live
`helm install` (c-8cfbfe) or a live compose run with a built image. **Still clear text:** the
worker→control recording-proxy hop (plain-HTTP targets and the scope token), carded `c-677166`.

**2026-11-24 — Worker-node findings carry their captured proof (`c-3b781a`, E7; in review).** A
worker-node finding binds the control capture through a signed node RPC (`POST /api/node/rpc/{op}`,
`server/node_rpc.go`, HMAC under `db.NodeRPCKey`); design in `docs/DISTRIBUTED.md` "Evidence on worker
nodes", verification in Deltas. Gap: `c-7ad4da` (capture switch read at start-up).

**2026-11-24 — E10 code-security lane + its UI and exports (`c-72dd9d`, `c-d059ae`; in review)** on
`ej/20261124-170322-736db7`, which merges the unlanded E10 branch `ej/20261124-150533-1ba29e` and main
(E2 backend + UI are on main as migration 0015). **Land this branch, not the E10 branch alone.**
- E10: SARIF imports get a lane (`runtime`/`sast`/`sca`); a task import with `asset_ids` writes an
  exploration fact yielding a finding node per new finding; `GET /api/report` shows a Source line and
  per-lane counts. Migration `0024_code_security_lane.sql` (E10's renumber of E2 to 0023 is dropped).
- c-d059ae: `?lane=runtime|sast|sca` in the shared findings filter (list, groups, tree, export;
  runtime = worker findings + runtime-scanner imports); finding DTOs carry `source` (lane, tool, rule,
  path:line, import id; no raw record); findings Markdown exports get the Source line and CSV gets
  Lane/Source Tool/Source Rule/Source Location/Import ID; import rows carry `lane_counts`.
- Web: lane badge + `path:line` on the findings table and detail, a lane dropdown (URL `?lane=`), an
  anchor-asset picker on Scanner imports (enabled once a task is chosen; lists `GET /assets?task_id=`),
  and Lanes + Fact node columns on Past imports.
- Verified on Postgres 16: `TestFindingFilterByLane`, `TestCodeScanJoinsTaskGraphAndReport` (now also
  lane filter, DTO source, CSV/MD exports, unknown anchor leaves no import row), `report` tests;
  `scripts/verify.sh` (only the known macOS SOCKS5 test fails) and `--web-only` pass. Not checked in a
  browser. Gaps: `c-7651e5` (stat cards ignore lane/import filters; fact node has no deep link).
- Detail of E2 and E10 (formats, decisions, verification): `docs/handoff/2026-11-24-e2-e10-integrations.md`.

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

**2026-11-24 — E5 web UI (`c-869050`, in review).** Detail moved to `docs/handoff/2026-11-24-e5-web-ui.md`.

**2026-11-24 (c-d25ff9, E3)** — RSI Loop C gate and Loop B lesson graduation built, flag off (`ARTEX_RSI_LOOP`). Detail moved to `docs/handoff/2026-11-24-e3-rsi-loops.md`.

**2026-11-24 (c-c0bb07, E4)** — OWASP WSTG 4.2 and API Top 10 2023 checklists drive coverage (`methodology/`). Detail moved to `docs/handoff/2026-11-24-e4-methodology-coverage.md`.

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

**2026-11-24 — client_read bound to companies (card `c-5b5421`, in review).** Detail (branch, verification, gaps) moved to `docs/handoff/2026-11-24-client-read-company-bindings.md`.

**2026-11-23** — Security-audit High/Medium fixes are on `main` (PR #1). Detail moved to `docs/handoff/2026-11-24-state-archive.md`.

Operator-facing changes from that audit (unsandboxed-start refusal, TLS on non-loopback binds, and more): `docs/handoff/2026-11-23-security-audit-operator-changes.md`.

**2026-11-24: full Docker image verified (card `c-a6891b`).** Detail moved to `docs/handoff/2026-11-24-state-archive.md`.

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
8. *(unclaimed)* — **E2×E5 tracker RBAC** (S), card `c-5c4009`. Done = tracker create/update/delete
   need admin (like notify channels), push/sync stay operator; RBAC tests pass.
9. *(unclaimed)* — **Findings stat cards honour lane/import filters; fact-node deep link** (S), card
   `c-7651e5`. Done = stat cards match the filtered list under `lane=`/`import_id=` (db test); the
   Past imports fact node opens the task graph on that node.

- *(unclaimed)* — **RSI gate hardening** (M), card `c-0d277f`: server-side per-task arm ledger, suite
  pre-registration, adopted-lesson retire sync. Done = Postgres tests for each.
- *(unclaimed)* — **RSI gate web UI** (M), card `c-936308`. Done = adopt/rollback from a proposals table in ≤2 clicks + password.

- *(unclaimed)* — **E7: TLS on the worker→control proxy hop** (M), card `c-677166`. Done = a plain-HTTP
  target request from a worker crosses the network inside TLS; Helm/compose default to it.

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
- E5 shipped a lightweight artifact (reference, signatory, scope, window, technique classes). Decide
  whether tasks with an artifact must have the target gate forced on (card `c-f4115c` item 2), and
  whether the extra admin-only routes in `server/rbac_policy.go` (tools, MCP, agents, LLM config,
  notify) suit how operators work.

**Blocked**
- **E2 browser e2e as a CI job** (S), card `c-1198a2`: blocked on c-cc25b6's SHA-pinned ci.yml landing. Done = a
  `web-e2e` job runs `scripts/e2e/integrations.sh` on pull requests and fails when the import filter is broken.

## Set up / run / test / deploy

- **Toolchain:** Go (see `go.mod`), Node (web), Docker + Compose, PostgreSQL. Verified present
  locally: Go 1.27.1, Docker 29.4, Compose v5.1, Node v24.14.
- **Build backend:** `go build ./...` (verified: exit 0).
- **Web deps:** `cd web && npm ci`, then `npx tsc --noEmit` (clean) and `npm run lint` (84 known errors, `c-834a5a`). **Embedded-UI build:** `./build.sh`.
- **Run (Docker):** `cp .env.example .env` (set `POSTGRES_PASSWORD`), `docker compose up -d --build`,
  open http://localhost:8787, setup token via `docker compose logs artex | grep "Setup token"`.
- **Verify (reproduces CI):** `scripts/verify.sh` (fast set, no Docker); `--full` adds `go test -race`
  + web static build; `--security` runs `govulncheck ./...`.
- **Browser e2e (E2 integrations):** `scripts/e2e/integrations.sh`. It needs Go, Node, and initdb/pg_ctl,
  or `ARTEX_E2E_PG_DSN` set to an empty database. It installs Playwright unless it resolves on `NODE_PATH`.
  `ARTEX_E2E_KEEP=1` keeps the logs and screenshots. Ports are set with `ARTEX_E2E_{API,UI,PG}_PORT`.
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
- **E2** [`c-6def05`] Backend, UI (`c-597eb6`) and UI follow-ups (`c-2d91eb`) are done, in review on `ej/20261124-150516-d78c96` (see Current state). Follow-ups:
  [`c-119206`] role-gate the E2 UI controls (in review); [`c-81193b`] the same for the finding detail (in review); [`c-1198a2`] run `scripts/e2e/integrations.sh` in CI (after c-cc25b6);
  [`c-da9198`] scheduled sync plus a per-tracker lock; [`c-ea60a0`] move agent `scan_ingest` onto
  `scanimport`, normalize default ports, and settle two dedup edge cases.
- **E3** [`c-d25ff9`] — gate built (in review). Follow-ups: hardening `c-0d277f`, UI `c-936308`, first live run `c-772190`, proposer/critic `c-c89f5b`.
- **E4** [`c-c0bb07`] Methodology-driven coverage: landed 2026-11-24 (see Current state). Follow-ups:
  [`c-ca2bd0`] an operator-chosen methodology per task (landed, in review; gaps `c-aed5fd`); [`c-ed937d`] checklist completion in the report export (after E1).
- **E5** [`c-669406`] Engagement governance. The backend is done on `ej/20261124-055929-bd2253`. Follow-ups:
  `c-5b5421` (client binding), `c-869050` (UI, in review), `c-f4115c` (lifecycle gaps), `c-92bb52`
  (role-aware controls on existing pages), `c-7c4715` (client_read console, in review), `c-601ec6` (chain paging).
- **TEST** [`c-3157b4`] `TestGateOffTaskIsUnchanged` fails on main with Postgres (also fails before E5).
- **E6** [`c-cb0093`] Cost/observability dashboard + OpenTelemetry export, building on existing context telemetry.
- **E7** [`c-d60149`] Distributed/remote workers (queue-backed intents) for larger engagements; Helm chart.
  Follow-ups: `c-3b781a` and `c-548e8b` (TLS to the control API) in review; `c-677166` (TLS on the
  proxy hop), `c-7ad4da`, `c-29ff19`, `c-18f7a8`, `c-8cfbfe` (live install, human).
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
- **E10** [`c-72dd9d`] Code-security lane: built 2026-11-24, in review (see Current state). Follow-ups:
  [`c-d059ae`] lane in UI and exports (built, in review on `ej/20261124-170322-736db7`);
  [`c-7651e5`] stat cards + fact-node link; [`c-c978e3`] SAST-guided runtime verification;
  [`c-5c4009`] tracker RBAC (E2×E5).

## Known issues and gotchas

- **bbot `portscan` (masscan) cannot run as the executor**: it needs raw sockets and fails setup with
  `No such file or directory: 'sudo'`. This is by design (see Decisions, c-18c9c6); use `nmap -sT`.
- **`next dev` writes `web/AGENTS.md`** (Next 16 agent rules) on start. Delete it before committing; it
  is not this repo's file.
- **SSE in dev goes through the Next rewrite** (`/api/*` → `AUTOPENTEST_API`, default :8787). Keep
  `compress: false` in the dev branch of `web/next.config.mjs` and the `: open` comment in `openSSE`, or
  streams buffer or stay unopened in the browser.

- **E2's migration is 0015 (on main); E10's lane columns are 0024.** The E10 branch
  (`ej/20261124-150533-1ba29e`) had renumbered E2 to `0023_integrations.sql`, but E2 landed on main as
  0015 and main's 0023 is `0023_distributed_workers.sql`, so c-d059ae's branch drops the renumber. Do
  not land the E10 branch alone: land c-d059ae's branch (it carries E10). A dev database that applied
  E10's `0023_integrations` refuses to start: `DELETE FROM schema_migrations WHERE version IN (23,24)`
  (0015 and 0024 are idempotent) or drop it. Server tests use per-package databases (`artex_server`, `artex_db`, …).
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

- **2026-11-24** — c-18c9c6: bbot's `portscan` (masscan) stays **unsupported** for the executor
  (option a); masscan does not get `cap_net_raw` (option b rejected). Two reasons. (1) It would not
  work without weakening an invariant: the executor runs under no_new_privs (Docker's
  `no-new-privileges` and ARTEX's own `setNoNewPrivs` in `server/execpriv_nnp_linux.go`), which
  ignores file capabilities on execve. Making it work means lifting NNP or granting ambient caps to
  the executor. (2) It would bypass the egress governor: masscan transmits through libpcap/AF_PACKET
  with its own TCP/IP stack, and packet-socket frames skip netfilter's OUTPUT chain, so
  `egress.sh`'s `-m owner --uid-owner` rules never see them. Cloud metadata, the host gateway and the
  API port would be reachable, and raw sockets also allow forged source addresses. The bbot-recon
  skill now says to use `nmap -sT` instead. Revisit only with a server-side (root) port-scan service
  that applies the scope gate and egress denies itself, never by giving the executor raw sockets.

- **2026-11-24** — c-548e8b: the worker pins the control API's CA (`ARTEX_CONTROL_CA`) rather than
  trusting system roots, so a publicly valid certificate for the same name cannot impersonate the
  control node. TLS covers the control API only; the recording-proxy hop stays HTTP for now because
  agent tools cannot use a TLS proxy (needs a worker-side relay, c-677166). Plain `http://` still works
  with a warning, for existing single-network deployments. Compose uses a new `artex node-tls`
  subcommand because the runtime image has no openssl; it discards the CA key so nothing else can be
  signed with it.

- **2026-11-24** — c-3b781a: worker nodes reach the capture and evidence stores through a control-node
  RPC (option a), not a Postgres capture index (c) or after-the-fact matching (b). (c) is a large
  migration of the sqlite capture and its blob store. (b) binds by guesswork, which the evidence model
  forbids. The RPC authenticates with a sub-key of `ARTEX_SECRET_KEY`, not the scope token: the token
  rides in agent processes' proxy URL, so an agent's shell could have recorded or bound evidence past
  the tool-layer checks. Record/bind/assess run the unchanged control-side code; every agent-side check
  still runs on the worker first.
- **2026-11-24 E10:**
  - Code findings come in through the E2 SARIF import, not a new ingest path: one parser, and one
    provenance table with a `lane` column, instead of a SAST-specific store.
  - A task import writes into the task's exploration graph (a fact for the scan run that `yields`
    finding nodes, `findings.node_id` set), so the existing report, planner tools and exports read one
    record. No new node kind or edge relation, so no CHECK migration.
  - Code is not a new asset type. Code findings anchor to existing runtime assets the operator
    names (`asset_ids`), which is where a SAST and a runtime finding on one application meet. A
    `repository` asset type can come later if a consumer needs it.
  - Imported findings stay unverified leads: excluded from `findings_total`, and `prove_goal`
    refuses them (no verified traffic). Only a reproduced worker finding proves a goal.
  - SCA is classified from typed fields only (tool, advisory-style rule id, manifest file name).
- **2026-11-24 E2:**
  - Imported findings are scanner claims: `unverified`, POE 0, `worker=import:<tool>`. A known dedup
    key leaves the existing finding untouched.
  - Imports register no notification events, so bulk uploads do not flood the channels.
  - Scanner free text goes only into quoted evidence; summary and name come from typed tokens.
  - When both sides changed, the ticket sync lets the tracker win, because remediation is tracked
    there.
  - A tracker destination change drops its ticket links.
  - Evidence goes into a ticket only when `include_evidence` is on, and then with credentials
    redacted.
- **2026-11-24 c-2d91eb (E2 follow-ups):**
  - This branch merges the unmerged E2 UI branch (which carries the backend) instead of waiting, as c-597eb6
    did. Landing it lands E2.
  - `import_id` matches the findings an import **created** (`finding_import_sources.import_id`). A later
    import that only matched one by dedup key does not claim it, so "its new findings" is exact.
  - Migration 0015 was not edited for an `import_id` index: its checksum is enforced, so any database that
    applied it would refuse to start. The filter is an `EXISTS` on the `finding_id` primary key.
  - The import filter lives in the URL, not in the saved list preferences, so it never sticks to a later visit.
  - E2 under E5: a finding's tickets and raw scanner record are operator data (`read`). Trackers are data
    sinks, so their CRUD needs `admin`, the same as notification channels.
  - The e2e uses a production `next build` + `next start` (its `/api` rewrite points at the backend) rather
    than embedding the UI, so the runner never writes into `server/webui/dist`.

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

- **2026-11-24 E2 UI (c-597eb6):**
  - The UI branch merges the unmerged E2 backend branch rather than waiting for it, because the UI is
    useless without the routes. Only HANDOFF.md conflicted.
  - Tracker field layout lives in the web (`tracker-fields.ts`); only `secret_keys` comes from the
    server. This is the same split Notifications uses, so the shared masked-field component now serves both.
  - Push has no confirmation: it is idempotent (an existing ticket is updated, never duplicated).
    Delete keeps its confirmation.
  - Integrations and Imports are System sidebar entries (the settings area), each one click away.
- **2026-11-24 E2:**
    there.
    redacted.
  - Migration 0015 was edited in place before any release, because it was never on main.

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
- **2026-11-24 17:39 MST** · claude-code · `ej/20261124-173102-01b458` `71c1d47`→`bedb3b5` (1 commit) · `h-50b1c4` — c-81193b (now in review): the finding detail hides controls the role is refused. Severity/triage-status/report-stage pickers need review; Start retest, Link traffic (edit/reorder/remove) and retest session links need operate. FindingRetestPanel and FindingTrafficPanel gate themselves with useCan. roles-e2e.js asserts reviewer keeps the 3 pickers without retest/link and client_read sees none; scripts/e2e/integrations.sh passes 51 checks. Branch merges the c-119206 branch (ej/20261124-171041-463f80), so landing it lands c-119206 too. **Next:** Land this branch (it carries c-119206). c-1198a2: run scripts/e2e/integrations.sh in CI once c-cc25b6's ci.yml lands. **Decided:** Gate inside the shared retest/traffic panels rather than per page, so every consumer is covered. Merged the unlanded c-119206 branch because roles-e2e.js lives there; resolved its imports-page conflict with main's Lanes/anchor work by taking main's version and re-applying the canImport gate, and biome-formatted its 80-column files. Fixed the e2e import-link locator broken by main's Lanes column (aria-label name).
- **2026-11-24 17:30 MST** · claude-code · `ej/20261124-172040-ead433` `dd269fd`→`0450525` (1 commit) · `h-8ce710` — c-597b06 (E7, in review): stale worker_nodes rows no longer pile up under Kubernetes. db.NodeRetention = 20 × NodeTTL (15 min); ListNodes hides rows silent longer than that and every node heartbeat runs PruneNodes (db/worker_nodes.go, server/node.go). New test db/worker_nodes_test.go (live + recently dead listed, stale hidden then deleted) passes on Postgres 18; existing two-node server tests pass. docs/DISTRIBUTED.md gains a "Node registry" bullet. Moved the E3 and E4 Current state detail to docs/handoff/ to keep HANDOFF.md under 60 KB. **Next:** Land this branch with E7. Optionally confirm on the kind install (c-8cfbfe) that after a few rollouts GET /api/settings/nodes shows only live + recently dead pods once 15 min pass. **Decided:** Retention is a fixed 20 × NodeTTL (15 min) rather than a setting: long enough to see a crashed pod on the nodes page, short enough that rollouts do not accumulate. Both hide in the query (correct even if pruning fails) and delete from the heartbeat (no separate sweeper; any running node cleans up).
- **2026-11-24 17:28 MST** · claude-code · `ej/20261124-170157-3aeba5` `a1d772d`→`b7be1bc` (4 commits) · `h-1cba84` — c-45c921 (now in review), on a branch that merges the unlanded E9 branch (ej/20261124-145257-79147f), so landing it lands E9 too. GET /api/events/stream is a company-scoped SSE push for task status, findings and the approval queue. Triggers on findings, intercept_pending and tasks NOTIFY artex_changes (schema.sql + migration 0027). One pooled connection LISTENs (db.ListenChanges), and changeHub (server/events_hub.go, started on the first subscriber) fans out, turning overruns and reconnects into a resync frame. Frames carry ids only. client_read is bound by bindCompany and sees only its company's findings and tasks, never approvals or deletions. The dashboard (use-dashboard-data.ts via hooks/use-live-events.ts) reloads the pushed panels 250ms after a frame, polls them every 60s while the stream is open and every 5s without it. Verified on Postgres 16: TestEventsStreamPushesScopedChangesPG (finding change reached a client_read session in 15ms; another company's findings and approvals withheld), the hub unit tests, the full db/server/intercept suites (SOCKS5 macOS test skipped), and scripts/verify.sh. HANDOFF trimmed under its size limit (older Current state and settled decisions moved to docs/handoff/2026-11-24-*-archive.md). **Next:** Land ej/20261124-170157-3aeba5; it carries E9. Not checked: the dashboard in a real browser (open /dashboard, triage a finding in another tab, and the tables should update within about 1s; DevTools shows /api/events/stream open). Optional: show Live (push) vs Polling in LiveStatus using the new `live` flag. Remaining E9 follow-ups: c-4b7989, c-f729e8. **Decided:** Events come from Postgres triggers + NOTIFY rather than Go call sites, because findings, approvals and task status are written from about a dozen places and other processes. Frames are id-only hints, and pages re-read through the access-checked endpoints, so the stream cannot leak row data. Delivery is lossy with a resync frame, and the DB stays the source of truth. client_read visibility reuses FindingMatches (strict) and TaskCompaniesIn; deletions are withheld from clients. The activity feed and tokens are not pushed and poll every 15s. The migration was renumbered to 0027 because 0023–0026 are taken on main or open branches.
- **2026-11-24 17:26 MST** · claude-code · `ej/20261124-171846-d9aa95` `41cda01`→`1ac2d47` (1 commit) · `h-1e1166` — c-c18f43 (mechanism landed on this branch; renumbering stays open per landing): scripts/check-migration-versions.sh (+ self-test, verify.sh step, PR-only CI job migration-versions) fails a migration not numbered above main's newest, duplicates, and edited/removed published migrations; --branches reports all ej/* branches. db/migrate_test.go assertEmbeddedApplied derives its expected list from loadMigrations, so no per-migration test edit. go test ./db passes against Postgres 16. **Next:** As each branch lands, renumber its migration above main's newest (Known issues lists them: 0017_finding_classification, 0021_lesson_scope_scrub, 0023_finding_import_links, 0023_change_notify, one of the two 0026s; drop 0023_integrations, a dup of main's 0015). Run scripts/check-migration-versions.sh --branches to refresh the list. **Decided:** Check, don't reserve: a ledger would itself conflict. Ordering is judged against the base tip; edits/deletions against the merge-base so a branch that is only behind passes.
- _Older hand-offs, a line each; `ej handoff log` has them whole:_
- **2026-11-24 17:21 MST** · claude-code · `h-c66079` — c-cea6ac (now in review): explained and fixed why c-4c8fd6 task 2 was `done` mid-decomposition. **Next:** Land ej/20261124-170856-f6245f.
- **2026-11-24 17:21 MST** · claude-code · `h-585760` — c-119206 (now in review; card c-81193b opened): E2 integrations UI gates its controls by role with useCan, matching server/rbac_policy.go — tracker add/edit/delete/enable admin; Sync, Push to tracker (bulk + detail) and the scanner import form operate; the fi… **Next:** c-81193b: hide Start retest, Link traffic and the severity/status/report-stage pickers on the finding detail for client_read (extend roles-e2e.js client block).
- **2026-11-24 17:18 MST** · claude-code · `h-5e6f9c` — c-d059ae (now in review; card c-7651e5 opened): the E10 lane is visible and filterable outside the task report. **Next:** Land ej/20261124-170322-736db7 (it carries E10); do not land the E10 branch alone.
- **2026-11-24 17:17 MST** · claude-code · `h-adc4a6` — c-0d277f (RSI gate hardening, now in review; card closed): evidence is checked against the server's own records. **Next:** c-936308 (RSI gate web UI: show suite runs and the new log actions).
- **2026-11-24 17:14 MST** · claude-code · `h-917e8c` — c-1d3e0a (E7, now in review; cards c-c18f43 and c-20c400 opened), on a branch that merges the unmerged E7 branches ej/20261124-164948-393f2a (c-18f7a8 stack) and ej/20261124-155353-b1c60a (c-8cfbfe). **Next:** Land the E7 stack with this branch on top.
- **2026-11-24 16:45 MST** · claude-code · `h-aa4a90` — c-7361b4 (in review): the goals runtime refuses LLM calls on a terminal task, and an in-flight decomposition is cancelled when its task ends, so it writes no fallback goal. **Next:** c-83ae7b: per-role default output caps for planner/worker/mainagent/compactor. **Issues:** opened #c-83ae7b.
- **2026-11-24 16:43 MST** · me · `h-c93187` — c-18f7a8 (E7 smaller gaps, now in review; branch merges the unmerged E7 branch ej/20261124-150525-1a95b3): browser MCP on worker nodes goes through a loopback credential relay (traffic.StartCredentialRelay) to ARTEX_CONTROL_PROXY with the run's scope token, c… **Next:** c-1d3e0a: worker delete-drain ack and LLM profile reload. **Issues:** opened #c-1d3e0a.
- **2026-11-24 16:40 MST** · claude-code · `h-97338f` — c-548e8b (E7, now in review): worker nodes reach the control API (node RPC) over TLS with the CA pinned by default. **Next:** Land this branch (it carries E7 and c-3b781a). **Issues:** opened #c-677166.
- **2026-11-24 16:40 MST** · claude-code · `h-55d7e2` — c-990198 (now in review): Cost & efficiency wall time now includes the first call's latency. **Next:** Land ej/c-990198-wall-time in place of (or after) ej/20261124-145313-129188; it lands E6 + c-f4a69d with it.
- **2026-11-24 16:33 MST** · claude-code · `h-a38b70` — c-2af578 (card, now in review; follow-up card c-1229ff opened): the Docker image no longer runs `bbot --install-all-deps`. **Next:** c-1229ff: make the pinned tools and bbot.yml visible to executor-run scans (per-run HOME).
- **2026-11-24 16:33 MST** · claude-code · `h-0857b6` — c-29ff19 (E7 follow-up, in review): worker transcripts are shared through PostgreSQL, so any node resumes any intent. **Next:** Land after E7 (ej/20261124-150525-1a95b3) and after the branches carrying 0023/0024; a database that applied 0025 refuses older pending migrations.
<!-- /composite:deltas -->

_(written by `merge_handoff`)_

## Updating this document

Keep the sections above in the standard form. Refresh **Current state** (dated stamp) and **Triage**
at the end of any session that changed code/state/config/plans; add or close GitHub issues and record
**Decisions** with their reason. `merge_handoff` writes the Deltas block and the one-line stamp — do
not hand-maintain a history chain in Current state.
