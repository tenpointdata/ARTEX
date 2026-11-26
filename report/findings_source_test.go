package report

import (
	"strings"
	"testing"
	"time"

	"github.com/Autumn-27/artex/db"
)

// The findings exports carry each finding's lane and code location (c-d059ae);
// a worker's finding is runtime with no Source line.
func TestFindingsExportsCarryLane(t *testing.T) {
	imp := int64(9)
	code := &db.DBFinding{ID: 1, Name: "SQL injection", Severity: "high", CreatedAt: time.Unix(0, 0),
		ImportSource: &db.FindingImportSource{FindingID: 1, ImportID: &imp, Tool: "semgrep", RuleID: "py.sqli",
			Lane: "sast", Location: "app/views.py", Line: 42}}
	worker := &db.DBFinding{ID: 2, Name: "XSS", Severity: "low", CreatedAt: time.Unix(0, 0)}
	fs := []*db.DBFinding{code, worker}

	csv := string(FindingsCSV(fs))
	if !strings.Contains(csv, ",Lane,Source Tool,Source Rule,Source Location,Import ID\r\n") &&
		!strings.Contains(csv, ",Lane,Source Tool,Source Rule,Source Location,Import ID\n") {
		t.Fatalf("csv header lacks the lane columns:\n%s", csv)
	}
	if !strings.Contains(csv, ",sast,semgrep,py.sqli,app/views.py:42,9") || !strings.Contains(csv, ",runtime,,,,") {
		t.Fatalf("csv rows:\n%s", csv)
	}
	md := FindingsMarkdown(fs, time.Unix(0, 0))
	if !strings.Contains(md, "- **Source**: code scan (SAST), imported from semgrep rule `py.sqli` at `app/views.py:42` (import #9)") {
		t.Fatalf("markdown lacks the Source line:\n%s", md)
	}
	if strings.Count(md, "- **Source**:") != 1 {
		t.Fatalf("a worker finding got a Source line:\n%s", md)
	}
	if one := SingleFindingMarkdown(code, time.Unix(0, 0)); !strings.Contains(one, "at `app/views.py:42`") {
		t.Fatalf("single-finding markdown lacks the location:\n%s", one)
	}
}
