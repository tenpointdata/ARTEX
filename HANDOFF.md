# HANDOFF.md — ARTEX

**Last updated:** 2026-11-24 11:03 MST, by claude-code: c-a6891b (card, now in review): built the full Docker image (bbot, chromium, Playwright) from 1abe24f with the committed Dockerfile on Docker Desktop arm64 in 34 min. Earlier hand-offs are under Deltas and in `ej handoff log`.

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

**2026-11-24: agent egress floor (card `c-b73542`, in review).** On Linux with the privilege drop on,
the server now installs its own iptables chain `ARTEX_AGENT_FLOOR` at start-up, so native root installs
without `egress.sh` are covered too. The chain refuses `intercept.MetadataPrefixes()` (link-local, metadata
addresses, NAT64 forms; matched by `-d` and `conntrack --ctorigdst`) to the exec uid, the MCP uid and the
`-exec-uid-range`; DNS and `ARTEX_EGRESS_ALLOW` pass. The isolation self-test then dials the metadata
endpoints as the executor, and a connection stops a Linux root start unless `ARTEX_ALLOW_METADATA_EGRESS=1`.
**Behaviour change:** a container without NET_ADMIN, or `ARTEX_EGRESS_FILTER=0`, on a cloud host that
answers metadata now refuses to start. Code: `server/egress_floor.go`, `ownerfw/` (a product-neutral
owner-match rule builder), `server/isolation_unix.go` (`NoDial`). `egress.sh` is unchanged and its chain
coexists. Verified as root with NET_ADMIN in `golang:1.26.11-bookworm`
(`TestAgentEgressFloorRefusesMetadataAsRoot`): against a listener on 169.254.169.254, curl as uid 10001
works before the floor and fails after it, the MCP uid fails too and root still works; a DNAT of
169.254.170.2 to loopback is refused. The real binary started natively logs `[egress] the agent egress
floor is installed (ipv4, ipv6) for uids 10001, 10002`. With the filter off and metadata answering it
refuses to start; with the opt-out it starts. Not yet run in the full Docker image together with
`egress.sh`. Follow-ups are on `c-02e554`.

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

## Triage (next handful)

**Agents' lane**
1. *(unclaimed)* — **SEC: per-task uid follow-ups** (S each): `c-12df03` (single-uid start-up fails on a
   socket; MCP uid not egress-filtered), `c-dd160a` (spill dirs readable across tasks), `c-e70e5c`
   (release ended tasks at start-up; per-key lock). The "Done when" is on each card.
2. *(unclaimed)* — **SEC: gorilla/websocket bump, GO-2026-6278** (XS), card `c-c10f2c`; needs network.
   Done = govulncheck clean.
3. *(unclaimed)* — **SEC: upstream-gate residuals** (M), card `c-5a6f96`: task-scoped rules on proxy
   dials, go-mitmproxy keep-alive host pinning, and the global-egress-proxy bypass. Done = a test or a decision for each.
4. *(unclaimed)* — **SEC: logout revocation** (S), card `c-a0af33`. Done = a token replayed after
   logout or a password change gets 401, by a test.
5. *(unclaimed)* — **SEC: agent egress floor follow-ups** (S), card `c-02e554`: re-check the floor
   while running, rebuild it atomically, remove stale jumps. Done = a root-Linux test for each.
6. *(unclaimed)* — **SEC: Docker Desktop named volumes** (S), card `c-775dc2`. Done = `docker compose up`
   starts on a Mac with the isolation self-test passing; Linux bind-mount path documented.
7. *(unclaimed)* — **E1 Client-grade reporting & deliverables** (M). Turn `report/` into
   CVSS/EPSS-scored, remediation-bearing, standards-mapped deliverables (OWASP ASVS/WSTG, MITRE
   ATT&CK). Done = a report export with per-finding CVSS vector + remediation + framework tags,
   covered by tests. Card `c-8f9cca`.
8. *(unclaimed)* — **E4 Methodology-driven coverage** (M), card `c-c0bb07`. Extend the coverage ledger to score
   coverage against a codified methodology checklist, not only asset touch. Done = planner can
   read a methodology and the coverage graph shows checklist completion.

**Human lane**
- Revisit tracking: E1–E10 are on the private Composite board (decided 2026-11-23, repo is public).
  File GitHub issues only if the repo goes private or you want them public.
- Decide engagement-governance direction (E5): how formal the rules-of-engagement/authorization
  artifact should be for your use.

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
- **E3** [`c-d25ff9`] Loop-B lesson library graduation + Loop-C benchmark gate per the `RSI.md` contract (owner-approval, immutable surfaces).
- **E4** [`c-c0bb07`] Methodology-driven coverage (OWASP WSTG / API Top 10 as structured checklists).
- **E5** [`c-669406`] Engagement lifecycle & governance: RBAC, per-engagement authorization artifact, scope expiry, tamper-evident (hash-chained) audit log.
- **E6** [`c-cb0093`] Cost/observability dashboard + OpenTelemetry export, building on existing context telemetry.
- **E7** [`c-d60149`] Distributed/remote workers (queue-backed intents) for larger engagements; Helm chart.
- **E8** [`c-5487e8`] Planner/worker evaluation suite (regression targets, prompt-version A/B via the benchmark gate).
- **E9** [`c-73af97`] Frontend/UX pass (tables over cards, fewer-clicks triage workflow, live engagement dashboard).
- **SEC** [`c-5a6f96`] Capture-proxy upstream gate residuals (task-scoped rules, go-mitmproxy host pinning, egress-proxy bypass).
- **SEC** [`c-a0af33`] Logout does not revoke the session token (valid until the 7-day JWT TTL).
- **SEC** [`c-775dc2`] Docker Desktop bind mounts defeat executor isolation; stock compose will not start on a Mac.
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
- **Embedded UI** requires `-tags embedui`; a plain `go build` omits the web assets.
- **Root tests on Linux:** run them as root in a container, e.g. `docker run --rm --cap-add NET_ADMIN
  -v "$PWD":/src -w /src -e ARTEX_PG_DSN=postgres://… golang:1.26.11-bookworm go test ./...`. Without
  root, the `*AsRoot` tests skip; without `ARTEX_PG_DSN`, the PG-backed tests skip (macOS runs miss both).
- **Docker Desktop (Mac):** bind-mounted `data/`, `keys/` and `skills/` are writable by any uid in the
  container, so the isolation self-test refuses to start. Use named volumes (`c-775dc2`).
- **Egress floor test:** `TestAgentEgressFloorRefusesMetadataAsRoot` needs root, NET_ADMIN, iptables,
  iproute2 and curl (`apt-get install -y iptables iproute2` in `golang:1.26.11-bookworm`); it skips without
  them. The floor's rules show up in `iptables -S ARTEX_AGENT_FLOOR`.
- **Egress probes:** as the executor, `/app/artex __artex-exec-drop 10001 10001 -- curl …`; rule hits
  show up in `iptables -L ARTEX_EGRESS -v -n -x`.
- **Secrets** are AES-256-GCM sealed with per-column AAD; losing the key (`keys/secret.key` or
  `ARTEX_SECRET_KEY`) loses stored secrets.

## Decisions

- **2026-11-24** — c-b73542: the agent egress floor is a separate chain (`ARTEX_AGENT_FLOOR`) installed by
  the server, and `egress.sh` was not rewritten. Rebuilding egress.sh's `ARTEX_EGRESS` from Go would have
  dropped its Docker-only gateway and API-port rules whenever their owners differed. The floor is checked
  by a dial from the executor rather than by reading the rules, so any cause of a leak stops the start.
  A start is refused only when a metadata address answers; a host that does not answer starts with a
  warning. DNS (port 53) passes, because native resolvers can be link-local (GCP, AWS) or the Azure
  WireServer. A bad `ARTEX_EGRESS_ALLOW` entry is skipped with a warning rather than dropping the floor.

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

## Deltas

<!-- composite:deltas · newest first; Composite rewrites this block on every merge (ej handoff merge / merge_handoff) -->
- **2026-11-24 11:03 MST** · claude-code · `ej/20261124-102550-4fdb3c` `1abe24f`→`85d5f39` (8 commits) · `h-721d29` — c-a6891b (card, now in review): built the full Docker image (bbot, chromium, Playwright) from 1abe24f with the committed Dockerfile on Docker Desktop arm64 in 34 min. The bbot dry-run grep passed: trufflehog installed with no ERRR. Run as a separate compose project with named volumes, the image logs `[artex] egress: installed` (IPv4 and IPv6) and passes the isolation self-test. The executor gets REJECT for metadata, the API port (loopback and container IP) and host.docker.internal; root does not. The LAN and the internet stay reachable for both. The Playwright login/logout over the HttpOnly SameSite=Strict cookie passes. Added scripts/docker-egress-probe.sh and scripts/docker-login-probe.js so the check can be repeated. **Next:** Re-run both probe scripts after c-a0af33 (logout revocation) merges: the old-cookie replay should become 401. Also re-run with -exec-uid-range (main #18) to see the range rules. The main checkout has an uncommitted arm64 Dockerfile patch that softens the bbot grep; it is not needed (the committed grep passes), so its owner can drop it. **Decided:** Ran verification as its own compose project (-p artex-verify, ports 18787/18788, named volumes), not with the running artex-artex-1. That container was built from the main checkout's old 7eaf424 without egress.sh, and someone was using it for a live task.
- **2026-11-24 09:41 MST** · claude-code · `ej/20261124-092436-5933f8` at `730ddb1`, no commits · `h-691201` — c-ba9f64 done (board card): migration 0018 recreates the intercept_pending, artex_users and engagement_authorizations audit triggers as DEFERRABLE INITIALLY DEFERRED constraint triggers, so audit_chain_head is locked only at COMMIT. A task archive no longer stalls gate decisions, logins and rbac.denied entries, and write-then-lock transactions cannot deadlock on the head. New PG tests in db/audit_chain_concurrency_test.go (archive alongside gate decisions, lock-order deadlock, rollback) fail without 0018 and pass with it. go test ./db ./intercept ./guard pass against Postgres 17. ./server fails only the three tests HANDOFF already lists. verify.sh passes. This branch merged the unmerged E5 branch ej/20261124-055929-bd2253, so it carries E5 too. **Next:** Merge order: E5 (0016), then this branch (0018), then E1 (0017 on ej/20261124-083744-02fa3c). The loader fails closed on a pending version older than one already applied, so a DB that ran 0018 would refuse 0017: renumber E1 to 0019 (or this to 0017 / E1 to 0018) at merge, and update the migrate_test.go want-list. Remaining cost: commits carrying audited rows are still serialized while they append, for example a big archive's per-row DELETE entries. **Decided:** Deferred constraint triggers, rather than moving the archive's deletes (fixes one caller only) or an unchained staging table plus a sequencer (leaves committed-but-unchained writes and adds a worker). Accepted: chain order is commit order. The actor is artex.actor at commit. ALTER/TRUNCATE of an audited table after writing to it in the same tx fails. Numbered 0018 because E1 holds 0017.
- **2026-11-24 09:33 MST** · claude-code · `ej/20261124-092031-1d05de` `1abe24f`→`5be8c4b` (1 commit) · `h-e8d3e5` — Card c-bd9a6d: every residual gap is now either fixed or recorded as accepted in SECURITY.md (commit 5be8c4b). Fixes: - Constraints are rendered by origin. Main-agent constraints become data-framed proposals, and an agent-written allow grants nothing. The overview marks them Agent-proposed with an Adopt button; any operator edit records the origin as human. - The workspace file manager no longer follows symlinks (server/nofollow_root.go), which closes the in-root redirect into traffic/_ca, _index and _blobs. - The capture proxy has optional auth through ARTEX_PROXY_AUTH (HTTP Basic, plus SOCKS5 RFC 1929). - egress.sh now also filters the MCP uid 10002. - web: removed getToken() and the Bearer headers. Accepted: rows stored as human before the fix, the shared executor uid, MCP secrets passed in argv, no metadata deny on Bash command text, and hard links in the file manager. scripts/verify.sh passes on macOS. **Decided:** Agent-written constraints are proposals, and adopting one is an operator edit, so no migration was needed. Rows from before the fix stay operator-owned because they cannot be told apart from the operator's own. Proxy auth is opt-in, and ARTEX_PROXY_ALLOW_NONLOOPBACK stays as the explicit open-relay opt-out. Overwrites in the file manager replace the file and keep its owner and mode.
- **2026-11-24 08:48 MST** · claude-code · `ej/20261124-083747-53e005` `1abe24f`→`d9781b8` (1 commit) · `h-2396f3` — E8 (c-5487e8, now in review): added the planner/worker evaluation suite in scripts/benchmark. eval_suite.py run applies a baseline and a candidate prompt arm over a held-out suite. Both arms run under one stamp: model chain, schedule seed, per-task budget, judge and content-hashed ground truth. Runs are sequential and the starting prompts are restored afterwards. The output is per-gate passes/n with Wilson and Newcombe intervals, a paired gained/lost table and cost per arm; compare refuses mismatched stamps. Added suites heldout-v1 (Juice Shop v20.4.0, DVGA 2.2.0, DrostBench 005…095) and a disjoint dev-v1, DVGA ground truth, and a run_bench refactor (build_parser/prepare/run). 15 new tests; verify.sh passes; never run live. Board cards opened: c-f1518c, c-085c80, c-75c593, c-d73308. **Next:** c-f1518c (human): first live held-out run on a bench instance with ARTEX_RSI_LESSONS=off; check that DVGA is reachable through the port mapping. c-085c80: hand-verify the DVGA ground truth. c-75c593: bench image reproducible from a clean clone. c-d73308: optional provider sampling seed. E3 (c-d25ff9) can build its Loop-C gate on eval_suite compare. **Decided:** Arms change only the editable goals/planner/worker body, through the versioned prompt API, so the immutable surfaces cannot be in an arm. The suite refuses a busy instance and runs sequentially, because prompts are read live. The seed fixes the schedule, not the LLM, since ARTEX has no sampling seed; replicates control variance. Gates are counted over ground-truth entries. Failed runs are excluded from the denominators and counted separately. A judge failure aborts the suite.
- _Older hand-offs, a line each; `ej handoff log` has them whole:_
- **2026-11-24 08:07 MST** · claude-code · `h-c42a5d` — c-eb32cc done (board cards: closed c-eb32cc; opened c-15b39d, c-3b9252). **Next:** c-3b9252: check whether go-mitmproxy reuses upstream connections across scopes.
- **2026-11-24 07:59 MST** · claude-code · `h-6bba55` — Board card c-f89815 is fixed. **Next:** Reconcile TestGateOffTaskIsUnchanged with the c-3157b4 fix when it lands: the http tool now makes one dial lookup with the gate off.
- **2026-11-24 06:49 MST** · claude-code · `h-7dc82c` — E5 engagement governance backend. **Next:** c-ba9f64 (audit head-lock contention), c-5b5421 (bind client_read to a client), c-869050 (web UI incl. **Issues:** opened #c-3157b4, #c-5b5421, #c-869050, #c-ba9f64, #c-f4115c.
- **2026-11-24 06:21 MST** · claude-code · `h-9348ac` — E2 backend landed (card c-6def05 → review): package ticketing (GitHub/Jira/DefectDojo over notify.Do with masking + SSRF guard + stable dedup key), two-way finding↔ticket status sync, package scanimport (Nmap, Nuclei, ZAP, Burp, SARIF) and an import API that… **Next:** c-597eb6: web UI for trackers and imports.
- **2026-11-24 06:17 MST** · claude-code · `h-537f2b` — c-8d54d5 done: the capture proxy verifies upstream TLS per host. **Next:** Run `ARTEX_PG_DSN=… go test ./db -run ActiveTaskScopeRulesPG`.
- **2026-11-24 05:18 MST** · claude-code · `h-5a3839` — c-5a6f96 is done; follow-up cards c-eb32cc and c-9b48b7 are open. **Next:** c-eb32cc: scope the chat and conversation agents, the custom HTTP tools and the transparent listener.
- **2026-11-23 21:08 MST** · claude-code · `h-43cc9e` — c-775dc2: docker-compose.yml and docker-compose.ghcr.yml now keep /app/data, /app/keys and /app/skills in named volumes, so the stock `docker compose up` starts on Docker Desktop with the isolation self-test passing. **Next:** c-965b0e: Docker's `restart: unless-stopped` still restarts the container after an exit 78.
- **2026-11-23 21:07 MST** · claude-code · `h-ee4b2b` — c-a0af33: logout now revokes the session server-side. **Next:** Re-run `ARTEX_PG_DSN=… go test ./server` once Docker is healthy. **Issues:** opened #c-8dea4d.
- **2026-11-23 16:51 MST** · claude-code · `h-61510f` — Verified the security fixes (now on main via PR #1, plus main's c-485de0 upstream gate) on Linux as root with Postgres, in Docker with the egress iptables filter, and in a browser over the HttpOnly cookie. **Next:** c-a0af33: server-side logout revocation (token epoch).
- **2026-11-23 15:14 MST** · claude-code · `h-04e098` — Fixed the High and Medium findings (#1–#16) from the 2026-11-23 ARTEX security audit on the local branch ej/20261123-145205-6a3af6. **Next:** c-485de0: route the MITM proxy's upstream dials through the metadata/block check. **Issues:** opened #c-485de0, #c-8d54d5, #c-bd9a6d, #c-c10f2c, #c-cc25b6, #c-dadc14.
- **2026-11-23 13:37 MST** · claude-code · `h-e3d121` — Stood ARTEX up locally as a Composite project and produced a comprehensive expansion plan.
<!-- /composite:deltas -->

_(written by `merge_handoff`)_

## Updating this document

Keep the sections above in the standard form. Refresh **Current state** (dated stamp) and **Triage**
at the end of any session that changed code/state/config/plans; add or close GitHub issues and record
**Decisions** with their reason. `merge_handoff` writes the Deltas block and the one-line stamp — do
not hand-maintain a history chain in Current state.
