package memo

import (
	"errors"
	"testing"
	"time"
)

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func TestZeroCacheCallsLoaderEveryTime(t *testing.T) {
	var c Cache[int, int]
	n := 0
	for range 3 {
		if _, err := c.Get(1, func() (int, error) { n++; return n, nil }); err != nil {
			t.Fatal(err)
		}
	}
	if n != 3 {
		t.Fatalf("loads = %d, want 3", n)
	}
}

func TestCacheServesUntilTTLAndInvalidate(t *testing.T) {
	clk := &clock{t: time.Unix(1000, 0)}
	c := Cache[string, int]{TTL: time.Second, Now: clk.now}
	n := 0
	load := func() (int, error) { n++; return n, nil }
	if v, _ := c.Get("a", load); v != 1 {
		t.Fatalf("first = %d", v)
	}
	if v, _ := c.Get("a", load); v != 1 {
		t.Fatalf("cached = %d, want 1", v)
	}
	clk.t = clk.t.Add(time.Second)
	if v, _ := c.Get("a", load); v != 2 {
		t.Fatalf("after ttl = %d, want 2", v)
	}
	c.Invalidate("a")
	if v, _ := c.Get("a", load); v != 3 {
		t.Fatalf("after invalidate = %d, want 3", v)
	}
	c.InvalidateAll()
	if v, _ := c.Get("a", load); v != 4 {
		t.Fatalf("after invalidate all = %d, want 4", v)
	}
}

func TestCacheDoesNotCacheErrors(t *testing.T) {
	c := Cache[int, int]{TTL: time.Hour}
	boom := errors.New("boom")
	if _, err := c.Get(1, func() (int, error) { return 0, boom }); !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	if v, err := c.Get(1, func() (int, error) { return 7, nil }); err != nil || v != 7 {
		t.Fatalf("v, err = %d, %v; want 7 (error not cached)", v, err)
	}
}

// A load that races an invalidation must not store its stale result.
func TestInvalidateDuringLoadIsNotLost(t *testing.T) {
	c := Cache[int, string]{TTL: time.Hour}
	v, _ := c.Get(1, func() (string, error) {
		c.Invalidate(1) // the writer commits and invalidates while we read
		return "stale", nil
	})
	if v != "stale" {
		t.Fatalf("v = %q", v)
	}
	if v, _ := c.Get(1, func() (string, error) { return "fresh", nil }); v != "fresh" {
		t.Fatalf("after racing invalidate = %q, want fresh", v)
	}
}

func TestCacheBoundsEntries(t *testing.T) {
	c := Cache[int, int]{TTL: time.Hour, Max: 2}
	for i := range 5 {
		_, _ = c.Get(i, func() (int, error) { return i, nil })
	}
	if len(c.entries) > 2 {
		t.Fatalf("entries = %d, want <= 2", len(c.entries))
	}
}
