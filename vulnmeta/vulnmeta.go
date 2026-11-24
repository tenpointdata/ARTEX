// Package vulnmeta holds the client-facing classification of a vulnerability
// finding: a CVSS v4.0 vector and the score computed from it, an optional EPSS
// estimate, remediation guidance, and tags into the standard frameworks (OWASP
// ASVS and WSTG, MITRE ATT&CK, CWE).
//
// It is a leaf package with no ARTEX dependency, so any consumer (the findings
// store, report rendering, ticket exporters, scanner importers) can share one
// validated contract. Invariants every Classification returned by Normalize
// holds:
//
//   - a CVSS score is never taken from the caller: it is computed from the
//     vector with the FIRST v4.0 algorithm, so a stored score always matches
//     its vector;
//   - EPSS probability and percentile lie in [0, 1];
//   - tags are canonical (upper-case identifiers, lower-case framework keys),
//     well-formed for their framework, de-duplicated and sorted.
package vulnmeta

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	cvss40 "github.com/pandatix/go-cvss/40"
)

// CVSS is a CVSS v4.0 assessment. Score and Rating are derived from Vector.
type CVSS struct {
	Vector string  `json:"vector"`
	Score  float64 `json:"score"`  // 0.0–10.0, one decimal
	Rating string  `json:"rating"` // none | low | medium | high | critical
}

// EPSS is an Exploit Prediction Scoring System estimate for the finding's CVE
// (FIRST EPSS). It only applies to findings that map to a published CVE.
type EPSS struct {
	Probability float64 `json:"probability"`          // 0–1: chance of exploitation in the next 30 days
	Percentile  float64 `json:"percentile,omitempty"` // 0–1: rank among all scored CVEs
	Date        string  `json:"date,omitempty"`       // YYYY-MM-DD the estimate was published
	CVE         string  `json:"cve,omitempty"`        // the CVE the estimate belongs to
}

// Framework identifies a standards catalogue a finding is mapped to.
type Framework string

const (
	FrameworkCWE         Framework = "cwe"          // CWE-79
	FrameworkOWASPASVS   Framework = "owasp-asvs"   // V5.3.4 (ASVS 4) or V1.2.3 (ASVS 5)
	FrameworkOWASPWSTG   Framework = "owasp-wstg"   // WSTG-INPV-05
	FrameworkMITREATTACK Framework = "mitre-attack" // T1190, T1059.007, TA0001
)

// Frameworks lists every supported framework in display order.
var Frameworks = []Framework{FrameworkCWE, FrameworkOWASPASVS, FrameworkOWASPWSTG, FrameworkMITREATTACK}

// Label is the human-readable name of the framework.
func (f Framework) Label() string {
	switch f {
	case FrameworkCWE:
		return "CWE"
	case FrameworkOWASPASVS:
		return "OWASP ASVS"
	case FrameworkOWASPWSTG:
		return "OWASP WSTG"
	case FrameworkMITREATTACK:
		return "MITRE ATT&CK"
	}
	return string(f)
}

// Tag maps a finding to one entry of a framework.
type Tag struct {
	Framework Framework `json:"framework"`
	ID        string    `json:"id"`
}

// String renders the tag as "<Framework label> <id>".
func (t Tag) String() string { return t.Framework.Label() + " " + t.ID }

// Classification is the full client-facing classification of one finding.
// The zero value means "not classified".
type Classification struct {
	CVSS        *CVSS  `json:"cvss,omitempty"`
	EPSS        *EPSS  `json:"epss,omitempty"`
	Remediation string `json:"remediation,omitempty"`
	Tags        []Tag  `json:"framework_tags,omitempty"`
}

// IsZero reports whether nothing is classified.
func (c Classification) IsZero() bool {
	return c.CVSS == nil && c.EPSS == nil && strings.TrimSpace(c.Remediation) == "" && len(c.Tags) == 0
}

// TagsFor returns the tags of one framework, in canonical order.
func (c Classification) TagsFor(f Framework) []string {
	var out []string
	for _, t := range c.Tags {
		if t.Framework == f {
			out = append(out, t.ID)
		}
	}
	return out
}

// MaxRemediationBytes bounds the remediation text a finding can carry.
const MaxRemediationBytes = 64 << 10

// MaxTags bounds the framework tags on one finding.
const MaxTags = 64

// ErrInvalid wraps every validation failure, so callers can map it to a 400.
var ErrInvalid = errors.New("invalid classification")

func invalid(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, a...))
}

// ScoreCVSS4 parses and validates a CVSS v4.0 vector and computes its score.
// The returned vector is the canonical form (metrics in specification order).
func ScoreCVSS4(vector string) (CVSS, error) {
	v := strings.TrimSpace(vector)
	parsed, err := cvss40.ParseVector(v)
	if err != nil {
		return CVSS{}, invalid("cvss vector %q: %v", v, err)
	}
	score := math.Round(parsed.Score()*10) / 10
	return CVSS{Vector: parsed.Vector(), Score: score, Rating: Rating(score)}, nil
}

// Normalize validates c and returns its canonical form: the CVSS score is
// recomputed from the vector, EPSS is range-checked, remediation is trimmed and
// tags are canonicalized. A CVSS or EPSS that is present but empty is dropped.
func Normalize(c Classification) (Classification, error) {
	out := Classification{Remediation: strings.TrimSpace(c.Remediation)}
	if len(out.Remediation) > MaxRemediationBytes {
		return Classification{}, invalid("remediation exceeds %d bytes", MaxRemediationBytes)
	}
	if c.CVSS != nil && strings.TrimSpace(c.CVSS.Vector) != "" {
		s, err := ScoreCVSS4(c.CVSS.Vector)
		if err != nil {
			return Classification{}, err
		}
		out.CVSS = &s
	}
	if c.EPSS != nil {
		e, err := normalizeEPSS(*c.EPSS)
		if err != nil {
			return Classification{}, err
		}
		out.EPSS = e
	}
	tags, err := NormalizeTags(c.Tags)
	if err != nil {
		return Classification{}, err
	}
	out.Tags = tags
	return out, nil
}

func normalizeEPSS(e EPSS) (*EPSS, error) {
	if e == (EPSS{}) {
		return nil, nil
	}
	for name, v := range map[string]float64{"probability": e.Probability, "percentile": e.Percentile} {
		if math.IsNaN(v) || v < 0 || v > 1 {
			return nil, invalid("epss %s %v is outside [0, 1]", name, v)
		}
	}
	e.Date = strings.TrimSpace(e.Date)
	if e.Date != "" {
		if _, err := time.Parse(time.DateOnly, e.Date); err != nil {
			return nil, invalid("epss date %q is not YYYY-MM-DD", e.Date)
		}
	}
	e.CVE = strings.ToUpper(strings.TrimSpace(e.CVE))
	if e.CVE != "" && !cveRE.MatchString(e.CVE) {
		return nil, invalid("epss cve %q is not CVE-YYYY-NNNN", e.CVE)
	}
	return &e, nil
}

// Rating is the CVSS v4.0 qualitative rating of a score: none (0.0), low
// (0.1–3.9), medium (4.0–6.9), high (7.0–8.9) or critical (9.0–10.0). An
// out-of-range score rates "".
func Rating(score float64) string {
	r, err := cvss40.Rating(score)
	if err != nil {
		return ""
	}
	return strings.ToLower(r)
}
