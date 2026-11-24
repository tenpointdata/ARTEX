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
// logged in successfully recently pass freely; every other source is paced to
// one attempt per ceilingWindow/ceilingMaxFailures across all of them (the
// password is still checked, so a correct one gets in), and refused only when
// that queue is longer than ceilingMaxQueue.
const (
	ceilingMaxFailures = 100         // failed logins allowed per window, all sources together
	ceilingWindow      = time.Minute // the counting window
	ceilingKnownTTL    = jwtTTL      // how long a successful source stays exempt
	ceilingMaxKnown    = 1000        // cap on remembered sources
	ceilingMaxQueue    = 5 * time.Second
)

// ceilingInterval is the global pace past the ceiling.
const ceilingInterval = ceilingWindow / ceilingMaxFailures

type failCeiling struct {
	mu          sync.Mutex
	now         func() time.Time
	windowStart time.Time
	failures    int
	next        time.Time            // next free paced slot past the ceiling
	known       map[string]time.Time // limitKey -> last successful login
}

func newFailCeiling(now func() time.Time) *failCeiling {
	return &failCeiling{now: now, known: map[string]time.Time{}}
}

// admit decides whether key may attempt a login now. ok false means refuse
// with Retry-After delay; ok true means attempt after sleeping delay (0 below
// the ceiling and for known sources). A nil ceiling always admits.
func (c *failCeiling) admit(key string) (delay time.Duration, ok bool) {
	if c == nil {
		return 0, true
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.roll()
	if c.failures < ceilingMaxFailures {
		return 0, true
	}
	if at, known := c.known[key]; known && now.Sub(at) < ceilingKnownTTL {
		return 0, true
	}
	slot := c.next
	if slot.Before(now) {
		slot = now
	}
	if wait := slot.Sub(now); wait > ceilingMaxQueue {
		return wait - ceilingMaxQueue, false
	}
	c.next = slot.Add(ceilingInterval)
	return slot.Sub(now), true
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
	// A new window starts from the previous one's overflow, not from zero,
	// so a source refused at rollover cannot be crowded out by a burst that
	// re-spends the whole window in milliseconds.
	if now.Sub(c.windowStart) >= ceilingWindow {
		over := max(c.failures-ceilingMaxFailures, 0)
		c.windowStart, c.failures = now, min(over, ceilingMaxFailures)
	}
	return now
}
