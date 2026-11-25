# HANDOFF.md — ARTEX

**Last updated:** 2026-11-24 15:58 MST, by claude-code: c-2d91eb (E2 UI follow-ups, now in review; board cards c-1198a2 and c-119206 opened) on a branch that merges the unmerged E2 UI + backend branch (ej/20261124-132955-7aef3f), so landing it lands all of E2. Earlier hand-offs are under Deltas and in `ej handoff log`.

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
- **Follow-ups:** `c-45c921` (dashboard push channel), `c-4b7989` (per-exchange traffic bulk delete/export), `c-f729e8` (approvals triage with `lib/triage`, grouped-view page advance, biome lint debt in moved code).

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

Older Current state blocks (bench subject image, E5 governance, client binding, the 2026-11-23 audit fixes,
the full Docker image, the E8 suite) are in `docs/handoff/2026-11-24-current-state-archive.md`.


## Triage (next handful)

**Agents' lane**
1. *(unclaimed)* — **E9 follow-ups** (S–M): `c-4b7989` (per-exchange traffic bulk delete/export),
   `c-f729e8` (approvals triage via `lib/triage`, biome debt). (`c-45c921`, the dashboard push, is in
   review on `ej/20261124-170157-3aeba5`.)
   "Done when" is on each card.
2. *(unclaimed)* — **SEC: per-task uid follow-ups** (S each): `c-12df03` (single-uid start-up fails on a
   socket; MCP uid not egress-filtered), `c-e70e5c` (release ended tasks at start-up; per-key lock). The "Done when" is on each card.
3. *(unclaimed)* — **E5 UI: hide role-lacking actions on existing pages** (M), card `c-92bb52`. Done = as
   reviewer and operator, no visible control answers 403. Then `c-601ec6` (chain newest-first paging, S).
   (`c-7c4715`, the client_read console, is built and in review on `ej/20261124-144554-67afc9`.)
4. *(unclaimed)* — **SEC: gorilla/websocket bump, GO-2026-6278** (XS), card `c-c10f2c`; needs network.
   Done = govulncheck clean.
5. *(unclaimed)* — **SEC: upstream-gate residuals** (M), card `c-5a6f96`: task-scoped rules on proxy
   dials, go-mitmproxy keep-alive host pinning, and the global-egress-proxy bypass. Done = a test or a decision for each.
6. *(unclaimed)* — **SEC: logout revocation** (S), card `c-a0af33`. Done = a token replayed after
   logout or a password change gets 401, by a test.
7. *(unclaimed)* — **SEC: Docker Desktop named volumes** (S), card `c-775dc2`. Done = `docker compose up`
   starts on a Mac with the isolation self-test passing; Linux bind-mount path documented.
8. *(unclaimed)* — **E1 Client-grade reporting & deliverables** (M). Turn `report/` into
   CVSS/EPSS-scored, remediation-bearing, standards-mapped deliverables (OWASP ASVS/WSTG, MITRE
   ATT&CK). Done = a report export with per-finding CVSS vector + remediation + framework tags,
   covered by tests. Card `c-8f9cca`.
8. *(unclaimed)* — **E2 UI: gate integrations controls by role** (S), card `c-119206`. Done = as reviewer
   and as client_read, no E2 control the server refuses is shown, and the finding detail has no 403.

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
- Settled decisions from 2026-11-23 and early 2026-11-24 (security fail-closed defaults, per-task uids, proxy scope,
  browser MCP scope, E5/E8 design, audit locking, client binding) are in `docs/handoff/2026-11-24-decisions-archive.md`.


## Deltas

<!-- composite:deltas · newest first; Composite rewrites this block on every merge (ej handoff merge / merge_handoff) -->
- **2026-11-24 15:58 MST** · claude-code · `ej/20261124-150516-d78c96` `f1ec47a`→`9fdfa3b` (4 commits) · `h-0548f2` — c-2d91eb (E2 UI follow-ups, now in review; board cards c-1198a2 and c-119206 opened) on a branch that merges the unmerged E2 UI + backend branch (ej/20261124-132955-7aef3f), so landing it lands all of E2. (1) Findings filter by import: db.FindingFilter.ImportID (an EXISTS on the finding_import_sources primary key), parsed from ?import_id= in the shared filter, so it applies to the list, groups, asset tree and export. The findings page shows an "Import #N" chip that clears it, and each Past imports row's New findings count links to its findings. (2) A committed browser e2e: scripts/e2e/integrations.sh (throwaway Postgres, the backend, a Next production build, Playwright) runs integrations-e2e.js against fake-github.js. 24/24 checks passed locally, including exactly the import's findings (3 of 7). (3) The imports list pages: limit/offset → {imports, total}, with TablePagination. Also fixed an E2×E5 merge gap: GET /findings/{id}/integrations was open to client_read with no binder; it now needs read, and tracker CRUD needs admin. Verified: db and server PG tests (only the known environment failures), verify.sh --web-only, and the e2e. **Next:** Land ej/20261124-150516-d78c96 (it carries the E2 backend, UI and these follow-ups). c-119206: role-gate the E2 UI controls. c-1198a2: wire scripts/e2e/integrations.sh into CI after c-cc25b6's pinned ci.yml lands. The E2 backend follow-ups c-da9198 and c-ea60a0 are unchanged. **Decided:** Merged the unmerged E2 UI branch rather than waiting. import_id matches the findings an import created; a dedup re-match does not claim them. Migration 0015 was not edited for an index (its checksum is enforced); the EXISTS on the primary key is enough. The import filter lives in the URL, not in saved preferences. E2 under E5: finding integrations need read, and tracker CRUD needs admin like notification channels; push, sync and import stay operate. The e2e uses next build + next start instead of embedding the UI. The CI job is deferred until the SHA-pinning branch lands.
- **2026-11-24 15:56 MST** · claude-code · `ej/20261124-141024-67b6b6` at `81edb00`, no commits · `h-f62f2d` — c-4c8fd6 (E6 live check, now in review): built the E6 branch into an isolated compose stack (local Ollama with prices, nginx target on its own network, OTel collector via host.docker.internal). The cost page in headless Chromium lists tasks with a correct EUR cost; Task/Provider/Role/Model regroup without a reload; the URL keeps by/days. The collector got 51 spans = 51 ledger rows, one trace per task, and all four metrics. The egress filter refuses the collector to uid 10001 while the server exports. Gap: no single trace with planner and worker together (the local model never finished a planner intent). No code changed. **Next:** Land ej/20261124-123231-b78990 (E6, not on main yet) with c-f4a69d. c-990198: fix wall time (first_ts = min(ts - latency)). c-7361b4: goals role calling after a terminal status and 32k runaway outputs. Optionally repeat the trace check with a hosted model to see planner+worker spans in one trace. **Decided:** Ran the check from the unmerged E6 branch in an isolated stack (artex-e6check, port 18797), not on the running artex-artex-1 instance and without merging E6 into this branch: E6 is in review, and merging it here would land it on main uncontrolled. Used the local Ollama (keyless) after reading the blend gateway key from the keychain was denied. The target was an nginx container the stack itself ran, the only scope entry. The stack, volumes and image were removed afterwards. **Issues:** opened #c-7361b4, #c-990198.
- **2026-11-24 15:30 MST** · claude-code · `ej/20261124-145313-129188` `f70fe41`→`33e6d6e` (1 commit) · `h-124b6f` — c-f4a69d (E6 follow-up, now in review; card c-888292 opened): the Cost & efficiency view now includes archived tasks. Archives record aggregate_stats.cost_cells (per profile, worker, model and UTC day: calls, errors, tokens, latency, first/last call), and db.CostBreakdown folds them in, so archived tasks appear in every dimension and day window. Older archives (token_profiles only) count under All by task/profile and are reported as archived_omitted elsewhere; the page says how many. GET /api/llm/cost returns a distinct `tasks` total, which the totals row uses instead of summing. New web test npm run test:cost (cost-metrics helpers) is in verify.sh; biome is clean on the cost files. This branch merges the unmerged E6 branch ej/20261124-123231-b78990, so landing it lands E6. The 2026-11-23 audit verification detail moved from Current state to docs/handoff/security-audit-2026-11-23.md to fit the size limit. Verified: TestCostBreakdownIncludesArchivedTasks and the archive round-trip on Postgres 16, verify.sh (only the known macOS 127.0.0.2 SOCKS5 test fails), the web tests, tsc and i18n. **Next:** c-4c8fd6: check the cost page in a browser against real LLM traffic, including an archived task. c-888292 (low): backfill cost_cells for archives made before this change. c-9ca57a (RBAC on cost routes) can rebase on main once this lands. **Decided:** Archived cost comes from a new additive archive aggregate (cost_cells) instead of reading cold archive files, so the view stays one DB read. Old archives are not backfilled: their per-profile totals are used where they fit and counted as omitted elsewhere, which the UI states. The server computes the distinct task total, because only it can dedupe a task across groups. first_ts/last_ts are nullable for groups with no call times.
- **2026-11-24 15:29 MST** · claude-code · `ej/20261124-145257-79147f` `ca784d2`→`942972f` (2 commits) · `h-0c292e` — E9 (c-73af97). Findings triage now takes one key per finding: a verdict bar with keys and auto-advance, stat tiles as one-click filters, and URL params on the list. Measured in Chromium on the same build, triaging 3 findings went from 11 interactions to 4. The dashboard is now tables plus KPI tiles that link to their lists, polling with a Live/Paused indicator. Assets and traffic gained bulk-bar additions. Every web/src TS/TSX file is now ≤600 lines and no `any` remains; lib/types, lib/api and lib/mock keep their paths as barrels. tsc, all web node tests, i18n and the static build pass, and a browser smoke pass over 17 routes in mock mode was clean apart from two existing mock-data gaps (settings noise card, notify). **Next:** Land this branch. It will conflict with h-683872 (c-92bb52, role-gating controls on existing pages), which edits pages split here; re-apply its gating inside the extracted _components/. Then c-45c921 (global SSE for the dashboard), c-4b7989 (per-exchange traffic bulk delete/export), c-f729e8 (approvals triage via lib/triage, grouped-view page advance, biome lint debt in moved code). **Decided:** Triage logic lives in the pure web/src/lib/triage.ts, so approvals and imports can reuse it, and the findings bindings stay thin. Verdict keys are mnemonic and gated by the review permission; navigation keys work for any role. Big lib files became barrels at their old paths so no importer changed. The dashboard stays on polling because no global push channel exists, and no backend was added in a UI pass. Traffic got no per-exchange delete, because no such endpoint exists. Follow-ups are board cards, not GitHub issues, since tracking is on the board while the repo is public. The 2026-11-23 audit entry moved to docs/handoff/ to keep HANDOFF.md under the size limit. **Issues:** opened #c-45c921, #c-4b7989, #c-f729e8.
- **2026-11-24 15:21 MST** · claude-code · `ej/20261124-144553-d5978e` `ca784d2`→`ee3d7a2` (5 commits) · `h-683872` — c-92bb52 (E5 UI follow-up, closed; c-4ee114 opened): every existing web page now hides or disables the mutating controls the session's role lacks, per server/rbac_policy.go. Admin: guardrail and platform config. Operate: task, chat, traffic, assets, workspace and skills actions, plus operate-only reads. Review: approval decisions and finding triage. Failed calls show errorText. Every touched file over 600 lines shrank by extraction (for example tasks/page 3599→3221, intercept 1042→735, agent-editor 1234→783). rich() is now one lib/rich-text.tsx, and Allow/Deny is one gated ApprovalDecisionButtons. Verified: tsc and scripts/verify.sh --web-only pass. A headless Chromium probe against a real backend on Postgres 16 covered 23 pages as reviewer and operator: no 403 on load, and no enabled control the role lacks. **Next:** c-4ee114: re-probe with seeded rows (MCP, channels, LLM profiles, pending approvals, traffic exchange sheet, files, skills, hooks), hide the assets checkboxes without operate, switch credentials-tab to errorText, and fix biome lint in the extracted files. Then c-7c4715 (client_read console) and c-601ec6. **Decided:** Admin-only forms are shown read-only (a disabled fieldset with Save hidden) rather than hidden, so reviewers can still read the configuration. Pages that only answer 403 to read (LLM records, chat messages, credentials) skip the fetch and show a notice. Agent triggers split by endpoint: creating one and the policy are admin (/api/agents), while editing or deleting one is operate (/api/triggers/{id}). Over-600-line files were reduced by extracting components rather than grown.
- **2026-11-24 14:52 MST** · claude-code · `ej/20261124-142817-7e3c57` `b89fab2`→`dc77cdc` (6 commits) · `h-6ea377` — c-13e86c fixed: the activity stream 401 after a console login predates E5. Since the cookie move, getToken() is null, but sseUrl sent non-production builds cross-origin to :8787, where the EventSource carried no credential. Now web/src/lib/sse.ts keeps every stream same-origin (NEXT_PUBLIC_SSE_BASE removed), next.config turns dev gzip off (it buffered proxied SSE), and server openSSE writes ': open' when a stream opens (the dev rewrite held the headers until the first body byte, the 20 s ping). PG test server/activity_stream_auth_pg_test.go covers legacy ARTEX and a console user with the cookie only, and no session → 401. Also web/src/lib/sse.test.mjs. Browser-checked on the E5 stack (61d777 + this change, next dev + PG): both logins connect, no Reconnecting. HANDOFF: the stale E5 entry is condensed (it is on main as #31), and the 2026-11-23 audit operator list moved to docs/handoff/ so Current state fits its limit. **Next:** Land with or after the E5 stack branches (c-f4115c, c-01377e). The patch applied cleanly on ej/20261124-140004-61d777, where the test passes. **Decided:** SSE is same-origin only. Credentialed CORS for dev origins was rejected because cookies are not port-isolated, so a lab target on another port of the same host could read the streams.
- **2026-11-24 14:24 MST** · claude-code · `ej/20261124-141028-5cc420` `5c56fe9`→`d42cd76` (1 commit) · `h-8a2866` — c-ca2bd0 (E4 follow-up, now in review; board card c-aed5fd opened): an operator can choose a task's methodologies. Migration 0022 adds tasks.methodologies TEXT[] (empty = automatic, unchanged). The create request and create form (methodology-picker.tsx, a table fed by the new GET /api/methodologies) set it. Ids are matched loosely and stored canonical, and an unknown id returns 400. db.TaskMethodologies uses the choice when set, so the coverage API and the planner block show only the chosen checklists. The choice is carried through archive/restore and returned on the task DTO. This branch merges the unmerged E4 branch ej/20261124-123222-d3dc8a. Verified on Postgres 16: TestTaskMethodologiesOperatorChoice, the archive round-trip, TestCreateTaskMethodologies, the full go test ./... (only the known macOS 127.0.0.2 SOCKS5 test fails), web tsc and i18n. The form was not checked in a browser. **Next:** Land the E4 branch (ej/20261124-123222-d3dc8a) and this branch together. Migration 0022 must land after the 0015–0021 branches. c-aed5fd: a "none" choice and editing the choice after creation. Check the create form's picker in a browser. **Decided:** An empty list means automatic, so other create paths and existing tasks keep the goal-word selection. A non-empty list replaces the automatic set rather than adding to it, which also covers excluding a checklist. An unknown id is refused at creation, so a typo never silently falls back to automatic. Stored ids whose checklist was later removed are skipped. The migration is numbered 0022 because 0015–0021 are taken on open branches.
- _Older hand-offs, a line each; `ej handoff log` has them whole:_
- **2026-11-24 13:51 MST** · claude-code · `h-ed4135` — c-597eb6 (now in review; follow-up card c-2d91eb opened): built the E2 web UI, and merged the unmerged E2 backend branch ej/20261124-055326-6bcfa9 so both land together. **Next:** c-2d91eb: an import_id filter on the findings list so each import links to its findings; commit the browser e2e scenario as a script; pagination for the imports list.
- **2026-11-24 13:50 MST** · claude-code · `h-2d8bcf` — c-f4115c (now in review): closed the six E5 artifact lifecycle gaps on a branch that merges the E5 + client_read branch (ej/20261124-110647-182fa9, PR #16). **Next:** Land PR #16 (E5) and c-5b5421's branch first, then this branch.
- **2026-11-24 13:46 MST** · claude-code · `h-7f6f23` — c-869050 (now in review): web UI for E5. **Next:** c-92bb52: gate mutating controls on existing pages by role (reviewer currently sees Interception Rules edit controls). **Issues:** opened #c-601ec6, #c-7c4715, #c-92bb52.
- **2026-11-24 13:36 MST** · claude-code · `h-59bfe6` — c-9b48b7: confirmed go-mitmproxy keep-alive host pinning is still unfixed through v1.9.5 (= upstream main 07e0ca5), and no upstream report exists. **Next:** Ian: approve filing (human lane).
- **2026-11-24 13:26 MST** · claude-code · `h-4efcbc` — c-965b0e done and c-775dc2 landed: merged the unmerged branch ej/20261123-192351-fab3f4 (named volumes, exit 78, start.sh idle hold e1d7a27) and added an image HEALTHCHECK that is unhealthy while start.sh holds on an isolation refusal (root-only marker /run/a… **Next:** Run docker-compose.bind.yml on a real Linux host (human lane).
- **2026-11-24 13:25 MST** · me · `h-7df71f` — c-7e0087 (now in review): each ARTEX server's agent egress floor has its own chain, ARTEX_AGENT_FLOOR_<exec uid>, with jumps tagged artex-floor-<exec uid>. **Next:** Land c-b73542 (ej/20261124-113533-f16c00), then c-02e554 (ej/20261124-122709-d136a7), then this branch.
- **2026-11-24 13:13 MST** · claude-code · `h-7925fc` — c-9ca57a built and verified on branch ej/c-9ca57a-llm-cost-rbac (pushed, commit 7d6b4cf, no PR), stacked on the client_read branch ej/20261124-110647-182fa9 (carries E5 / PR #16) and E6 (PR #28), because neither has landed on main. **Next:** After PR #16, the c-5b5421 branch and PR #28 land: merge main into ej/c-9ca57a-llm-cost-rbac (only HANDOFF.md should conflict; the stacked branch kept stale HANDOFF copies), run scripts/verify.sh and…
- **2026-11-24 13:03 MST** · claude-code · `h-ce7d36` — E3 (c-d25ff9, now in review): built the RSI.md Loop C benchmark gate and Loop B lesson graduation, with ARTEX_RSI_LOOP off by default. **Next:** c-0d277f (hardening: a server-side per-task arm ledger, suite pre-registration, sync for retired adopted lessons). **Issues:** opened #c-0d277f, #c-772190, #c-936308, #c-c89f5b.
<!-- /composite:deltas -->

_(written by `merge_handoff`)_

## Updating this document

Keep the sections above in the standard form. Refresh **Current state** (dated stamp) and **Triage**
at the end of any session that changed code/state/config/plans; add or close GitHub issues and record
**Decisions** with their reason. `merge_handoff` writes the Deltas block and the one-line stamp — do
not hand-maintain a history chain in Current state.
