package agent

import (
	"fmt"
	"log"
	"strings"

	"github.com/Autumn-27/artex/methodology"
)

// Methodology-driven coverage (db/coverage_methodology.go, methodology package):
// the planner sees the applicable checklists' completion each round and cites the
// item an intent advances (add_intent methodology_item); workers close items by
// recording coverage under the item id or one of its risk areas. Coverage-only,
// like the ledger: nothing here runs when the task's coverage feature is off.

// plannerMethodologyPending caps the pending items listed per methodology.
const plannerMethodologyPending = 6

// taskMethodologies returns the methodologies that apply to this task; nil when
// coverage is off, there is no task context, or the lookup fails.
func (t *ToolSet) taskMethodologies() []methodology.Methodology {
	if t.coverageDisabled || t.as == nil || t.taskID <= 0 {
		return nil
	}
	ms, err := t.as.TaskMethodologies(t.taskID)
	if err != nil {
		log.Printf("[coverage] task %d: methodology lookup failed: %v", t.taskID, err)
		return nil
	}
	return ms
}

// resolveMethodologyItem maps a cited item to its canonical id among the task's
// methodologies. An empty citation is fine (not every intent maps to an item);
// an unknown one is refused so a typo never reads as progress.
func resolveMethodologyItem(ms []methodology.Methodology, cited string) (string, error) {
	if strings.TrimSpace(cited) == "" {
		return "", nil
	}
	for _, m := range ms {
		if it, ok := m.Item(cited); ok {
			return it.ID, nil
		}
	}
	if len(ms) == 0 {
		return "", fmt.Errorf("methodology_item %q: no methodology applies to this task (or coverage is off); omit methodology_item", cited)
	}
	names := make([]string, 0, len(ms))
	for _, m := range ms {
		names = append(names, m.Name)
	}
	return "", fmt.Errorf("methodology_item %q is not an item of %s; use an item id from the [Methodology coverage] block, or omit it", cited, strings.Join(names, " / "))
}

// methodologyBlock is the planner's per-round checklist block; empty when no
// methodology applies or coverage is off.
func (t *ToolSet) methodologyBlock() string {
	if t.coverageDisabled || t.as == nil || t.taskID <= 0 {
		return ""
	}
	scores, err := t.as.MethodologyCoverage(t.taskID)
	if err != nil {
		log.Printf("[coverage] task %d: methodology coverage failed: %v", t.taskID, err)
		return ""
	}
	return renderMethodologyBlock(scores)
}

// renderMethodologyBlock renders the checklist block. Every line is code-owned
// (the embedded methodology data and counters); agent-reported text never
// appears here, so it needs no data envelope.
func renderMethodologyBlock(scores []methodology.Score) string {
	if len(scores) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\n[Methodology coverage] Checklists that apply to this task. When an intent advances an item, pass its id as methodology_item on add_intent; a worker closes the item by record_coverage with risk_area = the item id (or one of its risk areas). An item with no entry is untested, not clean. Depth on a promising route still comes first; the checklist is a floor, not the plan.")
	for _, s := range scores {
		fmt.Fprintf(&b, "\n%s %s: covered %d/%d, in progress %d, open %d", s.Name, s.Version, s.Covered, s.Total, s.InProgress, s.Open)
		pending := s.Pending(plannerMethodologyPending)
		if len(pending) == 0 {
			continue
		}
		b.WriteString("; next:")
		for _, it := range pending {
			fmt.Fprintf(&b, "\n- %s %s [%s]", it.ID, it.Title, it.Status)
			if len(it.Intents) > 0 {
				ids := make([]string, 0, len(it.Intents))
				for _, id := range it.Intents {
					ids = append(ids, fmt.Sprintf("#%d", id))
				}
				fmt.Fprintf(&b, " intents %s", strings.Join(ids, ","))
			}
		}
		if n := s.InProgress + s.Open - len(pending); n > 0 {
			fmt.Fprintf(&b, "\n- and %d more not covered", n)
		}
	}
	return b.String()
}
