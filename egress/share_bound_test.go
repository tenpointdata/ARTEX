package egress

import (
	"math"
	"testing"
)

// The bound Share promises n nodes together (docs/DISTRIBUTED.md, HANDOFF
// Decisions c-18f7a8): the request rate is divided exactly, so n nodes never
// exceed the profile's RPS; the in-flight limits floor at 1 per node, so n
// nodes hold at most max(limit, n) requests at once (never more than one each
// beyond the profile's own limit). A DB-backed per-host slot that would make
// concurrency exact was decided against; this test pins the bound instead.
func TestShareBoundAcrossNodes(t *testing.T) {
	for name, p := range Profiles {
		for n := 1; n <= 16; n++ {
			s := Share(p, n)
			if total := s.RPS * float64(n); math.Abs(total-p.RPS) > 1e-9 {
				t.Errorf("%s x%d: total RPS %v, want %v", name, n, total, p.RPS)
			}
			for what, pair := range map[string][2]int{"MaxPerHost": {s.MaxPerHost, p.MaxPerHost}, "MaxGlobal": {s.MaxGlobal, p.MaxGlobal}, "Burst": {s.Burst, p.Burst}} {
				per, limit := pair[0], pair[1]
				if per < 1 {
					t.Errorf("%s x%d: %s %d per node; a node must keep at least 1", name, n, what, per)
				}
				if total := per * n; total > max(limit, n) {
					t.Errorf("%s x%d: %s %d together, want at most max(%d, %d)", name, n, what, total, limit, n)
				}
			}
		}
	}
}
