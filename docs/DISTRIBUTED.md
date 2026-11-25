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
- **Node registry.** Every process upserts its `worker_nodes` row every 10 s
  and is live for 45 s after (`db.NodeTTL`); a clean shutdown deletes it. A
  row silent for longer than `db.NodeRetention` (20 × NodeTTL = 15 min) is
  hidden from `GET /api/settings/nodes` and deleted by the next heartbeat of
  any node, so pods replaced by a Kubernetes rollout (node id = pod name)
  do not pile up as dead rows.

## Identical gating

A worker node is built by the same `server.New` as the control node. It gets the
same guard hooks, tool catalog, intercept rules, LLM fallback reviewer, target
gate, executor isolation self-test and egress filter (`start.sh`/`egress.sh`).
The control node keeps some state in its own memory. For each such piece, the
table shows what the worker node reads from Postgres instead, and what happens
when that read fails.

| Decision | Single process | Worker node | Read failure |
|---|---|---|---|
| Intercept rules / enabled tools | in-memory cache, `Invalidate` on edit | every rule or tool-config write bumps `settings.intercept_rules_revision` in its own transaction (a failed bump rolls the write back); every node reloads when it changes (checked at most 1 s apart; full reload every 30 s regardless) | deny every matcher (`loadFailed`) |
| Ask approval decided by a human | in-memory channel | the waiter also polls its `intercept_pending` row every 2 s | keep waiting (a read error is not a decision) |
| Target approvals, pins, block rules, authorization artifact | read from Postgres per call | the same | as before (deny) |
| Target-gate switch | `atomic.Bool` updated by the API | read from `tasks.target_gate` on every gated call | gate **on** |
| Task pause / queued / terminal / deadline passed | in-memory flags | `GetTaskControl` before every claim and again right after it (a claim that lost the race goes back to open); pause or queued cancels running intents (polled every 2 s) | no claim; after 15 failed polls (~30 s) the node pauses the task locally |
| Timeout or budget settle | `settling` flag | `tasks.dispatch_closed='settling'`: no new claim; running intents finish under the deadline / zero-budget wrap-up | no claim |
| Delete / archive | in-memory barrier | `dispatch_closed='deleting'`, remote claims revoked; each worker node stops the task, joins its routines and then writes a `task_drain_acks` row (only while the fence stands; any fence change clears the rows); the control drain waits for an ack from every live worker node, up to 8 s (`remoteDrainAckBound`), then goes on and logs the silent nodes | no claim; no ack |
| Run budget | env `ARTEX_*_BUDGET_*`, ledger seeded from `llm_usage` | the control node publishes its budget in `settings.distributed_run_budgets`; a worker uses the last value it read, never its own env; every node syncs spend from `llm_usage` every 2 s (running intents' meters included) and the control node starts the wrap-up as soon as the synced budget cannot pay for a worker | no claim until a budget was read |
| Noise governor | one governor | each live node runs `egress.Share(profile, liveNodes)`; the profile follows `settings.noise_profile` | keep the current governor |
| Capture switch, global proxy, web search | in memory, set by the API | re-read every poll; a change blocks claims for that poll and rebuilds the task agents on the new path | no claim |
| LLM profiles and config (profile save/activate/delete, `/api/llm`, retry policy, failover switches, agent profile bindings) | in memory, rebuilt by the API handler | every change bumps `settings.llm_config_revision`; a worker node follows it every poll (with `settings.agent_settings_revision`), drops its provider caches and re-applies the active profile, so its next intent runs on the new config; a running intent keeps its bundle | keep the current agents |
| LLM recording switch, an agent's saved settings (max turns, run seconds, web search flag: `PUT /api/agents/{key}/config`) | in memory / read when a task bundle is built | the save bumps `settings.agent_settings_revision`; a worker node follows it every poll, reloads the switch and drops its task bundles, so its next claim builds them from the saved rows (c-6bcdd3) | keep the current agents |
| Claim lease | renewed every 15 s, lost on rejection | also cancelled after 60 s without a successful renewal, before another node may take it over at 90 s | run cancelled |
| Control node alive | n/a | a worker claims only while a live `control` node is in `worker_nodes` (planner, deadline coordinator and settle run there) | no claim |
| kill_work / steer_work / work pause and cancel | in-memory work map | `work_signals`, consumed by the node holding the claim (polled every 2 s); work pause/cancel waits until the intent leaves `running` | signal stays queued |
| Settle drain | local in-flight count | plus live remote claims (`LiveRemoteRunningIntents`); when the grace expires, remote claims are revoked | counted as running |

Everything else an agent is built from is read from Postgres per run or per
call, on every node alike, so nothing has to be followed: agent prompts and
wrap-up prompts, the tools table (bindings, enabled flags, descriptions),
custom tools, MCP servers and their per-agent visibility, agent skill
visibility, constraint injection switches, noa compaction, the LLM pool
switches, the worker count, the task concurrency cap, the Python interpreter
and the finding-traffic binding switch. Rewrite rules, tamper hooks and the
browser MCP's proxy wiring belong to the recording proxy, which runs only on
the control node. The audit behind this list (every `applyLLM` caller and
every setting read only in `NewManager`) is card c-6bcdd3; what a worker node
cannot follow is under Limits.

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
- **Clients without a token.** A client that presents no token from a
  non-loopback address is refused, so the proxy is never an open proxy. This
  holds in every role, also for a single node started with
  `ARTEX_PROXY_ALLOW_NONLOOPBACK=1`. `ARTEX_PROXY_ALLOW_UNSCOPED_REMOTE=1` is
  the explicit opt-out (remote manual capture on the global rules only); it is
  ignored on a control node.
- **Browser MCP on a worker node.** The shared MCP row names the control node's
  loopback proxy. Each spawn on a worker replaces it with a loopback credential
  relay (`traffic.StartCredentialRelay`) that forwards to `ARTEX_CONTROL_PROXY`
  with the run's scope token as `Proxy-Authorization`, which Chromium cannot
  send itself, and points `NODE_EXTRA_CA_CERTS` at the fetched control CA. A
  run without a task scope gets a relay whose dials are refused.
- **Finding wake-ups.** The control node pages the task's finding nodes every
  poll and wakes the planner for those a worker node wrote, as a local
  `report_finding` does.
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
- **Transport.** See "Transport" below: the node RPC runs over TLS with the
  control API's CA pinned by default.

## Transport

Worker nodes make two kinds of connection to the control node:

| Connection | Carries | Default protection |
| --- | --- | --- |
| Control API (`ARTEX_CONTROL_API`), node RPC | `tool`/`preview` payloads with captured target traffic (session cookies, credentials), findings | **TLS, CA pinned** (Helm and compose), plus the HMAC on every request and reply |
| Recording proxy (`ARTEX_CONTROL_PROXY`) | the agents' target traffic | plain HTTP to the proxy; HTTPS targets stay TLS inside `CONNECT` |

- **Control side.** The control node serves its API over TLS when
  `ARTEX_TLS_CERT`/`ARTEX_TLS_KEY` (or `-tls-cert`/`-tls-key`) are set. It
  rereads the pair when either file changes (checked at most every 30 s), so a
  renewed certificate (cert-manager, a rotated Secret) is served without a
  restart. A renewal that fails to load keeps the last good pair and logs.
- **Worker side.** `ARTEX_CONTROL_CA` names a PEM file holding the CA that
  signed the control API certificate. With it the worker accepts only
  `https://` and trusts only that CA, never the system roots, so a publicly
  valid certificate for the same name does not pass. The file must hold CA
  certificates only (a private key block is refused). When `ARTEX_CONTROL_API`
  is empty, it is derived from the proxy host as `https://<host>:8787` if a CA
  is pinned, else `http://`. Without a CA an `https://` API is checked against
  the system roots. A plain `http://` API still works (compatibility), but the
  worker logs a WARNING at start-up. The worker reads the CA once at start-up,
  so restart the workers after a CA change.
- **Helm.** `controlTLS.enabled` (default `true`) with `controlTLS.source`:
  `generate` (the chart makes a private CA and certificate once and keeps them
  through `lookup`, which is not available under `helm template`/GitOps),
  `secret` (`controlTLS.existingSecret` with `tls.crt`, `tls.key` and the CA
  under `controlTLS.caKey`), or `certManager` (a `Certificate` from
  `controlTLS.certManager.issuerRef`, which must be a CA-type issuer so the
  Secret's `ca.crt` is the signing CA). The certificate covers
  `<release>-control[.<ns>[.svc[.cluster.local]]]`, `localhost`, `127.0.0.1`
  and `controlTLS.extraDnsNames`. Worker pods mount only the CA key of that
  Secret, never the control key. Probes use HTTPS, the Service port has
  `appProtocol: https`, and the Ingress gets
  `nginx.ingress.kubernetes.io/backend-protocol: HTTPS` (other controllers
  need their own backend-TLS setting).
- **Compose.** `docker-compose.distributed.yml` runs `artex node-tls` once
  (`artex-tls-init`), which writes the control pair to the `artex-tls` volume
  (mounted on the control node only) and `ca.crt` to `artex-control-ca`
  (mounted on the workers). The UI is then `https://localhost:8787`.
- **`artex node-tls`.** `artex node-tls -dir D [-ca-dir C] -names a,b -ips
  10.0.0.5 [-days N]` makes a fresh CA and a server certificate for those names
  (ECDSA P-256) and writes `D/tls.crt`, `D/tls.key` (0600) and `C/ca.crt`. The
  CA key is discarded, so nothing else can be signed with it. Existing files
  are kept, so it is safe to run on every start. To renew, delete the files,
  rerun the command and restart the workers.
- **The proxy hop is still plain HTTP.** Agent tools speak to the recording
  proxy as an ordinary HTTP proxy, and most of them cannot use a TLS proxy.
  HTTPS targets are unaffected: the MITM TLS session runs from the worker to
  the control proxy inside `CONNECT`. Plain-HTTP target traffic and the
  per-task scope token in `Proxy-Authorization` do cross the network in clear
  text. Until a worker-side TLS relay exists (card c-677166), keep
  the proxy port on a trusted network. The Helm `networkPolicy.enabled` admits
  only worker pods to it.

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
  `ARTEX_SECRET_KEY`, LLM configuration, `ARTEX_CONTROL_PROXY=http://<control>:8788`,
  `ARTEX_CONTROL_API=https://<control>:<api port>` and `ARTEX_CONTROL_CA`.
  Make the files with `artex node-tls -dir tls -names <control host> -ips
  <control ip>`, start the control node with `ARTEX_TLS_CERT=tls/tls.crt
  ARTEX_TLS_KEY=tls/tls.key`, and copy only `tls/ca.crt` to the workers.
  Worker nodes must reach the control API as well as the proxy.

## Limits

These are known and carded on the ARTEX board:

- **Transcripts stay on the node that ran the intent.** If a claim is taken
  over by another host, or a reopened intent is rerun elsewhere, the run starts
  over instead of resuming its transcript.
- **`egress.Share` floors in-flight limits at 1 per node.** With N nodes, up to
  max(limit, N) requests to one host can be in flight at once. The request rate
  is divided exactly. A DB-backed per-host slot was decided against (HANDOFF
  Decisions, c-18f7a8); `egress/share_bound_test.go` pins the bound.
- **Agent processes can reach other cluster services.** On a worker host they
  can reach the network like any target, including the control API and
  Postgres, just as the bundled compose Postgres is reachable today. Deny those
  addresses with `ARTEX_EGRESS_DENY` or a NetworkPolicy.
- **The capture-store tools need the control node.** On a worker node they
  are calls to the control node (see "Evidence on worker nodes"). While it is
  down they fail, and so does a finding that carries traffic refs.
- **The compose overlay changes how the host reaches the capture proxy.** The
  overlay binds the control proxy to `0.0.0.0` inside the container. A
  host-side client (for example the browser extension through the published
  `127.0.0.1:8788`) then arrives from the Docker gateway address and is
  refused without a scope token.
- **A worker node needs the control node to start first.** It exits at start
  (and is restarted) until it can read the control node's published CA
  certificate. It claims nothing until a control node has published its run
  budget.
- **Skills are files on each node's disk.** A worker node loads the skills an
  agent may see from its own skill directory; the database holds only which
  skills each agent may see. A skill created or edited on the control node
  does not exist on a worker node until the same files are there (a shared
  volume, or the same image). A skill missing on the worker is left out of
  the agent's tool list for that run; nothing fails.
- **Configuration from the environment and the config file stays per
  node.** An LLM provider from the environment (used only when no profile is
  active), `task_user_agents` from the config file and the archive store are
  read once at start; `ARTEX_RSI_LESSONS` is read from the environment on
  each use. Each node reads its own. Set them alike on every node, and restart
  a node after changing the ones read at start.
- **The lesson block is chosen when a task's bundle is built.** A changed
  bench lesson pin (`PUT /api/rsi/bench-lessons`) reaches a task when its
  bundle is next rebuilt, on the control node as on a worker node.
