#!/bin/sh
# Run the E7 install checks (docs/DISTRIBUTED.md "Recorded install") against a live
# Helm release and print them as a Markdown table, ready to paste into the record.
#
# Usage: scripts/k8s-install-checks.sh NAMESPACE [RELEASE]
#
# Environment (all optional; a check whose input is missing prints "skipped"):
#   ARTEX_URL     the UI/API base through the Ingress, e.g. https://artex.example.com.
#                 Its certificate is verified against the system roots (no -k).
#   ARTEX_TOKEN   a JWT for that API (POST /api/auth/login), for the nodes/task/traffic rows
#   ARTEX_TASK    id of a task already run against an authorized target
#   ARTEX_TARGET  host:port of that target, for the egress probe
#   LLM_SECRET    name of the Secret given as llm.existingSecret
#
# Read-only except the NetworkPolicy row, which starts one short-lived busybox pod in
# NAMESPACE (np-probe) that tries the control proxy port; it is deleted afterwards.
# Requires kubectl, curl, jq and openssl. Point ARTEX only at systems you are
# authorized to test.
set -u
ns="${1:?namespace}"
rel="${2:-artex}"
case "$rel" in *artex*) full="$rel" ;; *) full="$rel-artex" ;; esac
url="${ARTEX_URL:-}"; token="${ARTEX_TOKEN:-}"; task="${ARTEX_TASK:-}"; target="${ARTEX_TARGET:-}"
k() { kubectl -n "$ns" "$@"; }
row() { printf '| %s | %s |\n' "$1" "$2"; }
api() { curl -fsS -H "Authorization: Bearer $token" "$url$1"; }
sel="app.kubernetes.io/instance=$rel"

echo "| Check | Result |"
echo "|---|---|"

# Hosts: distinct nodes, and whether those nodes are real machines (not kind containers).
nodes="$(k get pods -l "$sel,app.kubernetes.io/component=worker" -o jsonpath='{range .items[*]}{.spec.nodeName}{"\n"}{end}' | sort)"
distinct="$(printf '%s\n' "$nodes" | sort -u | grep -c .)"
total="$(printf '%s\n' "$nodes" | grep -c .)"
ctl="$(k get pods -l "$sel,app.kubernetes.io/component=control" -o jsonpath='{.items[0].spec.nodeName}')"
hostinfo=""
for n in $(printf '%s\n%s\n' "$nodes" "$ctl" | sort -u); do
	pid="$(kubectl get node "$n" -o jsonpath='{.spec.providerID}')"
	ip="$(kubectl get node "$n" -o jsonpath='{.status.addresses[?(@.type=="InternalIP")].address}')"
	hostinfo="$hostinfo $n ($ip, ${pid:-no providerID});"
done
case "$hostinfo" in *kind://*) kind=" **kind nodes, not separate machines**" ;; *) kind="" ;; esac
row "Pods on separate hosts" "$total workers on $distinct nodes, control on $ctl.$kind Nodes:$hostinfo"

# CNI and NetworkPolicy enforcement.
cni="$(kubectl get pods -A -o jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}' | grep -oE '^(calico-node|cilium|kindnet|flannel|weave-net|antrea-agent)' | sort -u | tr '\n' ' ')"
nps="$(k get networkpolicy -l "$sel" -o name | tr '\n' ' ')"
if [ -n "$nps" ]; then
	# A non-worker pod must be refused at the recording proxy (8788); the API (8787) stays open.
	k delete pod np-probe --ignore-not-found >/dev/null 2>&1
	np="$(k run np-probe --rm -i --restart=Never --quiet --image=busybox:1.36 -- sh -c \
		"nc -z -w4 $full-control-proxy 8788 && echo proxy=open || echo proxy=refused; nc -z -w4 $full-control 8787 && echo api=open || echo api=refused" 2>&1 | tr '\n' ' ')"
	case "$np" in *proxy=refused*api=open*) verdict="enforced" ;; *) verdict="**not enforced**" ;; esac
	row "CNI / NetworkPolicy" "CNI: ${cni:-unknown}; policies: $nps; non-worker pod: $np→ $verdict"
else
	row "CNI / NetworkPolicy" "CNI: ${cni:-unknown}; **no NetworkPolicy** (set networkPolicy.enabled)"
fi

# Storage: every PVC bound, with its class and provisioner.
pvc=""
for p in $( (k get pvc -l "$sel" -o name; k get pvc -o name | grep -- "$full-postgresql") | sort -u); do
	sc="$(k get "$p" -o jsonpath='{.spec.storageClassName}')"
	ph="$(k get "$p" -o jsonpath='{.status.phase}')"
	prov="$(kubectl get storageclass "$sc" -o jsonpath='{.provisioner}' 2>/dev/null)"
	pvc="$pvc ${p#*/}: $ph, $sc ($prov);"
done
case "$pvc" in *rancher.io/local-path*) pvc="$pvc **local-path (kind default)**" ;; esac
row "Storage" "${pvc:-no PVCs found}"

# Isolation self-test and control TLS, per pod, from the logs.
iso=""; tls=""
for pod in $(k get pods -l "$sel" -o name | grep -v postgresql); do
	l="$(k logs "$pod" 2>/dev/null)"
	case "$l" in *"[isolation] self-test passed"*) r=passed ;; *) r="**not passed**" ;; esac
	iso="$iso ${pod#pod/}: $r;"
	case "$pod" in *worker*)
		api_env="$(k get "$pod" -o jsonpath='{.spec.containers[0].env[?(@.name=="ARTEX_CONTROL_API")].value}')"
		case "$l" in *"over plain HTTP"*) t="**plain HTTP**" ;; *) t="${api_env:-api from configmap}, no plain-HTTP warning" ;; esac
		tls="$tls ${pod#pod/}: $t;" ;;
	esac
done
row "Isolation self-test" "$iso"
row "Control API TLS (worker → control)" "$tls"

# Ingress TLS: the certificate must verify against the system roots.
if [ -n "$url" ]; then
	code="$(curl -sS -o /dev/null -w '%{http_code}' "$url/" 2>&1)"
	host="$(printf '%s' "$url" | sed -E 's#^https?://##; s#/.*##')"
	cert="$(printf '' | openssl s_client -connect "$host:443" -servername "${host%:*}" 2>/dev/null | openssl x509 -noout -issuer -enddate 2>/dev/null | tr '\n' ' ')"
	row "Ingress TLS" "$url → HTTP $code with a verified certificate; $cert"
else
	row "Ingress TLS" "skipped (ARTEX_URL unset)"
fi

# LLM key: the Secret exists and is wired into every ARTEX pod.
if [ -n "${LLM_SECRET:-}" ]; then
	keys="$(k get secret "$LLM_SECRET" -o json | jq -r '.data | keys | join(",")')"
	wired="$(k get pods -l "$sel" -o json | jq -r --arg s "$LLM_SECRET" '[.items[] | select(.metadata.name | test("postgresql") | not) | select([.spec.containers[0].envFrom[]?.secretRef.name] | index($s))] | length')"
	row "LLM key" "Secret $LLM_SECRET (keys: $keys) wired into $wired ARTEX pods; model used is proven by the task row"
else
	row "LLM key" "skipped (LLM_SECRET unset)"
fi

# API rows: nodes, task run, traffic.
if [ -n "$url" ] && [ -n "$token" ]; then
	nodes_json="$(api /api/settings/nodes)"
	row "\`GET /api/settings/nodes\`" "$(printf '%s' "$nodes_json" | jq -r '[.nodes[] | select(.live)] | group_by(.role) | map("\(length) \(.[0].role)") | join(", ")') live; $(printf '%s' "$nodes_json" | jq -r '[.nodes[] | select(.live | not)] | length') not live"
	if [ -n "$task" ]; then
		row "Task run" "$(api "/api/exploration/intents?task=$task" | jq -r '"\(length) intents (\(group_by(.state) | map("\(length) \(.[0].state)") | join(", "))); owners: \([.[].owner // "" | select(. != "") | sub("/work#[0-9]+$"; "")] | unique | join(", "))"')"
	else
		row "Task run" "skipped (ARTEX_TASK unset)"
	fi
	row "Traffic" "$(api /api/traffic | jq -r '"\(.count // 0) exchanges recorded on the control node (enabled: \(.enabled // true))"')"
else
	row "API rows (nodes, task, traffic)" "skipped (ARTEX_URL or ARTEX_TOKEN unset)"
fi

# Agent egress: run the probe and count results that differ from the chart defaults.
probe="$(dirname "$0")/k8s-egress-probe.sh"
out="$(sh "$probe" "$ns" "$rel" "$target" 2>&1)"
bad="$(printf '%s\n' "$out" | awk -v ctl="$full-control:" '
	$1 == "root" && $3 != "open" { n++ }
	$1 == "exec" {
		deny = ($2 ~ /:5432$/) || index($2, ctl) == 1 || ($2 ~ /^[0-9.]+:8787$/)
		if (deny && $3 != "refused") n++
		if (!deny && $3 != "open") n++
	}
	END { print n + 0 }')"
log="${TMPDIR:-/tmp}/artex-egress-probe-$ns.log"
printf '%s\n' "$out" >"$log"
row "Agent egress (\`scripts/k8s-egress-probe.sh\`)" "$bad unexpected results across $(printf '%s\n' "$out" | grep -c '^== ') pods (full output: $log)"
