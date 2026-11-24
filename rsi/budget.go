package rsi

import "math"

// Budget is the bounded, annealed edit budget (RSI.md rule 6, after RRSI,
// arXiv 2609.24972): round r may propose at most max(Floor, floor(Base *
// Decay^r)) edits, so later rounds explore less as the harness settles.
type Budget struct {
	Base  int
	Decay float64
	Floor int
}

// DefaultBudget allows 4 edits in round 0, then 2, 1, 1, ... .
var DefaultBudget = Budget{Base: 4, Decay: 0.6, Floor: 1}

// Edits returns how many edits round may propose. A negative round has none.
func (b Budget) Edits(round int) int {
	if round < 0 || b.Base <= 0 {
		return 0
	}
	decay := b.Decay
	if decay <= 0 || decay > 1 {
		decay = 1
	}
	n := int(math.Floor(float64(b.Base) * math.Pow(decay, float64(round))))
	if n < b.Floor {
		n = b.Floor
	}
	if n > b.Base {
		n = b.Base
	}
	return n
}
