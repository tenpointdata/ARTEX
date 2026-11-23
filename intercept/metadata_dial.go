package intercept

import (
	"context"
	"net"
	"net/netip"
	"strconv"
)

// DialRefusingMetadata is the floor for a dialer that has no Interceptor (no
// asset rules, no scope): it resolves addr once, refuses the connection when
// any address is link-local or a cloud instance-metadata address
// (IsMetadataAddr), and dials only the addresses it checked, never the name,
// so a name that rebinds between check and dial cannot reach one. The error of
// a refusal is a TARGET_RULE_BLOCKED dialError (IsTargetBlocked reports it).
func DialRefusingMetadata(ctx context.Context, network, addr string) (net.Conn, error) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, &net.AddrError{Err: "invalid port", Addr: addr}
	}
	var ips []netip.Addr
	if a, err := netip.ParseAddr(host); err == nil {
		ips = []netip.Addr{a}
	} else if ips, err = net.DefaultResolver.LookupNetIP(ctx, "ip", host); err != nil {
		return nil, err
	}
	for _, a := range ips {
		if IsMetadataAddr(a) {
			return nil, &dialError{code: TargetRuleBlocked, target: addr, reason: a.String() +
				" is a link-local or cloud instance-metadata address; the platform never connects there"}
		}
	}
	var d net.Dialer
	var last error = &net.AddrError{Err: "no suitable address found", Addr: addr}
	for _, a := range ips {
		if network == "tcp4" && !a.Unmap().Is4() || network == "tcp6" && a.Unmap().Is4() {
			continue
		}
		conn, err := d.DialContext(ctx, network, netip.AddrPortFrom(a, uint16(port)).String())
		if err == nil {
			return conn, nil
		}
		last = err
	}
	return nil, last
}
