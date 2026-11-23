package db

import "testing"

// Allow rules match fuzzy patterns on name and address boundaries only; block
// rules keep their substring semantics.
func TestMatchAssetAllowRulesBoundaries(t *testing.T) {
	cases := []struct {
		kind, pattern string
		domains, ips  []string
		urls          []string
		want          bool
	}{
		{"fuzzy_domain", "example.com", []string{"example.com"}, nil, nil, true},
		{"fuzzy_domain", "example.com", []string{"api.EXAMPLE.com."}, nil, nil, true},
		{"fuzzy_domain", ".example.com", []string{"api.example.com"}, nil, nil, true},
		{"fuzzy_domain", "example.com", []string{"example.com.attacker.net"}, nil, nil, false},
		{"fuzzy_domain", "example.com", []string{"badexample.com"}, nil, nil, false},
		{"fuzzy_domain", "example", []string{"example.com"}, nil, nil, false},
		{"fuzzy_domain", ".", []string{"example.com"}, nil, nil, false},
		{"fuzzy_ip", "10.0.0.1", nil, []string{"10.0.0.1"}, nil, true},
		{"fuzzy_ip", "10.0.0.1", nil, []string{"110.0.0.1"}, nil, false},
		{"fuzzy_ip", "10.0.0.1", nil, []string{"10.0.0.10"}, nil, false},
		{"fuzzy_ip", "203.0.113.", nil, []string{"203.0.113.99"}, nil, true},
		{"fuzzy_ip", "203.0.113", nil, []string{"203.0.113.99"}, nil, true},
		{"fuzzy_ip", "203.0.11", nil, []string{"203.0.113.99"}, nil, false},
		{"fuzzy_ip", "2001:db8:", nil, []string{"2001:DB8::1"}, nil, true},
		{"fuzzy_url", "https://example.com", nil, nil, []string{"https://example.com/x"}, true},
		{"fuzzy_url", "https://example.com", nil, nil, []string{"https://example.com:8443/x"}, true},
		{"fuzzy_url", "https://example.com", nil, nil, []string{"https://example.com.attacker.net/"}, false},
		{"fuzzy_url", "https://example.com", nil, nil, []string{"https://example.com@evil.example/"}, false},
		{"fuzzy_url", "example.com/app", nil, nil, []string{"http://example.com/app/x"}, true},
		{"fuzzy_url", "example.com/app", nil, nil, []string{"http://example.com/application"}, false},
		{"fuzzy_url", "example.com", nil, nil, []string{"https://attacker.net/?u=example.com"}, false},
		// Exact and CIDR kinds are the same in both modes; exact_ip compares addresses.
		{"exact_ip", "2001:db8::1", nil, []string{"2001:DB8:0::1"}, nil, true},
		{"exact_ip", "10.0.0.1", nil, []string{"110.0.0.1"}, nil, false},
		{"cidr", "10.0.0.0/8", nil, []string{"10.9.9.9"}, nil, true},
	}
	for _, c := range cases {
		rules := []AssetInterceptRule{rule(c.kind, c.pattern, true)}
		if _, _, got := MatchAssetAllowRules(rules, c.domains, c.ips, c.urls); got != c.want {
			t.Errorf("allow %s %q on %v%v%v = %v, want %v", c.kind, c.pattern, c.domains, c.ips, c.urls, got, c.want)
		}
	}
	// Block rules still match on substrings: the built-in .gov rule blocks .gov.uk.
	if _, _, ok := MatchAssetInterceptRules([]AssetInterceptRule{rule("fuzzy_domain", ".gov", true)}, []string{"www.gov.uk"}, nil, nil); !ok {
		t.Error("block .gov should still hit www.gov.uk")
	}
	if _, _, ok := MatchAssetInterceptRules([]AssetInterceptRule{rule("fuzzy_ip", "0.113.", true)}, nil, []string{"203.0.113.9"}, nil); !ok {
		t.Error("block fuzzy_ip should still match a substring")
	}
	// exact_ip compares addresses in block rules too.
	if _, _, ok := MatchAssetInterceptRules([]AssetInterceptRule{rule("exact_ip", "2001:db8::1", true)}, nil, []string{"2001:DB8:0::1"}, nil); !ok {
		t.Error("block exact_ip should match another spelling of the address")
	}
}

// The task gate applies the allow rules with the boundary semantics.
func TestEvaluateAssetGateAllowBoundaries(t *testing.T) {
	allow := []AssetInterceptRule{rule("fuzzy_domain", "example.com", true), rule("fuzzy_ip", "10.0.0.1", true)}
	for _, c := range []struct {
		domains, ips []string
		want         bool
	}{
		{[]string{"api.example.com"}, nil, true},
		{[]string{"example.com.attacker.net"}, nil, false},
		{nil, []string{"10.0.0.1"}, true},
		{nil, []string{"110.0.0.1"}, false},
	} {
		if d := EvaluateAssetGate(nil, allow, c.domains, c.ips, nil); d.Allowed != c.want {
			t.Errorf("EvaluateAssetGate(%v %v) allowed=%v, want %v", c.domains, c.ips, d.Allowed, c.want)
		}
	}
}
