package report

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Autumn-27/artex/db"
)

func importedNode(id int64, severity, lane, loc string, line int) *db.Node {
	raw, _ := json.Marshal(map[string]any{"name": "rule." + lane, "severity": severity, "summary": "semgrep: rule (imported)",
		"source": db.ImportSourceOrigin, "lane": lane, "tool": "semgrep", "rule_id": "py.sqli", "location": loc, "line": line, "import_id": 7,
		"evidence": map[string]any{"by": "import:semgrep", "poc": "quoted scanner text"}})
	return &db.Node{ID: id, Kind: db.KindFinding, Payload: raw}
}

// TestReportMixesRuntimeAndCodeFindings is the E10 done check at the report
// level: code (SAST/SCA) and runtime findings render in one report, each code
// finding with its provenance, and the summary breaks findings down by lane.
func TestReportMixesRuntimeAndCodeFindings(t *testing.T) {
	md := Markdown(Input{Title: "t", GeneratedAt: time.Unix(0, 0), Quality: map[int64]db.FindingQuality{}, Findings: []*db.Node{
		reportNode(1, "high", "Runtime SQLi"),
		importedNode(2, "critical", "sast", "app/views/orders.py", 42),
		importedNode(3, "medium", "sca", "web/package-lock`.json", 0),
	}})
	for _, want := range []string{
		"- Confirmed findings: **1**",
		"- Imported scanner findings, unverified until reproduced: **2**",
		"- By lane: runtime testing 1, code scan (SAST) 1, dependency scan (SCA) 1\n",
		"Runtime SQLi",
		"- **Source**: code scan (SAST), imported from semgrep rule `py.sqli` at `app/views/orders.py:42` (import #7); a scanner claim until reproduced",
		"- **Source**: dependency scan (SCA), imported from semgrep rule `py.sqli` at `web/package-lock'.json` (import #7)",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("report lacks %q:\n%s", want, md)
		}
	}
	if strings.Index(md, "app/views/orders.py") > strings.Index(md, "Runtime SQLi") {
		t.Error("findings must still sort by severity across lanes (critical SAST before high runtime)")
	}
}

func TestReportWithoutImportsIsUnchanged(t *testing.T) {
	md := Markdown(Input{Title: "t", GeneratedAt: time.Unix(0, 0), Quality: map[int64]db.FindingQuality{}, Findings: []*db.Node{reportNode(1, "high", "x")}})
	if strings.Contains(md, "By lane") || strings.Contains(md, "**Source**") {
		t.Fatalf("worker-only report gained lane lines:\n%s", md)
	}
}
