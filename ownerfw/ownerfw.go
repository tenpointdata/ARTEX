// Package ownerfw builds and applies packet-filter rules that match local
// processes by their uid (iptables/ip6tables -m owner --uid-owner) and refuse
// their connections to given destinations. It knows nothing of the product that
// uses it: the caller names the chain, the owners, the destinations to refuse and
// the exceptions.
//
// A Rules value is turned into a Plan per address family: a list of Steps that
// Apply runs through an iptables binary. Applying is idempotent. The chain is
// created or flushed and rebuilt, and the OUTPUT jump for each owner is inserted
// at the top only when it is missing, so a second start converges on the same
// rules. A jump left from an earlier start for an owner that is gone stays: it
// refuses more, never less.
//
// Only locally generated packets of the owners pass through the chain; the
// traffic of every other uid (root included) is untouched.
package ownerfw

import (
	"fmt"
	"net/netip"
	"strconv"
)

// Family is an address family, with the binary that filters it.
type Family int

const (
	IPv4 Family = 4
	IPv6 Family = 6
)

// Binary is the iptables binary of the family.
func (f Family) Binary() string {
	if f == IPv6 {
		return "ip6tables"
	}
	return "iptables"
}

func (f Family) holds(p netip.Prefix) bool {
	if f == IPv6 {
		return p.Addr().Is6()
	}
	return p.Addr().Is4()
}

// Port is a transport port that passes the chain whatever its destination.
type Port struct {
	Proto string // "tcp" or "udp"
	Port  uint16
}

// Rules is what one chain enforces for a set of owners.
type Rules struct {
	// Chain is the name of the chain the rules live in (at most 28 characters).
	Chain string
	// Owners are uids or inclusive uid ranges, as --uid-owner takes them:
	// "10001" or "20000-29999".
	Owners []string
	// Pass are destinations that leave the chain before any refusal (RETURN):
	// operator exceptions. Rules in other chains still apply to them.
	Pass []netip.Prefix
	// PassPorts leave the chain before any refusal too, for every destination.
	PassPorts []Port
	// Refuse are the destinations the owners are refused (REJECT).
	Refuse []netip.Prefix
}

// Step is one iptables invocation (arguments after the binary). When it fails
// and Fallback is set, Fallback runs instead and its result counts.
type Step struct {
	Args     []string
	Fallback []string
}

// Plan is the ordered steps that install Rules for one family.
type Plan struct {
	Family Family
	Steps  []Step
}

// Validate reports a Rules value that iptables would refuse or misread.
func (r Rules) Validate() error {
	if r.Chain == "" || len(r.Chain) > 28 {
		return fmt.Errorf("ownerfw: chain name %q must be 1 to 28 characters", r.Chain)
	}
	if len(r.Owners) == 0 {
		return fmt.Errorf("ownerfw: no owner to filter")
	}
	for _, o := range r.Owners {
		if !validOwner(o) {
			return fmt.Errorf("ownerfw: owner %q is not a decimal uid or LO-HI uid range", o)
		}
	}
	for _, p := range r.PassPorts {
		if p.Proto != "tcp" && p.Proto != "udp" || p.Port == 0 {
			return fmt.Errorf("ownerfw: pass port %s/%d is not a tcp or udp port", p.Proto, p.Port)
		}
	}
	return nil
}

// validOwner accepts canonical decimal uids and ranges: iptables reads a
// leading zero as octal.
func validOwner(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == '-' {
			return i > 0 && validUID(s[:i]) && validUID(s[i+1:]) && atoi(s[:i]) <= atoi(s[i+1:])
		}
	}
	return validUID(s)
}

func validUID(s string) bool {
	n, err := strconv.ParseUint(s, 10, 32)
	return err == nil && strconv.FormatUint(n, 10) == s
}

func atoi(s string) uint64 { n, _ := strconv.ParseUint(s, 10, 32); return n }

// Plan returns the steps that install r for family f. Destinations of the other
// family are left out.
func (r Rules) Plan(f Family) Plan {
	p := Plan{Family: f}
	add := func(args ...string) { p.Steps = append(p.Steps, Step{Args: append([]string{"-w"}, args...)}) }
	p.Steps = append(p.Steps, Step{Args: []string{"-w", "-N", r.Chain}, Fallback: []string{"-w", "-F", r.Chain}})
	for _, o := range r.Owners {
		match := []string{"OUTPUT", "-m", "owner", "--uid-owner", o, "-j", r.Chain}
		p.Steps = append(p.Steps, Step{
			Args:     append([]string{"-w", "-C"}, match...),
			Fallback: append([]string{"-w", "-I", "OUTPUT", "1"}, match[1:]...),
		})
	}
	for _, port := range r.PassPorts {
		add("-A", r.Chain, "-p", port.Proto, "--dport", strconv.Itoa(int(port.Port)), "-j", "RETURN")
	}
	for _, d := range r.Pass {
		if f.holds(d) {
			add("-A", r.Chain, "-d", d.Masked().String(), "-j", "RETURN")
		}
	}
	// A refused destination is matched twice: as the packet's destination and as
	// the connection's original destination. The nat table rewrites addresses
	// before the filter table sees them, so a DNAT of a refused address (ECS
	// redirects 169.254.170.2 to a loopback agent) would otherwise pass.
	for _, d := range r.Refuse {
		if f.holds(d) {
			add("-A", r.Chain, "-d", d.Masked().String(), "-j", "REJECT")
			add("-A", r.Chain, "-m", "conntrack", "--ctorigdst", d.Masked().String(), "-j", "REJECT")
		}
	}
	return p
}
