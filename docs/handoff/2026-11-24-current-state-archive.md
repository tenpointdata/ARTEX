# ARTEX — archived Current state blocks (moved from HANDOFF.md on 2026-11-24)

Older, mostly landed status blocks, moved out to keep HANDOFF.md under its size limit. HANDOFF.md's Current state holds what is in flight; `ej handoff log` has the full history.

**2026-11-24 — E5 engagement governance (card `c-669406`)** is on `main` (#31); see `docs/GOVERNANCE.md`.
Roles `admin`/`operator`/`reviewer`/`client_read` over the JWT (`artex_users`, `/api/users`, policy in
`server/rbac_policy.go`, enforced in `requireAuth`; `ARTEX` is the break-glass admin). The per-task
authorization artifact is enforced only inside the target gate (`intercept/authorization.go`). `audit_chain`
is hash-chained by deferred row triggers (migrations 0016, 0018); `/api/intercept/history/chain[/verify]`.
With `ARTEX_PG_DSN`, `go test ./...` fails only `TestGateOffTaskIsUnchanged` (`c-3157b4`),
`TestConnectWithoutPasswordRealPG` (trust auth) and the macOS SOCKS5 UDP test.

**2026-11-24 — client_read bound to companies (card `c-5b5421`, in review).** Detail (branch, verification, gaps) moved to `docs/handoff/2026-11-24-client-read-company-bindings.md`.

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

Operator-facing changes from that audit (unsandboxed-start refusal, TLS on non-loopback binds, and more): `docs/handoff/2026-11-23-security-audit-operator-changes.md`.

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

## Moved from HANDOFF.md Current state on 2026-11-24 (c-81b834, size limit)

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
