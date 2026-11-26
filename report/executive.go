package report

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Autumn-27/artex/db"
	"github.com/Autumn-27/artex/vulnmeta"
)

// SummaryItem is the per-finding input of an executive summary. It is
// deliberately small so any finding source (exploration nodes, standalone
// rows, imported scanner results) can feed the same summary.
type SummaryItem struct {
	Title    string
	Severity string // critical | high | medium | low | "" (unrated)
	Verified bool
	// Imported marks an imported scanner claim (E10): listed and counted in
	// the distribution, but never called confirmed until reproduced.
	Imported       bool
	Classification vulnmeta.Classification
}

// SeverityCount is one row of the severity distribution.
type SeverityCount struct {
	Severity string
	Label    string
	Count    int
}

// TagCount is how many findings carry one framework id.
type TagCount struct {
	ID    string
	Count int
}

// FrameworkCoverage lists the most frequent ids of one framework.
type FrameworkCoverage struct {
	Framework vulnmeta.Framework
	Tags      []TagCount
}

// ExecutiveSummary is the typed, rendering-independent digest a client reads
// first: overall risk, severity distribution, the most urgent findings, the
// framework categories they fall into, and the gaps in the deliverable itself
// (unscored findings, urgent findings without remediation).
type ExecutiveSummary struct {
	Total      int
	Overall    string // the highest severity present; "" when there are no findings
	Severities []SeverityCount
	Verified   int
	Imported   int            // imported scanner claims among Total, not yet reproduced
	Scored     int            // findings with a CVSS v4.0 score
	MaxCVSS    *vulnmeta.CVSS // the highest score, nil when nothing is scored
	// UrgentWithoutRemediation counts critical/high findings that carry no
	// remediation guidance: a gap in the deliverable, reported rather than hidden.
	UrgentWithoutRemediation int
	Top                      []SummaryItem // the most severe findings, at most summaryTop
	Frameworks               []FrameworkCoverage
}

const (
	summaryTop     = 5 // findings listed under "most urgent"
	summaryTopTags = 5 // ids listed per framework
)

var severityOrder = []SeverityCount{
	{Severity: db.SeverityCritical, Label: "Critical"}, {Severity: db.SeverityHigh, Label: "High"},
	{Severity: db.SeverityMedium, Label: "Medium"}, {Severity: db.SeverityLow, Label: "Low"},
}

// Summarize builds the executive summary of items.
func Summarize(items []SummaryItem) ExecutiveSummary {
	s := ExecutiveSummary{Total: len(items), Severities: append([]SeverityCount(nil), severityOrder...)}
	tagCounts := map[vulnmeta.Tag]int{}
	for _, it := range items {
		for i := range s.Severities {
			if s.Severities[i].Severity == it.Severity {
				s.Severities[i].Count++
			}
		}
		if it.Verified {
			s.Verified++
		}
		if it.Imported {
			s.Imported++
		}
		if c := it.Classification.CVSS; c != nil {
			s.Scored++
			if s.MaxCVSS == nil || c.Score > s.MaxCVSS.Score {
				s.MaxCVSS = c
			}
		}
		if isUrgent(it.Severity) && strings.TrimSpace(it.Classification.Remediation) == "" {
			s.UrgentWithoutRemediation++
		}
		for _, t := range it.Classification.Tags {
			tagCounts[t]++
		}
	}
	for _, sc := range s.Severities {
		if sc.Count > 0 {
			s.Overall = sc.Severity
			break
		}
	}
	top := append([]SummaryItem(nil), items...)
	sort.SliceStable(top, func(i, j int) bool { return summaryLess(top[i], top[j]) })
	if len(top) > summaryTop {
		top = top[:summaryTop]
	}
	s.Top = top
	s.Frameworks = frameworkCoverage(tagCounts)
	return s
}

func isUrgent(sev string) bool { return sev == db.SeverityCritical || sev == db.SeverityHigh }

// summaryLess orders by severity, then CVSS score, both descending.
func summaryLess(a, b SummaryItem) bool {
	if ra, rb := sevRank[a.Severity], sevRank[b.Severity]; ra != rb {
		return ra < rb
	}
	return cvssScore(a.Classification) > cvssScore(b.Classification)
}

func cvssScore(c vulnmeta.Classification) float64 {
	if c.CVSS == nil {
		return -1
	}
	return c.CVSS.Score
}

func frameworkCoverage(counts map[vulnmeta.Tag]int) []FrameworkCoverage {
	var out []FrameworkCoverage
	for _, f := range vulnmeta.Frameworks {
		var tags []TagCount
		for t, n := range counts {
			if t.Framework == f {
				tags = append(tags, TagCount{ID: t.ID, Count: n})
			}
		}
		if len(tags) == 0 {
			continue
		}
		sort.Slice(tags, func(i, j int) bool {
			if tags[i].Count != tags[j].Count {
				return tags[i].Count > tags[j].Count
			}
			return tags[i].ID < tags[j].ID
		})
		if len(tags) > summaryTopTags {
			tags = tags[:summaryTopTags]
		}
		out = append(out, FrameworkCoverage{Framework: f, Tags: tags})
	}
	return out
}

// Headline is the one-sentence risk statement that opens the summary.
func (s ExecutiveSummary) Headline() string {
	if s.Total == 0 {
		return "No vulnerabilities were confirmed in the assessed scope."
	}
	urgent := 0
	for _, sc := range s.Severities {
		if isUrgent(sc.Severity) {
			urgent += sc.Count
		}
	}
	out := fmt.Sprintf("Overall risk is %s: %d finding(s) confirmed", severityLabel(s.Overall), s.Total)
	if s.Imported > 0 {
		out = fmt.Sprintf("Overall risk is %s: %d finding(s), %d confirmed by testing and %d imported scanner claim(s) not yet reproduced",
			severityLabel(s.Overall), s.Total, s.Total-s.Imported, s.Imported)
	}
	if urgent > 0 {
		out += fmt.Sprintf(", %d rated high or critical and requiring prompt remediation", urgent)
	}
	return out + "."
}

func severityLabel(sev string) string {
	for _, sc := range severityOrder {
		if sc.Severity == sev {
			return sc.Label
		}
	}
	return "Unrated"
}

// Markdown renders the summary as a "## Executive Summary" section.
func (s ExecutiveSummary) Markdown() string {
	var b strings.Builder
	b.WriteString("## Executive Summary\n\n")
	b.WriteString(s.Headline() + "\n\n")
	b.WriteString("| Severity | Count |\n| --- | --- |\n")
	for _, sc := range s.Severities {
		fmt.Fprintf(&b, "| %s | %d |\n", sc.Label, sc.Count)
	}
	b.WriteString("\n")
	if s.Total == 0 {
		return b.String()
	}
	fmt.Fprintf(&b, "- **Verified**: %d of %d\n", s.Verified, s.Total)
	fmt.Fprintf(&b, "- **CVSS v4.0 scored**: %d of %d", s.Scored, s.Total)
	if s.MaxCVSS != nil {
		fmt.Fprintf(&b, " (highest %.1f %s)", s.MaxCVSS.Score, titleCase(s.MaxCVSS.Rating))
	}
	b.WriteString("\n")
	if s.UrgentWithoutRemediation > 0 {
		fmt.Fprintf(&b, "- **Remediation gap**: %d high/critical finding(s) have no remediation guidance yet\n", s.UrgentWithoutRemediation)
	}
	b.WriteString("\n### Most urgent\n\n")
	for i, it := range s.Top {
		line := fmt.Sprintf("%d. **[%s]** %s", i+1, strings.ToUpper(nz(it.Severity, "info")), mdInline(it.Title))
		if c := it.Classification.CVSS; c != nil {
			line += fmt.Sprintf(" — CVSS %.1f", c.Score)
		}
		if it.Imported {
			line += " (scanner claim, unverified)"
		}
		b.WriteString(line + "\n")
	}
	b.WriteString("\n")
	if len(s.Frameworks) > 0 {
		b.WriteString("### Framework mapping\n\n| Framework | Most frequent |\n| --- | --- |\n")
		for _, fc := range s.Frameworks {
			ids := make([]string, 0, len(fc.Tags))
			for _, t := range fc.Tags {
				ids = append(ids, fmt.Sprintf("%s (%d)", t.ID, t.Count))
			}
			fmt.Fprintf(&b, "| %s | %s |\n", fc.Framework.Label(), strings.Join(ids, ", "))
		}
		b.WriteString("\n")
	}
	return b.String()
}

// mdInline keeps a title on one line and stops it from breaking a table or list.
func mdInline(s string) string {
	return strings.ReplaceAll(oneLine(s), "|", `\|`)
}

// summaryItemsFromDB adapts standalone finding rows.
func summaryItemsFromDB(fs []*db.DBFinding) []SummaryItem {
	out := make([]SummaryItem, 0, len(fs))
	for _, f := range fs {
		out = append(out, SummaryItem{Title: findingTitle(f), Severity: f.Severity,
			Verified: f.VerificationStatus == db.VerificationVerified, Imported: SourceOf(f).Imported, Classification: f.Classification})
	}
	return out
}
