// Package ownerfw builds and keeps packet-filter rules that match local
// processes by their uid (iptables/ip6tables -m owner --uid-owner) and refuse
// their connections to given destinations. It knows nothing of the product that
// uses it: the caller names the chain, the tag, the owners, the destinations to
// refuse and the exceptions.
//
// Rules are installed in one iptables-restore --noflush transaction per address
// family (Keeper.Apply): the chain is rebuilt and every OUTPUT jump to it is
// replaced by one jump per current owner, inserted at the top of OUTPUT and
// tagged with a comment. The kernel never sees an empty or half-built chain,
// and a jump for an owner that is no longer current (an old uid range) is
// removed. Keeper.Drift then tells when something else (a firewalld or ufw
// reload, a netfilter-persistent restore, a rule inserted above the jumps)
// changed the result, so the caller can re-apply.
//
// Only locally generated packets of the owners pass through the chain; the
// traffic of every other uid (root included) is untouched.
package ownerfw

import (
	"fmt"
	"net/netip"
	"strconv"
	"strings"
)

// Family is an address family, with the binaries that filter it.
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

// RestoreBinary is the iptables-restore binary of the family.
func (f Family) RestoreBinary() string { return f.Binary() + "-restore" }

func (f Family) String() string { return "ipv" + strconv.Itoa(int(f)) }

func (f Family) holds(p netip.Prefix) bool {
	if f == IPv6 {
		return p.Addr().Is6()
	}
	return p.Addr().Is4()
}

// Port is a transport port that passes the chain. To limits it to one
// destination; the zero prefix means every destination.
type Port struct {
	Proto string // "tcp" or "udp"
	Port  uint16
	To    netip.Prefix
}

// Rules is what one chain enforces for a set of owners.
type Rules struct {
	// Chain is the name of the chain the rules live in (at most 28 characters).
	// Every OUTPUT rule that jumps to it belongs to these Rules.
	Chain string
	// Tag is the comment put on the OUTPUT jumps, so an operator reading
	// iptables -S sees whose they are. Letters, digits, '-', '_' and '.' only.
	Tag string
	// Owners are uids or inclusive uid ranges, as --uid-owner takes them:
	// "10001" or "20000-29999".
	Owners []string
	// Pass are destinations that leave the chain before any refusal (RETURN):
	// operator exceptions. Rules in other chains still apply to them.
	Pass []netip.Prefix
	// PassPorts leave the chain before any refusal too.
	PassPorts []Port
	// Refuse are the destinations the owners are refused (REJECT).
	Refuse []netip.Prefix
}

// Validate reports a Rules value that iptables would refuse or misread.
func (r Rules) Validate() error {
	if r.Chain == "" || len(r.Chain) > 28 || strings.ContainsAny(r.Chain, " \t\n\"'") {
		return fmt.Errorf("ownerfw: chain name %q must be 1 to 28 characters without spaces or quotes", r.Chain)
	}
	if !validTag(r.Tag) {
		return fmt.Errorf("ownerfw: tag %q must be 1 to 64 letters, digits, '-', '_' or '.'", r.Tag)
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

func validTag(s string) bool {
	if s == "" || len(s) > 64 {
		return false
	}
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_' || c == '.') {
			return false
		}
	}
	return true
}

// validOwner accepts canonical decimal uids and ranges: iptables reads a
// leading zero as octal.
func validOwner(s string) bool {
	if lo, hi, ok := strings.Cut(s, "-"); ok {
		return validUID(lo) && validUID(hi) && atoi(lo) <= atoi(hi)
	}
	return validUID(s)
}

func validUID(s string) bool {
	n, err := strconv.ParseUint(s, 10, 32)
	return err == nil && strconv.FormatUint(n, 10) == s
}

func atoi(s string) uint64 { n, _ := strconv.ParseUint(s, 10, 32); return n }

// jumpSpec is the OUTPUT rule (after "-A OUTPUT"/"-I OUTPUT 1") for one owner.
func (r Rules) jumpSpec(owner string) string {
	return "-m owner --uid-owner " + owner + " -m comment --comment " + r.Tag + " -j " + r.Chain
}

// chainSpecs are the chain's rules for family f, in order. Destinations of the
// other family are left out.
func (r Rules) chainSpecs(f Family) []string {
	var out []string
	for _, p := range r.PassPorts {
		if p.To.IsValid() && !f.holds(p.To) {
			continue
		}
		dst := ""
		if p.To.IsValid() {
			dst = "-d " + p.To.Masked().String() + " "
		}
		out = append(out, dst+"-p "+p.Proto+" --dport "+strconv.Itoa(int(p.Port))+" -j RETURN")
	}
	for _, d := range r.Pass {
		if f.holds(d) {
			out = append(out, "-d "+d.Masked().String()+" -j RETURN")
		}
	}
	// A refused destination is matched twice: as the packet's destination and as
	// the connection's original destination. The nat table rewrites addresses
	// before the filter table sees them, so a DNAT of a refused address (ECS
	// redirects 169.254.170.2 to a loopback agent) would otherwise pass.
	for _, d := range r.Refuse {
		if f.holds(d) {
			out = append(out, "-d "+d.Masked().String()+" -j REJECT",
				"-m conntrack --ctorigdst "+d.Masked().String()+" -j REJECT")
		}
	}
	return out
}

// Render is what r installs for family f, as iptables-restore lines without the
// table header: the OUTPUT jumps in order, then the chain's rules. Two Rules
// with the same Render install the same thing.
func (r Rules) Render(f Family) string {
	var b strings.Builder
	for _, o := range r.Owners {
		b.WriteString("-A OUTPUT " + r.jumpSpec(o) + "\n")
	}
	for _, s := range r.chainSpecs(f) {
		b.WriteString("-A " + r.Chain + " " + s + "\n")
	}
	return b.String()
}

// restoreInput is the iptables-restore --noflush transaction that installs r for
// f, given the current OUTPUT rules (iptables -S OUTPUT). Declaring the chain
// creates or flushes it inside the transaction; every OUTPUT rule that jumps to
// the chain is deleted and the current owners' jumps are inserted at the top.
func (r Rules) restoreInput(f Family, output []string) string {
	var b strings.Builder
	b.WriteString("*filter\n:" + r.Chain + " - [0:0]\n")
	for _, l := range output {
		if isJumpTo(l, r.Chain) {
			b.WriteString("-D OUTPUT" + strings.TrimPrefix(l, "-A OUTPUT") + "\n")
		}
	}
	for i := len(r.Owners) - 1; i >= 0; i-- { // inserted at 1 in reverse: final order is Owners
		b.WriteString("-I OUTPUT 1 " + r.jumpSpec(r.Owners[i]) + "\n")
	}
	for _, s := range r.chainSpecs(f) {
		b.WriteString("-A " + r.Chain + " " + s + "\n")
	}
	b.WriteString("COMMIT\n")
	return b.String()
}

// removeInput deletes every OUTPUT jump to chain and, when it exists, the chain.
func removeInput(chain string, output []string, chainExists bool) string {
	var b strings.Builder
	b.WriteString("*filter\n")
	for _, l := range output {
		if isJumpTo(l, chain) {
			b.WriteString("-D OUTPUT" + strings.TrimPrefix(l, "-A OUTPUT") + "\n")
		}
	}
	if chainExists {
		b.WriteString("-F " + chain + "\n-X " + chain + "\n")
	}
	b.WriteString("COMMIT\n")
	return b.String()
}

// fields splits an iptables -S line, honouring double quotes.
func fields(l string) []string {
	var out []string
	var cur strings.Builder
	in, quoted := false, false
	for i := 0; i < len(l); i++ {
		c := l[i]
		switch {
		case c == '"':
			in, quoted = !in, true
		case c == '\\' && in && i+1 < len(l):
			i++
			cur.WriteByte(l[i])
		case (c == ' ' || c == '\t') && !in:
			if quoted || cur.Len() > 0 {
				out = append(out, cur.String())
			}
			cur.Reset()
			quoted = false
		default:
			cur.WriteByte(c)
		}
	}
	if quoted || cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}

// flagValue is the token after flag in f, or "".
func flagValue(f []string, flag string) string {
	for i := 0; i+1 < len(f); i++ {
		if f[i] == flag {
			return f[i+1]
		}
	}
	return ""
}

// isJumpTo reports whether an iptables -S OUTPUT line jumps to chain.
func isJumpTo(l, chain string) bool {
	f := fields(l)
	return len(f) > 1 && f[0] == "-A" && f[1] == "OUTPUT" && (flagValue(f, "-j") == chain || flagValue(f, "--jump") == chain)
}
