# E7 TLS proxy hop: agent-driven check on kind (c-67993f)

Follow-up of c-6e9c74 (one Docker host, token row inserted by hand). Run 2026-11-24 (01:08–01:22 UTC on
2026-11-25) on a 3-node kind cluster (`kind-3node.yaml`, kind v0.33.0, kindnetd v20260820, which enforces
NetworkPolicy). Lab only: target OWASP Juice Shop in the cluster's own `lab` namespace.

## What was built

The proxy-hop TLS code (c-677166) and the kind/lab fixes (c-8cfbfe, c-6bcdd3) are on two unmerged
branches. The image was built from a **throwaway** merge tree made with
`git merge-tree --write-tree ej/20261124-163818-870578 ej/20261124-172031-c2bb7b` (tree `d4c89232`),
extracted to `/tmp` and not committed anywhere. Conflicts resolved only to build:
`traffic/scope_relay.go` took the c-677166 side (its `connSet`), with the duplicate `maxRelayConns`
const dropped since `relay_conns.go` (c-6bcdd3) also declares it; `db/migrate_test.go` took the
c-677166 side. `go build ./...`, `go vet` and `go test ./traffic -run 'Relay|TLSFront|Scope'` passed on
that tree. **The two branches carry parallel extractions of the same relay helper (`connSet` in
`conn_relay.go` vs `relayConns` in `relay_conns.go`); whoever lands the second must unify them**
(card c-ccc7a8).

Install: `deploy/lab/README.md` steps, `helm install … -f deploy/lab/values-kind.yaml
--set networkPolicy.enabled=true` (controlTLS on by default, source `generate`). Control pod on
`artex-e7-worker2`; workers `…-94lm2` on worker2 (same host as control) and `…-pckqg` on `artex-e7-worker`.

## (1) Real agent-driven task, token minted by remoteTaskProxy

LLM = `deploy/lab/llmstub` (scripted OpenAI-compatible stub, `STUB_INTENTS=6`). Traffic capture on via
`PUT /api/settings`. Task 1: 6 intents, owners `…-94lm2/work#1..3` and `…-pckqg/work#1..3`, 6
`STUBFACT … HTTP_STATUS=200` facts, status `done`. The control node recorded 6 exchanges
(`GET http://juice.lab…:3000/rest/products/search?q=apple`, 200). The recorded requests carry no
`Proxy-Authorization`.

Task 2, with `proxy_scope_tokens` polled every second: one row per worker node appeared at task start
(`task_id=2`, `node_id=artex-worker-…-94lm2` and `…-pckqg`, created 01:20:30) and stayed for the run;
0 rows after the task ended (ForgetNodeProxyScopeTokens). 6 more exchanges recorded (12 total). Nothing was
inserted by hand.

## (2) Pod-network capture

`tcpdump -i any` in privileged hostNetwork pods on both kind workers (`tcp port 8789 or 8788 or 3000`),
read with tshark, across both tasks:

| node | TLS handshakes to 8789 (SNI) | HTTP on 8789 | plaintext markers in 8789 payload* | packets on 8788 |
|---|---|---|---|---|
| artex-e7-worker (cross-host leg) | 16, SNI `artex-control`, TLS 1.3 | 0 | 0 | 0 |
| artex-e7-worker2 (control's host) | 24, SNI `artex-control`, TLS 1.3 | 0 | 0 | 0 |

\* all 8789 TCP payload reassembled to bytes and grepped for `proxy-authorization|GET /rest|juice|HTTP/1`;
the same grep over port 3000 found 35 hits, so the method sees plaintext when it is there. Frames tshark
did not label TLS on worker2 are second copies of the same segments (seen on eth0 and the veth, marked
retransmissions) and are inside that grep. Paths seen: worker pod → Service `10.96.34.179:8789` →
control pod `10.244.2.6:8789`; plaintext HTTP appears only from the control pod to the target (expected:
the recording proxy dials it). The front→proxy `127.0.0.1:8788` leg never leaves the control pod.

## NetworkPolicy on 8789

`openssl s_client` to `artex-control-proxy.artex.svc:8789`: from a worker pod CONNECTED; from a stray
netshoot pod in `artex` and one in `lab`, timed out (twice each). Control: with
`networkPolicy.enabled=false` the stray pod in `artex` CONNECTED; re-enabled, it timed out again.

## Not checked

A real model (stub only), a CNI other than kindnet, separate machines (kind shares one Docker host).
