package methodology

import "sort"

// Item statuses of a scored checklist.
const (
	StatusOpen       = "open"        // nothing recorded and no intent cites it
	StatusInProgress = "in_progress" // an intent cites it, or an entry asks for follow-up, but nothing closed it
	StatusCovered    = "covered"     // a closing observation matches it
)

// Observation is one recorded test result, as a ledger would hold it. Closes
// says whether the outcome settles the risk area (a reported issue, a
// no-issue-found with evidence, a reasoned ruled-out or not-applicable); a
// follow-up request does not. The caller owns that judgement, so the package
// stays independent of any one ledger's outcome vocabulary.
type Observation struct {
	RiskArea string
	Outcome  string
	Closes   bool
}

// Citation is one intent that names the item it advances.
type Citation struct {
	ItemID   string
	IntentID int64
}

// ItemStatus is one scored checklist item.
type ItemStatus struct {
	ID       string   `json:"id"`
	Category string   `json:"category"`
	Title    string   `json:"title"`
	Status   string   `json:"status"`
	Outcomes []string `json:"outcomes"` // distinct outcomes of the matching observations, sorted
	Intents  []int64  `json:"intents"`  // intents citing the item, ascending
}

// Score is a methodology's checklist completion for one task.
type Score struct {
	ID         string       `json:"id"`
	Name       string       `json:"name"`
	Version    string       `json:"version"`
	Source     string       `json:"source"`
	Total      int          `json:"total"`
	Covered    int          `json:"covered"`
	InProgress int          `json:"in_progress"`
	Open       int          `json:"open"`
	Pct        float64      `json:"pct"` // covered / total, 0..1
	Items      []ItemStatus `json:"items"`
}

// Evaluate scores m against observations and citations. An observation counts
// toward the item whose id or risk area equals its risk area; a citation counts
// toward the item it names (unknown item ids are ignored). Absence of an
// observation never covers an item: an item is covered only by a closing one.
func Evaluate(m Methodology, obs []Observation, cites []Citation) Score {
	owner := map[string]int{}
	for i, it := range m.Items {
		owner[Key(it.ID)] = i
		for _, ra := range it.RiskAreas {
			owner[Key(ra)] = i
		}
	}
	type acc struct {
		closed, followUp bool
		outcomes         map[string]bool
		intents          map[int64]bool
	}
	accs := make([]acc, len(m.Items))
	for _, o := range obs {
		i, ok := owner[Key(o.RiskArea)]
		if !ok {
			continue
		}
		a := &accs[i]
		if a.outcomes == nil {
			a.outcomes = map[string]bool{}
		}
		if o.Outcome != "" {
			a.outcomes[o.Outcome] = true
		}
		if o.Closes {
			a.closed = true
		} else {
			a.followUp = true
		}
	}
	for _, c := range cites {
		i, ok := owner[Key(c.ItemID)]
		if !ok || Key(m.Items[i].ID) != Key(c.ItemID) { // cite by item id, not alias
			continue
		}
		a := &accs[i]
		if a.intents == nil {
			a.intents = map[int64]bool{}
		}
		a.intents[c.IntentID] = true
	}
	s := Score{ID: m.ID, Name: m.Name, Version: m.Version, Source: m.Source, Total: len(m.Items), Items: make([]ItemStatus, 0, len(m.Items))}
	for i, it := range m.Items {
		a := accs[i]
		st := ItemStatus{ID: it.ID, Category: it.Category, Title: it.Title, Status: StatusOpen, Outcomes: []string{}, Intents: []int64{}}
		for o := range a.outcomes {
			st.Outcomes = append(st.Outcomes, o)
		}
		sort.Strings(st.Outcomes)
		for id := range a.intents {
			st.Intents = append(st.Intents, id)
		}
		sort.Slice(st.Intents, func(x, y int) bool { return st.Intents[x] < st.Intents[y] })
		switch {
		case a.closed:
			st.Status = StatusCovered
			s.Covered++
		case a.followUp || len(a.intents) > 0:
			st.Status = StatusInProgress
			s.InProgress++
		default:
			s.Open++
		}
		s.Items = append(s.Items, st)
	}
	if s.Total > 0 {
		s.Pct = float64(s.Covered) / float64(s.Total)
	}
	return s
}

// Pending returns up to n items that are not covered, in checklist order:
// in-progress items first (finish what was started), then open ones.
func (s Score) Pending(n int) []ItemStatus {
	out := make([]ItemStatus, 0, n)
	for _, want := range []string{StatusInProgress, StatusOpen} {
		for _, it := range s.Items {
			if len(out) == n {
				return out
			}
			if it.Status == want {
				out = append(out, it)
			}
		}
	}
	return out
}
