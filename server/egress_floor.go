package server

import (
	"fmt"
	"log"
	"net/netip"
	"os"
	"slices"
	"strings"

	"github.com/Autumn-27/artex/intercept"
	"github.com/Autumn-27/artex/ownerfw"
)

// The agent egress floor. The platform's own dials never reach a link-local or
// instance-metadata address (intercept.DialRefusingMetadata), but an agent
// process (Bash, a custom tool, a stdio MCP server) dials for itself. In the
// Docker image egress.sh refuses those dials with iptables owner rules; a
// native Linux root install has no egress.sh, and a container without
// CAP_NET_ADMIN cannot install it. So the server installs a floor itself at
// start-up, on Linux with the privilege drop on (the server is root): a chain
// of its own (egressFloorChain) that refuses intercept.MetadataPrefixes to the
// executor uid, the MCP uid and the per-task uid range. It is independent of
// egress.sh's chain, which adds the Docker-only rules (gateway, API port), and
// leaves it as it is.
//
// The floor is then checked, not trusted: the isolation self-test dials the
// metadata addresses from an executor process (metadataProbeAddrs). A dial that
// connects stops the start on Linux root, whether the floor could not be
// installed or something else lets the packet through, unless the operator
// accepts it with AllowMetadataEgressEnv. A host that does not answer on those
// addresses starts with a warning when the floor is missing.

// AllowMetadataEgressEnv set to 1 lets the server start although an agent
// process reached an instance-metadata address in the self-test.
const AllowMetadataEgressEnv = "ARTEX_ALLOW_METADATA_EGRESS"

// egressFilterEnv set to 0 turns the agent egress filter off: egress.sh's and
// the floor. The self-test still dials the metadata addresses.
const egressFilterEnv = "ARTEX_EGRESS_FILTER"

// egressAllowEnv lists CIDRs or IPs the filter lets through (egress.sh reads it too).
const egressAllowEnv = "ARTEX_EGRESS_ALLOW"

// egressFloorChain is the floor's iptables chain. egress.sh uses ARTEX_EGRESS.
const egressFloorChain = "ARTEX_AGENT_FLOOR"

// egressFloorTag is the comment on the floor's OUTPUT jumps.
const egressFloorTag = "artex-floor"

// metadataProbeAddrs are the instance-metadata endpoints the self-test dials as
// the executor: AWS/GCP/Azure/OpenStack, AWS over IPv6, Alibaba, Azure
// WireServer and Oracle (see intercept.IsMetadataAddr).
var metadataProbeAddrs = []string{
	"169.254.169.254:80",
	"[fd00:ec2::254]:80",
	"100.100.100.200:80",
	"168.63.129.16:80",
	"192.0.0.192:80",
}

// MetadataProbeAddrs returns the addresses the self-test dials, or none when the
// operator accepted metadata egress with AllowMetadataEgressEnv.
func MetadataProbeAddrs() []string {
	if envTrue(AllowMetadataEgressEnv) {
		log.Printf("[egress] WARNING: %s=1: the self-test does not check that agent processes are refused the cloud instance-metadata addresses", AllowMetadataEgressEnv)
		return nil
	}
	return append([]string(nil), metadataProbeAddrs...)
}

func envTrue(name string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "1", "true", "yes":
		return true
	}
	return false
}

// EgressFloorStatus is what InstallAgentEgressFloor did, for the log and the
// self-test's failure message.
type EgressFloorStatus struct {
	// Installed lists the families the floor is in force for.
	Installed []ownerfw.Family
	// Problem says why the floor is missing or partial; empty when complete.
	Problem string
}

// String is a one-line summary.
func (s EgressFloorStatus) String() string {
	if len(s.Installed) == 0 {
		return "the agent egress floor is not installed: " + s.Problem
	}
	fams := make([]string, len(s.Installed))
	for i, f := range s.Installed {
		fams[i] = fmt.Sprintf("ipv%d", f)
	}
	out := "the agent egress floor is installed (" + strings.Join(fams, ", ") + ")"
	if s.Problem != "" {
		out += "; " + s.Problem
	}
	return out
}

// agentEgressRules are the floor's rules for p: refuse the metadata prefixes to
// every uid an agent process can run as. allow are operator exceptions
// (egressAllowEnv); entries that are not an address come back in bad. resolv is
// the text of /etc/resolv.conf: DNS (port 53) passes to a nameserver in a
// refused range, since a native host's resolver can be link-local (AWS
// 169.254.169.253, GCP 169.254.169.254) or the Azure WireServer. Port 53 to any
// other refused address is refused like the rest.
func agentEgressRules(p ExecPrivilege, allow, resolv string) (r ownerfw.Rules, bad []string, err error) {
	owners := []string{execIDString(p.UID), execIDString(p.mcpUID())}
	if p.Range.Enabled() {
		owners = append(owners, p.Range.String())
	}
	pass, bad := parsePrefixList(allow)
	refuse := intercept.MetadataPrefixes()
	var dns []ownerfw.Port
	for _, ns := range refusedNameservers(resolv, refuse) {
		dns = append(dns, ownerfw.Port{Proto: "udp", Port: 53, To: ns}, ownerfw.Port{Proto: "tcp", Port: 53, To: ns})
	}
	r = ownerfw.Rules{
		Chain:     egressFloorChain,
		Tag:       egressFloorTag,
		Owners:    owners,
		Pass:      pass,
		PassPorts: dns,
		Refuse:    refuse,
	}
	return r, bad, r.Validate()
}

// refusedNameservers are the resolv.conf nameservers that fall in refuse, as
// host prefixes, without duplicates.
func refusedNameservers(resolv string, refuse []netip.Prefix) []netip.Prefix {
	var out []netip.Prefix
	for _, line := range strings.Split(resolv, "\n") {
		f := strings.Fields(line)
		if len(f) < 2 || f[0] != "nameserver" {
			continue
		}
		a, err := netip.ParseAddr(f[1])
		if err != nil {
			continue
		}
		a = a.WithZone("").Unmap()
		h := netip.PrefixFrom(a, a.BitLen())
		if slices.ContainsFunc(refuse, func(p netip.Prefix) bool { return p.Contains(a) }) && !slices.Contains(out, h) {
			out = append(out, h)
		}
	}
	return out
}

// parsePrefixList reads space or comma separated CIDRs and IPs. Entries that are
// neither come back in bad: one typo must not drop the whole floor.
func parsePrefixList(s string) (out []netip.Prefix, bad []string) {
	for _, f := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' || r == '\n' }) {
		if p, err := netip.ParsePrefix(f); err == nil {
			out = append(out, p)
		} else if a, err := netip.ParseAddr(f); err == nil {
			out = append(out, netip.PrefixFrom(a, a.BitLen()))
		} else {
			bad = append(bad, f)
		}
	}
	return out, bad
}

// metadataReachedHint is appended to a self-test failure where an executor
// process reached a metadata address: what to do, chosen from the floor's state.
func metadataReachedHint(floor EgressFloorStatus) string {
	accept := "or set " + AllowMetadataEgressEnv + "=1 to accept it (that only skips this check; " + egressAllowEnv + " is what lets an address through)"
	if len(floor.Installed) > 0 {
		return floor.String() + ", so another rule lets the packet through: check " + egressAllowEnv + " and the host's other OUTPUT rules, " + accept
	}
	return floor.String() + ". Install iptables and run the server as root with CAP_NET_ADMIN (Docker: cap_add: NET_ADMIN), " + accept
}
