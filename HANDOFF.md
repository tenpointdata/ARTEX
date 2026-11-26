# HANDOFF.md — ARTEX

**Last updated:** 2026-11-26 00:18 MST, by codex-mcp-client: Completed c-085c80 source review; DVGA ground truth v3 corrects upload side effects and limits trace disclosure to GraphQLError. Earlier hand-offs are under Deltas and in `ej handoff log`.

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
  that dials with `Interceptor.DialSOCKS5`. This applies global rules only; task rules are tracked on `c-5a6f96`.
  Block rules apply even with the gate off. Fuzzy allow rules match on name and address boundaries.
- Main-agent writes carry origin `mainagent` and are framed as agent-authored. Norma compaction
  summaries are framed as untrusted data.
- Per-run executor HOME; MCP servers run as uid 10002 (`ARTEX_EXEC_MCP_UID`). The workspace file
  manager uses `os.Root`.

**2026-11-24: E8 evaluation suite** (card `c-5487e8`) is in `scripts/benchmark`. `eval_suite.py run` runs a
baseline and a candidate prompt arm over a held-out suite. Both arms share one stamp: model chain, schedule
seed, per-task budget, judge, and content-hashed ground truth. Runs go one at a time and the arms alternate in
an order drawn from the seed. The output is per-gate passes/n for `detected`, `component`, `class`, `keywords`,
`proof` and `flag_captured`, with Wilson and Newcombe intervals, a paired gained/lost table and cost per arm.
`compare` refuses two results whose stamps differ. Arms change only the editable `goals`/`planner`/`worker`
body, through the versioned prompt API, and the starting bodies are restored afterwards. Suites:
`suites/heldout-v1.json` (Juice Shop v20.4.0, DVGA 2.2.0, DrostBench 005…095) and a disjoint
`suites/dev-v1.json`. New ground truth: `ground_truth/dvga.json` (v3, hand-verified 2026-11-24 and reviewed 2026-11-26 against DVGA tag 2.2.0 commit `a961308`; card `c-085c80` done, see Decisions). `run_bench.py` now exposes
`build_parser`/`prepare`/`run` and stamps `/api/tasks/{id}/outcome`. Unit-tested (15 new tests,
`scripts/verify.sh` passes); never yet run live (card `c-f1518c`).

## Triage (next handful)

**Agents' lane**
1. *(unclaimed)* — **SEC: gorilla/websocket bump, GO-2026-6278** (XS), card `c-c10f2c`; needs network.
   Done = govulncheck clean.
2. *(unclaimed)* — **SEC: upstream-gate residuals** (M), card `c-5a6f96`: task-scoped rules on proxy
   dials, go-mitmproxy keep-alive host pinning, and the global-egress-proxy bypass. Done = a test or a decision for each.
3. *(unclaimed)* — **E1 Client-grade reporting & deliverables** (M). Turn `report/` into
   CVSS/EPSS-scored, remediation-bearing, standards-mapped deliverables (OWASP ASVS/WSTG, MITRE
   ATT&CK). Done = a report export with per-finding CVSS vector + remediation + framework tags,
   covered by tests. Card `c-8f9cca`.
4. *(unclaimed)* — **E4 Methodology-driven coverage** (M), card `c-c0bb07`. Extend the coverage ledger to score
   coverage against a codified methodology checklist, not only asset touch. Done = planner can
   read a methodology and the coverage graph shows checklist completion.

- *(unclaimed)* — **Bench image reproducible from a clean clone** (S), card `c-75c593`.

**Human lane**
- **E8 first live held-out run** (card `c-f1518c`): needs a bench instance, an LLM, a judge key and a
  drost-benchmarks checkout. Done = a `report.md` with per-gate passes/n for baseline vs builtin.
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
- **SEC** [`c-5a6f96`] Capture-proxy upstream gate residuals (task-scoped rules, go-mitmproxy host pinning, egress-proxy bypass).
- **SEC** [`c-dadc14`] Verify the security branch on Linux root, in Docker and against Postgres, plus the web typecheck.
- **SEC** [`c-8d54d5`] Capture proxy: insecure upstream only for in-scope targets.
- **SEC** [`c-c10f2c`] gorilla/websocket bump (GO-2026-6278).
- **SEC** [`c-bd9a6d`] Residual gaps from the High/Medium fixes.
- **SEC** [`c-cc25b6`] Low-severity audit items.
- **E8 follow-ups** [`c-f1518c`] first live run, [`c-75c593`] bench
  image from a clean clone, [`c-d73308`] optional provider sampling seed per LLM profile.
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
- **Secrets** are AES-256-GCM sealed with per-column AAD; losing the key (`keys/secret.key` or
  `ARTEX_SECRET_KEY`) loses stored secrets.

## Decisions

- **2026-11-26** — DVGA ground truth v3 corrects the upload failure proof: the file is not created, but the resolver still writes a private paste and audit entry. GraphiQL trace disclosure is limited to GraphQLError instances. Source review and benchmark tests complete; a live evaluation remains card `c-f1518c`.

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

- **2026-11-24** — E8: the A/B suite runs one task at a time on a dedicated instance and refuses a busy one,
  because the server reads the active prompt live and a swap would reach a concurrent task. `--seed` fixes the
  schedule, not the LLM, because ARTEX has no sampling-seed control; replicates control variance instead
  (`c-d73308`). Gate rates are counted over ground-truth entries, not over detected findings, so arms with
  different detection counts stay comparable. A failed run is counted but left out of the denominators. A judge
  failure aborts the suite. Arms may vary only the editable goals/planner/worker body; the `RSI.md` immutable
  surfaces stay outside the prompt API by construction.

## Deltas

<!-- composite:deltas · newest first; Composite rewrites this block on every merge (ej handoff merge / merge_handoff) -->
- **2026-11-26 00:18 MST** · codex-mcp-client · `ej/20261124-084655-ccdab2` at `45f5dd3`, no commits · `h-67308f` — Completed c-085c80 source review; DVGA ground truth v3 corrects upload side effects and limits trace disclosure to GraphQLError. Benchmark unit tests pass. **Next:** First live held-out benchmark remains c-f1518c; verify.sh requires newer Bash and writable Go cache. **Decided:** Retain 12 source-cited findings and exclude DoS; version v3 reflects corrections to v2.
- **2026-11-26 00:11 MST** · codex-mcp-client · `ej/20261126-000938-74d42f` at `d1a5d70`, no commits · `h-38d371` — c-f1518c remains blocked on subject and independent judge configuration. Verified ten Drost IDs at 762d2872dff7a9b30fde4c3c16cbb4cdeee7b208 and DVGA binding; added per-image env forwarding and set WEB_HOST for DVGA. Benchmark unit tests pass. **Next:** Provide dedicated bench subject model and independent judge credentials, validate Docker Desktop target routing, run held-out suite and compare planner before/after. Key-minted token was rejected by the existing instance; use a valid bench session. **Decided:** Keep the existing instance read-only because it is not established as disposable. Add environment support to fix the first live use of the pinned DVGA target.
- **2026-11-24 20:54 MST** · claude-code · `ej/20261124-204623-7a8920` `cce7bf1`→`b76ff66` (1 commit) · `h-575a78` — c-81b834 (also closes card c-1a74a4): #79's SOCKS5 checkCredential looked scope tokens up only in the local registry, so remote-worker tokens got 0x01 before the remote resolver ran. That was a real E7 regression. It now uses resolveScope (local registry, then remote resolver); everything else in SOCKS5 auth is unchanged. TestSOCKS5RemoteScope now expects junk and unresolvable tokens to be refused at auth. Also added the six #80 webhook i18n strings that the go failure had hidden. go test ./traffic and scripts/verify.sh pass. Moved six older Current state entries to docs/handoff/2026-11-24-current-state-archive.md to meet the size limit. **Next:** Land ej/20261124-204623-7a8920. Optionally have a native speaker review the tr/zh-CN webhook strings and run a real two-node worker SOCKS5 smoke test. **Decided:** SOCKS5 auth accepts exactly the tokens the scope gate accepts (resolveScope), with the remote resolver as the authority; the policy was not loosened.
- **2026-11-24 20:51 MST** · claude-code · `ej/20261124-203755-708a1e` `30a1782`→`dac6220` (1 commit) · `h-8ecb71` — c-0aaa24 (in review): propose.Drafter hook (Input.Drafter, nil = fixed guidance). Build drafts in rank order after the transfer filter and the fixed-body ValidateEdit, so only in-space roles reach the model and a refused body frees its slot. DraftRequest carries code-owned text, counts and the active body only. Output is refused (never repaired) on template-action changes, stray delimiters, empty, unchanged or prior bodies, or a ValidateEdit failure. WriteArm re-checks. CheckDrafter refuses a drafter equal to the judge, or any drafter when no judge is named. CLI: --draft-model/--draft-provider/--draft-url/--judge-model, $ARTEX_DRAFT_API_KEY. New agent.Complete (tool-less one-shot) and agent.FrameData (exported data envelope). Arms carry drafted_by. Hostile-output tests in rsi/propose/drafter_test.go and cmd/artex-rsi-propose/drafter_test.go, mutation-checked. Older Current state blocks moved to docs/handoff/. **Next:** Land ej/20261124-203755-708a1e. Then c-4bbf29: run a real-model --draft-model dry run on a bench instance and send one drafted arm through eval_suite with a different judge. Optional: cap the share of a body a rewrite may delete, if real drafts strip useful content. **Decided:** A refused drafted body skips the rule instead of falling back to the fixed text, so the plan says why and the operator can rerun without the flag. Drafting happens after budget ranking to keep model calls within the round's slots. Drafted proposals are tagged "[drafted rule <id>]" in the rationale, because a rewrite need not contain the guidance text the old pruner matches. An unknown judge counts as a refusal, since independence cannot be checked. Template actions must stay exactly the same set as in the active body.
- _Older hand-offs, a line each; `ej handoff log` has them whole:_
- **2026-11-24 19:39 MST** · claude-code · `h-be59c2` — c-bd9b7a (in review on ej/20261124-191154-865292): RouteGuard in the (main) layout (route-access.ts resolver from the sidebar's need, tested) sends a role to its landing page without mounting a page it can't see; c-92bb52's useCan gating re-applied onto main'… **Next:** Land ej/20261124-191154-865292 (carries c-80fe4e and c-bd9b7a).
- **2026-11-24 19:35 MST** · claude-code · `h-8712ef` — c-936308 (in review): /system/rsi page — proposals table with newest verdict, expandable per-gate rates (with n), paired gained/lost, regressions with targets, adoption log, prompt-history link; inline Adopt (password + newest evidence_id), Reject, Roll back… **Next:** Land ej/20261124-192153-48c72c.
- **2026-11-24 19:31 MST** · claude-code · `h-8d9748` — c-c89f5b (in review): RSI Loop C proposer. **Next:** Land ej/20261124-192202-fbe9e3. **Issues:** opened #c-0aaa24, #c-4bbf29.
- **2026-11-24 19:12 MST** · claude-code · `h-43b8d4` — c-80fe4e (in review): scripts/e2e/role-probe.sh, a role probe on the integrations.sh harness. **Next:** Land c-92bb52 (ej/20261124-144553-d5978e; main still lacks it, so the probe fails widely on main). **Issues:** opened #c-bd9b7a.
- **2026-11-24 19:11 MST** · claude-code · `h-e9f875` — c-1e3b53 (E7, in review): before worker.Execute a distributed node pulls the stored files of the intent's run dir, the task's spills and the session's sidechains (pullRunFilesFrom), makes the dirs as the run does, then hands what it pulled to the executor (sh… **Next:** Land ej/20261124-185425-cbc1cf instead of ej/20261124-180332-433a2b (it carries it).
- **2026-11-24 19:02 MST** · claude-code · `h-b23a91` — c-ccc7a8 (in review): this branch merges c-6bcdd3 (ej/20261124-172031-c2bb7b, carrying c-20c400/c-d4946f/c-8cfbfe and the E7 stack) and c-677166 (ej/20261124-163818-870578, proxy-hop TLS), so landing it lands both. **Next:** Land ej/20261124-185428-8c1cc4; do not land ej/20261124-163818-870578 or ej/20261124-172031-c2bb7b separately afterwards.
- **2026-11-24 18:49 MST** · claude-code · `h-7cf4df` — c-1bbeb6 (E7, in review): worker nodes sweep their local tasks/<T>, spill/tasks/<T> and transcripts/exp<E>-… of deleted, archived or vanished tasks on start-up and every 10 min (server/worker_node_sweep.go, db.RetiredTasks in db/task_retirement.go); stageTask… **Next:** Land ej/20261124-184127-104862 (card c-1bbeb6 in review).
- **2026-11-24 18:23 MST** · claude-code · `h-0668f2` — c-67993f done (closed; c-ccc7a8 opened): the E7 TLS proxy hop was checked on a 3-node kind cluster with an agent-driven (stub LLM) task. **Next:** Land c-677166 (ej/20261124-163818-870578, fork h-c56630): rebase onto main and unify the relay helper with c-6bcdd3's relayConns (c-ccc7a8).
- **2026-11-24 18:23 MST** · claude-code · `h-970bcd` — c-d73308 (E8 follow-up) built, in review: an optional per-profile LLM sampling pin. **Next:** Land ej/20261124-180334-421555.
- **2026-11-24 18:23 MST** · claude-code · `h-c952c8` — c-43dbfc (E7, in review): worker-node run directories, spills and subagent sidechains are mirrored into PostgreSQL (new filesync package, db.TaskFiles, migration 0026) when an intent run ends. **Next:** Land ej/20261124-180332-433a2b (migrations 0025, 0026). **Issues:** opened #c-1bbeb6, #c-1e3b53.
- **2026-11-24 18:21 MST** · claude-code · `h-9bfe0c` — c-601ec6 (in review): GET /api/intercept/history/chain takes order=desc and before_seq (db.AuditFilter Desc/BeforeSeq; ascending stays the default). **Next:** Land ej/20261124-180916-b60b04.
<!-- /composite:deltas -->

_(written by `merge_handoff`)_

## Updating this document

Keep the sections above in the standard form. Refresh **Current state** (dated stamp) and **Triage**
at the end of any session that changed code/state/config/plans; add or close GitHub issues and record
**Decisions** with their reason. `merge_handoff` writes the Deltas block and the one-line stamp — do
not hand-maintain a history chain in Current state.
