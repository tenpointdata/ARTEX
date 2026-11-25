# HANDOFF.md — ARTEX

**Last updated:** 2026-11-25 by claude-code: c-2e4034 merged with main; c-da9198 had already landed via #80, so this adds only `TestTicketSyncDefaultCapLoopsToCompletion`. Earlier hand-offs are under Deltas and in `ej handoff log`.

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

**2026-11-25 — E2 ticket sync lock/cursor/background (`c-da9198`) is on main via #80** (the c-c2bdc8 webhook
branch carried the cherry-pick of `0f8cd35f`); card `c-2e4034` added only
`TestTicketSyncDefaultCapLoopsToCompletion` (151 links: sync while `more` reconciles each link once in 2
calls, the UI Sync loop's contract). `ticketSyncRunner` starts on the control node only (`!worker` block
in server.go). The UI half (`c-224317`: interval field, Sync loops while `more`, 409 = "already running") is
**not on main yet**; it is on `ej/20261124-191201-980dda`. Until it lands, the UI's Sync does one capped call.

**2026-11-24 — Role route guard + c-92bb52 gates (`c-bd9b7a`, in review on `ej/20261124-191154-865292`).**
`scripts/e2e/role-probe.sh` now **passes for reviewer, operator and client_read** (no /api >= 400, no
request needing a missing permission). The probe itself (`c-80fe4e`) is on this branch too.
- `RouteGuard` (`web/src/app/(main)/_components/route-guard.tsx`, pure resolver `web/src/lib/route-access.ts`
  + `route-access.test.mjs`) takes a path's permission from the sidebar's `need` (detail pages inherit their
  parent; uncovered paths need read) and sends a role lacking it to `landingPath` without mounting the page.
- c-92bb52's gating (`ej/20261124-144553-d5978e`) could not be git-merged: its one commit both gated and split
  files, conflicting with main's E9 split in ~40 files. Its intent was **re-applied onto main's files** here, so
  this branch supersedes it: **do not land `ej/20261124-144553-d5978e`.** Not ported: its `errorText(e)` swaps.
- Beyond the branch: task Overview `TokensCard` (`/api/llm/records/by-model`) needs operate; integrations page
  (tracker create/edit/enable/delete admin, sync operate) and tracker reads on findings pages skipped for client_read.
- Run the probe with spare ports when another harness is up: `ARTEX_E2E_API_PORT=38787 ARTEX_E2E_UI_PORT=35173
  ARTEX_E2E_PG_PORT=46432 ARTEX_E2E_PROXY_PORT=38788 scripts/e2e/role-probe.sh`.
- Findings are in `docs/handoff/2026-11-24-role-probe-findings.md`.
- `integrations.sh` now runs the backend from its work dir (it used to write `jwt.key`/`secret.key` into the
  caller's cwd) and takes `ARTEX_E2E_PROXY_PORT`.

**2026-11-24 — SOCKS5 auth regression after #79 fixed (`c-831dfa`).** `checkCredential` looked up a
scope-token password in the local registry only, so worker-minted tokens that the remote resolver holds got
RFC 1929 0x01. It now uses `Traffic.resolveScope` (local, then remote). `TestSOCKS5RemoteScope` now expects
#79's refusal stage: unknown or ended tokens get 0x01 at auth, and the bare pace marker passes open-mode auth
and then gets 0x02 at CONNECT. `TestManagerSOCKS5UDP…` was failing only because 127.0.0.2 can't be bound on
macOS. It now skips there and passes on Linux with real PG (golang:1.26.11-bookworm, private Docker network).
`scripts/verify.sh` Go steps pass. Its web i18n step fails on #80's strings (`c-be8a4f`).
`c-1a74a4` was the same fix made in parallel; its merge kept main's code and added one assertion (an
unresolvable worker token as `egress.ScopeUser` gets 0x01).

**2026-11-24 — E2 push updates via signed webhooks (`c-c2bdc8`; in review on `ej/20261124-191156-37e38c`).**
This branch also carries the c-da9198 commit (per-tracker advisory lock, capped sync cursor, background
sync; cherry-picked from `ej/20261124-135159-6d9961`, which main did not have yet), because the webhook
runs under that lock. `POST /api/hooks/trackers/{id}` (`server/ticket_webhook.go`) is exempt from
`requireAuth` and authenticated by an HMAC-SHA256 of the body under the tracker's optional
`webhook_secret` (sealed in the tracker config, masked, scrubbed; ≥16 chars). GitHub: `X-Hub-Signature-256`,
`issues` events, repo must match. Jira Cloud: `X-Hub-Signature` (admin webhook with secret), project must
match. The payload only names the ticket; `ticketSyncer.sync` reads the state from the tracker API under the
lock (busy → 202 `deferred`). Uniform 401, 1 MiB, 4 in flight, 10-min delivery-id window, `ticket.webhook`
audit entries. A destination change re-asks only the outbound credentials (`ticketing.OutboundSecretKeys`).
The tracker sheet has a Webhook secret field and shows the URL. Docs: `docs/integrations.md`. No migration.
Verified against a throwaway Postgres 18: `TestGitHubWebhookClosesFinding`, `TestGitHubWebhookRejectsBadSignature`,
`TestWebhookNeedsConfiguredSecret`, `TestWebhookDeferredWhileTrackerLocked` (held lock → 202 `deferred`, finding
unchanged, redelivery of the same id then processed), the ticketing webhook unit tests, all existing E2 ticket tests, web
`test:integrations`. An independent security review found no bypass or leak; its slowloris, timing, dedup-order
and id-length findings are fixed (10 s read deadline, every refusal hashes the body). `scripts/verify.sh`:
only the two known environment failures (`TestConnectWithoutPasswordRealPG` on a trust-auth Postgres,
`TestManagerSOCKS5UDPOptInAndAssetBlockRuleDropsDatagram` without 127.0.0.2) (`TestSOCKS5RemoteScope`, then
also failing on main, is fixed by `c-831dfa`). Not verified: a live
GitHub/Jira delivery (needs a publicly reachable instance), the sheet in a browser.

**2026-11-24 — RSI proposer (`c-c89f5b`, on main via #77) + model drafter (`c-0aaa24`, in review on
`ej/20261124-203755-708a1e`).** `cmd/artex-rsi-propose`
(core `rsi/propose`) drafts goals/planner/worker body edits from closed tasks when a rule's signal holds
on ≥2 tasks across ≥2 scopes, inside the round budget; `--draft-model` (off by default) lets a model
rewrite the body through the same checks; the drafter must not be the judge. `verify.sh` there fails only
on `TestSOCKS5RemoteScope` (since fixed, `c-831dfa`) and the web i18n check (`c-be8a4f`). Detail:
`docs/handoff/2026-11-24-e3-rsi-loops.md`. Not checked: a real instance (`c-4bbf29`).

**2026-11-24 — LLM sampling pin (`c-d73308`, E8; on main via #69).** Migration 0028 adds nullable
`llm_profiles.sampling_seed`/`sampling_temperature`; `agent/sampling.go` splices them into the wire body
(seed on openai/ollama only; temperature only where the caller set none). Suite stamps `llm_sampling`.
Verified against a real PostgreSQL: `db` package all green (incl. `TestProfileSamplingRoundTrip`),
`server/llm_sampling_api_test.go` (POST/GET, 400 on seed for anthropic/openai-responses and on temperature
outside 0..2). Not done: a live run against a real provider (folds into `c-f1518c`).

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

**Older 2026-11-24 blocks** (E7 install kit/kind/TLS/stale rows/proof, dashboard push, SSE 401, E4 methodology choice, client_read console) moved to `docs/handoff/2026-11-24-current-state-archive.md`.

**2026-11-24 — E5 web UI (`c-869050`, in review).** Detail moved to `docs/handoff/2026-11-24-e5-web-ui.md`.

**2026-11-24 (c-d25ff9, E3)** — RSI Loop C gate and Loop B lesson graduation built, flag off (`ARTEX_RSI_LOOP`). Detail moved to `docs/handoff/2026-11-24-e3-rsi-loops.md`.

**2026-11-24 (c-c0bb07, E4)** — OWASP WSTG 4.2 and API Top 10 2023 checklists drive coverage (`methodology/`). Detail moved to `docs/handoff/2026-11-24-e4-methodology-coverage.md`.

**2026-11-24 (c-4b7989, E9 follow-up)** — Traffic bulk bar acts on exactly the selected exchanges:
`POST /api/traffic/bulk-delete` and `POST /api/traffic/bulk-export` (`{"ids": [...]}`, max 5,000; RBAC
`operate`; audited as `traffic.bulk_delete` / `traffic.bulk_export`). Store: `traffic.DeleteIDs` and
`PageQuery.IDs` (`traffic/bulk_ids.go`). UI: "Export selected as HAR" and "Delete selected" (confirm) beside
"Delete targets of selection". Documented in `docs/traffic-har.md`. Tested in `traffic/bulk_ids_test.go` and
`server/traffic_bulk_test.go` (PG). On macOS, `TestManagerSOCKS5UDPOptInAndAssetBlockRuleDropsDatagram`
fails because it binds 127.0.0.2; that is environmental, not this change.

Older Current state blocks (bench subject image, E5 governance, client binding, the 2026-11-23 audit fixes,
the full Docker image, the E8 suite) are in `docs/handoff/2026-11-24-current-state-archive.md`.


## Triage (next handful)

**Agents' lane**
0. *(unclaimed)* — **`go test ./traffic` fails on main** (S), card `c-81b834`: `TestSOCKS5RemoteScope`
   gets auth reply 0x01 since #79's SOCKS5 auth; it stops `scripts/verify.sh` before the web steps. Done = cause found
   (without loosening auth), verify.sh green.
1. *(unclaimed)* — **E7: worker nodes miss other runtime control-node state** (M), card `c-6bcdd3`: LLM profiles,
   MCP servers, custom tools and similar are rebuilt only at start. Done = each one is followed or listed in
   DISTRIBUTED.md Limits.
2. *(in review)* — **E9 follow-ups**: `c-4b7989` (per-exchange traffic bulk delete/export) is built on
   `ej/20261124-191203-08f678`. (`c-f729e8`, approvals triage + grouped advance + biome errors, in review on `ej/20261124-175807-17ca91`.) (`c-45c921`, the dashboard push, is in
   review on `ej/20261124-170157-3aeba5`.)
   "Done when" is on each card.
3. *(unclaimed)* — **SEC: per-task uid follow-ups** (S each): `c-12df03` (single-uid start-up fails on a
   socket; MCP uid not egress-filtered), `c-e70e5c` (release ended tasks at start-up; per-key lock). The "Done when" is on each card.
4. *(unclaimed)* — **E5: chain newest-first paging** (S), card `c-601ec6`. (`c-bd9b7a` role guard + c-92bb52
   gates are in review on `ej/20261124-191154-865292`; `c-7c4715`, the client_read console, is in review on
   `ej/20261124-144554-67afc9`.)
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
- **Role probe (RBAC in the UI):** `scripts/e2e/role-probe.sh` (about 5 minutes, same harness).
  `ARTEX_E2E_ROLES=reviewer` and `ARTEX_E2E_PAGES=/system` narrow a run. With `ARTEX_E2E_KEEP=1`, per-role
  JSON reports and screenshots land in the work dir's `shots/`. Use free ports when another dev backend
  holds 28787/28788.
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
  [`c-da9198`] scheduled sync plus a per-tracker lock (on main via #80; UI `c-224317` pending on
  `ej/20261124-191201-980dda`); [`c-c2bdc8`] signed webhooks (on main via #80); [`c-ea60a0`] move agent `scan_ingest` onto
  `scanimport`, normalize default ports, and settle two dedup edge cases.
- **E3** [`c-d25ff9`] — gate built (in review). Follow-ups: hardening `c-0d277f`, UI `c-936308`, first live run `c-772190`, proposer/critic `c-c89f5b` (on main; follow-ups `c-4bbf29` live run, `c-0aaa24` model drafter in review).
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
  image from a clean clone, [`c-d73308`] per-profile sampling pin (landed, #69; live check rides on `c-f1518c`).
- **E10** [`c-72dd9d`] Code-security lane: built 2026-11-24, in review (see Current state). Follow-ups:
  [`c-d059ae`] lane in UI and exports (built, in review on `ej/20261124-170322-736db7`);
  [`c-7651e5`] stat cards + fact-node link; [`c-c978e3`] SAST-guided runtime verification;
  [`c-5c4009`] tracker RBAC (E2×E5).

## Known issues and gotchas
- Ticket push/sync of one tracker refuses with 409 while another holds `ticket_sync:<id>`; callers retry rather than wait. The sync cursor is per process (in memory), so a restart or a second instance starts a new pass (an extra visit per link, never a skipped one).
- **PG-backed Go tests on a Mac with a throwaway cluster:** `ARTEX_PG_DSN` must be a `postgres://` URL
  (a key=value DSN makes every PG test skip silently), and Homebrew `postgres` aborts at start
  ("postmaster became multithreaded") unless `LC_ALL` is set. Recipe: `export LC_ALL=en_US.UTF-8;
  initdb -D /tmp/pg/d -U artex -A trust; pg_ctl -D /tmp/pg/d -o "-p 55439 -k /tmp/pg" start;
  ARTEX_PG_DSN=postgres://artex@localhost:55439/artex?sslmode=disable go test ./db ./server`. Under trust auth
  `TestConnectWithoutPasswordRealPG` fails by design, and `TestManagerSOCKS5UDP…` needs 127.0.0.2 (absent on macOS).

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

- **2026-11-24 c-cce755 (RSI action roles):** `POST /api/rsi/proposals/{id}/rollback` needs `admin`
  (one exact rule in `server/rbac_policy.go`): it re-points an agent's live prompt version, the same effect as
  `PUT /api/agents/...`. No password, so the owner (admin) keeps one-step rollback. Reject, propose and evidence stay
  `operate` (the default): reject applies only before adoption and changes no live prompt, and the proposer and bench
  harness post with operator tokens; adopt stays `operate` + owner password. Not a blanket `/api/rsi` mutate rule,
  which would lock the proposer out. Reverses the earlier "reject and rollback need only a session" line in `RSI.md`.
  The RSI page (c-936308, unmerged branch `ej/20261124-192153-48c72c`) must gate the Rollback button on
  `useCan("admin")` and keep reject on `useCan("operate")` (follow-up c-a842a0).
- **2026-11-24 c-c2bdc8 (tracker webhooks):** a delivery is a hint, never state: it names a ticket and the
  normal sync reads the tracker API, so forged or replayed payloads cannot set a finding status. Jira is
  supported only through Jira Cloud's HMAC-signed admin webhooks (`X-Hub-Signature`); a secret in the URL was
  rejected (it lands in proxy/access logs) and Connect JWT is out of scope. The webhook secret is not an
  outbound credential, so a destination change keeps it. Every refusal is the same 401 so tracker ids are not
  probeable.

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
- **2026-11-24 20:51 MST** · claude-code · `ej/20261124-203755-708a1e` `30a1782`→`dac6220` (1 commit) · `h-8ecb71` — c-0aaa24 (in review): propose.Drafter hook (Input.Drafter, nil = fixed guidance). Build drafts in rank order after the transfer filter and the fixed-body ValidateEdit, so only in-space roles reach the model and a refused body frees its slot. DraftRequest carries code-owned text, counts and the active body only. Output is refused (never repaired) on template-action changes, stray delimiters, empty, unchanged or prior bodies, or a ValidateEdit failure. WriteArm re-checks. CheckDrafter refuses a drafter equal to the judge, or any drafter when no judge is named. CLI: --draft-model/--draft-provider/--draft-url/--judge-model, $ARTEX_DRAFT_API_KEY. New agent.Complete (tool-less one-shot) and agent.FrameData (exported data envelope). Arms carry drafted_by. Hostile-output tests in rsi/propose/drafter_test.go and cmd/artex-rsi-propose/drafter_test.go, mutation-checked. Older Current state blocks moved to docs/handoff/. **Next:** Land ej/20261124-203755-708a1e. Then c-4bbf29: run a real-model --draft-model dry run on a bench instance and send one drafted arm through eval_suite with a different judge. Optional: cap the share of a body a rewrite may delete, if real drafts strip useful content. **Decided:** A refused drafted body skips the rule instead of falling back to the fixed text, so the plan says why and the operator can rerun without the flag. Drafting happens after budget ranking to keep model calls within the round's slots. Drafted proposals are tagged "[drafted rule <id>]" in the rationale, because a rewrite need not contain the guidance text the old pruner matches. An unknown judge counts as a refusal, since independence cannot be checked. Template actions must stay exactly the same set as in the active body.
- **2026-11-24 19:39 MST** · claude-code · `ej/20261124-191154-865292` `e9fbefe`→`af602b8` (12 commits) · `h-be59c2` — c-bd9b7a (in review on ej/20261124-191154-865292): RouteGuard in the (main) layout (route-access.ts resolver from the sidebar's need, tested) sends a role to its landing page without mounting a page it can't see; c-92bb52's useCan gating re-applied onto main's E9 file layout (system/*, tasks, traffic, chat, findings, assets, records, sync, workspace, agent editor, approvals); task Overview by-model card needs operate; integrations page and findings tracker reads gated. scripts/e2e/role-probe.sh passes for reviewer, operator and client_read; scripts/verify.sh passes. The branch also carries the role probe (c-80fe4e). **Next:** Land ej/20261124-191154-865292 (carries c-80fe4e and c-bd9b7a). Do NOT land ej/20261124-144553-d5978e (c-92bb52): superseded. Then c-601ec6. Optional: port c-92bb52's errorText(e) swaps; hide the Overview "click Re-run all" footnote without operate. **Decided:** c-92bb52's single commit (gating + file splits) conflicted with main's E9 split in ~40 files, so its gating intent was ported onto main's files instead of merging; the traffic row selection stays for read roles (Copy URLs) while the sheet opens only with operate. Route permission comes from the sidebar's need so nav and guard share one source.
- **2026-11-24 19:35 MST** · claude-code · `ej/20261124-192153-48c72c` `5e3f5b4`→`96e3134` (1 commit) · `h-8712ef` — c-936308 (in review): /system/rsi page — proposals table with newest verdict, expandable per-gate rates (with n), paired gained/lost, regressions with targets, adoption log, prompt-history link; inline Adopt (password + newest evidence_id), Reject, Roll back (reason); ARTEX_RSI_LOOP state from /api/rsi/status. GET /api/rsi/proposals now returns latest_evidence (db.LatestRSIEvidence). Verified with tsc, biome, verify.sh, TestRSIGateEndToEndPG on Postgres 17, and headless Chromium: adopt then rollback in 2 clicks each. Issues: opened c-cce755. **Next:** Land ej/20261124-192153-48c72c. Then c-cce755 (decide whether RSI reject/rollback need admin). The live end-to-end run stays c-772190. **Decided:** Newest evidence is attached to the list response (one DISTINCT ON query) rather than a per-row detail call; the field is additive, so existing API users are unaffected. The loop flag is shown, not toggled: turning it on stays an owner decision in the environment (RSI.md rule 5). Actions are shown to operate sessions to match the current server RBAC; c-cce755 tracks whether that should be admin.
- **2026-11-24 19:31 MST** · claude-code · `ej/20261124-192202-fbe9e3` `5e3f5b4`→`077451f` (1 commit) · `h-8d9748` — c-c89f5b (in review): RSI Loop C proposer. cmd/artex-rsi-propose + rsi/propose read closed tasks' outcome/lessons and /api/skill-gaps, keep only rules whose signal holds on >=2 tasks across >=2 scopes, append fixed guidance to one goals/planner/worker body, validate with rsi.ValidateEdit (draft and write time), write arm files (proposed_from task:<id>, rationale) + plan.json within the round's remaining status budget; --post registers them. Refuses while ARTEX_RSI_LOOP is off; client can only GET and POST /api/rsi/proposals. Tail markers moved to agent.CodeOwnedTailMarkers; status reports max_open. **Next:** Land ej/20261124-192202-fbe9e3. Then c-4bbf29: real-instance dry run and one proposal through eval_suite + evidence (fold with c-f1518c). Later c-0aaa24 (model-drafted critic). **Decided:** Go, not Python, so the proposer calls rsi.ValidateEdit directly. Guidance is a fixed code-written table and no live task text enters a body (live output is untrusted). The proposer stays in the newest round and opens the next only when --round names it, since advancing closes the current round. Dry run is the default. Loop flag is required even for a dry run (checked at start and before posting). **Issues:** opened #c-0aaa24, #c-4bbf29.
- _Older hand-offs, a line each; `ej handoff log` has them whole:_
- **2026-11-24 19:12 MST** · claude-code · `h-43b8d4` — c-80fe4e (in review): scripts/e2e/role-probe.sh, a role probe on the integrations.sh harness. **Next:** Land c-92bb52 (ej/20261124-144553-d5978e; main still lacks it, so the probe fails widely on main). **Issues:** opened #c-bd9b7a.
- **2026-11-24 19:11 MST** · claude-code · `h-e9f875` — c-1e3b53 (E7, in review): before worker.Execute a distributed node pulls the stored files of the intent's run dir, the task's spills and the session's sidechains (pullRunFilesFrom), makes the dirs as the run does, then hands what it pulled to the executor (sh… **Next:** Land ej/20261124-185425-cbc1cf instead of ej/20261124-180332-433a2b (it carries it).
- **2026-11-24 19:02 MST** · claude-code · `h-b23a91` — c-ccc7a8 (in review): this branch merges c-6bcdd3 (ej/20261124-172031-c2bb7b, carrying c-20c400/c-d4946f/c-8cfbfe and the E7 stack) and c-677166 (ej/20261124-163818-870578, proxy-hop TLS), so landing it lands both. **Next:** Land ej/20261124-185428-8c1cc4; do not land ej/20261124-163818-870578 or ej/20261124-172031-c2bb7b separately afterwards.
- **2026-11-24 18:49 MST** · claude-code · `h-7cf4df` — c-1bbeb6 (E7, in review): worker nodes sweep their local tasks/<T>, spill/tasks/<T> and transcripts/exp<E>-… of deleted, archived or vanished tasks on start-up and every 10 min (server/worker_node_sweep.go, db.RetiredTasks in db/task_retirement.go); stageTask… **Next:** Land ej/20261124-184127-104862 (card c-1bbeb6 in review).
- **2026-11-24 18:23 MST** · claude-code · `h-0668f2` — c-67993f done (closed; c-ccc7a8 opened): the E7 TLS proxy hop was checked on a 3-node kind cluster with an agent-driven (stub LLM) task. **Next:** Land c-677166 (ej/20261124-163818-870578, fork h-c56630): rebase onto main and unify the relay helper with c-6bcdd3's relayConns (c-ccc7a8).
- **2026-11-24 18:23 MST** · claude-code · `h-970bcd` — c-d73308 (E8 follow-up) built, in review: an optional per-profile LLM sampling pin. **Next:** Land ej/20261124-180334-421555.
- **2026-11-24 18:23 MST** · claude-code · `h-c952c8` — c-43dbfc (E7, in review): worker-node run directories, spills and subagent sidechains are mirrored into PostgreSQL (new filesync package, db.TaskFiles, migration 0026) when an intent run ends. **Next:** Land ej/20261124-180332-433a2b (migrations 0025, 0026). **Issues:** opened #c-1bbeb6, #c-1e3b53.
- **2026-11-24 18:21 MST** · claude-code · `h-9bfe0c` — c-601ec6 (in review): GET /api/intercept/history/chain takes order=desc and before_seq (db.AuditFilter Desc/BeforeSeq; ascending stays the default). **Next:** Land ej/20261124-180916-b60b04.
- **2026-11-24 18:13 MST** · claude-code · `h-6293db` — c-f729e8 (in review), on a branch that merges the c-45c921 branch (ej/20261124-170157-3aeba5, which carries E9), so landing it lands both. **Next:** Land ej/20261124-175807-17ca91 (carries c-45c921 and E9).
- **2026-11-24 18:12 MST** · claude-code · `h-bb3c3b` — c-4ee114 done (closed; c-80fe4e opened). **Next:** c-80fe4e: probe the stream-messages view, the detail pages and client_read.
- **2026-11-24 18:09 MST** · claude-code · `h-8a0153` — c-888292 (E6 follow-up, now in review): archives made before cost_cells are backfilled on start. **Next:** Land ej/20261124-175804-41533a in place of ej/c-990198-wall-time (it carries E6, c-f4a69d and c-990198).
<!-- /composite:deltas -->

_(written by `merge_handoff`)_

## Updating this document

Keep the sections above in the standard form. Refresh **Current state** (dated stamp) and **Triage**
at the end of any session that changed code/state/config/plans; add or close GitHub issues and record
**Decisions** with their reason. `merge_handoff` writes the Deltas block and the one-line stamp — do
not hand-maintain a history chain in Current state.
