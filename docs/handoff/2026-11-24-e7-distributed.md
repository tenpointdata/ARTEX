# E7 distributed workers: Current state entries (2026-11-24)

Moved from HANDOFF.md Current state to fit its size limit (c-20c400). Newest first.

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
