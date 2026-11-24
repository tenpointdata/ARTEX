package report

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Autumn-27/artex/db"
)

// Source is where a finding came from: the runtime lane (ARTEX's own workers,
// or an imported runtime scanner) or the code lanes (SAST, SCA) fed by
// scanner imports (E10). Code and runtime findings share the exploration graph,
// so one report lists both; Source says which lane each one came from.
type Source struct {
	Imported bool
	Lane     string // runtime | sast | sca; "" for a worker's finding
	Tool     string
	RuleID   string
	Location string // repo-relative path, code lanes only
	Line     int
	ImportID int64
}

// sourceFromPayload reads the import provenance an import-written finding
// node carries (db.importedFindingNodeTx); a worker's node has none.
func sourceFromPayload(raw json.RawMessage) Source {
	var p struct {
		Source   string `json:"source"`
		Lane     string `json:"lane"`
		Tool     string `json:"tool"`
		RuleID   string `json:"rule_id"`
		Location string `json:"location"`
		Line     int    `json:"line"`
		ImportID int64  `json:"import_id"`
	}
	_ = json.Unmarshal(raw, &p)
	if p.Source != db.ImportSourceOrigin {
		return Source{}
	}
	return Source{Imported: true, Lane: p.Lane, Tool: p.Tool, RuleID: p.RuleID, Location: p.Location, Line: p.Line, ImportID: p.ImportID}
}

// LaneKey is the lane a finding is counted under.
func (s Source) LaneKey() string {
	if s.Lane == "" {
		return "runtime"
	}
	return s.Lane
}

// laneNames are the report's names for each lane, in report order.
var laneNames = []struct{ key, name string }{
	{"runtime", "runtime testing"},
	{"sast", "code scan (SAST)"},
	{"sca", "dependency scan (SCA)"},
}

func laneName(key string) string {
	for _, l := range laneNames {
		if l.key == key {
			return l.name
		}
	}
	return key
}

// codeSpan renders s as Markdown inline code; a backtick in it cannot close
// the span.
func codeSpan(s string) string {
	return "`" + strings.ReplaceAll(oneLine(s), "`", "'") + "`"
}

// bullet is the Source line of a finding in the report; "" for a worker's
// finding, which the proof bullets already describe.
func (s Source) bullet() string {
	if !s.Imported {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "- **Source**: %s, imported from %s", laneName(s.LaneKey()), nz(s.Tool, "a scanner"))
	if s.RuleID != "" {
		fmt.Fprintf(&b, " rule %s", codeSpan(s.RuleID))
	}
	if s.Location != "" {
		loc := s.Location
		if s.Line > 0 {
			loc = fmt.Sprintf("%s:%d", loc, s.Line)
		}
		fmt.Fprintf(&b, " at %s", codeSpan(loc))
	}
	if s.ImportID > 0 {
		fmt.Fprintf(&b, " (import #%d)", s.ImportID)
	}
	b.WriteString("; a scanner claim until reproduced\n")
	return b.String()
}

// laneSummary is the summary's per-lane breakdown, printed only when more
// than the runtime lane is present or any finding was imported.
func laneSummary(nodes []*db.Node) string {
	counts := map[string]int{}
	imported := 0
	for _, n := range nodes {
		s := sourceFromPayload(n.Payload)
		counts[s.LaneKey()]++
		if s.Imported {
			imported++
		}
	}
	if imported == 0 {
		return ""
	}
	parts := make([]string, 0, len(laneNames))
	for _, l := range laneNames {
		if counts[l.key] > 0 {
			parts = append(parts, fmt.Sprintf("%s %d", l.name, counts[l.key]))
		}
	}
	return fmt.Sprintf("- By lane: %s (%d imported from scanners, unverified until reproduced)\n", strings.Join(parts, ", "), imported)
}
