// Package methodology models testing methodologies (OWASP WSTG, OWASP API
// Security Top 10, ...) as declarative, typed checklists and scores a task's
// progress against them.
//
// The package is a leaf: it knows nothing about tasks, the database or the
// planner. A methodology is data (an embedded JSON file under data/); a consumer
// adds one by dropping another file there. Scoring (score.go) takes plain
// observations and citations, so any ledger can feed it.
//
// Matching contract: a ledger risk_area closes an item when it equals the item
// id or one of its risk_areas after folding (Key). Within one methodology an
// alias names at most one item (enforced by Validate), so one generic entry such
// as "xss" never ticks several items of the same checklist. The same alias may
// appear in different methodologies ("idor" is both WSTG-ATHZ-04 and API1:2023).
package methodology

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"sync"
	"unicode"
)

// Item is one checklist entry: a test the methodology expects to be performed.
type Item struct {
	ID        string   `json:"id"`
	Category  string   `json:"category"`
	Title     string   `json:"title"`
	RiskAreas []string `json:"risk_areas"` // ledger risk_area aliases that close this item
}

// Methodology is one codified checklist.
type Methodology struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	Source      string   `json:"source"`
	AppliesWhen []string `json:"applies_when"` // lower-case words in a task's goal that make it applicable
	Items       []Item   `json:"items"`
}

// Validate checks the structural invariants of a methodology: an id, at least
// one item, unique item ids, and every alias (item ids included) naming one item.
func (m Methodology) Validate() error {
	if strings.TrimSpace(m.ID) == "" || strings.TrimSpace(m.Name) == "" {
		return fmt.Errorf("methodology: id and name are required")
	}
	if len(m.Items) == 0 {
		return fmt.Errorf("methodology %s: no items", m.ID)
	}
	owner := map[string]string{}
	for _, it := range m.Items {
		if strings.TrimSpace(it.ID) == "" || strings.TrimSpace(it.Title) == "" {
			return fmt.Errorf("methodology %s: an item lacks id or title", m.ID)
		}
		for _, k := range append([]string{it.ID}, it.RiskAreas...) {
			k = Key(k)
			if k == "" {
				return fmt.Errorf("methodology %s: item %s has an empty risk area", m.ID, it.ID)
			}
			if prev, dup := owner[k]; dup && prev != it.ID {
				return fmt.Errorf("methodology %s: %q names both %s and %s", m.ID, k, prev, it.ID)
			} else if dup {
				return fmt.Errorf("methodology %s: item %s repeats %q", m.ID, it.ID, k)
			}
			owner[k] = it.ID
		}
	}
	return nil
}

// Item returns the item whose id equals id (case-insensitive).
func (m Methodology) Item(id string) (Item, bool) {
	k := Key(id)
	for _, it := range m.Items {
		if Key(it.ID) == k {
			return it, true
		}
	}
	return Item{}, false
}

// Key folds an item id or risk area to its matching form: trimmed, lower-case,
// with spaces and hyphens as underscores, so "SQL Injection", "sql-injection"
// and "sql_injection" match, and "WSTG-INPV-05" matches "wstg_inpv_05".
func Key(s string) string {
	return strings.NewReplacer(" ", "_", "-", "_").Replace(strings.ToLower(strings.Join(strings.Fields(s), " ")))
}

//go:embed data/*.json
var dataFS embed.FS

var (
	loadOnce sync.Once
	builtin  []Methodology
	loadErr  error
)

// Load parses every *.json file of fsys as one methodology and validates it.
// The result is sorted by id; a duplicate id is an error.
func Load(fsys fs.FS) ([]Methodology, error) {
	names, err := fs.Glob(fsys, "*.json")
	if err != nil {
		return nil, err
	}
	out := make([]Methodology, 0, len(names))
	seen := map[string]bool{}
	for _, n := range names {
		raw, err := fs.ReadFile(fsys, n)
		if err != nil {
			return nil, err
		}
		var m Methodology
		dec := json.NewDecoder(strings.NewReader(string(raw)))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&m); err != nil {
			return nil, fmt.Errorf("methodology %s: %w", n, err)
		}
		if err := m.Validate(); err != nil {
			return nil, err
		}
		if seen[m.ID] {
			return nil, fmt.Errorf("methodology id %s defined twice", m.ID)
		}
		seen[m.ID] = true
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// All returns the built-in methodologies (embedded data/), sorted by id. The
// embedded data is validated by the package tests, so an error here means a
// broken build; it is returned rather than panicking.
func All() ([]Methodology, error) {
	loadOnce.Do(func() {
		sub, err := fs.Sub(dataFS, "data")
		if err != nil {
			loadErr = err
			return
		}
		builtin, loadErr = Load(sub)
	})
	return builtin, loadErr
}

// Get returns the built-in methodology with the given id.
func Get(id string) (Methodology, bool) {
	all, _ := All()
	for _, m := range all {
		if m.ID == id {
			return m, true
		}
	}
	return Methodology{}, false
}

// Select returns the methodologies of ms that apply to text (a task's goal and
// description): those with an applies_when word among text's words. A URL in
// the text yields the word "http" or "https", so a web target selects WSTG.
// The order of ms is kept; nothing applies to text without a matching word.
func Select(ms []Methodology, text string) []Methodology {
	words := map[string]bool{}
	for _, w := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		words[w] = true
	}
	var out []Methodology
	for _, m := range ms {
		for _, w := range m.AppliesWhen {
			if words[Key(w)] {
				out = append(out, m)
				break
			}
		}
	}
	return out
}
