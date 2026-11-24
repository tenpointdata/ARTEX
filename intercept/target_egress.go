package intercept

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/netip"
	"slices"

	"github.com/Autumn-27/artex/db"
)

// The always-on egress checks. The target gate is off by default, and with it
// off nothing used to stand between a tool and an address: a hostile target
// could answer 302 -> http://169.254.169.254/ and the platform's own client
// would follow it and hand the cloud credentials to the model, and the asset
// block rules (the built-in .gov and .edu ones included) applied to no tool.
// These checks run whether the gate is on or off:
//
//   - The metadata deny: a connection the platform opens (a dial of WebFetch,
//     the custom http tool, the SOCKS5 and transparent listeners; a redirect
//     hop; the URL of WebFetch) never reaches a link-local or cloud
//     instance-metadata address, unless an exact_ip or cidr allow rule of the
//     task names the address. A human approval does not lift it.
//   - With the gate off, the asset block rules apply to the targets a tool
//     call declares (CheckUngated), to every redirect hop that leaves the
//     host of the original request, and to every connection the dialers open.
//
// With the gate off there are no pins, exemptions or approval requests: a
// call that no rule blocks runs as before. A block is logged as a decided row.

// metadataAddrs are instance-metadata and platform-host addresses that lie
// outside the link-local ranges and would otherwise pass as private or public
// unicast. 169.254.169.254 (AWS, GCP, Azure, OpenStack, DigitalOcean...) and
// 169.254.170.2 (ECS task credentials) are link-local and need no entry.
var metadataAddrs = []netip.Addr{
	netip.MustParseAddr("fd00:ec2::254"),   // AWS IMDS over IPv6
	netip.MustParseAddr("100.100.100.200"), // Alibaba Cloud
	netip.MustParseAddr("168.63.129.16"),   // Azure WireServer
	netip.MustParseAddr("192.0.0.192"),     // Oracle Cloud (legacy metadata)
}

// nat64Prefix is the well-known NAT64 prefix: 64:ff9b::a9fe:a9fe reaches
// 169.254.169.254 through a NAT64 gateway.
var nat64Prefix = netip.MustParsePrefix("64:ff9b::/96")

// IsMetadataAddr reports whether addr is link-local (169.254.0.0/16, which
// holds the cloud metadata address, and fe80::/10) or a known instance-metadata
// address, in any of its forms: IPv4-mapped, with a zone, or behind the NAT64
// prefix.
func IsMetadataAddr(addr netip.Addr) bool {
	addr = addr.Unmap().WithZone("")
	if nat64Prefix.Contains(addr) {
		b := addr.As16()
		addr = netip.AddrFrom4([4]byte(b[12:]))
	}
	return addr.IsLinkLocalUnicast() || slices.Contains(metadataAddrs, addr)
}

func isMetadataIP(ip string) bool {
	a, err := netip.ParseAddr(ip)
	return err == nil && IsMetadataAddr(a)
}

// metadataAllowed reports whether an enabled exact_ip or cidr allow rule names
// ip. A fuzzy or domain rule never lifts the metadata deny.
func metadataAllowed(rules []db.AssetInterceptRule, ip string) bool {
	var exact []db.AssetInterceptRule
	for _, r := range rules {
		if r.Kind == "exact_ip" || r.Kind == "cidr" {
			exact = append(exact, r)
		}
	}
	_, _, ok := db.MatchAssetAllowRules(exact, nil, []string{ip}, nil)
	return ok
}

// connectTool reports whether the target a tool declares is the one the
// platform connects to, rather than text in a command.
func connectTool(toolName string) bool { return toolName == "WebFetch" }

// metadataBlock applies the metadata deny to the addresses of the checked
// targets of a connect check. The allow rules are read only when an address
// is a metadata address, so the common path reads nothing more.
func (c *targetCheck) metadataBlock() TargetResult {
	if !c.connect {
		return TargetResult{}
	}
	var allow []db.AssetInterceptRule
	var derived map[string]bool
	loaded := false
	for _, ct := range c.checked {
		for _, ip := range ct.ips {
			if !isMetadataIP(ip) {
				continue
			}
			if derived == nil {
				// An endpoint the configuration names (a platform relay or proxy)
				// is the operator's own choice, as for the gate's exemptions.
				derived = map[string]bool{}
				for _, e := range c.i.DerivedExemptions() {
					derived[db.TargetKey(e.Host, e.Port)] = true
				}
			}
			if derived[db.TargetKey(ct.Host, ct.Port)] {
				break
			}
			if !loaded && c.i.targets != nil {
				var err error
				if allow, err = c.i.targets.AllowRules(c.ref); err != nil {
					_, res := c.block(TargetGateUnavailable, "gate_unavailable", ct, "the asset allow rules could not be read: "+err.Error())
					return res
				}
			}
			loaded = true
			if metadataAllowed(allow, ip) {
				continue
			}
			_, res := c.block(TargetRuleBlocked, "metadata_blocked", ct, fmt.Sprintf(
				"%s reaches %s, a link-local or cloud instance-metadata address; the platform never connects there "+
					"unless an exact_ip or cidr allow rule of the task names the address", ct.Target, ip))
			return res
		}
	}
	return TargetResult{}
}

// ungated is the decision on the targets of a scope whose gate is off: the
// asset block rules on what is declared and, for a connect check, on the
// resolved addresses, then the metadata deny. A name that does not resolve is
// not blocked here: the client fails on it as it would without the check (a
// dial returns the lookup error itself, see ungatedTarget).
func (c *targetCheck) ungated(targets []Target) TargetResult {
	var err error
	if c.ref, err = c.scope.ref(); err != nil {
		return TargetResult{Code: TargetGateUnavailable, Host: targets[0].Host, Port: targets[0].Port, Reason: err.Error()}
	}
	// The engagement authorization applies with the gate off too.
	if res := c.authorize(targets); res.Code != "" {
		return res
	}
	var rules []db.AssetInterceptRule
	if c.i.targets != nil {
		if rules, err = c.i.targets.BlockRules(c.ref); err != nil {
			_, res := c.block(TargetGateUnavailable, "gate_unavailable", &checkedTarget{Target: targets[0]}, "the asset rules could not be read: "+err.Error())
			return res
		}
	}
	checked, res := c.declaredRules(rules, targets)
	if res.Code != "" || !c.connect {
		return res
	}
	for _, ct := range checked {
		if ct.Kind != TargetDomain {
			continue
		}
		if c.resolve(ct, c.fresh); ct.err != nil {
			continue
		}
		if rule, hit := blockRule(rules, ct); hit {
			_, res := c.block(TargetRuleBlocked, "rule_blocked", ct, fmt.Sprintf("%s is blocked by an asset rule. %s", ct.Target, rule.Reason()))
			return res
		}
	}
	return c.metadataBlock()
}

// CheckUngated is the hook's decision on a tool call of a session whose gate
// is off (scope may be the zero scope: then only the global rules apply). It
// matches the asset block rules on the targets the call declares, as the gate
// would, without resolving them; for WebFetch, whose URL is what the platform
// connects to, it also resolves the name and applies the rules and the
// metadata deny to the addresses. It never asks and never waits: the result
// is empty when the call may run, else a TARGET_RULE_BLOCKED or
// TARGET_GATE_UNAVAILABLE block. With the gate on it returns at once: the
// gate decides (CheckTargets).
func (i *Interceptor) CheckUngated(ctx context.Context, scope TargetScope, toolName string, input []byte) TargetResult {
	if i == nil || scope.Enabled != nil && scope.Enabled() {
		return TargetResult{}
	}
	declared := extractDeclared(toolName, input)
	if len(declared.targets) == 0 {
		return i.authorizeCall(ctx, scope, toolName, input)
	}
	c := &targetCheck{i: i, ctx: WithCall(ctx, toolName, input), scope: scope, tool: toolName, input: input,
		urls: declared.urls, connect: connectTool(toolName)}
	return c.ungated(declared.targets)
}

// ungatedTarget decides one connection of a scope whose gate is off, with a
// fresh lookup, and returns the addresses to dial: the literal itself, or the
// addresses the name resolved to and that the decision compared. A lookup
// error is returned as it is.
func (i *Interceptor) ungatedTarget(ctx context.Context, scope TargetScope, toolName string, input []byte, t Target) ([]string, error) {
	c := &targetCheck{i: i, ctx: WithCall(ctx, toolName, input), scope: scope, tool: toolName, input: input, fresh: true, connect: true}
	if res := c.ungated([]Target{t}); res.Code != "" {
		return nil, &dialError{code: res.Code, target: t.String(), reason: res.Reason}
	}
	ct := c.checked[0]
	if ct.err != nil {
		return nil, ct.err
	}
	return ct.ips, nil
}

// dialUngated opens a connection of a scope whose gate is off: the decision of
// ungatedTarget, then a dial of the addresses it compared, never of the name.
func (i *Interceptor) dialUngated(ctx context.Context, scope TargetScope, toolName, network, addr string) (net.Conn, error) {
	t, err := addrTarget(network, addr)
	if err != nil {
		return nil, &dialError{code: TargetGateUnavailable, target: addr, reason: "the address of the connection could not be read (" + err.Error() + ")"}
	}
	input, _ := json.Marshal(map[string]string{"dial": addr})
	ips, err := i.ungatedTarget(ctx, scope, toolName, input, t)
	if err != nil {
		return nil, err
	}
	return i.dialAddrs(ctx, network, ips, t.Port)
}

// ungatedScope is the scope of a connection whose gate is off: the session's,
// when the context carries one, else the zero scope (global rules only).
func ungatedScope(ctx context.Context) TargetScope {
	scope, _ := TargetScopeFromContext(ctx)
	return scope
}

// checkUngatedHop is the redirect policy of a scope whose gate is off. A hop
// that stays on the host of the original request passes: that host was
// checked at the hook and is checked again at the dial. A hop that leaves it
// is a connect check (ungated) on the URL the client is about to request. A
// hop whose target cannot be read is left to the client, as the default
// policy does.
func checkUngatedHop(ctx context.Context, scope TargetScope, toolName, hop, original, from string) error {
	t, err := urlTarget(hop)
	if err != nil {
		return nil
	}
	o, err := urlTarget(original)
	if err == nil && o.Host == t.Host {
		return nil
	}
	var origin string
	if f, err := urlTarget(from); err == nil {
		origin = f.String()
	}
	input, _ := json.Marshal(map[string]string{"url": hop, "redirect_from": from})
	c := &targetCheck{i: scope.Interceptor, ctx: WithCall(ctx, toolName, input), scope: scope, tool: toolName, input: input,
		urls: []string{hop}, redirectFrom: origin, connect: true}
	if res := c.ungated([]Target{t}); res.Code != "" {
		return &redirectError{res: res, from: from, hop: hop, scope: scope.scopeName()}
	}
	return nil
}
