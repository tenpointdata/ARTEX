package netguard_test

import (
	"net/netip"
	"testing"

	"github.com/Autumn-27/artex/intercept"
	"github.com/Autumn-27/artex/netguard"
)

func TestBlocked(t *testing.T) {
	always := []string{ // blocked even with allowPrivate
		"169.254.169.254", "169.254.170.2", "fe80::1", "0.0.0.0", "::", "224.0.0.1", "ff02::1",
		"fd00:ec2::254", "100.100.100.200", "168.63.129.16", "192.0.0.192", "240.0.0.1", "255.255.255.255",
		"::ffff:169.254.169.254", "64:ff9b::a9fe:a9fe", "2002:a9fe:a9fe::1",
	}
	privateOnly := []string{ // blocked by default, allowed with allowPrivate
		"127.0.0.1", "::1", "::ffff:127.0.0.1", "10.0.0.5", "172.16.3.4", "192.168.1.10",
		"fd12::1", "100.64.0.1", "198.18.0.1", "64:ff9b::7f00:1",
	}
	public := []string{"8.8.8.8", "2606:4700::1111", "64:ff9b::808:808"}
	for _, s := range always {
		a := netip.MustParseAddr(s)
		if !netguard.Blocked(a, false) || !netguard.Blocked(a, true) {
			t.Errorf("%s must always be blocked", s)
		}
	}
	for _, s := range privateOnly {
		a := netip.MustParseAddr(s)
		if !netguard.Blocked(a, false) {
			t.Errorf("%s must be blocked by default", s)
		}
		if netguard.Blocked(a, true) {
			t.Errorf("%s must be allowed with allowPrivate", s)
		}
	}
	for _, s := range public {
		if netguard.Blocked(netip.MustParseAddr(s), false) {
			t.Errorf("%s is public and must pass", s)
		}
	}
	if !netguard.Blocked(netip.Addr{}, true) {
		t.Error("invalid address must be blocked")
	}
}

// TestParityWithIntercept pins this package's non-public / metadata judgement to
// the intercept package it mirrors (it cannot import it: db imports notify).
func TestParityWithIntercept(t *testing.T) {
	for _, s := range []string{
		"127.0.0.1", "10.1.1.1", "169.254.169.254", "fe80::1", "fd00:ec2::254", "100.100.100.200",
		"168.63.129.16", "192.0.0.192", "0.0.0.0", "224.0.0.1", "ff02::1", "8.8.8.8", "2606:4700::1111",
		"::ffff:10.0.0.1", "fc00::1",
	} {
		a := netip.MustParseAddr(s)
		if got, want := netguard.Blocked(a, false), intercept.IsNonPublicAddr(a); got != want {
			t.Errorf("%s: netguard=%v intercept.IsNonPublicAddr=%v", s, got, want)
		}
	}
}
