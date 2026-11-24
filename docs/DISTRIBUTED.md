# Distributed intent workers (E7)

ARTEX can run intent workers on hosts other than the one serving the UI and API.
Postgres is the only thing the processes share. There are three process roles,
all the same binary:

| Role | Flag / env | Runs | Serves |
|---|---|---|---|
| `all` (default) | `-role all` | API, planners, schedulers, intent workers | UI/API on `-addr` |
| `control` | `-role control`, `ARTEX_LOCAL_WORKERS=N` (default: the `workers` setting; `0` = none) | API, planners, deadline coordinators, schedulers, notifier, capture proxy, and N local workers per task | UI/API on `-addr`, capture proxy on `-proxy` |
| `worker` | `-role worker`, `ARTEX_NODE_WORKERS=N` (default 3), `ARTEX_NODE_ID` (default: host name), `ARTEX_CONTROL_PROXY` | intent worker loops only | `GET /healthz` on `-health-addr` (default `:8790`) |

Run exactly one control node, plus any number of worker nodes. A worker node
will not claim while a live `role=all` node shares the database. A single
process (`all`) behaves exactly as before.

**Authorized use.** Worker nodes act against live targets just as the control
node does. Every guardrail described below runs on every node. Only point ARTEX
at systems you are authorized to test.

## The queue

The work queue is the intent rows already in `exploration_nodes`.

- **Claiming.** A claim is one atomic `UPDATE … state='open' → 'running'`. It
  stamps an owner, a fresh 128-bit `unclaim_token` and a lease
  (`last_heartbeat_at`).
- **Lease.** The worker renews the lease every 15 s. A lease older than 90 s
  can be taken over, by the sweep on the control node or at any node's start-up.
- **Fenced writes.** Every write that settles an intent is conditional on the
  token. A worker whose claim was taken over cannot overwrite the new claimer.
  Its next heartbeat fails and its run stops (`AbortWorkClaimLost`).
- **Owner names.** Worker loops on worker nodes are named
  `<ARTEX_NODE_ID>/work#N`, so the owner column says which host ran an intent.
  Local loops on the control node keep the `work#N` name.

## Identical gating

A worker node is built by the same `server.New` as the control node. It gets the
same guard hooks, tool catalog, intercept rules, LLM fallback reviewer, target
gate, executor isolation self-test and egress filter (`start.sh`/`egress.sh`).
The control node keeps some state in its own memory. For each such piece, the
table shows what the worker node reads from Postgres instead, and what happens
when that read fails.

| Decision | Single process | Worker node | Read failure |
|---|---|---|---|
| Intercept rules / enabled tools | in-memory cache, `Invalidate` on edit | `Invalidate` bumps `settings.intercept_rules_revision`; every node reloads when it changes (checked at most 1 s apart; full reload every 30 s regardless) | deny every matcher (`loadFailed`) |
| Ask approval decided by a human | in-memory channel | the waiter also polls its `intercept_pending` row every 2 s | keep waiting (a read error is not a decision) |
| Target approvals, pins, block rules, authorization artifact | read from Postgres per call | the same | as before (deny) |
| Target-gate switch | `atomic.Bool` updated by the API | read from `tasks.target_gate` on every gated call | gate **on** |
| Task pause / queued / terminal / deadline passed | in-memory flags | `GetTaskControl` before every claim and again right after it (a claim that lost the race goes back to open); pause or queued cancels running intents (polled every 2 s) | no claim; after 15 failed polls (~30 s) the node pauses the task locally |
| Timeout or budget settle | `settling` flag | `tasks.dispatch_closed='settling'`: no new claim; running intents finish under the deadline / zero-budget wrap-up | no claim |
| Delete | in-memory barrier | `dispatch_closed='deleting'`, remote claims revoked; the node stops the task; the control drain waits two polls for workers to stop writing | no claim |
| Run budget | env `ARTEX_*_BUDGET_*`, ledger seeded from `llm_usage` | the control node publishes its budget in `settings.distributed_run_budgets`; a worker uses the last value it read, never its own env; every node syncs spend from `llm_usage` every 2 s (running intents' meters included) and the control node starts the wrap-up as soon as the synced budget cannot pay for a worker | no claim until a budget was read |
| Noise governor | one governor | each live node runs `egress.Share(profile, liveNodes)`; the profile follows `settings.noise_profile` | keep the current governor |
| Capture switch, global proxy, web search | in memory, set by the API | re-read every poll; a change blocks claims for that poll and rebuilds the task agents on the new path | no claim |
| Claim lease | renewed every 15 s, lost on rejection | also cancelled after 60 s without a successful renewal, before another node may take it over at 90 s | run cancelled |
| Control node alive | n/a | a worker claims only while a live `control` node is in `worker_nodes` (planner, deadline coordinator and settle run there) | no claim |
| kill_work / steer_work / work pause and cancel | in-memory work map | `work_signals`, consumed by the node holding the claim (polled every 2 s); work pause/cancel waits until the intent leaves `running` | signal stays queued |
| Settle drain | local in-flight count | plus live remote claims (`LiveRemoteRunningIntents`); when the grace expires, remote claims are revoked | counted as running |

Two signals flow from worker nodes back to the control node:

- **Planner wake-ups.** The control node polls for intents settled by worker
  nodes and calls `NotifyDone`, so the planner wakes as it does for a local
  worker.
- **Live activity.** Worker-node activity rows are relayed onto the control
  node's live event stream (SSE).

Both are polled every 2 s.

## Proxy path

Worker-node tool traffic goes through the control node's recording proxy. That
proxy records the traffic into the capture store, applies the upstream gate and
the task's TargetScope, and paces the traffic.

- **Scope tokens.** A worker node mints its own scope token per task. Postgres
  stores only the token's SHA-256 (`proxy_scope_tokens`). The control proxy
  resolves the presented token to the task it has loaded, and applies that
  task's scope exactly as it does for a local token. Deleting the task drops
  the tokens and closes their connections.
- **Clients without a token.** Once the proxy accepts remote clients, a client
  that presents no token from a non-loopback address is refused, so the proxy
  is never an open proxy.
- **CA certificate.** The control node publishes its CA certificate (the public
  cert only) for worker nodes to install for their agent processes.

## Deploying

- **Helm.** The chart is in `deploy/helm/artex` (see its README). It runs one
  control Deployment (`ARTEX_LOCAL_WORKERS=0`) and a worker Deployment. The
  workers have required pod anti-affinity on `kubernetes.io/hostname`, so two
  workers always land on separate hosts.
- **Shared configuration.** All nodes share one Secret (`ARTEX_SECRET_KEY`,
  database password, LLM keys) and one ConfigMap.
- **Rendering offline.** When rendering with `helm template` or GitOps, set
  `secretKey.existingSecret`. `lookup` cannot keep a generated key across
  renders.
- **Separate hosts by hand.** Run the control node with
  `-role control -proxy :8788`. On each other host, run
  `-role worker -health-addr :8790` with the same database environment,
  `ARTEX_SECRET_KEY`, LLM configuration and `ARTEX_CONTROL_PROXY=http://<control>:8788`.

## Limits

These are known and carded on the ARTEX board:

- **Transcripts stay on the node that ran the intent.** If a claim is taken
  over by another host, or a reopened intent is rerun elsewhere, the run starts
  over instead of resuming its transcript.
- **Finding wake-ups are not relayed.** The planner sees a worker node's
  findings on its next round. That round is triggered by the intent's
  settlement.
- **`egress.Share` floors in-flight limits at 1 per node.** With N nodes, up to
  N requests to one host can be in flight at once. The request rate is divided
  exactly.
- **Agent processes can reach other cluster services.** On a worker host they
  can reach the network like any target, including the control API and
  Postgres, just as the bundled compose Postgres is reachable today. Deny those
  addresses with `ARTEX_EGRESS_DENY` or a NetworkPolicy.
- **The capture store exists only on the control node.** A worker node has no
  local store. Its agents get no `traffic_search`/`traffic_get`, facts stay
  activity-only, and a finding cannot bind captured traffic as proof (`prove_goal`
  is refused). Findings without traffic references work. The traffic itself is
  recorded on the control node.
- **A browser MCP (stdio Playwright) on a worker node has no usable proxy.**
  The shared MCP row names the control node's loopback proxy, which a worker
  cannot reach, so its dials fail (closed). A worker-side credential relay is
  needed.
- **The compose overlay changes how the host reaches the capture proxy.** The
  overlay binds the control proxy to `0.0.0.0` inside the container. A
  host-side client (for example the browser extension through the published
  `127.0.0.1:8788`) then arrives from the Docker gateway address and is
  refused without a scope token.
- **A worker node needs the control node to start first.** It exits at start
  (and is restarted) until it can read the control node's published CA
  certificate. It claims nothing until a control node has published its run
  budget.
