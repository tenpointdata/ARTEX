package report

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"html/template"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Autumn-27/artex/db"
	"github.com/Autumn-27/artex/vulnmeta"
)

// Branding is the presentation of a client deliverable: who wrote it, for whom,
// its handling marking and accent colour. It carries no behaviour, so the same
// value can brand any export format.
type Branding struct {
	Organization string `json:"organization"` // the testing organization, shown in the header
	Client       string `json:"client"`       // the client the report is prepared for
	Title        string `json:"title"`        // overrides the default document title
	Marking      string `json:"marking"`      // handling marking, e.g. "CONFIDENTIAL"
	AccentColor  string `json:"accent_color"` // #rrggbb; default defaultAccent
	Footer       string `json:"footer"`       // free text in the page footer, e.g. contact or legal notice
}

const (
	defaultAccent        = "#1f4e79"
	defaultBrandTitle    = "Penetration Test Findings Report"
	maxBrandingFieldRune = 200
	maxBrandingFooter    = 1000
)

var hexColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// ErrInvalidBranding wraps every Branding validation failure.
var ErrInvalidBranding = errors.New("invalid branding")

// Normalize trims every field, applies the defaults and validates lengths and
// the colour. The colour is the only field placed inside CSS, so it is held to
// a strict #rrggbb shape; every other field is escaped as text.
func (b Branding) Normalize() (Branding, error) {
	trim := func(s *string, limit int, name string) error {
		*s = strings.TrimSpace(*s)
		if utf8.RuneCountInString(*s) > limit {
			return fmt.Errorf("%w: %s exceeds %d characters", ErrInvalidBranding, name, limit)
		}
		return nil
	}
	for _, f := range []struct {
		p     *string
		limit int
		name  string
	}{{&b.Organization, maxBrandingFieldRune, "organization"}, {&b.Client, maxBrandingFieldRune, "client"},
		{&b.Title, maxBrandingFieldRune, "title"}, {&b.Marking, maxBrandingFieldRune, "marking"},
		{&b.AccentColor, 7, "accent_color"}, {&b.Footer, maxBrandingFooter, "footer"}} {
		if err := trim(f.p, f.limit, f.name); err != nil {
			return Branding{}, err
		}
	}
	if b.AccentColor != "" && !hexColor.MatchString(b.AccentColor) {
		return Branding{}, fmt.Errorf("%w: accent_color %q is not #rrggbb", ErrInvalidBranding, b.AccentColor)
	}
	return b, nil
}

//go:embed templates/findings.html.tmpl
var findingsHTMLSource string

var findingsHTMLTemplate = template.Must(template.New("findings").Funcs(template.FuncMap{
	"upper":     strings.ToUpper,
	"title":     titleCase,
	"sevLabel":  severityLabel,
	"cvss":      func(c *vulnmeta.CVSS) string { return fmt.Sprintf("%.1f", c.Score) },
	"epss":      epssLine,
	"verify":    verificationLabel,
	"frameRows": frameworkRows,
	"time":      func(t time.Time) string { return t.Format("2006-01-02 15:04") },
	"trim":      strings.TrimSpace,
	"inc":       func(i int) int { return i + 1 },
}).Parse(findingsHTMLSource))

type htmlFinding struct {
	*db.DBFinding
	Index int
	Title string
}

type htmlReport struct {
	Brand       Branding
	Accent      template.CSS // validated #rrggbb only
	Title       string
	GeneratedAt time.Time
	Summary     ExecutiveSummary
	Findings    []htmlFinding
}

// FindingsHTML renders findings as one self-contained, branded HTML document
// (inline CSS, no scripts, no external resources) that prints cleanly to PDF.
// All finding text is escaped by html/template: finding content is untrusted
// target and model output and must never become markup in a client's browser.
func FindingsHTML(fs []*db.DBFinding, generatedAt time.Time, brand Branding) ([]byte, error) {
	brand, err := brand.Normalize()
	if err != nil {
		return nil, err
	}
	items := append([]*db.DBFinding(nil), fs...)
	sortFindingsForExport(items)
	accent := nz(brand.AccentColor, defaultAccent)
	r := htmlReport{Brand: brand, Accent: template.CSS(accent), Title: nz(brand.Title, defaultBrandTitle),
		GeneratedAt: generatedAt, Summary: Summarize(summaryItemsFromDB(items))}
	for i, f := range items {
		r.Findings = append(r.Findings, htmlFinding{DBFinding: f, Index: i + 1, Title: findingTitle(f)})
	}
	var buf bytes.Buffer
	if err := findingsHTMLTemplate.Execute(&buf, r); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
