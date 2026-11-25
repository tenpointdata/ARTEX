# E7 kind install (c-8cfbfe) and cluster egress re-resolve (c-d4946f)

Moved from HANDOFF.md Current state on 2026-11-24.

**2026-11-24 — Cluster egress deny follows moved pods (`c-d4946f`, E7; in review on
`ej/20261124-172839-c17308`).** This branch merges the unlanded c-8cfbfe branch (`ej/20261124-155353-b1c60a`),
so landing it lands both. `egress.sh` re-resolves `ARTEX_EGRESS_DENY_HOSTS`/`ALLOW_HOSTS` every
`ARTEX_EGRESS_REFRESH` s (chart `egress.refreshSeconds`, default 10) in a background loop `start.sh` starts,
and rewrites `ARTEX_EGRESS` in one `iptables-restore --noflush` commit when an answer changes (also at start).
A name that stops resolving keeps its last addresses. `NAME:PORT` denies one TCP port; the chart uses it
for a new headless `<release>-control-pods` (API port on the control pod IP). The PostgreSQL and
control-pods Services publish not-ready addresses. Verified live on the `artex-e7` kind cluster (own
namespace, since removed): after `kubectl delete pod artex-postgresql-0` (10.244.1.13 → .14) and a control
pod delete (.12 → .15), `scripts/k8s-egress-probe.sh` showed the executor refused at the new IPs on both
pods with 0 restarts, while root, the proxy and the Juice Shop target stayed open. Allow/uid-range paths
checked in a container. Window left: a moved pod is reachable for about the interval plus the DNS TTL;
`egress.clusterCIDRs` has none.

**2026-11-24 — E7 follow-up (`c-8cfbfe`, in review on `ej/20261124-155353-b1c60a`).** Built on the E7
branch before main's #46 squash and merged with main after; the kind run predates `controlTLS`. The chart was installed on a 3-node kind cluster and
the checks are recorded in `docs/DISTRIBUTED.md` ("Recorded install"): two workers on separate hosts, nodes API,
a task whose 6 intents split across both hosts, traffic recorded on the control node, and the caps set unchanged.
Five defects were fixed, each with a test:
- volume-root modes (`server/execpriv_volume_unix.go`);
- worker nodes follow agent settings (`server/worker_node_settings.go`, `settings.agent_settings_revision`),
  which closes `c-7ad4da`, the capture switch read at start-up;
- the planner wakes after an empty first settle poll (`settleCursor`, `server/engine_distributed.go`);
- `jwt.key` is created atomically (`server/auth.go`);
- worker rollouts use `maxSurge: 0`.

New chart default: agent egress deny for PostgreSQL and the control API (`egress.denyClusterServices`; the
`egress.sh` variables are `ARTEX_EGRESS_DENY_HOSTS` and `ARTEX_EGRESS_ALLOW_HOSTS`, and DNS stays allowed),
plus the strict `egress.clusterCIDRs` mode. The lab fixtures are in `deploy/lab/` (an LLM stub and the kind
values); `scripts/k8s-egress-probe.sh` runs the egress probe.

## Decisions and gotchas

- **2026-11-24** — c-d4946f: the re-resolve loop is in `egress.sh` (started by `start.sh`), not in the Go
  server. One code path writes the executor rules for Docker and Kubernetes, and the loop rebuilds with the
  same `egress_rules` text as start-up. Rebuilds use `iptables-restore --noflush` so the chain is replaced in
  one commit, with no unfiltered moment. Unresolvable names keep their last addresses (fail closed). CIDR
  auto-detection was rejected (no portable source of the pod/Service CIDRs inside a pod); strict mode stays
  opt-in.
- **2026-11-24** — c-8cfbfe: the worker-pod egress deny is in the uid-scoped `egress.sh` filter, not a
  NetworkPolicy. A NetworkPolicy applies to the whole pod, and the root server process in the same pod must
  reach PostgreSQL. The deny is name-based by default (zero configuration, resolved at start) and CIDR-based
  as an opt-in, because the pod and Service CIDRs differ per cluster. The volume roots are tightened by the
  binary rather than by the chart (fsGroup or an initContainer), so bare-metal bind mounts with loose modes
  are fixed too.
- **Socket-LB CNIs and the executor egress filter.** The filter matches the destination the pod's own
  netfilter sees. With kube-proxy that is the Service ClusterIP; a CNI that rewrites Services at
  `connect()` (Cilium kube-proxy replacement) shows the backend pod IP instead, so denying the control pod
  IP would also refuse the proxy Service. Use `egress.denyClusterServices=false` plus explicit
  `egress.denyHosts` there. Untested on such a CNI.
