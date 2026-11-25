package propose

import (
	"fmt"

	"github.com/Autumn-27/artex/rsi"
)

// openStatuses mirror the server's open-proposal count (db.CreateRSIProposal).
var openStatuses = map[string]bool{"proposed": true, "passed": true, "failed": true}

// Slots works out the round to draft for and how many edits it may still take,
// from the budget GET /api/rsi/status reports and the existing proposals. The
// proposer stays in the newest round (round 0 when there is none); moving to
// the next round closes the current one, so it happens only when the caller
// asks for that round explicitly (want >= 0). The server enforces the same
// rules; this keeps a dry-run honest about what a post would be allowed.
func Slots(b rsi.Budget, maxOpen int, prior []Prior, want int) (round, slots int, note string) {
	newest, used, open := -1, 0, 0
	for _, p := range prior {
		newest = max(newest, p.Round)
		if openStatuses[p.Status] {
			open++
		}
	}
	for _, p := range prior {
		if p.Round == newest {
			used++
		}
	}
	round = max(newest, 0)
	if want >= 0 {
		switch {
		case want < newest:
			return want, 0, fmt.Sprintf("round %d is closed; the newest round is %d", want, newest)
		case want > newest+1 || (newest < 0 && want != 0):
			return want, 0, fmt.Sprintf("round %d skips ahead; the next round is %d", want, newest+1)
		}
		round = want
	}
	if round != newest {
		used = 0
	}
	slots = b.Edits(round) - used
	if free := maxOpen - open; free < slots {
		slots = free
		note = fmt.Sprintf("%d proposals are open (cap %d)", open, maxOpen)
	}
	if slots <= 0 {
		slots = 0
		if note == "" {
			note = fmt.Sprintf("round %d has used its edit budget (%d of %d); pass --round %d to open the next round", round, used, b.Edits(round), round+1)
		}
	}
	return round, slots, note
}
