# HANDOFF.md — ARTEX

**Last updated:** 2026-11-24 17:14 MST, by claude-code: c-1d3e0a (E7, now in review; cards c-c18f43 and c-20c400 opened), on a branch that merges the unmerged E7 branches ej/20261124-164948-393f2a (c-18f7a8 stack) and ej/20261124-155353-b1c60a (c-8cfbfe). Earlier hand-offs are under Deltas and in `ej handoff log`.

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

**2026-11-24 — E7 drain ack + LLM profile reload on workers (`c-1d3e0a`, in review)** on
`ej/20261124-165722-c858f3`. The branch merges the unmerged E7 branches `ej/20261124-164948-393f2a` (c-18f7a8 and
its stack) and `ej/20261124-155353-b1c60a` (c-8cfbfe). It does **not** merge c-7ad4da's `ej/20261124-161438-32780e`:
that branch's `syncTrafficCapture` conflicts with, and is covered by, `syncSharedEgress` (consolidation card `c-20c400`).
(1) **Drain ack:** a worker node writes a `task_drain_acks` row (migration **0026**) once a task under the 'deleting'
fence is no longer served there and its routines are joined (`server/engine_drain.go` `ackDrains`, `db/drain_acks.go`).
The insert is conditional on the fence, and any fence change clears the rows. `waitTaskQuiescent` (delete and
archive) waits for every live worker node's ack, up to `remoteDrainAckBound` (8 s), and no longer assumes
2 polls + 1 s. (2) **LLM reload:** `invalidateProfileAgents` (profile save/activate/delete, `/api/llm`, retry policy,
failover switches, agent binding) bumps `settings.llm_config_revision`. The worker's `followAgentSettings` drops its
provider caches and re-applies the retry policy and the active profile. **Verified** against a local Postgres 17:
`TestDeleteWaitsForWorkerNodeDrainAck` (fails with the ack disabled: the drain hits the bound),
`TestDeleteDrainWaitsForTheBoundWhenAWorkerNodeIsGone`, `TestWorkerNodeFollowsLLMProfileChange` and the existing E7
tests. `scripts/verify.sh` is clean except two environment-only failures here (`TestConnectWithoutPasswordRealPG`:
the local PG uses trust auth; SOCKS5 UDP: macOS has no 127.0.0.2). **Not verified:** a real two-host delete.
**Merge risk:** migration numbers collide across unmerged branches (`c-c18f43`).

**2026-11-24 — E7 smaller gaps (`c-18f7a8`, in review)** on `ej/20261124-161440-11cbc4`, which merges the
unmerged E7 branch `ej/20261124-150525-1a95b3`. (1) Browser MCP on worker nodes: `traffic.StartCredentialRelay`
(`traffic/credential_relay.go`) is a loopback relay that adds the run's scope token as `Proxy-Authorization` and
forwards to `ARTEX_CONTROL_PROXY`; `server/mcp_proxy_remote.go` swaps the shared row's loopback `--proxy-server`
for it and points `NODE_EXTRA_CA_CERTS` at the fetched control CA. `ScopeRelay` and the new relay share
`traffic/relay_conns.go`. (2) Remote findings wake the planner: `remoteSettleLoop` pages
`ExplorationStore.FindingNodesSince` and calls `NotifyFinding` for worker-node writers. (3) The capture proxy
requires a scope token from every non-loopback client in every role; `ARTEX_PROXY_ALLOW_UNSCOPED_REMOTE=1`
opts out (ignored on a control node). (4) Noise-governor floor decided against (Decisions), bound pinned by
`egress/share_bound_test.go`. (5) Rule and enabled-tools writes bump the intercept revision in their own
transaction (`db.writeInterceptRules`); callers only `DropCache`. Verified: the new tests against Postgres 16
(`TestInterceptRuleWritesBumpInTheirTransaction`, `TestFindingNodesSince`, `TestWorkerBrowserMCPRelaysToControlProxy`,
`TestCredentialRelayCarriesRemoteScope`, `TestRemoteClientScopeRequiredByDefault`), and a real headless Chrome
through the credential relay (`TestCredentialRelayRealChrome`, `ARTEX_TEST_CHROME`). **Not verified:** the
Playwright MCP on a real worker host (falls under `c-8cfbfe`). **Merge risk:** `c-50b832`
(`ej/20261124-114604-e46087`) moves the browser proxy into a Playwright `--config` file; whichever lands second
must keep the worker path in `scopeMCPProxy`. Not in this card (left over from the E7 delta): delete-drain ack and LLM profile
reload on workers, now `c-1d3e0a`; proxy-hop TLS is `c-677166` (the credential relay already dials an https upstream,
but without CA pinning yet).

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
(`c-29ff19`), smaller items `c-18f7a8` (built, see the entry above).

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
**Landed:** this c-548e8b branch (carrying E7 c-d60149 and c-3b781a) is on main as #46 (`41cda010`, squash).

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

**2026-11-24** — **E2 integrations** (backend `c-6def05`, UI `c-597eb6`, follow-ups `c-2d91eb`, all in review)
are on branch `ej/20261124-150516-d78c96`. It merges the UI branch `ej/20261124-132955-7aef3f`, which carries
the backend `ej/20261124-055326-6bcfa9`, so landing it lands all of E2. See `docs/integrations.md`.
- **Findings out:** package `ticketing` (GitHub Issues, Jira, DefectDojo) goes through `notify.Do` (the SSRF
  guard and redaction). Tracker configs are sealed and masked, and a destination change needs the credentials again.
  `/api/integrations/trackers` has CRUD, `push` and `sync`. Status round-trips, and the tracker wins a conflict.
- **Scanner data in:** package `scanimport` reads Nmap, Nuclei, ZAP, Burp and SARIF. `POST /api/integrations/imports`
  writes assets with provenance and unverified POE-0 findings (`finding_import_sources`), and never edits on re-import.
  `GET /api/integrations/imports?limit=&offset=` returns `{imports, total}`.
- **UI:** System → Integrations (the trackers table and a masked-credential sheet) and Scanner imports (format
  detection, Past imports with a pager). Each import's New findings count links to
  `/function/findings?import_id=N`. `import_id` is in the shared findings filter (`db.FindingFilter.ImportID`),
  so it applies to the list, groups, asset tree and export, and the page shows a chip that clears it.
  The findings bulk bar and the detail page have Push to tracker; the detail has a Tickets card.
- **RBAC (with E5):** `GET /api/exploration/findings/{id}/integrations` needs `read`, not `client_read`.
  Tracker create/edit/delete need `admin`, the same as notification channels. Push, sync and import need `operate`.
  The E2 pages do not yet gate their controls by role (`c-119206`).
- **Browser e2e:** `scripts/e2e/integrations.sh` sets up a throwaway Postgres, the backend, a Next production
  build and Playwright, then runs `integrations-e2e.js` against `fake-github.js`. It passed 24 checks
  locally on 2026-11-24. It is not a CI job yet (`c-1198a2`, after c-cc25b6).
- Migration `0015_integrations.sql` is unreleased, and its checksum is enforced, so do not edit it. Fix forward.
- Local `./server` failures that are environment, not code: `TestConnectWithoutPasswordRealPG` (trust-auth Postgres)
  and the SOCKS5 test that needs a `127.0.0.2` alias.

Older entries (E3, E4 coverage, bench subject, E5 governance, client binding, security audit, full Docker
image, E8 eval suite) moved to `docs/handoff/2026-11-24-current-state-archive.md`.

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
8. *(unclaimed)* — **E2 UI: gate integrations controls by role** (S), card `c-119206`. Done = as reviewer
   and as client_read, no E2 control the server refuses is shown, and the finding detail has no 403.

- *(unclaimed)* — **RSI gate hardening** (M), card `c-0d277f`: server-side per-task arm ledger, suite
  pre-registration, adopted-lesson retire sync. Done = Postgres tests for each.
- *(unclaimed)* — **RSI gate web UI** (M), card `c-936308`. Done = adopt/rollback from a proposals table in ≤2 clicks + password.

- *(unclaimed)* — **DB: renumber colliding migrations as branches land** (S), card `c-c18f43`. Done = unique
  versions on main, `go test ./db` green.
- *(unclaimed)* — **E7: one worker settings follower** (S), card `c-20c400`. Done = one reload path, tests green,
  claims still blocked while the settings are unreadable.
- *(unclaimed)* — **E7: TLS on the worker→control proxy hop** (M), card `c-677166`. Done = a plain-HTTP
  target request from a worker crosses the network inside TLS; Helm/compose default to it.

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
- **E2 browser e2e as a CI job** (S), card `c-1198a2`: blocked on c-cc25b6's SHA-pinned ci.yml landing. Done = a
  `web-e2e` job runs `scripts/e2e/integrations.sh` on pull requests and fails when the import filter is broken.

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
  [`c-119206`] role-gate the E2 UI controls; [`c-1198a2`] run `scripts/e2e/integrations.sh` in CI (after c-cc25b6);
  [`c-da9198`] scheduled sync plus a per-tracker lock; [`c-ea60a0`] move agent `scan_ingest` onto
  `scanimport`, normalize default ports, and settle two dedup edge cases.
- **E3** [`c-d25ff9`] — gate built (in review). Follow-ups: hardening `c-0d277f`, UI `c-936308`, first live run `c-772190`, proposer/critic `c-c89f5b`.
- **E4** [`c-c0bb07`] Methodology-driven coverage: landed 2026-11-24 (see Current state). Follow-ups:
  [`c-ca2bd0`] an operator-chosen methodology per task (landed, in review; gaps `c-aed5fd`); [`c-ed937d`] checklist completion in the report export (after E1).
- **E5** [`c-669406`] Engagement governance. The backend is done on `ej/20261124-055929-bd2253`. Follow-ups:
  `c-5b5421` (client binding), `c-869050` (UI, in review), `c-f4115c` (lifecycle gaps), `c-92bb52`
  (role-aware controls on existing pages), `c-7c4715` (client_read console, in review), `c-601ec6` (chain paging).
- **DB** [`c-c18f43`] Migration version collisions across unmerged branches (0023 x3, 0024, 0025; c-1d3e0a uses 0026).
- **TEST** [`c-3157b4`] `TestGateOffTaskIsUnchanged` fails on main with Postgres (also fails before E5).
- **E6** [`c-cb0093`] Cost/observability dashboard + OpenTelemetry export, building on existing context telemetry.
- **E7** [`c-d60149`] Distributed workers + Helm: built, in review (see Current state). Follow-ups:
  `c-3b781a` (evidence on workers), `c-29ff19` (transcripts across hosts), `c-8cfbfe` (live two-host
  install), `c-18f7a8` (browser MCP relay, finding wake-ups, proxy scope default, rule-revision tx; in review),
  `c-548e8b` (TLS to the control API, landed on main as #46), `c-1d3e0a` (worker drain ack, LLM profile reload; in review), `c-20c400` (one settings follower),
  `c-677166` (proxy-hop TLS).
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

- **2026-11-24** — c-1d3e0a: (a) An ack means "fence up, and no runs of this task here". The worker writes it
  only from its dispatch loop (the only place a task starts being served), only after StopTask joined the
  routines, and only while the fence stands. No run starts under the fence, so no fence epoch is needed. A fence
  change clears the acks. (b) After the bound (8 s, inside the 10 s drain timeout) the delete **goes on** and logs
  the silent nodes, as the card asked. A stale registry row (NodeTTL 45 s) is not waited for. (c) The LLM reload
  reuses the c-8cfbfe revision follower with its own key, `llm_config_revision`. The bump lives in
  `invalidateProfileAgents`, so binding and pool changes are covered too. Workers never bump, which would loop.
  (d) Migration numbered 0026 to step past the 0023–0025 already taken on other unmerged branches.

- **2026-11-24** — c-18f7a8: (a) The capture proxy refuses token-less non-loopback clients in **every** role,
  not only on a control node. `ARTEX_PROXY_ALLOW_NONLOOPBACK=1` was meant to expose the proxy, not to make it an
  open relay on the global rules. Operators who really want remote manual capture set
  `ARTEX_PROXY_ALLOW_UNSCOPED_REMOTE=1`. A control node ignores that switch. (b) **Decided against** a DB-backed
  per-host slot for the noise governor. It would add a Postgres round trip and a new failure mode (block or
  fail open) to every target request. The rate is already divided exactly, and the in-flight overshoot is
  bounded at max(limit, N). Most worker traffic is also paced again at the control proxy. The bound is pinned by
  `egress/share_bound_test.go`. Revisit if `quiet` with several nodes is shown to trip a target's limits. (c) The
  intercept revision bump runs in the write's own transaction: a failed bump rolls the write back (an API 500),
  so no committed rule is ever unannounced. (d) A worker treats any loopback http `--proxy-server` in a shared
  MCP row as the capture proxy, because a worker runs no proxy of its own.

- **2026-11-24** — E7 (`c-d60149`): the queue is the existing `exploration_nodes` lease, not a broker.
  It already had atomic claims, fencing tokens and lease takeover, so a broker would have added a
  second source of truth. Worker nodes are the same binary and wiring, so the gating is identical by
  construction; every control decision a single process kept in memory is now read from Postgres, and
  a read failure blocks the claim. Worker traffic is recorded by the control node's proxy (tokens
  stored only as SHA-256), not by per-host stores. A worker never falls back to its own budget env,
  and claims nothing without a live control node or while a `role=all` node shares the database.
  Migration **0023** — renumber if another branch lands 0023 first.

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
  - Imported findings are scanner claims: `unverified`, POE 0, `worker=import:<tool>`. A known dedup
    key leaves the existing finding untouched.
  - Imports register no notification events, so bulk uploads do not flood the channels.
  - Scanner free text goes only into quoted evidence; summary and name come from typed tokens.
  - When both sides changed, the ticket sync lets the tracker win, because remediation is tracked
    there.
  - A tracker destination change drops its ticket links.
  - Evidence goes into a ticket only when `include_evidence` is on, and then with credentials
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
- **2026-11-24 17:14 MST** · claude-code · `ej/20261124-165722-c858f3` `f670dd4`→`4e80eb2` (1 commit) · `h-917e8c` — c-1d3e0a (E7, now in review; cards c-c18f43 and c-20c400 opened), on a branch that merges the unmerged E7 branches ej/20261124-164948-393f2a (c-18f7a8 stack) and ej/20261124-155353-b1c60a (c-8cfbfe). (1) Worker nodes ack the 'deleting' fence in task_drain_acks (migration 0026). An ack is written from the dispatch loop, after the task's routines are joined, and only while the fence stands; a fence change clears the acks. Delete and archive wait for every live worker node's ack, up to remoteDrainAckBound (8 s), instead of assuming quiet after 2 polls + 1 s. (2) invalidateProfileAgents bumps settings.llm_config_revision. The worker's followAgentSettings drops its provider caches and re-applies the retry policy and the active profile, so its next intent uses the new config. Two-manager tests for each pass against Postgres 17. **Next:** Land the E7 stack with this branch on top. Renumber colliding migrations as branches land (c-c18f43). Consolidate the worker settings followers and retire c-7ad4da's duplicate branch (c-20c400). c-8cfbfe live install: check a delete with a worker pod running an intent. **Decided:** Ack = fence up and no runs here. It is written only in the dispatch loop, after StopTask joins, conditional on the fence, so no fence epoch is needed. After the bound the delete goes on and logs the silent nodes, as the card asked; a stale registry row is not waited for. The LLM reload reuses the c-8cfbfe revision follower with its own key, and the bump sits in invalidateProfileAgents, so bindings and pool switches are covered; workers never bump. Not merged: c-7ad4da's branch, because syncSharedEgress already covers it. Migration numbered 0026 to step past numbers taken on other branches.
- **2026-11-24 16:45 MST** · claude-code · `ej/20261124-163821-1592b3` `5388ed6`→`6091cb0` (1 commit) · `h-aa4a90` — c-7361b4 (in review): the goals runtime refuses LLM calls on a terminal task, and an in-flight decomposition is cancelled when its task ends, so it writes no fallback goal. The decomposer caps replies at 8192 tokens (a tighter profile cap wins) and a run at 10 minutes. Root cause: profile max_tokens=0 meant norma's 32768 default plus 3 max_tokens resume-recoveries, and the decomposer runs alongside the seed intent with nothing to stop it at terminal. **Next:** c-83ae7b: per-role default output caps for planner/worker/mainagent/compactor. Then a live Ollama re-run of the c-4c8fd6 check to confirm there are no goals calls after done. **Decided:** Only the goals role is terminal-gated: the settle sequence's final planner round and human steering legitimately run on terminal tasks. Cap is 8192, not lower, so models with reasoning still fit a few tool calls. The other roles' defaults are left alone until their real output sizes are measured. **Issues:** opened #c-83ae7b.
- **2026-11-24 16:43 MST** · me · `ej/20261124-161440-11cbc4` at `001a1bb`, no commits · `h-c93187` — c-18f7a8 (E7 smaller gaps, now in review; branch merges the unmerged E7 branch ej/20261124-150525-1a95b3): browser MCP on worker nodes goes through a loopback credential relay (traffic.StartCredentialRelay) to ARTEX_CONTROL_PROXY with the run's scope token, checked with headless Chrome; the control node relays worker-node findings to the planner (FindingNodesSince + NotifyFinding); the capture proxy requires a scope token from every non-loopback client in every role (ARTEX_PROXY_ALLOW_UNSCOPED_REMOTE=1 opts out, ignored on control); intercept rule/enabled-tools writes bump the revision in their own transaction; the noise-governor per-node floor was decided against, with its bound pinned by a test **Next:** c-1d3e0a: worker delete-drain ack and LLM profile reload. c-677166: TLS on the worker->control proxy hop (the credential relay already dials https but does not pin a CA yet). c-8cfbfe: live two-host install, including the Playwright MCP on a worker pod. Merge risk with c-50b832 (Playwright --config proxy): keep the worker branch of scopeMCPProxy. **Decided:** Scope token required for every non-loopback proxy client regardless of role. No DB-backed per-host noise slot: an extra round trip and a new failure mode on every request, for an overshoot bounded at max(limit, N). The revision bump lives in the write transaction, so a failed bump rolls the write back. On a worker, any loopback http proxy in a shared MCP row counts as the capture proxy. **Issues:** opened #c-1d3e0a.
- **2026-11-24 16:40 MST** · claude-code · `ej/20261124-162011-f38608` `08b34de`→`c53bbeb` (14 commits) · `h-97338f` — c-548e8b (E7, now in review): worker nodes reach the control API (node RPC) over TLS with the CA pinned by default. New ARTEX_CONTROL_CA (server/node_tls.go): https only, trusts only that CA, refuses a CA file holding a key; plain http:// still works with a WARNING. The control listener hot-reloads a renewed ARTEX_TLS_CERT/KEY (cmd/artex/certreload.go). Helm controlTLS (enabled by default; generate | secret | certManager), HTTPS probes, appProtocol https, nginx backend-protocol annotation, worker pods mount only the CA. Compose overlay: one-shot artex-tls-init runs the new `artex node-tls` subcommand into separate key/CA volumes. docs/DISTRIBUTED.md "Transport" and chart README updated. The branch merges the unmerged c-3b781a branch (carries E7), so landing it lands E7 + c-3b781a + this. Verified: TestWorkerNodeFindingBindsControlCapture over pinned TLS on Postgres 16, new TLS/reload/node-tls tests, helm lint/template in every mode, chart-generated cert checked with the pinned client, compose config, verify.sh passed. Two long Current state entries (c-5b5421, c-869050) moved to docs/handoff/ to fit the size limit. **Next:** Land this branch (it carries E7 and c-3b781a). c-677166: TLS on the worker→control recording-proxy hop via a worker-side relay (combine with c-18f7a8 item 1). c-8cfbfe: live helm install, now also checking the TLS default (port-forward https, worker logs show no plain-HTTP warning). **Decided:** Pin the control CA instead of trusting system roots, so a public cert for the same name cannot impersonate the control node. TLS covers the API only; the proxy hop needs a relay because agent tools cannot use a TLS proxy (c-677166). http:// kept working with a warning for compatibility. `artex node-tls` because the runtime image has no openssl; the CA key is discarded. **Issues:** opened #c-677166.
- _Older hand-offs, a line each; `ej handoff log` has them whole:_
- **2026-11-24 16:40 MST** · claude-code · `h-55d7e2` — c-990198 (now in review): Cost & efficiency wall time now includes the first call's latency. **Next:** Land ej/c-990198-wall-time in place of (or after) ej/20261124-145313-129188; it lands E6 + c-f4a69d with it.
- **2026-11-24 16:33 MST** · claude-code · `h-a38b70` — c-2af578 (card, now in review; follow-up card c-1229ff opened): the Docker image no longer runs `bbot --install-all-deps`. **Next:** c-1229ff: make the pinned tools and bbot.yml visible to executor-run scans (per-run HOME).
- _Older hand-offs, a line each; `ej handoff log` has them whole:_
- **2026-11-24 16:33 MST** · claude-code · `h-0857b6` — c-29ff19 (E7 follow-up, in review): worker transcripts are shared through PostgreSQL, so any node resumes any intent. **Next:** Land after E7 (ej/20261124-150525-1a95b3) and after the branches carrying 0023/0024; a database that applied 0025 refuses older pending migrations.
- **2026-11-24 16:17 MST** · claude-code · `h-11f65f` — c-3b781a (E7 follow-up, now in review): a finding recorded by a worker-node intent now binds the control node's captured request/response. **Next:** Land this branch (it carries E7). **Issues:** opened #c-548e8b, #c-7ad4da.
- **2026-11-24 16:09 MST** · claude-code · `h-666bdf` — E7 (c-d60149) built, in review: -role all|control|worker. **Next:** c-8cfbfe: real helm install on a 2+ node cluster (human or infra). **Issues:** opened #c-18f7a8, #c-29ff19, #c-3b781a, #c-8cfbfe.
- **2026-11-24 16:07 MST** · claude-code · `h-59c24d` — E10 (c-72dd9d, now in review): code-security lane. **Next:** c-5c4009: tracker config RBAC (admin vs operate). **Issues:** opened #c-5c4009, #c-c978e3, #c-d059ae.
- **2026-11-24 15:58 MST** · claude-code · `h-0548f2` — c-2d91eb (E2 UI follow-ups, now in review; board cards c-1198a2 and c-119206 opened) on a branch that merges the unmerged E2 UI + backend branch (ej/20261124-132955-7aef3f), so landing it lands all of E2. **Next:** Land ej/20261124-150516-d78c96 (it carries the E2 backend, UI and these follow-ups).
- **2026-11-24 15:56 MST** · claude-code · `h-f62f2d` — c-4c8fd6 (E6 live check, now in review): built the E6 branch into an isolated compose stack (local Ollama with prices, nginx target on its own network, OTel collector via host.docker.internal). **Next:** Land ej/20261124-123231-b78990 (E6, not on main yet) with c-f4a69d. **Issues:** opened #c-7361b4, #c-990198.
- **2026-11-24 15:30 MST** · claude-code · `h-124b6f` — c-f4a69d (E6 follow-up, now in review; card c-888292 opened): the Cost & efficiency view now includes archived tasks. **Next:** c-4c8fd6: check the cost page in a browser against real LLM traffic, including an archived task.
- **2026-11-24 15:29 MST** · claude-code · `h-0c292e` — E9 (c-73af97). **Next:** Land this branch. **Issues:** opened #c-45c921, #c-4b7989, #c-f729e8.
- **2026-11-24 15:21 MST** · claude-code · `h-683872` — c-92bb52 (E5 UI follow-up, closed; c-4ee114 opened): every existing web page now hides or disables the mutating controls the session's role lacks, per server/rbac_policy.go. **Next:** c-4ee114: re-probe with seeded rows (MCP, channels, LLM profiles, pending approvals, traffic exchange sheet, files, skills, hooks), hide the assets checkboxes without operate, switch credentials-tab…
- **2026-11-24 14:52 MST** · claude-code · `h-6ea377` — c-13e86c fixed: the activity stream 401 after a console login predates E5. **Next:** Land with or after the E5 stack branches (c-f4115c, c-01377e).
- **2026-11-24 14:24 MST** · claude-code · `h-8a2866` — c-ca2bd0 (E4 follow-up, now in review; board card c-aed5fd opened): an operator can choose a task's methodologies. **Next:** Land the E4 branch (ej/20261124-123222-d3dc8a) and this branch together.
- **2026-11-24 13:51 MST** · claude-code · `h-ed4135` — c-597eb6 (now in review; follow-up card c-2d91eb opened): built the E2 web UI, and merged the unmerged E2 backend branch ej/20261124-055326-6bcfa9 so both land together. **Next:** c-2d91eb: an import_id filter on the findings list so each import links to its findings; commit the browser e2e scenario as a script; pagination for the imports list.
<!-- /composite:deltas -->

_(written by `merge_handoff`)_

## Updating this document

Keep the sections above in the standard form. Refresh **Current state** (dated stamp) and **Triage**
at the end of any session that changed code/state/config/plans; add or close GitHub issues and record
**Decisions** with their reason. `merge_handoff` writes the Deltas block and the one-line stamp — do
not hand-maintain a history chain in Current state.
