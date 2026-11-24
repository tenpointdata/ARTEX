#!/bin/sh
# ARTEX container egress filter, sourced by start.sh before artex starts.
#
# Agent processes run as the executor uid (-exec-uid, default 10001) and, with
# -exec-uid-range LO-HI, as the per-task uids of that range. This installs OUTPUT
# rules that match those uids only (-m owner --uid-owner); the server (root)
# and the Postgres/LLM connections it makes are untouched. Default-deny for the
# executor:
#   - cloud metadata: 169.254.0.0/16 (also covers link-local), 100.100.100.200,
#     fd00:ec2::254 and fe80::/10
#   - the Docker host gateway (the host's localhost services via host.docker.internal)
#   - the container's own ARTEX API port (any local address)
# The LAN and the internet stay reachable: they are the pentest targets.
#
# Environment (all optional):
#   ARTEX_EGRESS_FILTER  0 turns the whole filter off
#   ARTEX_EGRESS_DENY    extra space/comma separated CIDRs or IPs to deny
#   ARTEX_EGRESS_ALLOW   CIDRs or IPs accepted before any deny rule (exceptions, for
#                        example one host service the engagement needs)
#   ARTEX_EGRESS_DENY_HOSTS   host names resolved once at start (every IPv4/IPv6
#                        address) and denied like ARTEX_EGRESS_DENY: the database
#                        and the control API Services in Kubernetes. A name that
#                        does not resolve after a few tries is logged loudly.
#   ARTEX_EGRESS_ALLOW_HOSTS  host names resolved the same way and accepted like
#                        ARTEX_EGRESS_ALLOW (the control node's recording proxy when
#                        the cluster's CIDRs are denied)
# Needs root and CAP_NET_ADMIN (docker-compose.yml adds it). Without them it logs a
# loud warning and the agent runs unfiltered.

egress_log() { echo "[artex] egress: $*" >&2; }

# egress_resolve NAMES...: prints every address the names resolve to. Each name is
# retried for a few seconds (cluster DNS can lag a fresh pod); one that never
# resolves is logged and skipped.
egress_resolve() {
	for h in "$@"; do
		tries=0
		while :; do
			addrs="$(getent ahosts "$h" 2>/dev/null | awk '{print $1}' | sort -u)"
			[ -n "$addrs" ] && break
			tries=$((tries + 1))
			if [ "$tries" -ge "${ARTEX_EGRESS_RESOLVE_TRIES:-10}" ]; then
				egress_log "WARNING: $h does not resolve; it is NOT in the egress rules"
				break
			fi
			sleep 1
		done
		[ -n "$addrs" ] && echo "$addrs"
	done
}

# egress_build IPTABLES FAMILY: (re)creates the ARTEX_EGRESS chain and the OUTPUT jump.
egress_build() {
	ipt="$1"; fam="$2"
	"$ipt" -w -N ARTEX_EGRESS 2>/dev/null || "$ipt" -w -F ARTEX_EGRESS || return 1
	for owner in "$uid" $uidrange; do
		"$ipt" -w -C OUTPUT -m owner --uid-owner "$owner" -j ARTEX_EGRESS 2>/dev/null \
			|| "$ipt" -w -I OUTPUT 1 -m owner --uid-owner "$owner" -j ARTEX_EGRESS || return 1
	done
	for c in $allow; do
		case "$c" in *:*) [ "$fam" = 6 ] || continue ;; *) [ "$fam" = 4 ] || continue ;; esac
		"$ipt" -w -A ARTEX_EGRESS -d "$c" -j ACCEPT || return 1
	done
	# The API, on loopback and on every address of this container.
	"$ipt" -w -A ARTEX_EGRESS -m addrtype --dst-type LOCAL -p tcp --dport "$port" -j REJECT || return 1
	if [ "$fam" = 4 ]; then
		for c in 169.254.0.0/16 100.100.100.200 $gw4 $hd4 $extra; do
			case "$c" in ''|*:*) continue ;; esac
			"$ipt" -w -A ARTEX_EGRESS -d "$c" -j REJECT || return 1
		done
	else
		for c in fd00:ec2::254 fe80::/10 $gw6 $extra; do
			case "$c" in *:*) ;; *) continue ;; esac
			"$ipt" -w -A ARTEX_EGRESS -d "$c" -j REJECT || return 1
		done
	fi
}

# egress_apply ARTEX_ARGS...: reads -exec-uid and -addr from the artex arguments.
egress_apply() {
	if [ "${ARTEX_EGRESS_FILTER:-1}" = "0" ]; then
		egress_log "WARNING: filter disabled by ARTEX_EGRESS_FILTER=0; agent processes can reach cloud metadata, the host gateway and the API port"
		return 0
	fi

	uid=10001
	uidrange=""
	port=8787
	prev=""
	for a in "$@"; do
		case "$prev" in
			-exec-uid|--exec-uid) uid="$a" ;;
			-exec-uid-range|--exec-uid-range) uidrange="$a" ;;
			-addr|--addr) port="${a##*:}" ;;
		esac
		case "$a" in
			-exec-uid=*|--exec-uid=*) uid="${a#*=}" ;;
			-exec-uid-range=*|--exec-uid-range=*) uidrange="${a#*=}" ;;
			-addr=*|--addr=*) port="${a##*:}" ;;
		esac
		prev="$a"
	done
	case "$uid" in
		-*) egress_log "exec uid drop is off; no uid to filter, egress filter not installed"; return 0 ;;
		''|*[!0-9]*) egress_log "WARNING: unreadable -exec-uid '$uid'; egress filter NOT installed"; return 0 ;;
	esac
	case "$port" in ''|*[!0-9]*) port=8787 ;; esac
	# artex refuses a malformed range, so an unreadable one never runs unfiltered.
	# Canonical decimal, as artex reads it: iptables would read a leading zero as
	# octal, and artex trims spaces.
	uidrange="$(echo "$uidrange" | tr -d ' ')"
	if [ -n "$uidrange" ]; then
		lo="${uidrange%%-*}"; hi="${uidrange#*-}"
		case "$lo:$hi" in
			*[!0-9:]*|:*|*:) egress_log "WARNING: unreadable -exec-uid-range '$uidrange'; egress filter NOT installed (artex refuses to start with it)"; return 1 ;;
		esac
		uidrange="$(expr "$lo" + 0)-$(expr "$hi" + 0)"
	fi

	if [ "$(id -u)" != "0" ]; then
		egress_log "WARNING: not running as root; egress filter NOT installed (agent can reach metadata, host gateway, API port)"
		return 0
	fi
	if ! command -v iptables >/dev/null 2>&1; then
		egress_log "WARNING: iptables not found; egress filter NOT installed"
		return 0
	fi

	gw4="$(ip -4 route show default 2>/dev/null | awk '/default/ {print $3; exit}')"
	gw6="$(ip -6 route show default 2>/dev/null | awk '/default/ {print $3; exit}')"
	hd4="$(getent ahostsv4 host.docker.internal 2>/dev/null | awk '{print $1; exit}')"
	allow="$(echo "${ARTEX_EGRESS_ALLOW:-}" | tr ',' ' ')"
	extra="$(echo "${ARTEX_EGRESS_DENY:-}" | tr ',' ' ')"
	# shellcheck disable=SC2046 # word splitting of the name lists is intended
	denyhosts="$(egress_resolve $(echo "${ARTEX_EGRESS_DENY_HOSTS:-}" | tr ',' ' ') | tr '\n' ' ')"
	# shellcheck disable=SC2046
	allowhosts="$(egress_resolve $(echo "${ARTEX_EGRESS_ALLOW_HOSTS:-}" | tr ',' ' ') | tr '\n' ' ')"
	extra="$(echo $extra $denyhosts)"
	allow="$(echo $allow $allowhosts)"

	if egress_build iptables 4; then
		egress_log "installed for uid $uid${uidrange:+ and uids $uidrange} (ipv4): deny metadata, gateway ${gw4:-none}, host.docker.internal ${hd4:-none}, local port $port${extra:+, extra: $extra}${allow:+; allow: $allow}"
	else
		egress_log "WARNING: could not install iptables rules (missing CAP_NET_ADMIN? add cap_add: NET_ADMIN); agent processes are NOT egress filtered"
		return 0
	fi
	if command -v ip6tables >/dev/null 2>&1 && egress_build ip6tables 6; then
		egress_log "installed for uid $uid${uidrange:+ and uids $uidrange} (ipv6)"
	else
		egress_log "WARNING: ipv6 rules not installed (no ip6tables or IPv6 unavailable); fd00:ec2::254 stays reachable if the container has IPv6"
	fi
}
