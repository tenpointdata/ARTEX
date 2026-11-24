package server

import (
	"fmt"
	"testing"
	"time"
)

func TestLimitKeyGroupsIPv6By64(t *testing.T) {
	for in, want := range map[string]string{
		"192.0.2.7":                  "192.0.2.7",
		"::ffff:192.0.2.7":           "192.0.2.7",
		"2001:db8:1:2:aaaa::1":       "2001:db8:1:2::/64",
		"2001:db8:1:2:ffff:ffff::99": "2001:db8:1:2::/64",
		"2001:db8:1:3::1":            "2001:db8:1:3::/64",
		"fe80::1%en0":                "fe80::/64",
		"not-an-ip":                  "not-an-ip",
	} {
		if got := limitKey(in); got != want {
			t.Errorf("limitKey(%q) = %q, want %q", in, got, want)
		}
	}
}

// Rotating the low 64 bits of an IPv6 address no longer earns a fresh budget.
func TestLoginLimitIPv6RotationSharesOneBucket(t *testing.T) {
	_, post := loginServer(t, newFakeClock())
	for i := 0; i < loginMaxAttempts; i++ {
		if w := post(fmt.Sprintf("[2001:db8:1:2::%x]:1", i+1), "wrong-password", nil); w.Code != 401 {
			t.Fatalf("attempt %d: status %d", i, w.Code)
		}
	}
	if w := post("[2001:db8:1:2:dead:beef:0:1]:1", "wrong-password", nil); w.Code != 429 {
		t.Fatalf("a new address in the same /64 got status %d, want 429", w.Code)
	}
	if w := post("[2001:db8:1:3::1]:1", oldPassword, nil); w.Code != 200 {
		t.Fatalf("another /64 was blocked: status %d", w.Code)
	}
}

func TestFailCeilingBoundsDistributedGuessing(t *testing.T) {
	clk := newFakeClock()
	s, post := loginServer(t, clk)
	s.loginCeiling = newFailCeiling(clk.now)
	if w := post("198.51.100.1:1", oldPassword, nil); w.Code != 200 { // the operator's usual address
		t.Fatalf("operator login: %d", w.Code)
	}
	for i := 0; i < ceilingMaxFailures; i++ { // one failure from each of many /64s
		if w := post(fmt.Sprintf("[2001:db8:%x::1]:1", i+1), "wrong-password", nil); w.Code != 401 {
			t.Fatalf("failure %d: status %d", i, w.Code)
		}
	}
	w := post("[2001:db8:ffff::1]:1", oldPassword, nil)
	if w.Code != 429 || w.Header().Get("Retry-After") == "" {
		t.Fatalf("a new source past the ceiling got %d, want 429 with Retry-After", w.Code)
	}
	if w := post("198.51.100.1:2", oldPassword, nil); w.Code != 200 {
		t.Fatalf("the ceiling locked out a source with a recent successful login: %d", w.Code)
	}
	clk.advance(ceilingWindow)
	if w := post("[2001:db8:ffff::1]:1", oldPassword, nil); w.Code != 200 {
		t.Fatalf("the ceiling outlived its window: %d", w.Code)
	}
}

func TestFailCeilingNilAndKnownCap(t *testing.T) {
	var c *failCeiling
	c.fail()
	c.succeed("x")
	if c.wait("x") != 0 {
		t.Fatal("a nil ceiling must not block")
	}
	c = newFailCeiling(time.Now)
	for i := 0; i < ceilingMaxKnown+5; i++ {
		c.succeed(fmt.Sprint(i))
	}
	if len(c.known) != ceilingMaxKnown {
		t.Fatalf("known sources = %d, want the cap %d", len(c.known), ceilingMaxKnown)
	}
}
