package server

import (
	"crypto/sha256"
	"encoding/base64"
	"strings"
	"testing"
)

func hashOf(s string) string {
	sum := sha256.Sum256([]byte(s))
	return "sha256-" + base64.StdEncoding.EncodeToString(sum[:])
}

func TestConsoleCSPDirectives(t *testing.T) {
	csp := consoleCSP([]string{"sha256-AAAA"})
	for _, want := range []string{
		"default-src 'self'",
		"script-src 'self' 'sha256-AAAA'",
		"img-src 'self' data: blob:",
		"connect-src 'self'",
		"object-src 'none'",
		"base-uri 'self'",
		"form-action 'self'",
		"frame-ancestors 'self'",
	} {
		if !strings.Contains(csp, want) {
			t.Errorf("CSP %q lacks %q", csp, want)
		}
	}
	// Scripts must never be allowed wholesale.
	for _, bad := range []string{"script-src 'self' 'unsafe-inline'", "'unsafe-eval'", "script-src *"} {
		if strings.Contains(csp, bad) {
			t.Errorf("CSP %q contains %q", csp, bad)
		}
	}
	if !strings.Contains(consoleCSP(nil), "script-src 'self';") {
		t.Errorf("baseline CSP should allow only same-origin scripts: %q", consoleCSP(nil))
	}
}

func TestInlineScriptHashes(t *testing.T) {
	boot := `(self.__next_f=self.__next_f||[]).push([0])`
	data := `{"a":1}`
	html := `<html><head><script src="/_next/a.js" async=""></script>` +
		`<script id="_R_">` + boot + `</script>` +
		`<script type="application/json" id="d">` + data + `</script>` +
		`<script type="module">import("/x.js")</script>` +
		`<SCRIPT>` + boot + `</SCRIPT></head></html>`
	got := inlineScriptHashes([]byte(html))
	want := []string{hashOf(boot), hashOf(`import("/x.js")`)}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("hashes = %v, want %v", got, want)
	}
	if !strings.Contains(cspForHTML([]byte(html)), "'"+want[0]+"'") {
		t.Errorf("cspForHTML omits the inline script hash")
	}
}
