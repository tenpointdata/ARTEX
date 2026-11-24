package server

import (
	"sync"
	"time"
)

// The per-key limiter bounds one source; failCeiling bounds them all, so an
// attacker spreading guesses over many IPv4 addresses or IPv6 /64s still hits
// a fixed guess rate. It is deliberately not a per-account lockout: there is
// one account, and locking it would let anyone shut the admin out by failing
// on purpose. Instead, while the ceiling is exceeded only sources that have
// not logged in successfully recently are refused, so the operator's usual
// address keeps working during a distributed attack, and a new address waits
// at most one window.
const (
	ceilingMaxFailures = 100         // failed logins allowed per window, all sources together
	ceilingWindow      = time.Minute // the counting window
	ceilingKnownTTL    = jwtTTL      // how long a successful source stays exempt
	ceilingMaxKnown    = 1000        // cap on remembered sources
)

type failCeiling struct {
	mu          sync.Mutex
	now         func() time.Time
	windowStart time.Time
	failures    int
	known       map[string]time.Time // limitKey -> last successful login
}

func newFailCeiling(now func() time.Time) *failCeiling {
	return &failCeiling{now: now, known: map[string]time.Time{}}
}

// wait returns how long key must wait before trying, or 0. A nil ceiling never blocks.
func (c *failCeiling) wait(key string) time.Duration {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.roll()
	if c.failures < ceilingMaxFailures {
		return 0
	}
	if at, ok := c.known[key]; ok && now.Sub(at) < ceilingKnownTTL {
		return 0
	}
	return c.windowStart.Add(ceilingWindow).Sub(now)
}

// fail records one failed login.
func (c *failCeiling) fail() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.roll()
	c.failures++
}

// succeed remembers key as a source the operator logs in from.
func (c *failCeiling) succeed(key string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	if _, ok := c.known[key]; !ok && len(c.known) >= ceilingMaxKnown {
		var oldest string
		for k, at := range c.known {
			if oldest == "" || at.Before(c.known[oldest]) {
				oldest = k
			}
		}
		delete(c.known, oldest)
	}
	c.known[key] = now
}

// roll starts a new window when the current one is over; the caller holds mu.
func (c *failCeiling) roll() time.Time {
	now := c.now()
	if now.Sub(c.windowStart) >= ceilingWindow {
		c.windowStart, c.failures = now, 0
	}
	return now
}
