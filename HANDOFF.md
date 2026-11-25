# HANDOFF.md — ARTEX

**Last updated:** 2026-11-24 18:49 MST, by claude-code: c-1bbeb6 (E7, in review): worker nodes sweep their local tasks/<T>, spill/tasks/<T> and transcripts/exp<E>-… of deleted, archived or vanished tasks on start-up and every 10 min (server/worker_node_sweep.go, db.RetiredTa… Earlier hand-offs are under Deltas and in `ej handoff log`.

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

**2026-11-24 — Worker nodes sweep local files of retired tasks (`c-1bbeb6`, E7; in review).**
`server/worker_node_sweep.go`: on start-up and every 10 min, the worker dispatch loop lists `tasks/<T>`,
`spill/tasks/<T>` and `transcripts/exp<E>-…`, asks `db.RetiredTasks` (`db/task_retirement.go`) which are deleted,
archived or gone, and removes them through `.delete-staging` (the delete path's `stageTaskPaths`, split out of
`stageTaskFiles`). It skips tasks whose loops still run on the node; a DB error removes nothing. Verified on
Postgres 16: `TestWorkerNodeSweepsRetiredTaskFiles` (delete + real archive job, intents run on a worker node)
and `TestRetiredTasks`; `scripts/verify.sh` passes. Documented in `docs/DISTRIBUTED.md` "Local files on worker nodes".

**2026-11-24 — LLM sampling pin (`c-d73308`, E8; in review).** Migration 0028 adds nullable
`llm_profiles.sampling_seed`/`sampling_temperature`; `agent/sampling.go` splices them into the wire body
(seed on openai/ollama only; temperature only where the caller set none). Suite stamps `llm_sampling`.

**2026-11-24 — E9 follow-up `c-f729e8` (in review) on `ej/20261124-175807-17ca91`, which merges the
c-45c921 branch (`ej/20261124-170157-3aeba5`, carrying E9), so landing it lands both.** `lib/triage.ts` takes
a verdict set (`TriageVerdict<S>`, `APPROVAL_VERDICTS`); the key binding is the shared
`web/src/hooks/use-triage-keys.ts`. The interception approval queue (`components/approval-records/table.tsx`)
answers to A (allow) / D (deny) on the open request, then opens the next; J/K/Esc navigate. Keys send the same
single `interceptDecide` the buttons send, through the same lock; the gate is unchanged. The findings "By task"
view advances past the end of a group's page (`triageNextPage`/`firstNewKey`, `advanceGroup` in the findings
page). `npx biome check src` reports 0 errors (149 warnings and 692 infos remain). Verified: triage unit tests,
`scripts/verify.sh`, and a browser e2e on a throwaway stack,
`ARTEX_E2E_SCENARIO=triage-e2e.js scripts/e2e/integrations.sh` (16/16 PASS).

**2026-11-24 — Real-cluster install kit (`c-620510`, E7; blocked on a human).** On `ej/20261124-173856-b39c78`. `deploy/lab/values-multihost.yaml` (renders/lints), runbook
`deploy/lab/README.md` "Real cluster", and `scripts/k8s-install-checks.sh NS [RELEASE]`, which prints the check
table. Not run on any cluster.

**2026-11-24 — E7 kind install (`c-8cfbfe`) + cluster egress deny re-resolved (`c-d4946f`); in review on
`ej/20261124-172839-c17308`, which merges the c-8cfbfe branch.** `egress.sh` re-resolves the deny/allow names
every 10 s and swaps the chain atomically; verified live on kind. Detail: `docs/handoff/2026-11-24-e7-kind-install-egress.md`.

**2026-11-24 — Stale `worker_nodes` rows pruned (`c-597b06`, E7; in review).** Under Kubernetes the
node id is the pod name, so each rollout left dead rows. `db.NodeRetention` (20 × NodeTTL = 15 min):
`ListNodes` hides older rows and every node heartbeat runs `PruneNodes` (`db/worker_nodes.go`,
`server/node.go`). Test `db/worker_nodes_test.go` passes on Postgres 18; documented in
`docs/DISTRIBUTED.md` ("Node registry"). Not re-run on a kind cluster.
**2026-11-24 — dashboard push channel (`c-45c921`, in review)** on `ej/20261124-170157-3aeba5`, which merges
the unlanded E9 branch (`ej/20261124-145257-79147f`), so landing it lands E9 too. `GET /api/events/stream`
(`server/events_stream.go`) pushes `{kind: finding|approval|task|resync, op, id, task}` frames, with ids only.
Triggers on `findings`, `intercept_pending` and `tasks` (status, paused, queued, deleted_at) call
`NOTIFY artex_changes` (`schema.sql` + migration **0027**). One pooled connection runs `LISTEN`
(`db.ListenChanges`). `changeHub` (`server/events_hub.go`) starts on the first subscriber and fans the
events out; an overrun or a listener reconnect becomes `resync`. client_read is bound by `bindCompany`
and gets only its company's findings and tasks, never approvals or deletions. The web side is
`web/src/hooks/use-live-events.ts` + `web/src/lib/live-events.ts`; `use-dashboard-data.ts` reloads the
pushed panels 250ms after a frame, polls them every 60s while the stream is open and every 5s when it is
not. Verified on Postgres 16: `TestEventsStreamPushesScopedChangesPG` (finding change reached the client
session in 15ms; B's finding, approvals and other companies withheld), the hub unit tests, RBAC/binder
coverage tests and `scripts/verify.sh`. Not checked: the dashboard in a real browser.

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
nodes", verification in Deltas. Gap `c-7ad4da` (capture switch read at start-up) is closed by c-8cfbfe.

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

**2026-11-24 (c-73af97, E9 frontend/UX pass)**: findings triage, the live dashboard and file sizes.

- **File sizes and types.** Every `web/src` TS/TSX file is now ≤600 lines; the largest was 4290. No `any` remains: tools-page casts and `Record<string, any>` became typed narrowing and `unknown`. Split files keep their old paths as barrels, so imports are unchanged:
  - `lib/types.ts` → `lib/types/*`
  - `lib/api.ts` → `lib/api/*`
  - `lib/mock/*` → `lib/mock/data/*` and `lib/mock/handler/*`
- **Findings triage.** The pure core is `web/src/lib/triage.ts` (tests: `npm run test:triage`, added to `verify.sh`). An open finding has a verdict bar: Confirm, False positive, Duplicate, Accept risk, Ignore, In progress, with keys c/f/d/a/i/p, j/k to move, Esc to close. After a verdict the next finding opens on its own. Stat tiles are one-click filters, and Pending starts the queue.
- **Findings URL.** The list now reads URL params (`?status=pending&severity=…&task=…&view=…&triage=1`), so the dashboard deep-links straight into the triage queue.
- **Measured** in Chromium against the mock backend (`NEXT_PUBLIC_MOCK=1`), same build, triaging 3 pending findings:
  - old path (status filter dropdown, expand, status dropdown): 11 interactions
  - new path (Pending tile, then one key each): 4 interactions
  - both took pending from 15 to 12
- **Dashboard.** Now `dashboard/_components/*`: KPI tiles link to their lists; running tasks, newest findings and activity are tables. It is still polling (no global push channel exists): a Live/Paused indicator, pause when hidden, refresh on focus.
- **Assets.** Clear-selection was added to the existing bulk delete. The per-type lists were already tables and are now driven from data.
- **Traffic.** Select-all is now indeterminate when partly selected, and the selection survives paging. New bulk actions: copy URLs, and delete targets (whole hosts, behind a confirm). The hook-versions list is now a table.
- **Verified:** `tsc` clean, `test:i18n` and all web node tests pass, findings and dashboard checked in a browser on the mock backend.
- **Not checked:** the other split pages in a browser (they were split verbatim), and a real backend.
- **Merged with main (c-45c921 base):** main's api/types changes live in the split modules: `sseUrl` is gone from `lib/api` (use `lib/sse`), `import_id` in `findingFilterParams` (`lib/api/core.ts`), E2 types in `lib/types/integrations.ts`, the import filter and tracker push in the findings `_components/`.
- **Merge risk:** fork `h-683872` (`c-92bb52`, role-gating controls on existing pages) edits pages this pass split (system/*, tasks, traffic, assets). Expect conflicts; re-apply its gating inside the extracted `_components/`.
- **Follow-ups:** `c-45c921` (dashboard push channel), `c-4b7989` (per-exchange traffic bulk delete/export), `c-f729e8` (approvals triage, grouped-view page advance, biome errors; in review).

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

Older Current state blocks (bench subject image, E5 governance, client binding, the 2026-11-23 audit fixes,
the full Docker image, the E8 suite) are in `docs/handoff/2026-11-24-current-state-archive.md`.


## Triage (next handful)

**Agents' lane**
1. *(unclaimed)* — **E7: worker nodes miss other runtime control-node state** (M), card `c-6bcdd3`: LLM profiles,
   MCP servers, custom tools and similar are rebuilt only at start. Done = each one is followed or listed in
   DISTRIBUTED.md Limits.
2. *(unclaimed)* — **E9 follow-ups** (S–M): `c-4b7989` (per-exchange traffic bulk delete/export).
   (`c-f729e8`, approvals triage + grouped advance + biome errors, in review on `ej/20261124-175807-17ca91`.) (`c-45c921`, the dashboard push, is in
   review on `ej/20261124-170157-3aeba5`.)
   "Done when" is on each card.
3. *(unclaimed)* — **SEC: per-task uid follow-ups** (S each): `c-12df03` (single-uid start-up fails on a
   socket; MCP uid not egress-filtered), `c-e70e5c` (release ended tasks at start-up; per-key lock). The "Done when" is on each card.
4. *(unclaimed)* — **E5 UI: hide role-lacking actions on existing pages** (M), card `c-92bb52`. Done = as
   reviewer and operator, no visible control answers 403. Then `c-601ec6` (chain newest-first paging, S).
   (`c-7c4715`, the client_read console, is built and in review on `ej/20261124-144554-67afc9`.)
5. *(unclaimed)* — **SEC: gorilla/websocket bump, GO-2026-6278** (XS), card `c-c10f2c`; needs network.
   Done = govulncheck clean.
6. *(unclaimed)* — **SEC: upstream-gate residuals** (M), card `c-5a6f96`: task-scoped rules on proxy
   dials, go-mitmproxy keep-alive host pinning, and the global-egress-proxy bypass. Done = a test or a decision for each.
7. *(unclaimed)* — **SEC: logout revocation** (S), card `c-a0af33`. Done = a token replayed after
   logout or a password change gets 401, by a test.
8. *(unclaimed)* — **SEC: Docker Desktop named volumes** (S), card `c-775dc2`. Done = `docker compose up`
   starts on a Mac with the isolation self-test passing; Linux bind-mount path documented.
9. *(unclaimed)* — **E1 Client-grade reporting & deliverables** (M). Turn `report/` into
   CVSS/EPSS-scored, remediation-bearing, standards-mapped deliverables (OWASP ASVS/WSTG, MITRE
   ATT&CK). Done = a report export with per-finding CVSS vector + remediation + framework tags,
   covered by tests. Card `c-8f9cca`.
10. *(unclaimed)* — **E2 UI: gate integrations controls by role** (S), card `c-119206`. Done = as reviewer
   and as client_read, no E2 control the server refuses is shown, and the finding detail has no 403.

11. *(unclaimed)* — **E2×E5 tracker RBAC** (S), card `c-5c4009`. Done = tracker create/update/delete
   need admin (like notify channels), push/sync stay operator; RBAC tests pass.
12. *(unclaimed)* — **Findings stat cards honour lane/import filters; fact-node deep link** (S), card
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

- **E7 on a real multi-machine cluster with a real model** (card `c-620510`): separate hosts, a real CNI
  with `networkPolicy.enabled`, an LLM key and an Ingress with TLS. Follow `deploy/lab/README.md` "Real cluster";
  `scripts/k8s-install-checks.sh` prints the table to paste into docs/DISTRIBUTED.md.

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
  [`c-119206`] role-gate the E2 UI controls; [`c-1198a2`] run `scripts/e2e/integrations.sh` in CI (after c-cc25b6);
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
  proxy hop), `c-29ff19`, `c-18f7a8`; `c-8cfbfe` (kind install recorded, in review; `c-7ad4da` closed by it).
  From the install: `c-6bcdd3` (other state workers read only at start), `c-d4946f` (egress deny re-resolved;
  in review), `c-597b06` (stale node rows; in review), `c-620510` (real multi-machine cluster, human).
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
  image from a clean clone, [`c-d73308`] per-profile sampling pin (built, in review).
- **E10** [`c-72dd9d`] Code-security lane: built 2026-11-24, in review (see Current state). Follow-ups:
  [`c-d059ae`] lane in UI and exports (built, in review on `ej/20261124-170322-736db7`);
  [`c-7651e5`] stat cards + fact-node link; [`c-c978e3`] SAST-guided runtime verification;
  [`c-5c4009`] tracker RBAC (E2×E5).

## Known issues and gotchas

- **`scripts/*.sh` must run on macOS's stock bash 3.2**: no `mapfile`/`readarray`, `declare -A`,
  or `${x,,}`. A bash-4 builtin fails with "command not found" and the step silently passes (the gofmt
  check was skipped on every Mac run until c-c81e74 replaced `mapfile` with a read loop).
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

- **2026-11-24 c-f729e8 (approvals triage):** approval keys are A/D (separate verdict set from findings' C/F/D/A/I/P;
  the meaning of a key depends on the queue). They act only on an open pending request, never on a selection or with
  nothing open; batch decide keeps its confirmation dialog. The grouped view advances within the same group across
  pages, not into the next group. biome `useNullishCoalescing` hits were silenced with reasoned ignores, not `??`,
  where `||` deliberately treats "" as missing (display fallbacks).

- **2026-11-24** — c-8cfbfe / c-d4946f (cluster egress deny in `egress.sh`, re-resolved, atomic rebuild) and the socket-LB CNI gotcha: `docs/handoff/2026-11-24-e7-kind-install-egress.md`.
- **2026-11-24 c-45c921 (dashboard push):**
  - Change events come from Postgres triggers + `NOTIFY`, not from Go call sites. Findings, approvals and
    task status are written from about a dozen places (agents, imports, restores, retests, target pins),
    and a trigger covers all of them, including writers that are not this process.
  - Frames are hints with ids only. The page re-reads through the normal access-checked endpoints, so the
    stream cannot leak row data. Delivery is lossy (`resync`), and the database stays the source of truth.
  - client_read visibility reuses `FindingMatches` (strict company rule) and `TaskCompaniesIn`. Deletions
    are withheld from client sessions, because their company can no longer be judged; their slow poll
    picks them up.
  - The activity feed and token totals are not pushed (they change on every agent step) and poll every 15s.

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
- Settled decisions from 2026-11-23 and early 2026-11-24 (security fail-closed defaults, per-task uids, proxy scope,
  browser MCP scope, E5/E8 design, audit locking, client binding) are in `docs/handoff/2026-11-24-decisions-archive.md`.


## Deltas

<!-- composite:deltas · newest first; Composite rewrites this block on every merge (ej handoff merge / merge_handoff) -->
- **2026-11-24 18:49 MST** · claude-code · `ej/20261124-184127-104862` at `76f8840`, no commits · `h-7cf4df` — c-1bbeb6 (E7, in review): worker nodes sweep their local tasks/<T>, spill/tasks/<T> and transcripts/exp<E>-… of deleted, archived or vanished tasks on start-up and every 10 min (server/worker_node_sweep.go, db.RetiredTasks in db/task_retirement.go); stageTaskPaths split out of stageTaskFiles. Tests TestWorkerNodeSweepsRetiredTaskFiles (delete + real archive job, intents run on a worker node) and TestRetiredTasks pass on Postgres 16; verify.sh passes. Documented in DISTRIBUTED.md "Local files on worker nodes". **Next:** Land ej/20261124-184127-104862 (card c-1bbeb6 in review). Independent of c-43dbfc (mirroring into PG); either can land first. **Decided:** Sweep runs on the worker dispatch goroutine so no loop starts mid-decision; skips tasks still Started on the node; if a running task's exploration is unknown (no handle) no transcripts are removed that pass; a DB read error removes nothing. A missing exploration row counts as retired; an exploration no task ever named is kept. Only RoleWorker sweeps.
- **2026-11-24 18:23 MST** · claude-code · `ej/20261124-175803-248cdf` `9f7f3a9`→`2ecab99` (1 commit) · `h-0668f2` — c-67993f done (closed; c-ccc7a8 opened): the E7 TLS proxy hop was checked on a 3-node kind cluster with an agent-driven (stub LLM) task. Scope tokens were minted and registered by remoteTaskProxy (one per worker node, removed at task end). Worker→control-proxy:8789 traffic is TLS 1.3 only on host-netns captures from both nodes, and NetworkPolicy lets only worker pods reach 8789. Detail: docs/handoff/2026-11-24-e7-tls-hop-kind.md. **Next:** Land c-677166 (ej/20261124-163818-870578, fork h-c56630): rebase onto main and unify the relay helper with c-6bcdd3's relayConns (c-ccc7a8). **Decided:** Tested an image built from a throwaway git merge-tree of the c-677166 and c-6bcdd3 branches (extracted to /tmp, never committed), because the code under test lives on two unmerged branches; scope_relay.go took the c-677166 side only so it would build. The real reconciliation belongs to whoever lands, recorded as c-ccc7a8.
- **2026-11-24 18:23 MST** · claude-code · `ej/20261124-180334-421555` `9f7f3a9`→`7fed6bd` (2 commits) · `h-970bcd` — c-d73308 (E8 follow-up) built, in review: an optional per-profile LLM sampling pin. Migration 0026 adds nullable sampling_seed and sampling_temperature. agent/sampling.go applies them on the wire (seed on openai/ollama only; temperature only where the caller set none). The profile API validates the pin and the UI edits it. The E8 suite stamps llm_sampling as an invariant. **Next:** Land ej/20261124-180334-421555. Whoever lands second between this branch and the session-transcripts branch (0025) checks that the migration numbers are still ascending. For the first live suite run (c-f1518c), pin a seed on an openai or ollama profile so the stamp is meaningful. **Decided:** The pin fills and never overrides: a temperature set in code (the interception assessment, side questions) wins, so guard surfaces keep their deterministic settings. The seed is rejected with a 400, not silently dropped, on formats that cannot send it, so a stamped run never claims a seed the provider did not see. Both fields are applied in an outermost http transport, because the norma SDK has no seed field; the LLM recorder's raw capture therefore shows the true wire body. The migration is numbered 0026 because 0025 is taken by an in-flight branch.
- **2026-11-24 18:23 MST** · claude-code · `ej/20261124-180332-433a2b` `5e7acca`→`e4bb87d` (2 commits) · `h-c952c8` — c-43dbfc (E7, in review): worker-node run directories, spills and subagent sidechains are mirrored into PostgreSQL (new filesync package, db.TaskFiles, migration 0026) when an intent run ends. The archive gathers them onto the control node before packing, so the package matches a single-process run's (TestArchiveHoldsWorkerNodeRunFiles). The branch carries the c-29ff19 transcript-sharing commits, cherry-picked onto main. Hardened after an independent review: FIFO hang, digest race, long names, per-file skips become archive warnings. **Next:** Land ej/20261124-180332-433a2b (migrations 0025, 0026). Then c-1e3b53 (pull run files onto the node that reruns an intent) and c-1bbeb6 (sweep worker-node disks after task delete/archive). **Decided:** Mirror through PostgreSQL rather than require a shared volume: PG is the one store every distributed node already has, and RWX volumes are not portable. Push happens at run end, not continuously (archive-only need). The store is a union where the last push wins and nothing is deleted. The gather writes only allowlisted paths (never main/), never through links, and checks digests. Per-file failures become archive warnings, so one planted file cannot make a task unarchivable. The c-29ff19 commits were cherry-picked instead of merging its whole branch, whose E7 base conflicts with main's squashed E7 in about 25 files. Archiving now also drops the task's session_transcripts rows. **Issues:** opened #c-1bbeb6, #c-1e3b53.
- _Older hand-offs, a line each; `ej handoff log` has them whole:_
- **2026-11-24 18:21 MST** · claude-code · `h-9bfe0c` — c-601ec6 (in review): GET /api/intercept/history/chain takes order=desc and before_seq (db.AuditFilter Desc/BeforeSeq; ascending stays the default). **Next:** Land ej/20261124-180916-b60b04.
- **2026-11-24 18:13 MST** · claude-code · `h-6293db` — c-f729e8 (in review), on a branch that merges the c-45c921 branch (ej/20261124-170157-3aeba5, which carries E9), so landing it lands both. **Next:** Land ej/20261124-175807-17ca91 (carries c-45c921 and E9).
- **2026-11-24 18:12 MST** · claude-code · `h-bb3c3b` — c-4ee114 done (closed; c-80fe4e opened). **Next:** c-80fe4e: probe the stream-messages view, the detail pages and client_read.
- **2026-11-24 18:09 MST** · claude-code · `h-8a0153` — c-888292 (E6 follow-up, now in review): archives made before cost_cells are backfilled on start. **Next:** Land ej/20261124-175804-41533a in place of ej/c-990198-wall-time (it carries E6, c-f4a69d and c-990198).
- **2026-11-24 17:47 MST** · claude-code · `h-a90a9e` — c-6bcdd3 (E7, now in review), on a branch that merges ej/20261124-165722-c858f3 (c-1d3e0a and the E7 stack under it) and main. **Next:** Land the E7 stack (c-8cfbfe, c-18f7a8, c-1d3e0a), then this branch.
- **2026-11-24 17:41 MST** · claude-code · `h-62b82c` — c-8cfbfe (E7 follow-up, in review): the Helm chart was installed on a 3-node kind cluster (two worker hosts) and every check is recorded in docs/DISTRIBUTED.md ("Recorded install"). **Next:** Land ej/20261124-155353-b1c60a.
- **2026-11-24 17:40 MST** · claude-code · `h-1618d4` — c-20c400 (E7, now in review): one worker settings follower. **Next:** Land ej/20261124-173106-51fdcc (carries c-1d3e0a and the E7 stack) instead of ej/20261124-165722-c858f3.
- **2026-11-24 17:39 MST** · claude-code · `h-2932ac` — c-d4946f (E7, in review): the cluster egress deny now follows moved pods. **Next:** Land ej/20261124-172839-c17308; it carries c-8cfbfe.
- **2026-11-24 17:39 MST** · claude-code · `h-50b1c4` — c-81193b (now in review): the finding detail hides controls the role is refused. **Next:** Land this branch (it carries c-119206).
- **2026-11-24 17:30 MST** · claude-code · `h-8ce710` — c-597b06 (E7, in review): stale worker_nodes rows no longer pile up under Kubernetes. **Next:** Land this branch with E7.
- **2026-11-24 17:28 MST** · claude-code · `h-1cba84` — c-45c921 (now in review), on a branch that merges the unlanded E9 branch (ej/20261124-145257-79147f), so landing it lands E9 too. **Next:** Land ej/20261124-170157-3aeba5; it carries E9.
<!-- /composite:deltas -->

_(written by `merge_handoff`)_

## Updating this document

Keep the sections above in the standard form. Refresh **Current state** (dated stamp) and **Triage**
at the end of any session that changed code/state/config/plans; add or close GitHub issues and record
**Decisions** with their reason. `merge_handoff` writes the Deltas block and the one-line stamp — do
not hand-maintain a history chain in Current state.
