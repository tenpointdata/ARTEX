# Distributed intent workers (E7)

ARTEX can run intent workers on hosts other than the one serving the UI and API.
Postgres is the only thing the processes share. There are three process roles,
all the same binary:

| Role | Flag / env | Runs | Serves |
|---|---|---|---|
| `all` (default) | `-role all` | API, planners, schedulers, intent workers | UI/API on `-addr` |
| `control` | `-role control`, `ARTEX_LOCAL_WORKERS=N` (default: the `workers` setting; `0` = none) | API, planners, deadline coordinators, schedulers, notifier, capture proxy, and N local workers per task | UI/API on `-addr`, capture proxy on `-proxy` |
| `worker` | `-role worker`, `ARTEX_NODE_WORKERS=N` (default 3), `ARTEX_NODE_ID` (default: host name), `ARTEX_CONTROL_PROXY`, `ARTEX_CONTROL_API` (default: the control proxy's host on port 8787) | intent worker loops only | `GET /healthz` on `-health-addr` (default `:8790`) |

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
| Task pause / queued / terminal | in-memory flags | `GetTaskControl` before every claim; pause cancels running intents (polled every 2 s) | no claim |
| Timeout or budget settle | `settling` flag | `tasks.dispatch_closed='settling'`: no new claim; running intents finish under the deadline / zero-budget wrap-up | no claim |
| Delete | in-memory barrier | `dispatch_closed='deleting'`, remote claims revoked; the node stops the task | no claim |
| Run budget | env `ARTEX_*_BUDGET_*`, ledger seeded from `llm_usage` | the control node publishes its budget in `settings.distributed_run_budgets`; every node syncs spend from `llm_usage` before each dispatch decision | no claim |
| Noise governor | one governor | each live node runs `egress.Share(profile, liveNodes)`; the profile follows `settings.noise_profile` | keep the last share |
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

## Evidence on worker nodes

The capture store (the control node's local index of recorded exchanges) and
the evidence blob directory live on the control node. A worker node's agents
reach them through a **node RPC** on the control API,
`POST /api/node/rpc/{op}` (`server/node_rpc.go`):

| Op | Worker-node caller | Runs on the control node |
|---|---|---|
| `tool` | `traffic_search`, `traffic_get`, `traffic_blob`, `traffic_messages` | the capture store's own tool (only those names) |
| `fact` | fact provenance (a fact quoting a captured response is labelled `target_response`) | `ResponseContaining` |
| `record` | `report_finding` with `traffic_refs` | `evidence.Store.Record`: the snapshot bodies are written to the control node's evidence directory |
| `bind` | `bind_finding_traffic` | `evidence.Store.Bind` |
| `exists` | `prove_goal`'s check that a bound exchange is still captured | `TrafficExists` |
| `preview` | `get_finding_traffic` reading a bound body | the same read as on a single process |
| `assess` | `update_finding_report`'s S1 quote check | the same assessment, against the bodies on the control node |

- **Same checks, same place.** The worker node runs every agent-side check
  before the call, as a single process does: the agent traffic-binding
  setting, finding access and provenance, ref normalisation. The control node
  runs the evidence code unchanged. A finding without `traffic_refs` is
  recorded on the worker node directly.
- **Authentication.** Each request carries an HMAC-SHA256 under a sub-key of
  `ARTEX_SECRET_KEY` (`db.NodeRPCKey`), over the method, path, node id,
  timestamp, nonce and body hash. Every node holds that key (start-up refuses a
  key that does not open the shared data), and agent processes never see it.
  The per-task scope token is not used here, because it rides in the agents'
  proxy URL. The control node refuses a wrong signature, a timestamp more than
  60 s off, and a replayed nonce. The route exists only on `role=control`; it
  is outside the user-session check and is its own authentication.
- **Failures fail closed.** An unreachable control node makes the tool call or
  the `record`/`bind` fail with an error the agent sees (the finding is not
  recorded without its proof), and makes `assess` return `not_assessed`, never
  a verdict.
- **Signed replies.** Every reply carries an HMAC under the same key, bound to
  the request's nonce and the status. The worker drops a reply that is unsigned
  or signed for another request, so nothing on the path can inject a
  `verified` assessment or a fake binding.
- **Agent processes and the key.** `ARTEX_SECRET_KEY` is scrubbed from the
  environment, and the server re-executes with a sealed environment
  (`server/envseal.go`), so `/proc/<pid>/environ` does not show it. Agent
  processes run as per-task uids, which cannot read the server's key file.
  Anyone holding the key can already decrypt every stored secret, so the RPC
  does not re-run the agent-side checks on the control node.
- **Transport.** Plain HTTP inside the cluster by default. Payloads include
  captured target traffic (sessions, credentials) in clear text, so set
  `ARTEX_CONTROL_API=https://…` (TLS in front of the control API) when the
  network between the nodes is shared or untrusted.

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
  `ARTEX_SECRET_KEY`, LLM configuration, `ARTEX_CONTROL_PROXY=http://<control>:8788`
  and, when the API is not on port 8787 of the same host,
  `ARTEX_CONTROL_API=http://<control>:<api port>`. Worker nodes must reach the
  control API as well as the proxy.

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
- **The capture-store tools need the control node.** On a worker node they
  are calls to the control node (see "Evidence on worker nodes"). While it is
  down they fail, and so does a finding that carries traffic refs.
- **The capture switch is read at start-up on a worker node.** A worker node
  reads `traffic_capture` when it starts, so after the switch is flipped on the
  control node, restart the worker nodes.
