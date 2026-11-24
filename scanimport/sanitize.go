package scanimport

import (
	"crypto/sha256"
	"encoding/hex"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

var (
	tokenRe = regexp.MustCompile(`^[A-Za-z0-9_.>-]{1,128}$`)
	hostRe  = regexp.MustCompile(`^[a-z0-9_.-]{1,253}$`)
	// credRe matches the "scheme://userinfo@" part of a URL anywhere in text;
	// the authority run is greedy so the last "@" before a path delimiter wins.
	credRe = regexp.MustCompile(`(?i)\b([a-z][a-z0-9+.-]*://)[^\s/?#"'<>]*@`)
	cweRe  = regexp.MustCompile(`(?i)\bCWE-(\d{1,6})\b`)
)

// Token returns s when it is a plain identifier ([A-Za-z0-9_.>-], 1..128
// bytes), "" otherwise.
func Token(s string) string {
	if tokenRe.MatchString(s) {
		return s
	}
	return ""
}

// Host returns h as a lower-case IP or hostname, "" when it is neither.
func Host(h string) string {
	h = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(h)), ".")
	if ip := net.ParseIP(h); ip != nil {
		return ip.String()
	}
	if hostRe.MatchString(h) {
		return h
	}
	return ""
}

// URL returns raw re-serialized without userinfo or fragment (host lower-cased),
// and its validated host, when it is an http(s) URL with a valid host; ""
// otherwise.
func URL(raw string) (u, host string) {
	p, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (p.Scheme != "http" && p.Scheme != "https") {
		return "", ""
	}
	h := Host(p.Hostname())
	if h == "" {
		return "", ""
	}
	p.User = nil // credentials never reach an asset or a finding
	p.Fragment, p.RawFragment = "", ""
	p.Host = joinHost(h, p.Port())
	return p.String(), h
}

// Origin returns scheme://host[:port] of an http(s) URL, "" when it is not one.
func Origin(raw string) string {
	u, h := URL(raw)
	if u == "" {
		return ""
	}
	p, err := url.Parse(u)
	if err != nil {
		return ""
	}
	return p.Scheme + "://" + joinHost(h, p.Port())
}

func joinHost(h, port string) string {
	if port != "" {
		return net.JoinHostPort(h, port)
	}
	if strings.Contains(h, ":") {
		return "[" + h + "]"
	}
	return h
}

// StripUserinfo removes the userinfo (user:pass@) from raw, a URL or a bare
// host[:port] as a scanner prints it, keeping every other byte. Unlike URL it
// never rejects: a value with no userinfo is returned unchanged.
func StripUserinfo(raw string) string {
	start := 0 // the authority follows "scheme://" or "//"; a bare value is all authority
	if i := strings.Index(raw, "//"); i == 0 || i > 0 && raw[i-1] == ':' && !strings.ContainsAny(raw[:i-1], "/?#@") {
		start = i + 2
	} else if strings.ContainsAny(raw, "/?#") {
		return raw
	}
	auth := raw[start:]
	if j := strings.IndexAny(auth, "/?#"); j >= 0 {
		auth = auth[:j]
	}
	if at := strings.LastIndex(auth, "@"); at >= 0 {
		return raw[:start] + raw[start+at+1:]
	}
	return raw
}

// ScrubCreds removes "user:pass@" from every URL-looking authority in free
// text.
func ScrubCreds(s string) string {
	if !strings.Contains(s, "@") {
		return s
	}
	return credRe.ReplaceAllString(s, "$1")
}

// clip cuts s to at most n bytes on a rune boundary.
func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	s = s[:n]
	for !utf8.ValidString(s) && len(s) > 0 {
		s = s[:len(s)-1]
	}
	return s
}

// cleanText makes untrusted free text safe to store: valid UTF-8, control
// characters replaced by spaces, credentials scrubbed, clipped to n bytes.
func cleanText(s string, n int) string {
	s = scrubText(clip(s, n*2))
	s = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || r == utf8.RuneError {
			return ' '
		}
		return r
	}, s)
	return strings.TrimSpace(clip(s, n))
}

// scrubText clips nothing but makes s valid UTF-8 and scrubs credentials.
func scrubText(s string) string {
	return ScrubCreds(strings.ToValidUTF8(s, "�"))
}

// toolName turns a free driver/scanner name into a lower-case token.
func toolName(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(clip(s, 128)) {
		if r < 0x80 && (r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_' || r == '.') {
			b.WriteRune(r)
			dash = false
		} else if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
		}
	}
	return Token(strings.Trim(b.String(), "-"))
}

var versionRe = regexp.MustCompile(`^[A-Za-z0-9_.+-]{1,64}$`)

func versionToken(s string) string {
	s = strings.TrimSpace(s)
	if versionRe.MatchString(s) {
		return s
	}
	return ""
}

// targetOf validates a scanner target: an http(s) URL, host:port or a bare
// host. It returns the sanitized target and its host, "" when invalid.
func targetOf(s string) (target, host string) {
	s = strings.TrimSpace(s)
	if u, h := URL(s); u != "" {
		return u, h
	}
	if h, p, err := net.SplitHostPort(s); err == nil {
		if n, err := strconv.Atoi(p); err == nil && n > 0 && n <= 65535 && Host(h) != "" {
			return joinHost(Host(h), p), Host(h)
		}
		return "", ""
	}
	if h := Host(s); h != "" {
		return h, h
	}
	return "", ""
}

// parseCWE extracts CWE numbers ("CWE-79") from text, deduplicated, in order
// of first appearance.
func parseCWE(texts ...string) []int {
	var out []int
	seen := map[int]bool{}
	for _, t := range texts {
		for _, m := range cweRe.FindAllStringSubmatch(clip(t, 8192), -1) {
			if n, err := strconv.Atoi(m[1]); err == nil && n > 0 && !seen[n] {
				seen[n] = true
				out = append(out, n)
			}
		}
	}
	return out
}

// normSeverity maps a scanner severity word to critical|high|medium|low, ""
// for informational or unknown.
func normSeverity(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "critical":
		return "critical"
	case "high":
		return "high"
	case "medium", "moderate":
		return "medium"
	case "low":
		return "low"
	}
	return ""
}

func normTarget(t string) string {
	if u, _ := URL(t); u != "" {
		return u
	}
	return strings.ToLower(strings.TrimSpace(t))
}

func makeKey(tool, ruleID, target, location string, line int) string {
	h := sha256.Sum256([]byte(tool + "\x1f" + ruleID + "\x1f" + normTarget(target) + "\x1f" + location + "\x1f" + strconv.Itoa(line)))
	return "imp-" + hex.EncodeToString(h[:])[:24]
}
