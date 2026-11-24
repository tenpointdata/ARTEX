# ARTEX Helm chart

Deploys ARTEX: a control node plus horizontally scalable worker nodes, sharing one PostgreSQL.

## Authorized use only

ARTEX acts against live targets. Only deploy and run it against systems you are explicitly
authorized to test. The guardrails (scope/target gate, interception approvals, budgets, noise
governor, audit trail) run identically on every worker node; scaling out never relaxes them.
Do not weaken them through `extraEnv`.

## Roles

| Role | Workload | What it does |
| --- | --- | --- |
| `control` | Deployment, 1 replica, strategy `Recreate` | UI/API on 8787, planners, schedulers, notifier. Holds in-memory planner state, so it must stay a single replica. `control.localWorkers` (default `0`) sets how many intent workers run inside it; `0` sends all intents to worker nodes. |
| `worker` | Deployment, `worker.replicas` (default 2) | No API. Plaintext `GET /healthz` on `worker.healthPort` (8790). Claims intents from PostgreSQL, runs them through the same guards; tool traffic goes through the control node's recording proxy (`<release>-control-proxy:8788`, scope-token gated) and evidence calls go to the control API's signed node RPC over TLS. `ARTEX_NODE_ID` is the pod name; `worker.concurrency` sets `ARTEX_NODE_WORKERS`. |

All nodes share the same PostgreSQL, `ARTEX_SECRET_KEY`, LLM provider env, and egress/noise/budget
settings (one ConfigMap plus one Secret).

## Install

```sh
helm install artex ./deploy/helm/artex \
  --set llm.anthropicApiKey=sk-ant-... \
  --set image.tag=v0.7.1          # a published release tag
kubectl logs deploy/artex-control | grep "Setup token"
kubectl port-forward svc/artex-control 8787:8787
```

The default image tag is `v<appVersion>`. The 0.7.0 release predates the GHCR publish step, so set
`image.tag` to a release that is published there.

## Scaling and separate hosts

`helm upgrade artex ./deploy/helm/artex --set worker.replicas=6`. Workers carry a required
podAntiAffinity on `kubernetes.io/hostname` (`worker.antiAffinity=hard`), so two workers never share
a host. With fewer schedulable nodes than replicas the extra pods stay Pending; use `soft` to prefer
spreading or `none` plus your own `worker.affinity`. A PodDisruptionBudget (`maxUnavailable: 1`) and
a 120 s termination grace period let a worker finish or abandon its claim; a lost claim is retaken by
another node after 90 s.

## PostgreSQL

- Default: a single-instance bundled StatefulSet (`postgres:16-alpine`, PVC) for evaluation. No
  backups, no HA. **Production should use an external managed PostgreSQL.**
- External: `--set postgresql.bundled.enabled=false --set postgresql.external.host=db.example.com
  --set postgresql.external.existingSecret=artex-pg` (password under key `password`). `sslmode`
  defaults to `require` for external.

## Secrets

`secretKey.existingSecret` (key `secret-key`) holds `ARTEX_SECRET_KEY` (base64, 32 bytes). Without it
the chart generates one on first install and keeps it across upgrades via `lookup` (kept on
uninstall too). `lookup` returns nothing under `helm template` or GitOps renderers without cluster
access, which would regenerate the key; supply `existingSecret` in those cases. Back this key up:
without it stored secrets cannot be decrypted. LLM keys: `llm.existingSecret` (its keys become env
vars) or inline `llm.anthropicApiKey` / `llm.openaiApiKey`.

## Security context

ARTEX containers add `NET_ADMIN` (`egress.enabled`) so `egress.sh` can install the iptables filter
for the agent executor uid, set `allowPrivilegeEscalation: false`, and use seccomp `RuntimeDefault`.
`runAsNonRoot` is `false` on purpose: the binary runs as root to drop agent processes to an
unprivileged executor uid. All other capabilities are dropped except the small set in
`securityContext.capabilities.add`. Disabling `egress.enabled` removes the executor egress filter;
do that only if you enforce egress with a NetworkPolicy or firewall. The optional
`networkPolicy.enabled` allows worker ingress only on the health port.

## TLS between workers and the control node

On by default (`controlTLS.enabled=true`). The control API serves TLS, and workers reach
`https://<release>-control:8787` with the CA pinned (`ARTEX_CONTROL_CA`, mounted from the TLS
Secret's CA key only; worker pods never mount the control key). Pick the certificate source:

| `controlTLS.source` | Certificate | Notes |
| --- | --- | --- |
| `generate` (default) | private CA and certificate made by the chart, kept via `lookup` | not for `helm template`/GitOps (it would regenerate); renew by deleting `<release>-control-tls` and upgrading, then roll the workers |
| `secret` | `controlTLS.existingSecret` with `tls.crt`, `tls.key` and the CA under `controlTLS.caKey` | the certificate must cover `<release>-control` |
| `certManager` | a `Certificate` from `controlTLS.certManager.issuerRef` | the issuer must be CA-type (CA, Vault) so `ca.crt` is the signing CA; renewals are reloaded by the control node within 30 s |

With TLS on, probes use HTTPS, the Service port has `appProtocol: https`, the Ingress gets
`nginx.ingress.kubernetes.io/backend-protocol: HTTPS`, and `kubectl port-forward` serves
`https://localhost:8787`. The recording proxy hop (port 8788) stays plain HTTP. See
docs/DISTRIBUTED.md "Transport".

## Ingress

`control.ingress.*` creates a standard Ingress with TLS. Set `settings.trustedProxies` to your
ingress controller CIDR so client addresses are read correctly. The capture proxy's Service
(`<release>-control-proxy`) is for worker nodes only; workers have no Service.
