#!/bin/sh
# Probe the container egress filter (egress.sh) from inside a running artex container.
#
# Usage: scripts/docker-egress-probe.sh CONTAINER [HOST_PORT] [LAN_URL]
#   CONTAINER  name of the running artex container (docker compose ps)
#   HOST_PORT  a TCP port with a service listening on the Docker host, reached as
#              host.docker.internal:HOST_PORT (start one: python3 -m http.server PORT)
#   LAN_URL    an http URL on the LAN (for example the router), expected to stay reachable
#
# For each target it runs curl twice: as root (the server) and as the executor uid
# through `/app/artex __artex-exec-drop`. Expected: the executor is refused
# (curl exit 7, "Connection refused" from the REJECT rule) for metadata, the host
# gateway and the API port, root is not, and both reach the LAN.
set -u
c="${1:?container name}"
hp="${2:-}"
lan="${3:-}"
uid="${ARTEX_PROBE_UID:-10001}"

echo "== egress log line"
docker logs "$c" 2>&1 | grep '\[artex\] egress' || echo "(no [artex] egress line)"
echo "== iptables -S ARTEX_EGRESS"
docker exec "$c" iptables -S ARTEX_EGRESS
docker exec "$c" iptables -S OUTPUT
echo "== ip6tables -S ARTEX_EGRESS"
docker exec "$c" ip6tables -S ARTEX_EGRESS 2>&1 || true

probe() {
	url="$1"
	for who in root exec; do
		if [ "$who" = root ]; then pre=""; else pre="/app/artex __artex-exec-drop $uid $uid --"; fi
		# shellcheck disable=SC2086
		out="$(docker exec "$c" $pre curl -sS -o /dev/null -m 5 -w '%{http_code}' "$url" 2>&1)"; rc=$?
		printf '%-5s %-45s exit=%s %s\n' "$who" "$url" "$rc" "$(echo "$out" | tr '\n' ' ')"
	done
}

echo "== probes"
probe http://169.254.169.254/latest/meta-data/
probe http://127.0.0.1:8787/api/health
ip="$(docker exec "$c" hostname -i | awk '{print $1}')"
probe "http://$ip:8787/api/health"
[ -n "$hp" ] && probe "http://host.docker.internal:$hp/"
[ -n "$lan" ] && probe "$lan"
probe https://example.com/
