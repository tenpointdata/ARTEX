// Package netguard is the one outbound-destination policy shared by every
// feature that dials an operator-supplied address (model listing, notification
// webhooks and SMTP). It is a pure function of the resolved IP, so it can run in
// a net.Dialer Control hook, where the address has already been resolved and a
// DNS rebinding answer cannot slip past it.
package netguard

import (
	"fmt"
	"net"
	"net/netip"
	"slices"
	"syscall"
)

// Ranges that are not public Internet addresses and that
// the basic non-public test does not cover. The shared ones are in use on
// private networks (overlay networks, fake-IP resolvers), so an allow-listed
// endpoint may sit there; the reserved ones are never a legitimate endpoint.
var (
	sharedPrefixes = []netip.Prefix{
		netip.MustParsePrefix("100.64.0.0/10"), // carrier-grade NAT (RFC 6598)
		netip.MustParsePrefix("198.18.0.0/15"), // benchmarking (RFC 2544)
	}
	reservedPrefixes = []netip.Prefix{
		netip.MustParsePrefix("192.0.0.0/24"), // IETF protocol assignments (RFC 6890)
		netip.MustParsePrefix("240.0.0.0/4"),  // reserved, and the limited broadcast address
	}
	// IPv6 prefixes whose addresses reach the IPv4 host they embed. A DNS64
	// resolver answers with NAT64 addresses for every IPv4-only name, so the
	// prefixes are not blocked as a whole: the embedded address is judged.
	nat64Prefix = netip.MustParsePrefix("64:ff9b::/96") // the low 32 bits name an IPv4 host
	sixToFour   = netip.MustParsePrefix("2002::/16")    // bits 16 to 47 name an IPv4 host
)

// metadataAddrs are instance-metadata and platform-host addresses outside the
// link-local ranges (169.254.0.0/16 and fe80::/10 are caught by the link-local
// test). Kept in step with intercept.metadataAddrs; a test pins the parity.
var metadataAddrs = []netip.Addr{
	netip.MustParseAddr("fd00:ec2::254"),   // AWS IMDS over IPv6
	netip.MustParseAddr("100.100.100.200"), // Alibaba Cloud
	netip.MustParseAddr("168.63.129.16"),   // Azure WireServer
	netip.MustParseAddr("192.0.0.192"),     // Oracle Cloud (legacy metadata)
}

func isMetadata(addr netip.Addr) bool {
	return addr.IsLinkLocalUnicast() || slices.Contains(metadataAddrs, addr)
}

// nonPublic is true for private, loopback, link-local, unspecified, multicast
// and metadata addresses.
func nonPublic(addr netip.Addr) bool {
	return addr.IsPrivate() || addr.IsLoopback() || addr.IsUnspecified() ||
		addr.IsMulticast() || isMetadata(addr)
}

func embeddedIPv4(addr netip.Addr) (netip.Addr, bool) {
	b := addr.As16()
	switch {
	case nat64Prefix.Contains(addr):
		return netip.AddrFrom4([4]byte(b[12:16])), true
	case sixToFour.Contains(addr):
		return netip.AddrFrom4([4]byte(b[2:6])), true
	}
	return netip.Addr{}, false
}

func contains(prefixes []netip.Prefix, addr netip.Addr) bool {
	for _, p := range prefixes {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

// Blocked reports whether a dial to addr must be refused. By default only
// public unicast addresses pass. With allowPrivate, loopback, RFC1918 / unique
// local and the shared ranges also pass; link-local, unspecified, multicast,
// reserved and instance-metadata addresses (169.254.169.254, 100.100.100.200,
// fd00:ec2::254, ...) are refused regardless. IPv4-mapped, NAT64 and 6to4 forms
// are judged by the IPv4 address they carry. An invalid address is blocked.
func Blocked(addr netip.Addr, allowPrivate bool) bool {
	if !addr.IsValid() {
		return true
	}
	addr = addr.Unmap().WithZone("")
	if v4, ok := embeddedIPv4(addr); ok {
		addr = v4
	}
	shared := contains(sharedPrefixes, addr)
	if !nonPublic(addr) && !shared && !contains(reservedPrefixes, addr) {
		return false
	}
	if allowPrivate {
		if addr.IsLoopback() {
			return false
		}
		if (addr.IsPrivate() || shared) && !isMetadata(addr) {
			return false
		}
	}
	return true
}

// Control returns a net.Dialer Control hook that refuses a connection to a
// blocked address at connect time. allowPrivate is evaluated per dial so an
// operator opt-in can be read from the environment.
func Control(allowPrivate func() bool, hint string) func(network, address string, c syscall.RawConn) error {
	return func(_, address string, _ syscall.RawConn) error {
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			return err
		}
		addr, err := netip.ParseAddr(host)
		if err != nil {
			return fmt.Errorf("cannot parse target address %q", host)
		}
		if Blocked(addr, allowPrivate != nil && allowPrivate()) {
			return fmt.Errorf("refusing to connect to non-public address %s%s", addr.Unmap().WithZone(""), hint)
		}
		return nil
	}
}
