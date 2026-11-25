# Lab: ARTEX on a 3-node kind cluster (two worker hosts)

**Lab only.** Everything here targets OWASP Juice Shop in the cluster's own `lab`
namespace. Point ARTEX only at systems you are authorized to test.

This reproduces the E7 install record in `docs/DISTRIBUTED.md` without an LLM key:
`llmstub/` is a scripted OpenAI-compatible server (no model) that answers each role
deterministically: the goals role registers the goal, the planner adds `STUB_INTENTS`
intents and proves the goal once the workers have reported, and each worker runs one
`curl` against `STUB_TARGET_BASE` + `STUB_TARGET_PATH` and records the status as a fact.
`STUB_CMD_PREFIX` is prepended to that shell command (`sleep 6; ` makes intents overlap
so both worker nodes claim). It detects roles by their tool sets, and the planner's
state by the JSON overview in its prompt, so a prompt change can break it. It is a
smoke-test fixture, not a model.

```sh
kind create cluster --config deploy/lab/kind-3node.yaml --kubeconfig /tmp/kc
export KUBECONFIG=/tmp/kc
docker build -t artex:lab . && docker build -t llmstub:lab deploy/lab/llmstub
kind load docker-image artex:lab llmstub:lab --name artex-e7
kubectl create ns lab && kubectl create ns artex
kubectl -n lab create deployment juice --image=bkimminich/juice-shop:v17.1.1 --port=3000
kubectl -n lab expose deployment juice --port=3000
kubectl -n lab create deployment llmstub --image=llmstub:lab --port=8080
kubectl -n lab patch deployment llmstub -p '{"spec":{"template":{"spec":{"containers":[{"name":"llmstub","imagePullPolicy":"Never"}]}}}}'
kubectl -n lab set env deployment/llmstub STUB_INTENTS=6 'STUB_CMD_PREFIX=sleep 6; '
kubectl -n lab expose deployment llmstub --port=8080
kubectl -n artex create secret generic artex-key --from-literal=secret-key="$(head -c32 /dev/urandom | base64)"
helm install artex deploy/helm/artex -n artex -f deploy/lab/values-kind.yaml
kubectl -n artex get pods -o wide                    # two workers on two hosts
scripts/k8s-egress-probe.sh artex artex juice.lab.svc.cluster.local:3000
```

Then set the admin password from the setup token (`kubectl -n artex logs deploy/artex-control
| grep "Setup token"`), turn traffic capture on in Settings, and create a task whose goal names
`http://juice.lab.svc.cluster.local:3000`. Check `GET /api/settings/nodes`, the intents' `owner`
column (`<pod>/work#N` from both worker pods) and `GET /api/traffic` on the control node.
