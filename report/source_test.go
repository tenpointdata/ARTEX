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
	// Compare positions within the findings section: the executive summary
	// above it names the most urgent findings too.
	body := md[strings.Index(md, "## Findings"):]
	if strings.Index(body, "app/views/orders.py") > strings.Index(body, "Runtime SQLi") {
		t.Error("findings must still sort by severity across lanes (critical SAST before high runtime)")
	}
}

func TestReportWithoutImportsIsUnchanged(t *testing.T) {
	md := Markdown(Input{Title: "t", GeneratedAt: time.Unix(0, 0), Quality: map[int64]db.FindingQuality{}, Findings: []*db.Node{reportNode(1, "high", "x")}})
	if strings.Contains(md, "By lane") || strings.Contains(md, "**Source**") {
		t.Fatalf("worker-only report gained lane lines:\n%s", md)
	}
}

// TestExecutiveSummaryKeepsScannerClaimsUnconfirmed: the executive summary
// (E1) must not call imported scanner claims (E10) confirmed.
func TestExecutiveSummaryKeepsScannerClaimsUnconfirmed(t *testing.T) {
	md := Markdown(Input{Title: "t", GeneratedAt: time.Unix(0, 0), Quality: map[int64]db.FindingQuality{}, Findings: []*db.Node{
		reportNode(1, "high", "Runtime SQLi"),
		importedNode(2, "critical", "sast", "app/views/orders.py", 42),
	}})
	for _, want := range []string{
		"2 finding(s), 1 confirmed by testing and 1 imported scanner claim(s) not yet reproduced",
		"**[CRITICAL]** rule.sast (scanner claim, unverified)",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("executive summary lacks %q:\n%s", want, md)
		}
	}
	if strings.Contains(md, "2 finding(s) confirmed") {
		t.Errorf("executive summary counts a scanner claim as confirmed:\n%s", md)
	}
}
