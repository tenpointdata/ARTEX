package intercept

import (
	"net/netip"
	"testing"
)

// Every prefix the agent egress floor refuses is one IsMetadataAddr refuses, and
// every address IsMetadataAddr names explicitly is inside one of them.
func TestMetadataPrefixesMatchIsMetadataAddr(t *testing.T) {
	ps := MetadataPrefixes()
	covered := func(a netip.Addr) bool {
		for _, p := range ps {
			if p.Contains(a) {
				return true
			}
		}
		return false
	}
	for _, p := range ps {
		if !IsMetadataAddr(p.Addr()) {
			t.Errorf("%s is refused by the floor but not by IsMetadataAddr", p)
		}
	}
	for _, s := range []string{"169.254.169.254", "169.254.170.2", "fe80::1", "fd00:ec2::254", "100.100.100.200",
		"168.63.129.16", "192.0.0.192", "64:ff9b::a9fe:a9fe", "64:ff9b::6464:64c8"} {
		a := netip.MustParseAddr(s)
		if !IsMetadataAddr(a) || !covered(a) {
			t.Errorf("%s: IsMetadataAddr %v, covered %v", s, IsMetadataAddr(a), covered(a))
		}
	}
	if covered(netip.MustParseAddr("10.0.0.1")) || covered(netip.MustParseAddr("64:ff9b::a01:101")) {
		t.Error("a private address is covered")
	}
}
