package db

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Lesson scoping and scrubbing.
//
// A lesson is learned from one engagement's findings and later injected into
// another task's prompt, so its text must not carry that engagement's data and
// its reuse must stay inside the engagement unless the operator opts in.
//
//   - Scrubbing (this file) replaces target-specific strings with placeholders
//     when a lesson is written: URLs, emails, IPs, hostnames, the task's own
//     scope values and company names, and secret-looking strings. It is
//     best-effort pattern matching, which is why cross-engagement reuse is
//     opt-in (ARTEX_RSI_LESSONS_SHARED) and why selection also scopes by key.
//   - The scope key (lessonScopeKey) is the engagement identity for a task with
//     no company: a digest of its operator-declared scope, or the task itself
//     when it declares none. A company task is scoped by company_id and has an
//     empty key.

var (
	scrubURLRe    = regexp.MustCompile(`(?i)\b[a-z][a-z0-9+.\-]{1,15}://[^\s"'<>)\]]+`)
	scrubEmailRe  = regexp.MustCompile(`(?i)\b[a-z0-9._%+\-]+@[a-z0-9\-]+(?:\.[a-z0-9\-]+)+\b`)
	scrubJWTRe    = regexp.MustCompile(`\beyJ[A-Za-z0-9_\-]{8,}\.[A-Za-z0-9_\-]{4,}\.[A-Za-z0-9_\-]*`)
	scrubKVRe     = regexp.MustCompile(`(?i)\b(api[_-]?key|access[_-]?key|secret|token|passw(?:or)?d|pwd|authorization|bearer|cookie|session(?:id)?|jsessionid|phpsessid)\b(\s*[:=]\s*|\s+)("[^"]*"|'[^']*'|[^\s,;"']+)`)
	scrubIPv4Re   = regexp.MustCompile(`\b\d{1,3}(?:\.\d{1,3}){3}(?:/\d{1,2})?(?::\d{1,5})?\b`)
	scrubIPv6Re   = regexp.MustCompile(`(?i)\b(?:[0-9a-f]{1,4}:){7}[0-9a-f]{1,4}\b|\b(?:[0-9a-f]{1,4}:){1,7}:(?:[0-9a-f]{1,4}(?::[0-9a-f]{1,4}){0,6})?|::(?:[0-9a-f]{1,4}(?::[0-9a-f]{1,4}){0,6})`)
	scrubBearerRe = regexp.MustCompile(`(?i)\b(bearer|basic)\s+[A-Za-z0-9._~+/=\-]{6,}`)
	scrubHostRe   = regexp.MustCompile(`(?i)\b(?:[a-z0-9](?:[a-z0-9\-]*[a-z0-9])?\.)+([a-z]{2,24})\b(?::\d{1,5})?`)
	scrubSecretRe = regexp.MustCompile(`\b[A-Za-z0-9+/_\-]{24,}={0,2}`)
)

// scrubFileExt lists suffixes that look like a TLD but are almost always file
// names in a lesson ("config.php"). Scope values are removed separately and
// first, so a real target host is not left behind by this exception.
var scrubFileExt = map[string]bool{
	"php": true, "js": true, "json": true, "html": true, "htm": true, "css": true,
	"txt": true, "xml": true, "yml": true, "yaml": true, "log": true, "cgi": true,
	"asp": true, "aspx": true, "jsp": true, "bak": true, "sql": true, "env": true,
	"conf": true, "ini": true, "zip": true, "gz": true, "tar": true, "png": true,
	"jpg": true, "gif": true, "svg": true, "pdf": true, "git": true, "map": true,
}

// scrubSecretToken reports whether a long token looks like a secret rather than
// a word or an identifier slug: its longest alphanumeric run is at least 16
// characters and mixes letters and digits.
func scrubSecretToken(s string) bool {
	runs := strings.FieldsFunc(s, func(r rune) bool {
		return !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z'))
	})
	for _, run := range runs {
		if len(run) < 16 {
			continue
		}
		var letter, digit bool
		for _, r := range run {
			if r >= '0' && r <= '9' {
				digit = true
			} else {
				letter = true
			}
		}
		if letter && digit {
			return true
		}
	}
	return false
}

// ScrubLessonText replaces target-specific strings in s with placeholders.
// terms are the engagement's own names (scope domains, IPs, keywords, company
// names); they are matched without case, longest first, before the generic
// patterns so a partial host is never left behind.
func ScrubLessonText(s string, terms []string) string {
	if s == "" {
		return s
	}
	sorted := make([]string, 0, len(terms))
	for _, t := range terms {
		t = strings.TrimPrefix(strings.TrimSpace(t), "*.")
		if len([]rune(t)) >= 3 {
			sorted = append(sorted, t)
		}
	}
	sort.Slice(sorted, func(i, j int) bool { return len(sorted[i]) > len(sorted[j]) })
	s = scrubURLRe.ReplaceAllString(s, "<url>")
	s = scrubEmailRe.ReplaceAllString(s, "<email>")
	for _, t := range sorted {
		s = regexp.MustCompile(`(?i)`+regexp.QuoteMeta(t)).ReplaceAllString(s, "<target>")
	}
	s = scrubJWTRe.ReplaceAllString(s, "<secret>")
	s = scrubBearerRe.ReplaceAllString(s, "$1 <secret>")
	s = scrubKVRe.ReplaceAllString(s, "$1$2<secret>")
	s = scrubIPv4Re.ReplaceAllString(s, "<ip>")
	s = scrubIPv6Re.ReplaceAllString(s, "<ip>")
	s = scrubHostRe.ReplaceAllStringFunc(s, func(m string) string {
		sub := scrubHostRe.FindStringSubmatch(m)
		if len(sub) > 1 && scrubFileExt[strings.ToLower(sub[1])] {
			return m
		}
		return "<host>"
	})
	s = scrubSecretRe.ReplaceAllStringFunc(s, func(m string) string {
		if scrubSecretToken(m) {
			return "<secret>"
		}
		return m
	})
	return s
}

// taskLessonScopeTermsQ returns the strings that identify this task's target:
// every scope domain, network, icp/keyword value, and company name.
func taskLessonScopeTermsQ(ctx context.Context, q sqlExecQuerier, taskID int64) ([]string, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT COALESCE(NULLIF(ts.domain,''), host(ts.net), NULLIF(ts.value,''), c.name, '')
		FROM task_scope ts
		LEFT JOIN companies c ON c.id = ts.company_id
		WHERE ts.task_id=$1`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		if v != "" {
			out = append(out, v)
		}
	}
	return out, rows.Err()
}

// lessonScopeKey is the engagement key of a task with no company: a digest of
// its operator-declared (source='manual') target scope, so two tasks against
// the same declared target share lessons. A task that declares no target gets
// its own key and reuses only its own lessons. A company task returns "": its
// scope is company_id.
func lessonScopeKey(ctx context.Context, q sqlExecQuerier, taskID int64, companyIDs []int64) (string, error) {
	if len(companyIDs) > 0 {
		return "", nil
	}
	rows, err := q.QueryContext(ctx, `
		SELECT lower(ts.kind || ':' || COALESCE(NULLIF(ts.domain,''), ts.net::text, ts.value, ''))
		FROM task_scope ts
		WHERE ts.task_id=$1 AND ts.source='manual' AND ts.kind <> 'company'
		ORDER BY 1`, taskID)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var parts []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return "", err
		}
		parts = append(parts, v)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	if len(parts) == 0 {
		return fmt.Sprintf("task:%d", taskID), nil
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\n")))
	return "scope:" + hex.EncodeToString(sum[:8]), nil
}
