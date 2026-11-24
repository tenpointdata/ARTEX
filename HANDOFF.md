# HANDOFF.md — ARTEX

**Last updated:** 2026-11-24 11:23 MST, by claude-code: c-5b5421 (now in review): client_read users are bound to companies. Earlier hand-offs are under Deltas and in `ej handoff log`.

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
  while a LAN router, another container and the internet stay reachable. The full image did not
  finish building on a slow link (`c-a6891b`).
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
  that dials with `Interceptor.DialSOCKS5`. This applies global rules only; task rules are tracked on `c-5a6f96`.
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
  Root tests in Docker: build the test binary into a 0755 dir (`go test -c -o /t/server.test`). The
  default 0700 go-build dir makes the isolation self-tests fail on `main` too.

## Triage (next handful)

**Agents' lane**
1. *(unclaimed)* — **SEC: per-task uid follow-ups** (S each): `c-12df03` (single-uid start-up fails on a
   socket; MCP uid not egress-filtered), `c-dd160a` (spill dirs readable across tasks), `c-e70e5c`
   (release ended tasks at start-up; per-key lock). The "Done when" is on each card.
2. *(unclaimed)* — **E5: web UI for users / authorization / chain / client bindings** (M), card `c-869050`.
3. *(unclaimed)* — **SEC: gorilla/websocket bump, GO-2026-6278** (XS), card `c-c10f2c`; needs network (PR #17 open).
   Done = govulncheck clean.
4. *(unclaimed)* — **SEC: upstream-gate residuals** (M), card `c-5a6f96`: task-scoped rules on proxy
   dials, go-mitmproxy keep-alive host pinning, and the global-egress-proxy bypass. Done = a test or a decision for each.
5. *(unclaimed)* — **E1 Client-grade reporting & deliverables** (M). Turn `report/` into
   CVSS/EPSS-scored, remediation-bearing, standards-mapped deliverables (OWASP ASVS/WSTG, MITRE
   ATT&CK). Done = a report export with per-finding CVSS vector + remediation + framework tags,
   covered by tests. Card `c-8f9cca`.
6. *(unclaimed)* — **E4 Methodology-driven coverage** (M), card `c-c0bb07`. Extend the coverage ledger to score
   coverage against a codified methodology checklist, not only asset touch. Done = planner can
   read a methodology and the coverage graph shows checklist completion.

**Human lane**
- **Full-image Docker check** (card `c-a6891b`): needs a fast link for the recon-toolbox build; repeat
  the egress probes and the login script there (the slim-image run passed).
- Revisit tracking: E1–E10 are on the private Composite board (decided 2026-11-23, repo is public).
  File GitHub issues only if the repo goes private or you want them public.
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
- **Verify (reproduces CI):** `scripts/verify.sh` (fast set, no Docker); `--full` adds `go test -race`
  + web static build; `--security` runs `govulncheck ./...`.
- **Benchmark harness:** `docker-compose.bench.yml` + `scripts/benchmark`.

## Open work

Expansion items E1–E10 from the plan (see Decisions for the full document). Tracked privately on the
Composite ARTEX board, not as GitHub issues, because the repo is public (decision 2026-11-23).
Card ids in brackets:

- **E1** [`c-8f9cca`] Client-grade reporting & deliverables (scoring, remediation, framework mappings, branded export).
- **E2** [`c-6def05`] Finding/ticketing integrations out (Jira, GitHub issues, DefectDojo) and more ingest in (Nuclei, Nmap, ZAP/Burp, Semgrep).
- **E3** [`c-d25ff9`] Loop-B lesson library graduation + Loop-C benchmark gate per the `RSI.md` contract (owner-approval, immutable surfaces).
- **E4** [`c-c0bb07`] Methodology-driven coverage (OWASP WSTG / API Top 10 as structured checklists).
- **E5** [`c-669406`] Engagement governance. The backend is done on `ej/20261124-055929-bd2253`. Follow-ups:
  `c-5b5421` (client binding), `c-869050` (UI), `c-f4115c` (lifecycle gaps).
- **TEST** [`c-3157b4`] `TestGateOffTaskIsUnchanged` fails on main with Postgres (also fails before E5).
- **E6** [`c-cb0093`] Cost/observability dashboard + OpenTelemetry export, building on existing context telemetry.
- **E7** [`c-d60149`] Distributed/remote workers (queue-backed intents) for larger engagements; Helm chart.
- **E8** [`c-5487e8`] Planner/worker evaluation suite (regression targets, prompt-version A/B via the benchmark gate).
- **E9** [`c-73af97`] Frontend/UX pass (tables over cards, fewer-clicks triage workflow, live engagement dashboard).
- **SEC** [`c-5a6f96`] Capture-proxy upstream gate residuals (task-scoped rules, go-mitmproxy host pinning, egress-proxy bypass).
- **SEC** [`c-a0af33`] Logout does not revoke the session token (valid until the 7-day JWT TTL).
- **SEC** [`c-775dc2`] Docker Desktop bind mounts defeat executor isolation; stock compose will not start on a Mac.
- [`c-a6891b`] Repeat the egress/login check on the full image once it builds.
- [`c-834a5a`] web: 84 pre-existing biome lint errors; `npm run lint` cannot gate CI yet.
- **SEC** [`c-8d54d5`] Capture proxy: insecure upstream only for in-scope targets.
- **SEC** [`c-c10f2c`] gorilla/websocket bump (GO-2026-6278).
- **SEC** [`c-bd9a6d`] Residual gaps from the High/Medium fixes.
- **SEC** [`c-12df03`] Single-uid start-up fails on a socket in a run dir; MCP uid is not egress-filtered.
- **SEC** [`c-dd160a`] Per-task uids: spill dirs readable across tasks.
- **SEC** [`c-e70e5c`] Per-task uids: release ended tasks at start-up; per-key lock.
- **SEC** [`c-cc25b6`] Low-severity audit items.
- **E10** [`c-72dd9d`] Code-security testing lane (SAST/dependency ingest) feeding the same dual-graph + evidence model.

## Known issues and gotchas

- go-mitmproxy v1.9.2 has no upstream dial hook. Every capture-proxy upstream dial goes through its
  upstream-proxy URL, which is our loopback gate (random credential, `traffic/upstream_gate.go`). Do not
  set `SetUpstreamProxy` back to returning nil: nil means a bare `net.Dialer` as root, with no checks (`c-485de0`).

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

- **2026-11-23** — c-dadc14: the security branch's always-on metadata check resolves each name the
  platform dials (gate on or off); `TestGateOffTaskIsUnchanged` now asserts one lookup per platform
  connection instead of none, since this is the intended behaviour, not a regression. The isolation
  probe binary became a test hook (`isolationProbeBinary`) rather than being changed in production.
  Logout revocation and the Docker Desktop bind-mount problem were filed as cards, not fixed here: both
  predate the branch and need a design choice.
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
  uid, and the exec gid stays a supplementary group (script and spill dirs). As a result MCP servers no
  longer write into run dirs when the range is on. Release kills the uid's processes and gives its files
  back to root, so a uid returned to the pool owns nothing. The mode stays opt-in until it has run in a
  real deployment.

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
- **2026-11-24 11:23 MST** · claude-code · `ej/20261124-110647-182fa9` `ed9dfc4`→`b6682a1` (1 commit) · `h-6a42d5` — c-5b5421 (now in review): client_read users are bound to companies. Migration 0019 artex_user_companies (audited at commit); admin API GET/PUT /api/users/{id}/companies; server/client_scope.go applies a per-route binder after the role check. Findings routes are narrowed through the #425 company filter with StrictCompany, which ignores agent-added task_scope rows. Tasks and ids outside the binding return 404; an unbound client gets 403; a route without a binder is refused, enforced by TestClientBindersCoverClientRoutes, which found findings/{id}/traffic and its body route open to clients without a binder. The client report and stats no longer leak global counts. TestClientReadBoundToCompanyPG passes and was checked by mutation. This branch merged E5 (PR #16), so it carries E5 too. **Next:** Land PR #16 (E5) first, then this branch. Web UI for bindings is in scope of c-869050. Renumber E1's 0017 above 0019 if it lands later. **Decided:** Declarative per-route binder table that fails closed, not checks scattered across handlers. Task ownership counts only operator-made company scope rows, because agent output is untrusted. A multi-company task is fully visible to each bound client (accepted). Bindings restrict client_read only. Client report/stats need an explicit ?task id.
- **2026-11-24 11:22 MST** · claude-code · `ej/20261124-110649-dbd854` at `a329eee`, no commits · `h-d87101` — c-15b39d (board card, now in review; follow-up card c-a1c0fc opened): the Playwright browser MCP, and any stdio MCP whose --proxy-server or HTTP(S)_PROXY / ALL_PROXY / PLAYWRIGHT_MCP_PROXY_SERVER names the recording proxy, now dials under the calling run's scope. wireAgentAugment already spawns every stdio MCP per run. A new prepareMCP hook (Manager.scopeMCPProxy, server/mcp_proxy_scope.go) rewrites that reference at spawn time to a per-run loopback scope relay (traffic/scope_relay.go). The relay binds each of its proxy connections to the run's token, because Chromium drops userinfo from --proxy-server. The relay is closed with the run. A relay that cannot start skips the server for that run, and an ended relay connection leaves a refused tombstone binding. The branch merges PR #13's unmerged branch (ej/20261124-075745-5a4987), which provides ScopedProxyAddr and the scope registry. The new tests pass with -race and were mutation-checked; verify.sh passes. An independent review's findings were fixed (tombstone, host/port matching, dial timeout); the rest are on c-a1c0fc. **Next:** c-a1c0fc: PG-backed test of the prepareMCP hook (skip on error, closer released), concurrent/keep-alive/WebSocket relay tests, a per-relay connection cap, and a live Playwright check with capture on. Land PR #13 before or with this branch, since this branch carries it. **Decided:** The browser fix rewrites the spawn and leaves the process model alone: the card's premise of one shared process was wrong, because stdio MCPs are already spawned per run on a context that carries the TargetScope. The scope travels through a loopback relay rather than userinfo, because Chromium drops userinfo and only sends credentials after a 407. Fail closed: if the relay cannot start, the MCP is not started for the run, and an ended connection's binding becomes a refused tombstone, not a gap. UI tool discovery (no run) keeps the stored URL and so the global rules, since it only lists tools. A per-scheme proxy list that cannot be rewritten is logged, not refused. Stacked on PR #13 because the work cannot exist without it.
- **2026-11-24 11:13 MST** · claude-code · `ej/20261124-110652-e193a7` `0652b65`→`65420b5` (1 commit) · `h-8521a8` — c-3b9252 closed by a test and a decision, with no runtime code change: go-mitmproxy v1.9.2 never shares upstream connections across client connections in ARTEX. Added TestUpstreamConnectionsNotSharedAcrossClients (traffic/upstream_reuse_test.go). Forcing UseSeparateClient makes it fail, so it catches a regression. Added a comment at Traffic.upstreamFor that states the rule for c-5a6f96. **Next:** c-5a6f96: when adding task-scoped rules on proxy dials, return a distinct gate URL/credential per scope from upstreamFor (identifies the scope at the gate and keys go-mitmproxy's separate-client pool). **Decided:** The scope check stays at dial time, and the upstream pool is not changed. One upstream per client connection is pinned by a test. Today the proxy path carries only the global scope, so two scopes cannot meet on one upstream.
- **2026-11-24 11:03 MST** · claude-code · `ej/20261124-102550-4fdb3c` `1abe24f`→`85d5f39` (8 commits) · `h-721d29` — c-a6891b (card, now in review): built the full Docker image (bbot, chromium, Playwright) from 1abe24f with the committed Dockerfile on Docker Desktop arm64 in 34 min. The bbot dry-run grep passed: trufflehog installed with no ERRR. Run as a separate compose project with named volumes, the image logs `[artex] egress: installed` (IPv4 and IPv6) and passes the isolation self-test. The executor gets REJECT for metadata, the API port (loopback and container IP) and host.docker.internal; root does not. The LAN and the internet stay reachable for both. The Playwright login/logout over the HttpOnly SameSite=Strict cookie passes. Added scripts/docker-egress-probe.sh and scripts/docker-login-probe.js so the check can be repeated. **Next:** Re-run both probe scripts after c-a0af33 (logout revocation) merges: the old-cookie replay should become 401. Also re-run with -exec-uid-range (main #18) to see the range rules. The main checkout has an uncommitted arm64 Dockerfile patch that softens the bbot grep; it is not needed (the committed grep passes), so its owner can drop it. **Decided:** Ran verification as its own compose project (-p artex-verify, ports 18787/18788, named volumes), not with the running artex-artex-1. That container was built from the main checkout's old 7eaf424 without egress.sh, and someone was using it for a live task.
- _Older hand-offs, a line each; `ej handoff log` has them whole:_
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
