package report

import (
	"bytes"
	"encoding/csv"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Autumn-27/artex/db"
	"github.com/Autumn-27/artex/vulnmeta"
)

const testVector = "CVSS:4.0/AV:N/AC:L/AT:N/PR:N/UI:N/VC:H/VI:H/VA:H/SC:N/SI:N/SA:N"

func classified(t *testing.T) vulnmeta.Classification {
	t.Helper()
	c, err := vulnmeta.Normalize(vulnmeta.Classification{
		CVSS:        &vulnmeta.CVSS{Vector: testVector},
		EPSS:        &vulnmeta.EPSS{Probability: 0.42, Percentile: 0.9, CVE: "CVE-2024-0001", Date: "2026-11-20"},
		Remediation: "Use parameterized queries for every SQL statement.",
		Tags:        []vulnmeta.Tag{{Framework: "cwe", ID: "89"}, {Framework: "owasp-wstg", ID: "WSTG-INPV-05"}, {Framework: "owasp-asvs", ID: "V5.3.4"}, {Framework: "mitre-attack", ID: "T1190"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func exportFindings(t *testing.T) []*db.DBFinding {
	return []*db.DBFinding{
		{ID: 1, Name: "SQL injection", Severity: "critical", Status: "confirmed", VerificationStatus: db.VerificationVerified, Classification: classified(t)},
		{ID: 2, Name: "Missing header", Severity: "high", Status: "pending"},
		{ID: 3, Name: "Banner", Severity: "low", Status: "pending"},
	}
}

func assertContains(t *testing.T, where, doc string, want ...string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(doc, w) {
			t.Errorf("%s: missing %q in\n%s", where, w, doc)
		}
	}
}

// Every Markdown export carries CVSS, EPSS, framework tags and remediation per
// finding, states the gaps of an unclassified one, and opens with the
// executive summary.
func TestExportsCarryClassification(t *testing.T) {
	fs := exportFindings(t)
	now := time.Date(2026, 11, 24, 9, 0, 0, 0, time.UTC)
	perFinding := []string{
		"- **CVSS v4.0**: 9.3 Critical (`" + testVector + "`)",
		"- **EPSS**: 42.0% probability of exploitation in 30 days (above 90.0% of scored CVEs, CVE-2024-0001, as of 2026-11-20)",
		"- **CWE**: CWE-89", "- **OWASP ASVS**: V5.3.4", "- **OWASP WSTG**: WSTG-INPV-05", "- **MITRE ATT&CK**: T1190",
		"Use parameterized queries for every SQL statement.",
	}
	summary := FindingsMarkdown(fs, now)
	assertContains(t, "summary", summary, append(perFinding,
		"## Executive Summary",
		"Overall risk is Critical: 3 finding(s) confirmed, 2 rated high or critical and requiring prompt remediation.",
		"| Critical | 1 |", "| High | 1 |", "| Medium | 0 |",
		"- **Verified**: 1 of 3", "- **CVSS v4.0 scored**: 1 of 3 (highest 9.3 Critical)",
		"- **Remediation gap**: 1 high/critical finding(s) have no remediation guidance yet",
		"1. **[CRITICAL]** SQL injection — CVSS 9.3", "| CWE | CWE-89 (1) |",
		"- **CVSS v4.0**: not scored", "_No remediation guidance recorded yet._", "**Remediation:**")...)
	if strings.Index(summary, "## Executive Summary") > strings.Index(summary, "## Vulnerability Details") {
		t.Error("executive summary must precede the details")
	}
	assertContains(t, "single", SingleFindingMarkdown(fs[0], now), append(perFinding, "## Remediation")...)
}

// The task report renders the same classification from the quality lookup and
// adds no classification claims for a finding whose row is missing.
func TestTaskReportClassification(t *testing.T) {
	rows := map[int64]db.FindingQuality{8: {FindingMeta: db.FindingMeta{ID: 1}, VerificationStatus: db.VerificationVerified, Classification: classified(t)}}
	md := Markdown(Input{Title: "t", Findings: []*db.Node{reportNode(8, "critical", "SQLi"), reportNode(9, "low", "Banner")}, Quality: rows})
	assertContains(t, "task report", md, "## Executive Summary", "- **CVSS v4.0**: 9.3 Critical", "- **CWE**: CWE-89",
		"Use parameterized queries", "1. **[CRITICAL]** SQLi — CVSS 9.3", "- **Verified**: 1 of 2")
	banner := md[strings.Index(md, "[LOW] Banner"):]
	if strings.Contains(banner, "CVSS v4.0") || strings.Contains(banner, "Remediation") {
		t.Errorf("a finding with no row must not get classification lines:\n%s", banner)
	}
}

func TestExecutiveSummaryEmpty(t *testing.T) {
	s := Summarize(nil)
	if s.Overall != "" || s.MaxCVSS != nil || len(s.Top) != 0 {
		t.Fatalf("empty summary %+v", s)
	}
	assertContains(t, "empty", s.Markdown(), "No vulnerabilities were confirmed in the assessed scope.", "| Critical | 0 |")
}

// Top lists at most summaryTop findings, most severe and highest score first.
func TestExecutiveSummaryTopOrder(t *testing.T) {
	low, _ := vulnmeta.ScoreCVSS4("CVSS:4.0/AV:L/AC:H/AT:P/PR:H/UI:A/VC:L/VI:N/VA:N/SC:N/SI:N/SA:N")
	high, _ := vulnmeta.ScoreCVSS4(testVector)
	items := []SummaryItem{{Title: "m", Severity: "medium"}, {Title: "h-low", Severity: "high", Classification: vulnmeta.Classification{CVSS: &low}},
		{Title: "h-high", Severity: "high", Classification: vulnmeta.Classification{CVSS: &high}}, {Title: "l1", Severity: "low"},
		{Title: "l2", Severity: "low"}, {Title: "c", Severity: "critical"}, {Title: "l3", Severity: "low"}}
	s := Summarize(items)
	got := []string{}
	for _, it := range s.Top {
		got = append(got, it.Title)
	}
	if strings.Join(got, ",") != "c,h-high,h-low,m,l1" {
		t.Fatalf("top order %v", got)
	}
}

func TestFindingsCSVClassificationColumns(t *testing.T) {
	r := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(FindingsCSV(exportFindings(t)), []byte("\xEF\xBB\xBF"))))
	recs, err := r.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	head, row := recs[0], recs[1]
	idx := func(name string) int {
		for i, h := range head {
			if h == name {
				return i
			}
		}
		t.Fatalf("no column %q in %v", name, head)
		return -1
	}
	col := func(name string) string { return row[idx(name)] }
	if col("CVSS v4 Score") != "9.3" || col("CVSS v4 Vector") != testVector || col("EPSS Probability") != "0.42000" ||
		col("Framework Tags") != "CWE-89; V5.3.4; WSTG-INPV-05; T1190" || !strings.HasPrefix(col("Remediation"), "Use parameterized") {
		t.Fatalf("classification columns: %v", row)
	}
	if recs[2][idx("CVSS v4 Score")] != "" {
		t.Fatal("an unscored finding must leave the score cell empty")
	}
}

// The branded export applies the branding, carries the classification and
// executive summary, and escapes finding content, which is untrusted.
func TestFindingsHTMLBranded(t *testing.T) {
	fs := exportFindings(t)
	fs[1].Summary = `<script>alert(1)</script>`
	fs[1].Name = `<img src=x onerror=alert(1)>`
	out, err := FindingsHTML(fs, time.Date(2026, 11, 24, 9, 0, 0, 0, time.UTC), Branding{
		Organization: "Ten Point Data", Client: "Acme & Co", Marking: "CONFIDENTIAL", AccentColor: "#aa3300", Footer: "contact@example.test",
	})
	if err != nil {
		t.Fatal(err)
	}
	doc := string(out)
	assertContains(t, "html", doc, "--accent: #aa3300", "CONFIDENTIAL", "Prepared for <strong>Acme &amp; Co</strong>",
		"by <strong>Ten Point Data</strong>", "Executive Summary", "Overall risk is Critical", "9.3 Critical",
		"<code>"+testVector+"</code>", "WSTG-INPV-05", "T1190", "Use parameterized queries", "No remediation guidance recorded yet.",
		"&lt;script&gt;alert(1)&lt;/script&gt;", "contact@example.test", "Content-Security-Policy")
	if strings.Contains(doc, "<script>") || strings.Contains(doc, "<img") {
		t.Fatal("finding content was not escaped")
	}
	if !strings.Contains(string(mustHTML(t, Branding{})), "--accent: "+defaultAccent) {
		t.Fatal("default accent missing")
	}
}

func mustHTML(t *testing.T, b Branding) []byte {
	t.Helper()
	out, err := FindingsHTML(nil, time.Now(), b)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestBrandingNormalize(t *testing.T) {
	b, err := Branding{Client: "  Acme ", AccentColor: " #A1B2C3 "}.Normalize()
	if err != nil || b.Client != "Acme" || b.AccentColor != "#A1B2C3" {
		t.Fatalf("got %+v %v", b, err)
	}
	for _, bad := range []Branding{{AccentColor: "red;}body{x"}, {AccentColor: "#12345"}, {Client: strings.Repeat("x", 201)}, {Footer: strings.Repeat("x", 1001)}} {
		if _, err := bad.Normalize(); !errors.Is(err, ErrInvalidBranding) {
			t.Errorf("%+v: want ErrInvalidBranding, got %v", bad, err)
		}
	}
}
