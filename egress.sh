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
#   ARTEX_EGRESS_DENY_HOSTS   host names whose every IPv4/IPv6 address is denied like
#                        ARTEX_EGRESS_DENY: the database and the control API in
#                        Kubernetes. NAME:PORT denies only that TCP port (the control
#                        pods, whose recording proxy port must stay open). A name that
#                        does not resolve after a few tries at start is logged loudly.
#   ARTEX_EGRESS_ALLOW_DNS  1 (default) accepts port 53 (udp/tcp) to the nameservers
#                        in /etc/resolv.conf before any deny rule, so a denied range
#                        that holds the resolver (a cluster's Service CIDR) does not
#                        take name resolution from the agent; 0 leaves DNS to the rules
#   ARTEX_EGRESS_ALLOW_HOSTS  host names resolved the same way and accepted like
#                        ARTEX_EGRESS_ALLOW (the control node's recording proxy when
#                        the cluster's CIDRs are denied)
#   ARTEX_EGRESS_REFRESH seconds between re-resolutions of the *_HOSTS names (default
#                        10; 0 resolves once). A changed answer (a rescheduled database
#                        pod) rebuilds the chain in one iptables-restore commit; a name
#                        that stops resolving keeps its last addresses.
# Needs root and CAP_NET_ADMIN (docker-compose.yml adds it). Without them it logs a
# loud warning and the agent runs unfiltered.

egress_log() { echo "[artex] egress: $*" >&2; }

# egress_resolve TRIES NAMES...: prints "NAME ADDR" for every address of each name
# (ADDR@PORT for NAME:PORT), or "NAME -" when the name did not resolve within TRIES
# one-second tries (cluster DNS can lag a fresh pod).
egress_resolve() {
	max="$1"; shift
	for h in "$@"; do
		name="$h"; hport=""
		case "$h" in *:*) name="${h%%:*}"; hport="${h#*:}" ;; esac
		tries=0
		while :; do
			addrs="$(getent ahosts "$name" 2>/dev/null | awk '{print $1}' | sort -u)"
			[ -n "$addrs" ] && break
			tries=$((tries + 1))
			[ "$tries" -ge "$max" ] && break
			sleep 1
		done
		if [ -z "$addrs" ]; then echo "$h -"; continue; fi
		for a in $addrs; do echo "$h $a${hport:+@$hport}"; done
	done
}

# egress_carry NEW OLD: resolution lines; a name unresolved in NEW keeps its OLD
# addresses, so a database pod that is briefly gone stays denied (fail closed).
egress_carry() {
	printf '%s\n' "$1" | while read -r n a; do
		[ -n "$n" ] || continue
		if [ "$a" != "-" ]; then echo "$n $a"; continue; fi
		kept="$(printf '%s\n' "$2" | awk -v n="$n" '$1 == n && $2 != "-"')"
		if [ -n "$kept" ]; then echo "$kept"; else echo "$n -"; fi
	done
}

# egress_addrs LINES: the resolved addresses of resolution lines, one line, sorted.
egress_addrs() { printf '%s\n' "$1" | awk '$2 != "-" && $2 != "" {print $2}' | sort -u | tr '\n' ' ' | sed 's/ $//'; }

# egress_unresolved LINES: names without an address, for the log.
egress_unresolved() { printf '%s\n' "$1" | awk '$2 == "-" {print $1}' | tr '\n' ' ' | sed 's/ $//'; }

# egress_rules FAMILY: the ARTEX_EGRESS chain as iptables-restore input. Declaring the
# chain under --noflush replaces its rules in the same commit, so a rebuild never
# leaves the executor unfiltered.
egress_rules() {
	fam="$1"
	echo "*filter"
	echo ":ARTEX_EGRESS - [0:0]"
	for c in $allow; do
		case "$c" in *:*) [ "$fam" = 6 ] || continue ;; *) [ "$fam" = 4 ] || continue ;; esac
		echo "-A ARTEX_EGRESS -d $c -j ACCEPT"
	done
	for ns in $dns; do
		case "$ns" in *:*) [ "$fam" = 6 ] || continue ;; *) [ "$fam" = 4 ] || continue ;; esac
		for proto in udp tcp; do echo "-A ARTEX_EGRESS -d $ns -p $proto --dport 53 -j ACCEPT"; done
	done
	# The API, on loopback and on every address of this container.
	echo "-A ARTEX_EGRESS -m addrtype --dst-type LOCAL -p tcp --dport $port -j REJECT"
	if [ "$fam" = 4 ]; then set -- 169.254.0.0/16 100.100.100.200 $gw4 $hd4 $extra
	else set -- fd00:ec2::254 fe80::/10 $gw6 $extra; fi
	for c in "$@"; do
		a="${c%@*}"
		case "$a" in '') continue ;; *:*) [ "$fam" = 6 ] || continue ;; *) [ "$fam" = 4 ] || continue ;; esac
		case "$c" in
			*@*) echo "-A ARTEX_EGRESS -d $a -p tcp --dport ${c##*@} -j REJECT" ;;
			*) echo "-A ARTEX_EGRESS -d $a -j REJECT" ;;
		esac
	done
	echo "COMMIT"
}

# egress_build IPTABLES FAMILY: (re)writes the ARTEX_EGRESS chain and the OUTPUT jumps.
egress_build() {
	ipt="$1"; fam="$2"
	egress_rules "$fam" | "$ipt-restore" -w --noflush || return 1
	for owner in "$uid" $uidrange; do
		"$ipt" -w -C OUTPUT -m owner --uid-owner "$owner" -j ARTEX_EGRESS 2>/dev/null \
			|| "$ipt" -w -I OUTPUT 1 -m owner --uid-owner "$owner" -j ARTEX_EGRESS || return 1
	done
}

# egress_lists TRIES: resolves the *_HOSTS names (carrying over the last answers of
# names that fail) and sets extra and allow from the static lists plus the answers.
egress_lists() {
	# shellcheck disable=SC2046 # word splitting of the name lists is intended
	denyres="$(egress_carry "$(egress_resolve "$1" $(echo "${ARTEX_EGRESS_DENY_HOSTS:-}" | tr ',' ' '))" "${denyres:-}")"
	# shellcheck disable=SC2046
	allowres="$(egress_carry "$(egress_resolve "$1" $(echo "${ARTEX_EGRESS_ALLOW_HOSTS:-}" | tr ',' ' '))" "${allowres:-}")"
	# shellcheck disable=SC2086
	extra="$(echo $static_deny $(egress_addrs "$denyres"))"
	# shellcheck disable=SC2086
	allow="$(echo $static_allow $(egress_addrs "$allowres"))"
}

# egress_refresh: one re-resolution; rebuilds the installed families when an answer
# changed. A failed rebuild keeps the previous rules and is retried next round.
egress_refresh() {
	old_extra="$extra"; old_allow="$allow"; old_denyres="$denyres"; old_allowres="$allowres"
	egress_lists 1
	[ "$extra" = "$old_extra" ] && [ "$allow" = "$old_allow" ] && return 0
	for fam in $egress_families; do
		if [ "$fam" = 4 ]; then ipt=iptables; else ipt=ip6tables; fi
		if ! egress_build "$ipt" "$fam"; then
			egress_log "WARNING: re-resolved names changed but the ipv$fam rules could not be rebuilt; keeping the previous rules"
			extra="$old_extra"; allow="$old_allow"; denyres="$old_denyres"; allowres="$old_allowres"
			return 1
		fi
	done
	egress_log "re-resolved: deny now ${extra:-none} (was ${old_extra:-none}); allow now ${allow:-none} (was ${old_allow:-none})"
	gone="$(egress_unresolved "$(printf '%s\n%s' "$denyres" "$allowres")")"
	[ -z "$gone" ] || egress_log "WARNING: not resolving now: $gone"
}

# egress_refresh_start: runs egress_refresh every ARTEX_EGRESS_REFRESH seconds in the
# background when names are in use and the filter is installed; sets egress_refresh_pid.
egress_refresh_start() {
	egress_refresh_pid=""
	[ -n "${egress_families:-}" ] || return 0
	[ -n "${ARTEX_EGRESS_DENY_HOSTS:-}${ARTEX_EGRESS_ALLOW_HOSTS:-}" ] || return 0
	every="${ARTEX_EGRESS_REFRESH:-10}"
	case "$every" in ''|*[!0-9]*) egress_log "WARNING: unreadable ARTEX_EGRESS_REFRESH '$every'; using 10"; every=10 ;; esac
	if [ "$every" = 0 ]; then
		egress_log "WARNING: ARTEX_EGRESS_REFRESH=0; names are resolved once, a rescheduled pod's new address is NOT denied"
		return 0
	fi
	( trap - INT TERM; while sleep "$every"; do egress_refresh; done ) &
	egress_refresh_pid=$!
	egress_log "re-resolving $(echo ${ARTEX_EGRESS_DENY_HOSTS:-} ${ARTEX_EGRESS_ALLOW_HOSTS:-} | tr ',' ' ') every ${every}s"
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
	static_allow="$(echo "${ARTEX_EGRESS_ALLOW:-}" | tr ',' ' ')"
	static_deny="$(echo "${ARTEX_EGRESS_DENY:-}" | tr ',' ' ')"
	denyres=""; allowres=""
	egress_lists "${ARTEX_EGRESS_RESOLVE_TRIES:-10}"
	gone="$(egress_unresolved "$(printf '%s\n%s' "$denyres" "$allowres")")"
	[ -z "$gone" ] || egress_log "WARNING: $gone does not resolve; NOT in the egress rules until it does"
	dns=""
	if [ "${ARTEX_EGRESS_ALLOW_DNS:-1}" != "0" ] && [ -r /etc/resolv.conf ]; then
		dns="$(awk '$1 == "nameserver" {print $2}' /etc/resolv.conf | tr '\n' ' ')"
		dns="$(echo $dns)"
	fi
	egress_families=""

	if egress_build iptables 4; then
		egress_log "installed for uid $uid${uidrange:+ and uids $uidrange} (ipv4): deny metadata, gateway ${gw4:-none}, host.docker.internal ${hd4:-none}, local port $port${extra:+, extra: $extra}${allow:+; allow: $allow}${dns:+; dns: $dns}"
	else
		egress_log "WARNING: could not install iptables rules (missing CAP_NET_ADMIN? add cap_add: NET_ADMIN); agent processes are NOT egress filtered"
		return 0
	fi
	egress_families=4
	if command -v ip6tables >/dev/null 2>&1 && egress_build ip6tables 6; then
		egress_families="4 6"
		egress_log "installed for uid $uid${uidrange:+ and uids $uidrange} (ipv6)"
	else
		egress_log "WARNING: ipv6 rules not installed (no ip6tables or IPv6 unavailable); fd00:ec2::254 stays reachable if the container has IPv6"
	fi
}
