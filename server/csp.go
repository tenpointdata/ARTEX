package server

import (
	"crypto/sha256"
	"encoding/base64"
	"regexp"
	"strings"
	"sync"
)

// Content-Security-Policy for the operator console and the JSON API.
//
// The console is a static Next.js export. Every exported document carries inline
// bootstrap <script> blocks (the React Server Components payload,
// `self.__next_f.push(...)`), and the payload differs per page. Rather than
// allow 'unsafe-inline' for scripts -- which would make the policy useless
// against injected markup -- cspForHTML hashes the inline scripts of the exact
// document being served and lists those hashes in script-src. Everything else
// the console runs is a same-origin file under /_next/static/.
//
// Styles are the one place 'unsafe-inline' stays: the UI libraries (toasts, the
// graph canvases, the popover primitives) inject <style> elements at runtime,
// which a CSP can only permit with 'unsafe-inline' or a per-response nonce, and
// a static export has no per-response render step to put a nonce in. Style
// injection cannot run script, and form-action/img-src/connect-src below close
// the data-exfiltration channels it would otherwise offer.

// apiCSP is for JSON API replies: they are never rendered, so allow nothing and
// forbid framing outright.
const apiCSP = "default-src 'none'; frame-ancestors 'none'"

// consoleCSPBase is the document policy minus the script hashes. script-src is
// appended by consoleCSP so an HTML page lists exactly its own inline scripts.
//
//	img-src data: blob:   screenshots and evidence previews are inlined
//	connect-src 'self'    the /api calls and the SSE streams are same origin
//	worker-src blob:      graph layout workers are created from blob URLs
//	object-src 'none'     no plugins; base-uri/form-action pin navigation
const consoleCSPBase = "default-src 'self'; " +
	"style-src 'self' 'unsafe-inline'; " +
	"img-src 'self' data: blob:; " +
	"font-src 'self' data:; " +
	"connect-src 'self'; " +
	"worker-src 'self' blob:; " +
	"object-src 'none'; " +
	"base-uri 'self'; " +
	"form-action 'self'; " +
	"frame-ancestors 'self'"

// consoleCSP returns the console policy permitting the given inline-script
// hashes ("sha256-<base64>" tokens) in addition to same-origin scripts.
func consoleCSP(scriptHashes []string) string {
	src := "script-src 'self'"
	for _, h := range scriptHashes {
		src += " '" + h + "'"
	}
	return src + "; " + consoleCSPBase
}

// inlineScript matches <script ...>body</script>; the attribute group lets the
// caller skip external scripts (src=...). Go's RE2 has no backreferences or
// lookarounds, which is fine: Next's output is machine-generated and the tag
// text never contains a literal "</script>" inside a body (the serializer
// escapes "<").
var inlineScript = regexp.MustCompile(`(?is)<script([^>]*)>(.*?)</script>`)

// inlineScriptHashes returns the CSP source token for every executable inline
// <script> in an HTML document, de-duplicated and in document order. Scripts
// with a src attribute are covered by 'self'; data blocks (a non-JS type, e.g.
// application/json) do not execute and need no hash.
func inlineScriptHashes(html []byte) []string {
	var out []string
	seen := map[string]bool{}
	for _, m := range inlineScript.FindAllSubmatch(html, -1) {
		attrs := strings.ToLower(string(m[1]))
		if strings.Contains(attrs, "src=") || isDataScript(attrs) {
			continue
		}
		sum := sha256.Sum256(m[2])
		tok := "sha256-" + base64.StdEncoding.EncodeToString(sum[:])
		if !seen[tok] {
			seen[tok] = true
			out = append(out, tok)
		}
	}
	return out
}

// isDataScript reports whether a script's attributes declare a non-executing
// type. An absent type, "module" and the JavaScript MIME types execute.
func isDataScript(attrs string) bool {
	i := strings.Index(attrs, "type=")
	if i < 0 {
		return false
	}
	v := strings.Trim(strings.TrimSpace(attrs[i+len("type="):]), `"'`)
	for _, js := range []string{"module", "text/javascript", "application/javascript", "text/ecmascript", "application/ecmascript"} {
		if strings.HasPrefix(v, js) {
			return false
		}
	}
	return true
}

// cspForHTML builds the console policy for one served HTML document.
func cspForHTML(html []byte) string {
	return consoleCSP(inlineScriptHashes(html))
}

// docCSP memoizes cspForHTML per embedded document: the embedded tree is fixed
// for the life of the process, so each page is parsed and hashed once.
var docCSP sync.Map

// documentCSP is cspForHTML keyed by the document's path in the embedded tree.
func documentCSP(name string, html []byte) string {
	if v, ok := docCSP.Load(name); ok {
		return v.(string)
	}
	v, _ := docCSP.LoadOrStore(name, cspForHTML(html))
	return v.(string)
}
