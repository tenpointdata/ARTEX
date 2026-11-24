// Package memo is a small keyed read-through cache with a time-to-live and
// explicit invalidation, for values whose writes happen in this process (so
// the writer can invalidate) but which other processes or a database console
// may still change (so the TTL bounds how long such a change goes unseen).
//
// An invalidation is never lost to a racing load: a load that started before
// Invalidate does not store its (possibly stale) result. Errors are never
// cached. The zero Cache, or one with TTL <= 0, caches nothing and calls the
// loader every time, so a struct holding one is safe to build without setup.
package memo

import (
	"sync"
	"time"
)

// DefaultMax bounds the entries of a Cache whose Max is zero.
const DefaultMax = 4096

// Cache maps K to V for at most TTL per entry. Use a pointer or embed it in a
// struct used by pointer; a Cache must not be copied after first use.
type Cache[K comparable, V any] struct {
	// TTL is how long a loaded value is served without reloading. <= 0 disables caching.
	TTL time.Duration
	// Max bounds the entries; when full, expired entries are dropped and, if
	// still full, the whole map is cleared. 0 means DefaultMax.
	Max int
	// Now is the clock; nil means time.Now. Tests replace it.
	Now func() time.Time

	mu      sync.Mutex
	entries map[K]entry[V]
	// gen counts invalidations; a load stores its result only when gen did
	// not move while it ran.
	gen uint64
}

type entry[V any] struct {
	v       V
	expires time.Time
}

func (c *Cache[K, V]) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

// Get returns the cached value of k, or calls load and caches its result.
func (c *Cache[K, V]) Get(k K, load func() (V, error)) (V, error) {
	if c.TTL <= 0 {
		return load()
	}
	c.mu.Lock()
	now := c.now()
	if e, ok := c.entries[k]; ok && now.Before(e.expires) {
		c.mu.Unlock()
		return e.v, nil
	}
	gen := c.gen
	c.mu.Unlock()

	v, err := load()
	if err != nil {
		return v, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.gen != gen {
		return v, nil // invalidated while loading: serve it once, do not keep it
	}
	if c.entries == nil {
		c.entries = make(map[K]entry[V])
	}
	c.evictLocked(now)
	c.entries[k] = entry[V]{v: v, expires: now.Add(c.TTL)}
	return v, nil
}

func (c *Cache[K, V]) evictLocked(now time.Time) {
	limit := c.Max
	if limit <= 0 {
		limit = DefaultMax
	}
	if len(c.entries) < limit {
		return
	}
	for k, e := range c.entries {
		if !now.Before(e.expires) {
			delete(c.entries, k)
		}
	}
	if len(c.entries) >= limit {
		clear(c.entries)
	}
}

// Invalidate drops k and keeps any load already running from storing a value.
func (c *Cache[K, V]) Invalidate(k K) {
	c.mu.Lock()
	c.gen++
	delete(c.entries, k)
	c.mu.Unlock()
}

// InvalidateAll drops every entry and keeps running loads from storing.
func (c *Cache[K, V]) InvalidateAll() {
	c.mu.Lock()
	c.gen++
	clear(c.entries)
	c.mu.Unlock()
}
