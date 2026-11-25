# HANDOFF.md — ARTEX

**Last updated:** 2026-11-24 18:23 MST, by claude-code: c-d73308 (E8 follow-up) built, in review: an optional per-profile LLM sampling pin. Earlier hand-offs are under Deltas and in `ej handoff log`.

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

**2026-11-24 — E7 c-677166 + c-6bcdd3 land together, one relay helper (`c-ccc7a8`, in review)** on
`ej/20261124-185428-8c1cc4`. It merges `ej/20261124-172031-c2bb7b` (c-6bcdd3, carrying c-20c400, c-d4946f,
c-8cfbfe and the E7 stack) and `ej/20261124-163818-870578` (c-677166, proxy-hop TLS), so **landing it lands
both; neither branch should land on its own after this**. The two had each extracted the loopback-relay
bookkeeping from `traffic/scope_relay.go`; now `traffic/conn_relay.go` `connSet` is the only helper
(`relay_conns.go`/`relayConns` is gone). `connSet` owns the listener (`newConnSet`, `listenLoopback`),
`serve` (capped accept loop), `close` (once: listener + tracked conns), `track`/`trackPair`, `splice`, and
`pipeBoth` (full duplex from buffered readers). ScopeRelay, CredentialRelay, TLSRelay and ProxyTLSFront all use
it. The c-677166 TLS hop design and its kind run (c-67993f): `docs/handoff/2026-11-24-e7-tls-hop-kind.md` on
`ej/20261124-175803-248cdf`; c-677166 detail in its branch's HANDOFF. Verified: `go build ./...`, `go vet`,
`go test ./traffic` (and `-race` on the relay tests), `go test ./server ./db ./cmd/...`. Not re-run: the live kind lab.

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

**2026-11-24 — E7 start-up-only state audit (`c-6bcdd3`, in review)** on `ej/20261124-172031-c2bb7b`. It merges
c-20c400 (`ej/20261124-173106-51fdcc`, which carries the E7 stack), c-d4946f/c-8cfbfe and main, so **landing it lands
all of them**. Workers now follow two more things: the LLM recording switch (compared every poll) and agent config
saves (`settings.agent_config_revision`, which drops task agents). Everything else is read per run or per call, or is
control-only. `docs/DISTRIBUTED.md` has the list and three new Limits (skills on disk, per-node env, lesson block).
Tests: `server/worker_node_agent_test.go` (both fail with the fix removed). Full `verify.sh` on Postgres 16 fails only
the known macOS SOCKS5 UDP test. Its gofmt step does not run on macOS (`c-c81e74`); gofmt was run by hand: clean.

**2026-11-24 — E7: one worker settings follower (`c-20c400`, in review)** on `ej/20261124-173106-51fdcc`, which
merges the c-1d3e0a branch `ej/20261124-165722-c858f3` (so it carries the whole unmerged E7 stack). `syncSharedEgress`
/ `Manager.reloadSharedEgress` and the `agent_settings_revision` key are gone. `followAgentSettings`
(`server/worker_node_settings.go`) runs first in every `workerDispatchOnce` poll and returns a claim-block reason:
it compares the capture/global-proxy/web-search values (brave/tavily/keenable keys included) every poll via
`Manager.loadAgentSettings` (now error-returning), follows `llm_config_revision` for LLM profiles, blocks claims
while either read fails, and on a settings change drops the task agents and holds claims for that poll. c-7ad4da's
`TestWorkerNodeFollowsTrafficCaptureSwitch` and `startTestNodeOn` are ported here; **its branch
`ej/20261124-161438-32780e` must not land** (it would add a third copy and conflicts in `workerDispatchOnce`).

**2026-11-24 — E7 stack (`c-d60149` + `c-18f7a8`, `c-1d3e0a`, `c-548e8b`; c-548e8b landed as #46, the rest in review).**
Distributed workers + Helm, browser MCP relay, finding wake-ups, proxy scope default, drain ack, LLM profile
reload, TLS to the control API. Detail moved to `docs/handoff/2026-11-24-e7-distributed.md`.

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

**2026-11-24 — Worker-node findings carry their captured proof (`c-3b781a`, E7; in review).** A
worker-node finding binds the control capture through a signed node RPC (`POST /api/node/rpc/{op}`,
`server/node_rpc.go`, HMAC under `db.NodeRPCKey`); design in `docs/DISTRIBUTED.md` "Evidence on worker
nodes", verification in Deltas. The `c-7ad4da` gap (capture switch read only at start-up) is closed by `c-20c400`.
**Landed:** this c-548e8b branch (carrying E7 c-d60149 and c-3b781a) is on main as #46 (`41cda010`, squash).

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
1. E7 install follow-ups: `c-6bcdd3` (state workers read only at start) is in review on `ej/20261124-172031-c2bb7b`;
   `c-d4946f` (re-resolve the egress deny) and `c-597b06` (stale node rows) are in review on their branches.
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

- *(unclaimed)* — **DB: renumber colliding migrations as branches land** (S), card `c-c18f43`. Done = unique
  versions on main, `go test ./db` green.

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
  `c-548e8b` (TLS to the control API, landed on main as #46), `c-1d3e0a` (worker drain ack, LLM profile reload; in review), `c-20c400` (one settings follower; in review),
  `c-677166` (proxy-hop TLS; in review, lands with `c-6bcdd3` via `c-ccc7a8`).
  From the install: `c-6bcdd3` (state workers read only at start; in review), `c-d4946f` (egress deny
  re-resolved; in review), `c-597b06` (stale node rows; in review), `c-620510` (real multi-machine cluster, human).
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
- **A database created from an unmerged E7 branch does not start on `main`.** It already applied
  `0023_distributed_workers`, and `main` then added `0015_integrations` (E2). The migrator refuses an
  inserted migration ("migrations must be appended, never inserted"). Only databases from branch builds
  are affected; use a fresh database (seen in the c-8cfbfe kind lab and in test databases).

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

- **2026-11-24** — c-ccc7a8: `connSet` (c-677166) is the one relay helper, not `relayConns` (c-6bcdd3), because it
  already served three relays including the TLS front, whose accept listener (a `tls.NewListener`) differs from the
  raw one. It gained relayConns' listener ownership and once-close, so no relay carries its own `ln`/`once`. The
  splice now closes a destination that cannot half-close (CredentialRelay's old `pipeTunnel` behaviour) instead of
  leaving the other direction waiting; TCP and TLS conns both half-close, so the existing relays are unchanged.
- **2026-11-24** — c-677166: the proxy hop's TLS runs between two relays, not in the agents' tools
  (most cannot use a TLS proxy). go-mitmproxy cannot serve a caller's listener, so the control side is a
  TLS front that dials the proxy over loopback; its connections are marked remote (`sink.fronted`, kept
  until the proxy's disconnect, like the scope relay's tombstone) so loopback trust never applies to
  them. Helm reaches the front through the existing ClusterIP `-control-proxy` Service and checks the
  certificate against `<release>-control` (`ARTEX_CONTROL_PROXY_TLS_NAME`) rather than adding a name.
  An `http://` control proxy still works with a WARNING (compatibility) rather than being refused.

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

- **2026-11-24** — c-20c400: the one worker settings follower compares the agent-baked settings **by value every
  poll**, not by `agent_settings_revision` (removed). The revision bump in `putSettings` was best-effort (a failure
  only logged), and a lost bump left a worker's agents on the old egress path, past the recording proxy and its
  scope. Any direct write (c-7ad4da's test flips `SetTrafficEnabled` with no bump) was missed too. Comparing the
  values costs 8 single-row reads per poll per worker (syncSharedEgress already read 5). The LLM config keeps its
  revision because the profiles are many rows. An `applyLLM` failure after a change does not block claims: task
  agents are built per task from the Manager's already-updated settings, so dropping them fixes the egress path.
- Older decisions (E2–E10, E7 c-d60149…c-20c400, security, bench) moved to `docs/decisions.md` (c-ccc7a8).
- Settled decisions from 2026-11-23 and early 2026-11-24 (security fail-closed defaults, per-task uids, proxy scope,
  browser MCP scope, E5/E8 design, audit locking, client binding) are in `docs/handoff/2026-11-24-decisions-archive.md`.
- Further older decisions (security fixes, capture-proxy scope, per-task uids, E5, E8, client binding and others) are in
  `docs/handoff/decisions-archive-2026-11-24.md`.


## Deltas

<!-- composite:deltas · newest first; Composite rewrites this block on every merge (ej handoff merge / merge_handoff) -->
- **2026-11-24 18:23 MST** · claude-code · `ej/20261124-180334-421555` `9f7f3a9`→`7fed6bd` (2 commits) · `h-970bcd` — c-d73308 (E8 follow-up) built, in review: an optional per-profile LLM sampling pin. Migration 0028 adds nullable sampling_seed and sampling_temperature. agent/sampling.go applies them on the wire (seed on openai/ollama only; temperature only where the caller set none). The profile API validates the pin and the UI edits it. The E8 suite stamps llm_sampling as an invariant. **Next:** Land ej/20261124-180334-421555. Whoever lands second between this branch (0028) and the session-transcripts branch (0025, 0026) checks that the migration numbers are still ascending. For the first live suite run (c-f1518c), pin a seed on an openai or ollama profile so the stamp is meaningful. **Decided:** The pin fills and never overrides: a temperature set in code (the interception assessment, side questions) wins, so guard surfaces keep their deterministic settings. The seed is rejected with a 400, not silently dropped, on formats that cannot send it, so a stamped run never claims a seed the provider did not see. Both fields are applied in an outermost http transport, because the norma SDK has no seed field; the LLM recorder's raw capture therefore shows the true wire body. The migration is numbered 0028 because 0025/0026 are taken in flight and main has 0027.
- **2026-11-24 17:47 MST** · claude-code · `h-a90a9e` — c-6bcdd3 (E7, now in review): workers follow the LLM recording switch and agent config saves; DISTRIBUTED.md lists what is per-run, control-only, or a Limit. **Next:** land after c-20c400.
- **2026-11-24 17:41 MST** · claude-code · `ej/20261124-155353-b1c60a` `001e439`→`587e803` (1 commit) · `h-62b82c` — c-8cfbfe (E7 follow-up, in review): the Helm chart was installed on a 3-node kind cluster (two worker hosts) and every check is recorded in docs/DISTRIBUTED.md ("Recorded install"). The checks ran on the E7 branch and again on the merge with main (control TLS on by default, fresh database). Five defects found live were fixed, each with a test: world-writable volume roots broke the isolation self-test (tightenVolumeRoot); worker nodes read traffic capture, the global proxy and web search only at start (settings.agent_settings_revision; closes card c-7ad4da); the planner never woke when a fresh task's first settle poll was empty (settleCursor); a 0-byte jwt.key was left by a killed pod (atomic create); rollouts deadlocked under hard anti-affinity (maxSurge 0). New chart defaults: egress.denyClusterServices denies agent uids PostgreSQL and the control API (ARTEX_EGRESS_DENY_HOSTS/ALLOW_HOSTS in egress.sh, DNS stays open); the strict egress.clusterCIDRs mode; enableServiceLinks false. Lab fixtures are in deploy/lab (scripted LLM stub, kind values); the egress probe is scripts/k8s-egress-probe.sh. Cards opened: c-597b06, c-d4946f, c-6bcdd3, c-620510. **Next:** Land ej/20261124-155353-b1c60a. Other agents' branches already build on it (c-d4946f, c-597b06, c-20c400): reconcile c-20c400's "one worker settings follower" with server/worker_node_settings.go. c-620510: a real multi-machine cluster with a real model and CNI (human). **Decided:** The worker egress deny lives in the uid-scoped egress.sh filter, not a NetworkPolicy, because a NetworkPolicy is pod-wide and the root server in the same pod needs PostgreSQL. The deny is by name by default (no configuration) and by CIDR as an opt-in, because pod and Service CIDRs vary per cluster. Volume roots are tightened by the binary rather than by an fsGroup or initContainer, so bare-metal bind mounts are covered too. The LLM stub was committed as a lab fixture so the record can be reproduced without a key.
- **2026-11-24 17:40 MST** · claude-code · `ej/20261124-173106-51fdcc` at `1ef6b71`, no commits · `h-1618d4` — c-20c400 (E7, now in review): one worker settings follower. followAgentSettings (server/worker_node_settings.go) replaces syncSharedEgress/reloadSharedEgress and the agent_settings_revision key and covers c-7ad4da's syncTrafficCapture. It runs first in every dispatch poll and returns a claim-block reason: compares capture/global proxy/web search (incl. brave/tavily/keenable keys) by value every poll via an error-returning loadAgentSettings, follows llm_config_revision for LLM profiles, blocks claims while either read fails, and on a settings change drops task agents and holds claims for that poll. The branch merges the c-1d3e0a branch (whole unmerged E7 stack). Tests on Postgres 16: TestWorkerNodeFollowsAgentSettings, TestWorkerNodeFollowsLLMProfileChange, TestWorkerNodeFollowsTrafficCaptureSwitch (ported from c-7ad4da), new TestWorkerNodeBlocksClaimsWhileAgentSettingsUnreadable; verify.sh passes except the known macOS SOCKS5 127.0.0.2 test. E7 Current state entries and older Decisions moved to docs/handoff/2026-11-24-e7-distributed.md and docs/decisions.md for the size limit. **Next:** Land ej/20261124-173106-51fdcc (carries c-1d3e0a and the E7 stack) instead of ej/20261124-165722-c858f3. Do NOT land c-7ad4da's ej/20261124-161438-32780e; close it. c-c18f43 migration renumbering as branches land. **Decided:** Value comparison every poll, not a revision, for agent-baked settings: the putSettings bump was best-effort and direct writes were missed, either leaving worker agents off the recording proxy; cost is 8 one-row reads per poll. LLM config keeps its revision. An applyLLM failure after a change does not block claims: task agents are rebuilt per task from the Manager's updated settings.
- **2026-11-24 17:39 MST** · claude-code · `ej/20261124-172839-c17308` `9c6a6a7`→`4d5d6e4` (1 commit) · `h-2932ac` — c-d4946f (E7, in review): the cluster egress deny now follows moved pods. egress.sh re-resolves ARTEX_EGRESS_DENY_HOSTS/ALLOW_HOSTS every ARTEX_EGRESS_REFRESH s (chart egress.refreshSeconds, default 10) in a loop start.sh starts. When an answer changes it rewrites ARTEX_EGRESS in one iptables-restore --noflush commit (also used at start). A name that stops resolving keeps its last addresses; NAME:PORT denies one TCP port. Chart: new headless <release>-control-pods (API port on the control pod IP); the PostgreSQL Service publishes not-ready addresses. Verified live on kind (own namespace, removed): after deleting artex-postgresql-0 (.13 → .14) and the control pod (.12 → .15), k8s-egress-probe.sh showed the executor refused at the new IPs, with 0 restarts. scripts/verify.sh passed. The branch merges the unlanded c-8cfbfe branch ej/20261124-155353-b1c60a. **Next:** Land ej/20261124-172839-c17308; it carries c-8cfbfe. Optionally test on a socket-LB CNI such as Cilium KPR, where denying the control pod IP may also refuse the proxy Service (gotcha in docs/handoff/2026-11-24-e7-kind-install-egress.md). **Decided:** The re-resolve loop is in egress.sh rather than the Go server, so one code path writes the executor rules for Docker and Kubernetes. Rebuilds are atomic through iptables-restore --noflush. Unresolvable names keep their last addresses (fail closed). CIDR auto-detection was rejected: a pod has no portable source for the pod and Service CIDRs. Strict clusterCIDRs stays opt-in.
- **2026-11-24 17:39 MST** · claude-code · `ej/20261124-173102-01b458` `71c1d47`→`bedb3b5` (1 commit) · `h-50b1c4` — c-81193b (now in review): the finding detail hides controls the role is refused. Severity/triage-status/report-stage pickers need review; Start retest, Link traffic (edit/reorder/remove) and retest session links need operate. FindingRetestPanel and FindingTrafficPanel gate themselves with useCan. roles-e2e.js asserts reviewer keeps the 3 pickers without retest/link and client_read sees none; scripts/e2e/integrations.sh passes 51 checks. Branch merges the c-119206 branch (ej/20261124-171041-463f80), so landing it lands c-119206 too. **Next:** Land this branch (it carries c-119206). c-1198a2: run scripts/e2e/integrations.sh in CI once c-cc25b6's ci.yml lands. **Decided:** Gate inside the shared retest/traffic panels rather than per page, so every consumer is covered. Merged the unlanded c-119206 branch because roles-e2e.js lives there; resolved its imports-page conflict with main's Lanes/anchor work by taking main's version and re-applying the canImport gate, and biome-formatted its 80-column files. Fixed the e2e import-link locator broken by main's Lanes column (aria-label name).
- **2026-11-24 17:30 MST** · claude-code · `ej/20261124-172040-ead433` `dd269fd`→`0450525` (1 commit) · `h-8ce710` — c-597b06 (E7, in review): stale worker_nodes rows no longer pile up under Kubernetes. db.NodeRetention = 20 × NodeTTL (15 min); ListNodes hides rows silent longer than that and every node heartbeat runs PruneNodes (db/worker_nodes.go, server/node.go). New test db/worker_nodes_test.go (live + recently dead listed, stale hidden then deleted) passes on Postgres 18; existing two-node server tests pass. docs/DISTRIBUTED.md gains a "Node registry" bullet. Moved the E3 and E4 Current state detail to docs/handoff/ to keep HANDOFF.md under 60 KB. **Next:** Land this branch with E7. Optionally confirm on the kind install (c-8cfbfe) that after a few rollouts GET /api/settings/nodes shows only live + recently dead pods once 15 min pass. **Decided:** Retention is a fixed 20 × NodeTTL (15 min) rather than a setting: long enough to see a crashed pod on the nodes page, short enough that rollouts do not accumulate. Both hide in the query (correct even if pruning fails) and delete from the heartbeat (no separate sweeper; any running node cleans up).
- _Older hand-offs, a line each; `ej handoff log` has them whole:_
- **2026-11-24 18:23 MST** · claude-code · `h-c952c8` — c-43dbfc (E7, in review): worker-node run directories, spills and subagent sidechains are mirrored into PostgreSQL (new filesync package, db.TaskFiles, migration 0026) when an intent run ends. **Next:** Land ej/20261124-180332-433a2b (migrations 0025, 0026). **Issues:** opened #c-1bbeb6, #c-1e3b53.
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
- **2026-11-24 17:26 MST** · claude-code · `h-1e1166` — c-c18f43 (mechanism landed on this branch; renumbering stays open per landing): scripts/check-migration-versions.sh (+ self-test, verify.sh step, PR-only CI job migration-versions) fails a migration not numbered above main's newest, duplicates, and edited/rem… **Next:** As each branch lands, renumber its migration above main's newest (Known issues lists them: 0017_finding_classification, 0021_lesson_scope_scrub, 0023_finding_import_links, 0023_change_notify, one of…
- **2026-11-24 17:21 MST** · claude-code · `h-c66079` — c-cea6ac (now in review): explained and fixed why c-4c8fd6 task 2 was `done` mid-decomposition. **Next:** Land ej/20261124-170856-f6245f.
<!-- /composite:deltas -->

_(written by `merge_handoff`)_

## Updating this document

Keep the sections above in the standard form. Refresh **Current state** (dated stamp) and **Triage**
at the end of any session that changed code/state/config/plans; add or close GitHub issues and record
**Decisions** with their reason. `merge_handoff` writes the Deltas block and the one-line stamp — do
not hand-maintain a history chain in Current state.
