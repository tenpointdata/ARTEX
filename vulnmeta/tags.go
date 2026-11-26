package vulnmeta

import (
	"regexp"
	"slices"
	"strings"
)

// Identifier shapes per framework, matched against the upper-cased id.
var (
	cveRE    = regexp.MustCompile(`^CVE-\d{4}-\d{4,}$`)
	tagShape = map[Framework]*regexp.Regexp{
		FrameworkCWE:         regexp.MustCompile(`^CWE-\d{1,5}$`),
		FrameworkOWASPASVS:   regexp.MustCompile(`^V\d{1,2}(\.\d{1,2}){1,2}$`),
		FrameworkOWASPWSTG:   regexp.MustCompile(`^WSTG-(V\d{2}-)?[A-Z]{4}-\d{2}(\.\d{1,2})?$`),
		FrameworkMITREATTACK: regexp.MustCompile(`^(T\d{4}(\.\d{3})?|TA\d{4}|M\d{4})$`),
	}
	bareNumber = regexp.MustCompile(`^\d{1,5}$`)
)

// KnownFramework reports whether f is a supported framework key.
func KnownFramework(f Framework) bool { _, ok := tagShape[f]; return ok }

// NormalizeTag validates one tag and returns its canonical form. A bare CWE
// number ("79") becomes "CWE-79".
func NormalizeTag(t Tag) (Tag, error) {
	f := Framework(strings.ToLower(strings.TrimSpace(string(t.Framework))))
	id := strings.ToUpper(strings.TrimSpace(t.ID))
	re, ok := tagShape[f]
	if !ok {
		return Tag{}, invalid("unknown framework %q (want one of cwe, owasp-asvs, owasp-wstg, mitre-attack)", t.Framework)
	}
	if f == FrameworkCWE && bareNumber.MatchString(id) {
		id = "CWE-" + id
	}
	if !re.MatchString(id) {
		return Tag{}, invalid("%s id %q is malformed", f.Label(), t.ID)
	}
	return Tag{Framework: f, ID: id}, nil
}

// NormalizeTags canonicalizes, de-duplicates and sorts tags (framework display
// order, then id). It fails on the first malformed tag.
func NormalizeTags(in []Tag) ([]Tag, error) {
	if len(in) > MaxTags {
		return nil, invalid("%d framework tags exceed the limit of %d", len(in), MaxTags)
	}
	out := make([]Tag, 0, len(in))
	for _, t := range in {
		n, err := NormalizeTag(t)
		if err != nil {
			return nil, err
		}
		if !slices.Contains(out, n) {
			out = append(out, n)
		}
	}
	slices.SortFunc(out, func(a, b Tag) int {
		if d := slices.Index(Frameworks, a.Framework) - slices.Index(Frameworks, b.Framework); d != 0 {
			return d
		}
		return strings.Compare(a.ID, b.ID)
	})
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

// ParseTag reads a tag written as a bare identifier whose framework is implied
// by its prefix ("CWE-79", "WSTG-INPV-05", "T1190") or as "<framework>:<id>"
// ("owasp-asvs:V5.3.4"; ASVS ids need the explicit form). It is the shared
// entry point for free-text sources: scanner imports, CSV, agent tool input.
func ParseTag(s string) (Tag, error) {
	s = strings.TrimSpace(s)
	if f, id, ok := strings.Cut(s, ":"); ok {
		return NormalizeTag(Tag{Framework: Framework(f), ID: id})
	}
	up := strings.ToUpper(s)
	for _, f := range []Framework{FrameworkCWE, FrameworkOWASPWSTG, FrameworkMITREATTACK} {
		if tagShape[f].MatchString(up) {
			return Tag{Framework: f, ID: up}, nil
		}
	}
	return Tag{}, invalid("cannot infer the framework of tag %q; write it as <framework>:<id>", s)
}
