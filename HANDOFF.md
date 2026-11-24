# HANDOFF.md — ARTEX

**Last updated:** 2026-11-24 05:18 MST, by claude-code: c-5a6f96 is done; follow-up cards c-eb32cc and c-9b48b7 are open. Earlier hand-offs are under Deltas and in `ej handoff log`.

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

**2026-11-23** — Fixes for the High and Medium findings from the 2026-11-23 security audit are on the
local branch `ej/20261123-145205-6a3af6` (not pushed). It covers audit findings #1–#16 except as listed
under Open work. `go build ./...`, `go vet ./...` and `go test ./...` pass on macOS. Not yet run: the
Linux-root tests, the Docker egress iptables rules, the Postgres-backed tests and the web typecheck
(card `c-dadc14`). Operator-facing changes:
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
  block rules and, with its gate on, its pins/approvals; an ended task's token is refused. Clients
  without a token (operators, chat/conversation agents, custom HTTP tools, the transparent listener)
  keep the global rules only (`c-eb32cc`). go-mitmproxy's keep-alive host pinning is worked around
  (`unpinHostChange`; upstream report `c-9b48b7`). The global-egress-proxy bypass is accepted and
  documented (`docs/OPS.md` §6). `scripts/verify.sh` passes.
  Block rules apply even with the gate off. Fuzzy allow rules match on name and address boundaries.
- Main-agent writes carry origin `mainagent` and are framed as agent-authored. Norma compaction
  summaries are framed as untrusted data.
- Per-run executor HOME; MCP servers run as uid 10002 (`ARTEX_EXEC_MCP_UID`). The workspace file
  manager uses `os.Root`.

## Triage (next handful)

**Agents' lane**
1. *(unclaimed)* — **SEC: gorilla/websocket bump, GO-2026-6278** (XS), card `c-c10f2c`; needs network.
   Done = govulncheck clean.
2. *(unclaimed)* — **SEC: scope the remaining proxy clients** (S), card `c-eb32cc`: chat/conversation
   agents, custom HTTP tools, transparent listener. Done = scoped with a test, or a decision each.
3. *(unclaimed)* — **E1 Client-grade reporting & deliverables** (M). Turn `report/` into
   CVSS/EPSS-scored, remediation-bearing, standards-mapped deliverables (OWASP ASVS/WSTG, MITRE
   ATT&CK). Done = a report export with per-finding CVSS vector + remediation + framework tags,
   covered by tests. Card `c-8f9cca`.
4. *(unclaimed)* — **E4 Methodology-driven coverage** (M), card `c-c0bb07`. Extend the coverage ledger to score
   coverage against a codified methodology checklist, not only asset touch. Done = planner can
   read a methodology and the coverage graph shows checklist completion.

**Human lane**
- **SEC: verify on Linux/Docker** (card `c-dadc14`): Linux-root tests, the egress iptables rules,
  Postgres tests and the web typecheck. Needed before the security branch goes anywhere public.
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
- **Web deps (not yet installed):** `cd web && npm ci`. **Embedded-UI build:** `./build.sh`.
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
- **E5** [`c-669406`] Engagement lifecycle & governance: RBAC, per-engagement authorization artifact, scope expiry, tamper-evident (hash-chained) audit log.
- **E6** [`c-cb0093`] Cost/observability dashboard + OpenTelemetry export, building on existing context telemetry.
- **E7** [`c-d60149`] Distributed/remote workers (queue-backed intents) for larger engagements; Helm chart.
- **E8** [`c-5487e8`] Planner/worker evaluation suite (regression targets, prompt-version A/B via the benchmark gate).
- **E9** [`c-73af97`] Frontend/UX pass (tables over cards, fewer-clicks triage workflow, live engagement dashboard).
- **SEC** [`c-eb32cc`] Scope the remaining capture-proxy clients (chat/conversation agents, custom HTTP tools, transparent listener).
- **SEC** [`c-9b48b7`] Report go-mitmproxy keep-alive host pinning upstream; drop `unpinHostChange` once fixed.
- **SEC** [`c-dadc14`] Verify the security branch on Linux root, in Docker and against Postgres, plus the web typecheck.
- **SEC** [`c-8d54d5`] Capture proxy: insecure upstream only for in-scope targets.
- **SEC** [`c-c10f2c`] gorilla/websocket bump (GO-2026-6278).
- **SEC** [`c-bd9a6d`] Residual gaps from the High/Medium fixes.
- **SEC** [`c-cc25b6`] Low-severity audit items.
- **E10** [`c-72dd9d`] Code-security testing lane (SAST/dependency ingest) feeding the same dual-graph + evidence model.

## Known issues and gotchas

- go-mitmproxy v1.9.2 has no upstream dial hook. Every capture-proxy upstream dial goes through its
  upstream-proxy URL, which is our loopback gate (random credential, `traffic/upstream_gate.go`). Do not
  set `SetUpstreamProxy` back to returning nil: nil means a bare `net.Dialer` as root, with no checks (`c-485de0`).
- A task's proxy scope rides on the proxy URL userinfo (`egress/proxycred.go`). Code that rewrites a
  proxy URL must keep it (`MarkProxyURL` does); prompts and logs show it stripped
  (`egress.WithoutProxyCredential`). The client-connection binding is keyed by remote address, the only
  key go-mitmproxy hands `upstreamFor` (`traffic/scope.go`).

- **Guardrails are invariants.** Scope/target gate, interception gate, budgets, noise governor,
  untrusted-output boundary, audit trail — never weaken/bypass/route around them. `RSI.md`
  "immutable surfaces" must never enter an automated search space.
- **Migrations fix forward** — a published `db/migrations/*.sql` is never edited.
- **Embedded UI** requires `-tags embedui`; a plain `go build` omits the web assets.
- **Secrets** are AES-256-GCM sealed with per-column AAD; losing the key (`keys/secret.key` or
  `ARTEX_SECRET_KEY`) loses stored secrets.

## Decisions

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

- **2026-11-24** — Capture-proxy task scope (`c-5a6f96`): carried as a per-task token in the proxy URL
  userinfo, because a proxy client can only identify itself through its credential and every tool
  already reads `HTTP_PROXY`. The token only *adds* a task's rules, so it is not treated as a secret
  and a missing token falls back to the global rules (the floor), while an unknown token fails closed.
  Proxy dials never wait for approval (an unpinned host opens a request and is refused). The
  global-egress-proxy path is **accepted**, not routed through the gate: the operator's proxy resolves
  and reaches the target, so the gate could only make a name-only check. OPS.md tells operators not to
  run that proxy where it can reach metadata. Keep-alive host pinning is fixed locally with
  go-mitmproxy's separate client rather than closing the client connection, so clients see no resets.

## Deltas

<!-- composite:deltas · newest first; Composite rewrites this block on every merge (ej handoff merge / merge_handoff) -->
- **2026-11-24 05:18 MST** · claude-code · `ej/20261124-045249-222f7a` `1abe24f`→`114c0c8` (2 commits) · `h-5a3839` — c-5a6f96 is done; follow-up cards c-eb32cc and c-9b48b7 are open. Task agents' capture-proxy URLs now carry a task scope token. Proxy upstream dials, and SOCKS5 with the token as its password, now apply the task's block rules and, when its gate is on, its pins and approvals. A token whose task has ended is refused. go-mitmproxy's keep-alive host pinning is worked around with its separate client (not inside TLS tunnels). The global-egress-proxy bypass is accepted and documented in OPS.md §6. There are tests for each fixed item. An independent review found no fail-open; two of its findings are fixed. verify.sh passes; -race is clean on traffic, egress and agent. **Next:** c-eb32cc: scope the chat and conversation agents, the custom HTTP tools and the transparent listener. It also carries two low review items: the HTTP/2 pool is shared across scopes, and a token minted after forgetTask is never freed. c-9b48b7: report the pinning bug to go-mitmproxy, after confirming with Ian. Server-level wiring (TaskProxyAddr) has no Postgres-backed test. **Decided:** Scope travels as a per-task token in the proxy URL userinfo. It only adds rules: no token falls back to the global floor, and an unknown token fails closed. Proxy dials never wait for approval. The global egress proxy is accepted rather than routed through the gate, since a name-only check is all the gate could do there; operators must not run that proxy where it can reach metadata. Pinning is fixed by UseSeparateClient rather than by closing client connections.
- **2026-11-23 21:08 MST** · claude-code · `ej/20261123-192351-fab3f4` `5c5ac24`→`cb6d41c` (1 commit) · `h-43cc9e` — c-775dc2: docker-compose.yml and docker-compose.ghcr.yml now keep /app/data, /app/keys and /app/skills in named volumes, so the stock `docker compose up` starts on Docker Desktop with the isolation self-test passing. Added docker-compose.bind.yml, a Linux-only override for host directories. A refused isolation start now exits 78 (server.ExitIsolationRefused), and start.sh prints the remedy instead of relaunching. update.sh copies an old ./keys, ./data and ./skills into the new volumes once. scripts/backup.sh archives /app/data (and, with the new --with-keys, /app/keys) from inside the container. README, OPS and SECURITY are updated. Verified on Docker Desktop for Mac arm64: "backend listening", health 200, and uid 10001 cannot write keys or skills; the bind override exits 78 with the remedy. Opened card c-965b0e; closes card c-775dc2. **Next:** c-965b0e: Docker's `restart: unless-stopped` still restarts the container after an exit 78. Decide on the restart policy or an idle hold. Run the Linux bind-mount path on a real Linux host. Run the full image build (`docker compose build`) once; the Dockerfile is unchanged. **Decided:** Named volumes are the default for all three directories, so the stock file never needs weakened isolation. Skills became a volume, seeded from the image, because the UI writes skills; refresh them with `docker compose cp skills/. artex:/app/skills/`. The isolation refusal uses EX_CONFIG 78 so start.sh can tell it from a crash.
- **2026-11-23 21:07 MST** · claude-code · `ej/20261123-192346-aa7ae3` `1abe24f`→`748d1f3` (1 commit) · `h-ee4b2b` — c-a0af33: logout now revokes the session server-side. POST /api/auth/logout with a valid session advances the per-account session epoch (auth.password_version / JWT pwd claim), so every earlier token returns 401, copies included. Cookie sessions revoke only same-origin; a failed epoch write gives 503 with the cookies still cleared. Setup after the hash is removed advances the epoch instead of resetting it to 1. New db.IncrementCounter. auth.ts comment corrected. Tests in server/auth_logout_test.go pass against Postgres 16, and so does ./db. The full ./server run after the last change was cut off when Docker Desktop crashed under load. An earlier full run (before the setup-epoch change) failed only two tests, which fail the same way on main (c-8dea4d). **Next:** Re-run `ARTEX_PG_DSN=… go test ./server` once Docker is healthy. Optionally re-check in the browser against the compose stack (Sign out, then replay the cookie → 401). Then c-8dea4d (macOS test failures), c-c10f2c, c-5a6f96. **Decided:** Use the existing per-account epoch rather than a jti denylist: one account, no per-request DB lookup, no migration, and it gives log out everywhere. The cost is that sign-out also ends other browsers' sessions and bearer tokens. Revisit if E5 RBAC adds multiple users. jwtTTL stays 7 days; sliding renewal is left on c-cc25b6. **Issues:** opened #c-8dea4d.
- **2026-11-23 16:51 MST** · claude-code · `ej/20261123-152406-fccff3` `87f3a86`→`841aec9` (5 commits) · `h-61510f` — Verified the security fixes (now on main via PR #1, plus main's c-485de0 upstream gate) on Linux as root with Postgres, in Docker with the egress iptables filter, and in a browser over the HttpOnly cookie. go vet + go test ./... as root in golang:1.26.11-bookworm with ARTEX_PG_DSN: all packages pass after two test fixes (fc8c3f9): the isolation probe now runs a 0755 copy of the go-test binary via the isolationProbeBinary hook (it failed on main too), and TestGateOffTaskIsUnchanged asserts one lookup per platform connection. Through docker-compose.yml with NET_ADMIN on a slim image of the same runtime base, egress.sh installs on bookworm's nf_tables iptables, and the executor is refused for metadata, the gateway, host.docker.internal and :8787 while the LAN and the internet stay reachable. Web tsc is clean; lint has 84 errors, all already on main. Login and logout work over the cookie. Closed card c-dadc14; opened c-a0af33, c-775dc2, c-a6891b, c-834a5a. **Next:** c-a0af33: server-side logout revocation (token epoch). c-775dc2: named volumes so the compose stack starts on Docker Desktop. c-a6891b: repeat the probes on the full image when a fast link is available. c-834a5a: clear the lint baseline. Then c-c10f2c and c-5a6f96. **Decided:** The gate-off lookups are intended (the always-on metadata check), so the test changed, not the code. The probe binary is a test hook; production still execs os.Executable(). The egress check ran on a slim image because the full image's recon-toolbox layers would not finish on this link; base, iptables, binary and scripts are identical. Logout revocation and Docker Desktop bind mounts predate the branch and need design choices, so they were filed as cards.
- _Older hand-offs, a line each; `ej handoff log` has them whole:_
- **2026-11-23 15:14 MST** · claude-code · `h-04e098` — Fixed the High and Medium findings (#1–#16) from the 2026-11-23 ARTEX security audit on the local branch ej/20261123-145205-6a3af6. **Next:** c-485de0: route the MITM proxy's upstream dials through the metadata/block check. **Issues:** opened #c-485de0, #c-8d54d5, #c-bd9a6d, #c-c10f2c, #c-cc25b6, #c-dadc14.
- **2026-11-23 13:37 MST** · claude-code · `h-e3d121` — Stood ARTEX up locally as a Composite project and produced a comprehensive expansion plan.
<!-- /composite:deltas -->

_(written by `merge_handoff`)_

## Updating this document

Keep the sections above in the standard form. Refresh **Current state** (dated stamp) and **Triage**
at the end of any session that changed code/state/config/plans; add or close GitHub issues and record
**Decisions** with their reason. `merge_handoff` writes the Deltas block and the one-line stamp — do
not hand-maintain a history chain in Current state.
