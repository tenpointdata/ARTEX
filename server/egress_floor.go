package server

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/netip"
	"os"
	"strings"
	"time"

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

// egressFloorTimeout bounds all iptables calls of one start.
const egressFloorTimeout = 30 * time.Second

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
// every uid an agent process can run as. DNS passes, since a native host's
// resolver can be a link-local address (AWS 169.254.169.253) or the Azure
// WireServer; no metadata service answers HTTP on port 53. allow are operator
// exceptions (egressAllowEnv).
func agentEgressRules(p ExecPrivilege, allow string) (ownerfw.Rules, error) {
	owners := []string{execIDString(p.UID), execIDString(p.mcpUID())}
	if p.Range.Enabled() {
		owners = append(owners, p.Range.String())
	}
	pass, bad := parsePrefixList(allow)
	for _, b := range bad {
		log.Printf("[egress] WARNING: %s: %q is not an IP address or CIDR; ignored by the agent egress floor", egressAllowEnv, b)
	}
	r := ownerfw.Rules{
		Chain:     egressFloorChain,
		Owners:    owners,
		Pass:      pass,
		PassPorts: []ownerfw.Port{{Proto: "udp", Port: 53}, {Proto: "tcp", Port: 53}},
		Refuse:    intercept.MetadataPrefixes(),
	}
	return r, r.Validate()
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

// InstallAgentEgressFloor installs the floor on Linux with the privilege drop on
// and logs the result. run executes iptables (ownerfw.ExecRunner in production).
// It never fails the start itself: the self-test decides, from what an executor
// process can reach.
func InstallAgentEgressFloor(goos string, p ExecPrivilege, run ownerfw.Runner) EgressFloorStatus {
	st := installAgentEgressFloor(goos, p, run)
	switch {
	case len(st.Installed) == 0:
		log.Printf("[egress] WARNING: %s; agent processes are not refused the cloud instance-metadata and link-local addresses unless another filter does it", st)
	case st.Problem != "":
		log.Printf("[egress] WARNING: %s", st)
	default:
		log.Printf("[egress] %s for uids %s", st, strings.Join(egressOwners(p), ", "))
	}
	return st
}

func egressOwners(p ExecPrivilege) []string {
	r, _ := agentEgressRules(p, "")
	return r.Owners
}

func installAgentEgressFloor(goos string, p ExecPrivilege, run ownerfw.Runner) EgressFloorStatus {
	switch {
	case goos != "linux":
		return EgressFloorStatus{Problem: "it needs Linux (iptables owner match)"}
	case !p.Enabled:
		return EgressFloorStatus{Problem: "the privilege drop is off, so agent processes have no uid of their own to filter"}
	case strings.TrimSpace(os.Getenv(egressFilterEnv)) == "0":
		return EgressFloorStatus{Problem: egressFilterEnv + "=0 turns it off"}
	}
	rules, err := agentEgressRules(p, os.Getenv(egressAllowEnv))
	if err != nil {
		return EgressFloorStatus{Problem: err.Error()}
	}
	ctx, cancel := context.WithTimeout(context.Background(), egressFloorTimeout)
	defer cancel()
	var st EgressFloorStatus
	if err := ownerfw.Apply(ctx, run, rules.Plan(ownerfw.IPv4)); err != nil {
		return EgressFloorStatus{Problem: egressFloorHint(err)}
	}
	st.Installed = append(st.Installed, ownerfw.IPv4)
	if err := ownerfw.Apply(ctx, run, rules.Plan(ownerfw.IPv6)); err != nil {
		st.Problem = "ipv6 rules not installed (" + egressFloorHint(err) + "); fd00:ec2::254 and fe80::/10 stay reachable if the host has IPv6"
	} else {
		st.Installed = append(st.Installed, ownerfw.IPv6)
	}
	return st
}

func egressFloorHint(err error) string {
	if errors.Is(err, ownerfw.ErrNoBinary) {
		return err.Error() + " (install iptables)"
	}
	return err.Error() + " (missing CAP_NET_ADMIN, or no owner match in the kernel?)"
}
