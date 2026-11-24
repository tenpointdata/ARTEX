package egress

import (
	"encoding/base64"
	"testing"
)

func basic(userinfo string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(userinfo))
}

// A scoped URL marked for pacing keeps its scope token; the proxy reads both
// markers back from the Proxy-Authorization the client sends.
func TestScopedProxyURLAndPaceMarker(t *testing.T) {
	const tok = ScopeTokenPrefix + "abc"
	scoped := ScopedProxyURL("http://127.0.0.1:8080", tok)
	if want := "http://" + ScopeUser + ":" + tok + "@127.0.0.1:8080"; scoped != want {
		t.Fatalf("ScopedProxyURL = %q, want %q", scoped, want)
	}
	if got, want := MarkProxyURL(scoped), "http://"+PaceUser+":"+tok+"@127.0.0.1:8080"; got != want {
		t.Fatalf("MarkProxyURL(scoped) = %q, want %q", got, want)
	}
	if got := MarkProxyURL("http://op:secret@proxy:3128"); got != "http://op:secret@proxy:3128" {
		t.Fatalf("an operator credential was replaced: %q", got)
	}
	if got := WithoutProxyCredential(scoped); got != "http://127.0.0.1:8080" {
		t.Fatalf("WithoutProxyCredential = %q", got)
	}
	user, pass, ok := ProxyCredential(basic(PaceUser + ":" + tok))
	if !ok || user != PaceUser || pass != tok || !IsPaceMarker(basic(PaceUser+":"+tok)) {
		t.Fatalf("ProxyCredential = %q %q %v", user, pass, ok)
	}
	if got, ok := ScopeToken(pass); !ok || got != tok {
		t.Fatalf("ScopeToken(%q) = %q %v", pass, got, ok)
	}
	for _, p := range []string{"", PaceUser, ScopeTokenPrefix, "secret"} {
		if _, ok := ScopeToken(p); ok {
			t.Fatalf("ScopeToken(%q) named a scope", p)
		}
	}
	if _, _, ok := ProxyCredential("Bearer x"); ok {
		t.Fatal("a non-Basic value decoded")
	}
}
