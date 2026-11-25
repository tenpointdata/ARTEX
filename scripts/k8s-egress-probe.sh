#!/bin/sh
# Probe the agent egress filter (egress.sh) in every ARTEX pod of a Helm release.
#
# Usage: scripts/k8s-egress-probe.sh NAMESPACE [RELEASE] [TARGET_HOST:PORT]
#   RELEASE      Helm release name (default artex); Services are <fullname>-postgresql,
#                <fullname>-control and <fullname>-control-proxy, with fullname = RELEASE
#                when it contains "artex", else RELEASE-artex (the chart's rule)
#   TARGET       an authorized lab target expected to stay reachable for agents
#
# For each pod it opens a TCP connection as root (the server) and as the executor
# uid through `/app/artex __artex-exec-drop`. Expected with the chart defaults
# (egress.denyClusterServices): the executor is refused PostgreSQL (the Service
# name and the database pod IP) and the control API, and reaches the control
# proxy and the target; root reaches PostgreSQL and the control API.
set -u
ns="${1:?namespace}"
rel="${2:-artex}"
target="${3:-}"
uid="${ARTEX_PROBE_UID:-10001}"
case "$rel" in *artex*) full="$rel" ;; *) full="$rel-artex" ;; esac

pods="$(kubectl -n "$ns" get pods -l "app.kubernetes.io/instance=$rel" -o name | grep -v postgresql)"
for pod in $pods; do
	echo "== $pod"
	kubectl -n "$ns" logs "$pod" | grep '\[artex\] egress' | tail -2
	kubectl -n "$ns" exec "$pod" -- sh -c '
		uid="$1"; shift
		pgip="$(getent ahostsv4 "$1" | awk "{print \$1; exit}")"
		for t in "$1:5432" "$pgip:5432" "$2:8787" "$3:8788" ${4:+"$4"}; do
			h="${t%:*}"; p="${t##*:}"
			for who in root exec; do
				if [ "$who" = root ]; then pre=""; else pre="/app/artex __artex-exec-drop $uid $uid --"; fi
				# shellcheck disable=SC2086
				if $pre timeout 4 bash -c "</dev/tcp/$h/$p" 2>/dev/null; then r=open; else r=refused; fi
				printf "%-5s %-40s %s\n" "$who" "$t" "$r"
			done
		done' probe "$uid" "$full-postgresql" "$full-control" "$full-control-proxy" "$target"
done
