package intercept

import (
	"context"
	"net/netip"
	"strings"
	"testing"

	"github.com/Autumn-27/artex/db"
)

func TestIsMetadataAddr(t *testing.T) {
	for ip, want := range map[string]bool{
		"169.254.169.254":        true,
		"169.254.170.2":          true,
		"::ffff:169.254.169.254": true,
		"fe80::1%eth0":           true,
		"fd00:ec2::254":          true,
		"100.100.100.200":        true,
		"168.63.129.16":          true,
		"192.0.0.192":            true,
		"64:ff9b::a9fe:a9fe":     true, // 169.254.169.254 through NAT64
		"10.0.0.1":               false,
		"127.0.0.1":              false,
		"203.0.113.7":            false,
		"fd00::1":                false,
		"64:ff9b::cb00:7107":     false, // 203.0.113.7 through NAT64
	} {
		if got := IsMetadataAddr(netip.MustParseAddr(ip)); got != want {
			t.Errorf("IsMetadataAddr(%s) = %v, want %v", ip, got, want)
		}
	}
}

// offScope is the scope of a session whose gate is off (the default).
func (g *gateFixture) offScope() TargetScope {
	s := g.scope
	s.Enabled = func() bool { return false }
	return s
}

func (g *gateFixture) offCtx() context.Context {
	return WithTargetScope(context.Background(), g.offScope())
}

// The finding this closes: with the gate off, a hostile target's redirect to
// the metadata address was followed and its answer handed to the model.
func TestGateOffRedirectToMetadataBlocked(t *testing.T) {
	g := newGate(t)
	g.res.set("evil.example", "169.254.169.254")
	g.res.set("fine.example", "203.0.113.9")
	for _, to := range []string{
		"http://169.254.169.254/latest/meta-data/iam/security-credentials/",
		"http://[fd00:ec2::254]/latest/meta-data/",
		"http://0xa9.254.0xa9fe/", // 169.254.169.254, another spelling
		"http://evil.example/",    // a name that resolves there
	} {
		err := hop(g.offCtx(), to, "https://app.example.com/")
		if err == nil || !strings.Contains(err.Error(), TargetRuleBlocked) || !strings.Contains(err.Error(), "metadata") {
			t.Errorf("gate off, hop to %s: %v, want a metadata block", to, err)
		}
	}
	if err := hop(g.offCtx(), "https://fine.example/", "https://app.example.com/"); err != nil {
		t.Errorf("gate off, hop to a public host: %v", err)
	}
	if len(g.store.pins) != 0 {
		t.Errorf("gate off created %d pins", len(g.store.pins))
	}
	for _, o := range g.store.outcomes() {
		if o != "denied/metadata_blocked" {
			t.Errorf("rows = %v, want only metadata blocks", g.store.outcomes())
			break
		}
	}
	// Gate on, the deny holds too, before any request: a human approval does
	// not lift it.
	if err := hop(g.ctx(), "http://169.254.169.254/", "https://app.example.com/"); err == nil || !strings.Contains(err.Error(), TargetRuleBlocked) {
		t.Errorf("gate on, hop to the metadata address: %v", err)
	}
}

// Only an exact_ip or cidr allow rule of the task lifts the metadata deny.
func TestMetadataDenyLiftedByExactAllowRule(t *testing.T) {
	for _, c := range []struct {
		name  string
		allow []db.AssetInterceptRule
		lifts bool
	}{
		{"exact_ip", []db.AssetInterceptRule{{Enabled: true, Kind: "exact_ip", Pattern: "169.254.169.254"}}, true},
		{"cidr", []db.AssetInterceptRule{{Enabled: true, Kind: "cidr", Pattern: "169.254.0.0/16"}}, true},
		{"disabled", []db.AssetInterceptRule{{Enabled: false, Kind: "exact_ip", Pattern: "169.254.169.254"}}, false},
		{"other address", []db.AssetInterceptRule{{Enabled: true, Kind: "exact_ip", Pattern: "169.254.169.253"}}, false},
		{"fuzzy", []db.AssetInterceptRule{{Enabled: true, Kind: "fuzzy_ip", Pattern: "169.254."}}, false},
	} {
		g := newGate(t)
		g.store.allow = c.allow
		err := hop(g.offCtx(), "http://169.254.169.254/", "https://app.example.com/")
		if (err == nil) != c.lifts {
			t.Errorf("%s: %v, want lifted=%v", c.name, err, c.lifts)
		}
		res := g.ic.CheckUngated(context.Background(), g.offScope(), "WebFetch", []byte(`{"url":"http://169.254.169.254/"}`))
		if (res.Code == "") != c.lifts {
			t.Errorf("%s, WebFetch: %+v, want lifted=%v", c.name, res, c.lifts)
		}
	}
}

// With the gate off, a redirect that leaves the original host meets the block
// rules; one that stays on it is not re-checked.
func TestGateOffRedirectMeetsBlockRules(t *testing.T) {
	g := newGate(t)
	g.store.rules = []db.AssetInterceptRule{{Enabled: true, Kind: "fuzzy_domain", Pattern: ".gov", Note: "[built-in]"}}
	g.res.set("www.agency.gov", "203.0.113.20")
	err := hop(g.offCtx(), "https://www.agency.gov/", "https://app.example.com/")
	if err == nil || !strings.Contains(err.Error(), TargetRuleBlocked) || !strings.Contains(err.Error(), ".gov") {
		t.Fatalf("hop to a blocked domain: %v", err)
	}
	if n := g.res.lookups("www.agency.gov"); n != 0 {
		t.Errorf("a name blocked as declared was resolved %d times", n)
	}
	reads := g.store.reads
	if err := hop(g.offCtx(), "https://app.example.com/next", "https://app.example.com/"); err != nil || g.store.reads != reads {
		t.Errorf("same-host hop: %v, %d reads, want followed without a read", err, g.store.reads-reads)
	}
}

// With the gate off the dialers decide each connection on a fresh lookup and
// dial the address they checked: a metadata address or a blocked target is
// never dialled.
func TestGateOffDialBlocksMetadataAndRules(t *testing.T) {
	g, rec := dialGate(t)
	g.res.set("rebind.example", "169.254.169.254")
	g.res.set("blocked.example", "198.51.100.66")
	g.store.rules = []db.AssetInterceptRule{{Enabled: true, Kind: "exact_ip", Pattern: "198.51.100.66"}}
	ctx := g.offCtx()
	dial := PinnedDialer("WebFetch", true)
	for _, addr := range []string{"169.254.169.254:80", "[fe80::1]:80", "rebind.example:80", "blocked.example:443"} {
		_, err := dial(ctx, "tcp", addr)
		if code, ok := IsTargetBlocked(err); !ok || code != TargetRuleBlocked {
			t.Errorf("dial %s: %v, want TARGET_RULE_BLOCKED", addr, err)
		}
	}
	// The SOCKS5 and transparent listeners: the same checks without a scope.
	if _, err := g.ic.DialSOCKS5(context.Background(), "tcp", "169.254.169.254:80"); err == nil {
		t.Error("socks5 dial to the metadata address was not refused")
	}
	if _, err := g.ic.DialSOCKS5(ctx, "tcp", "rebind.example:80"); err == nil {
		t.Error("socks5 dial to a name that resolves to the metadata address was not refused")
	}
	if _, err := g.ic.ResolveUDP4(context.Background(), "169.254.169.254:53"); err == nil {
		t.Error("udp datagram to the metadata address was not refused")
	}
	if got := rec.calls(); len(got) != 0 {
		t.Fatalf("dialled %v, want nothing", got)
	}
}

// The hook's decision with the gate off: block rules on what any tool
// declares, and for WebFetch the metadata deny on its resolved URL. A command
// may carry a metadata address as an SSRF payload: it is not denied.
func TestCheckUngated(t *testing.T) {
	g := newGate(t)
	g.store.rules = []db.AssetInterceptRule{{Enabled: true, Kind: "fuzzy_domain", Pattern: ".gov"}}
	g.res.set("app.example.com", "203.0.113.7")
	g.res.set("meta.example", "169.254.169.254")
	check := func(tool, input string) TargetResult {
		return g.ic.CheckUngated(context.Background(), g.offScope(), tool, []byte(input))
	}
	for _, c := range []struct{ tool, input, code string }{
		{"WebFetch", `{"url":"https://app.example.com/"}`, ""},
		{"WebFetch", `{"url":"http://169.254.169.254/latest/meta-data/"}`, TargetRuleBlocked},
		{"WebFetch", `{"url":"http://meta.example/"}`, TargetRuleBlocked},
		{"WebFetch", `{"url":"https://www.agency.gov/"}`, TargetRuleBlocked},
		{"Bash", `{"command":"curl https://www.agency.gov/"}`, TargetRuleBlocked},
		{"Bash", `{"command":"nmap -sV portal.agency.gov"}`, TargetRuleBlocked},
		{"Bash", `{"command":"curl 'https://app.example.com/fetch?u=http://169.254.169.254/'"}`, ""},
		{"Bash", `{"command":"ffuf -u https://app.example.com/?u=FUZZ -d url=http://169.254.169.254/"}`, ""},
		{"Bash", `{"command":"ls -la"}`, ""},
	} {
		if res := check(c.tool, c.input); res.Code != c.code {
			t.Errorf("%s %s: %+v, want code %q", c.tool, c.input, res, c.code)
		}
	}
	if len(g.store.pins) != 0 {
		t.Errorf("gate off created %d pins", len(g.store.pins))
	}
	// A call without a target reads nothing; with the gate on the gate decides.
	reads := g.store.reads
	check("Bash", `{"command":"id"}`)
	if res := g.ic.CheckUngated(context.Background(), g.scope, "WebFetch", []byte(`{"url":"http://169.254.169.254/"}`)); res.Code != "" {
		t.Errorf("gate on: %+v, want CheckUngated to leave the call to CheckTargets", res)
	}
	if g.store.reads != reads {
		t.Errorf("%d store reads, want none", g.store.reads-reads)
	}
	// The rules cannot be read: the call is blocked, not let through.
	g.store.fail["BlockRules"] = context.DeadlineExceeded
	if res := check("WebFetch", `{"url":"https://app.example.com/"}`); res.Code != TargetGateUnavailable {
		t.Errorf("rules unreadable: %+v", res)
	}
}

// A pin does not open the metadata address, and neither does a stored
// exemption; an endpoint the configuration names does.
func TestGateOnMetadataDenyBeatsPin(t *testing.T) {
	g := newGate(t)
	g.store.approve(TargetRequest{Host: "169.254.169.254", Port: 80, ResolvedIPs: []string{"169.254.169.254"}})
	g.blocked("pinned metadata address", TargetRuleBlocked, g.fetch("http://169.254.169.254/"))
	g.store.allow = []db.AssetInterceptRule{{Enabled: true, Kind: "exact_ip", Pattern: "169.254.169.254"}}
	g.passes("pinned and allowed", g.fetch("http://169.254.169.254/"))

	g = newGate(t)
	g.ic.SetDerivedExemptions(func() []string { return []string{"http://169.254.10.10:8888"} })
	g.passes("derived relay", g.fetch("http://169.254.10.10:8888/v1"))
	// A command naming the address asks, as any other target: it may be a payload.
	g.requests("command", g.check("Bash", `{"command":"curl http://169.254.169.254/"}`))
}
