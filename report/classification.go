package report

import (
	"fmt"
	"strings"

	"github.com/Autumn-27/artex/vulnmeta"
)

// Rendering of the client-facing classification (E1): CVSS v4.0, EPSS,
// framework tags and remediation. Every deliverable (task report, findings
// summary, single-finding file, branded HTML, CSV) goes through these helpers,
// so one finding reads the same everywhere. An unclassified finding says so
// once instead of printing empty fields.

// cvssLine renders "9.3 Critical (CVSS:4.0/…)", or "" when unscored.
func cvssLine(c *vulnmeta.CVSS) string {
	if c == nil {
		return ""
	}
	return fmt.Sprintf("%.1f %s (`%s`)", c.Score, titleCase(c.Rating), c.Vector)
}

// epssLine renders "42.0% … (above 90.0% of scored CVEs, CVE-…, as of 2026-11-20)", or "".
func epssLine(e *vulnmeta.EPSS) string {
	if e == nil {
		return ""
	}
	parts := []string{}
	if e.Percentile > 0 {
		parts = append(parts, fmt.Sprintf("above %.1f%% of scored CVEs", e.Percentile*100))
	}
	if e.CVE != "" {
		parts = append(parts, e.CVE)
	}
	if e.Date != "" {
		parts = append(parts, "as of "+e.Date)
	}
	out := fmt.Sprintf("%.1f%% probability of exploitation in 30 days", e.Probability*100)
	if len(parts) > 0 {
		out += " (" + strings.Join(parts, ", ") + ")"
	}
	return out
}

// frameworkRows lists "<Framework>: id, id" for each framework with tags.
func frameworkRows(c vulnmeta.Classification) [][2]string {
	var out [][2]string
	for _, f := range vulnmeta.Frameworks {
		if ids := c.TagsFor(f); len(ids) > 0 {
			out = append(out, [2]string{f.Label(), strings.Join(ids, ", ")})
		}
	}
	return out
}

// classificationBullets renders the CVSS / EPSS / framework bullets, ending with
// a blank line. An unscored finding states it, since a deliverable without a
// score must say the score is missing rather than look complete.
func classificationBullets(c vulnmeta.Classification) string {
	var b strings.Builder
	if l := cvssLine(c.CVSS); l != "" {
		fmt.Fprintf(&b, "- **CVSS v4.0**: %s\n", l)
	} else {
		b.WriteString("- **CVSS v4.0**: not scored\n")
	}
	if l := epssLine(c.EPSS); l != "" {
		fmt.Fprintf(&b, "- **EPSS**: %s\n", l)
	}
	for _, r := range frameworkRows(c) {
		fmt.Fprintf(&b, "- **%s**: %s\n", r[0], r[1])
	}
	b.WriteString("\n")
	return b.String()
}

// remediationSection renders the remediation guidance under heading, or a
// one-line gap notice when none was written.
func remediationSection(c vulnmeta.Classification, heading string) string {
	if s := strings.TrimSpace(c.Remediation); s != "" {
		return fmt.Sprintf("%s\n\n%s\n\n", heading, s)
	}
	return fmt.Sprintf("%s\n\n_No remediation guidance recorded yet._\n\n", heading)
}

// csvTags flattens the framework tags into one cell: "CWE-89; WSTG-INPV-05".
func csvTags(c vulnmeta.Classification) string {
	ids := make([]string, 0, len(c.Tags))
	for _, t := range c.Tags {
		ids = append(ids, t.ID)
	}
	return strings.Join(ids, "; ")
}

func titleCase(s string) string {
	if s == "" {
		return ""
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
