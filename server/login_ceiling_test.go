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
	// Past the ceiling a new source is paced, not refused: its correct password still works.
	if w := post("[2001:db8:ffff::1]:1", oldPassword, nil); w.Code != 200 {
		t.Fatalf("a new source with the right password past the ceiling got %d, want 200", w.Code)
	}
	// Fill the paced queue: further unknown sources are refused with Retry-After ...
	for i := 0; ; i++ {
		if _, ok := s.loginCeiling.admit(fmt.Sprintf("filler-%d", i)); !ok {
			break
		}
	}
	w := post("[2001:db8:eeee::1]:1", oldPassword, nil)
	if w.Code != 429 || w.Header().Get("Retry-After") == "" {
		t.Fatalf("a new source with the queue full got %d, want 429 with Retry-After", w.Code)
	}
	// ... while a source with a recent successful login is never held back.
	if w := post("198.51.100.1:2", oldPassword, nil); w.Code != 200 {
		t.Fatalf("the ceiling locked out a source with a recent successful login: %d", w.Code)
	}
}

// Attempts the ceiling refuses do not use up the caller's own per-address
// allowance, so retrying during an attack does not earn a long per-IP block.
func TestFailCeilingRefusalsSparePerAddressLimit(t *testing.T) {
	clk := newFakeClock()
	s, post := loginServer(t, clk)
	s.loginCeiling = newFailCeiling(clk.now)
	for i := 0; i < ceilingMaxFailures; i++ {
		s.loginCeiling.fail()
	}
	for i := 0; ; i++ {
		if _, ok := s.loginCeiling.admit(fmt.Sprintf("filler-%d", i)); !ok {
			break
		}
	}
	for i := 0; i < 3*loginMaxAttempts; i++ {
		if w := post("203.0.113.9:1", oldPassword, nil); w.Code != 429 {
			t.Fatalf("attempt %d with the queue full: %d, want 429", i, w.Code)
		}
	}
	clk.advance(ceilingMaxQueue + ceilingInterval)
	if w := post("203.0.113.9:1", oldPassword, nil); w.Code != 200 {
		t.Fatalf("after the queue drained the operator got %d, want 200", w.Code)
	}
}

func TestFailCeilingPacesUnknownSources(t *testing.T) {
	clk := newFakeClock()
	c := newFailCeiling(clk.now)
	for i := 0; i < ceilingMaxFailures; i++ {
		c.fail()
	}
	for i := 0; i < 3; i++ {
		d, ok := c.admit("u")
		if !ok || d != time.Duration(i)*ceilingInterval {
			t.Fatalf("admit %d = %v, %v; want %v, true", i, d, ok, time.Duration(i)*ceilingInterval)
		}
	}
	// A new window keeps the previous overflow, so pacing does not lapse at rollover.
	for i := 0; i < ceilingMaxFailures; i++ {
		c.fail()
	}
	clk.advance(ceilingWindow)
	if c.roll(); c.failures != ceilingMaxFailures {
		t.Fatalf("failures after rollover = %d, want the carried overflow %d", c.failures, ceilingMaxFailures)
	}
}

func TestFailCeilingNilAndKnownCap(t *testing.T) {
	var c *failCeiling
	c.fail()
	c.succeed("x")
	if d, ok := c.admit("x"); d != 0 || !ok {
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
