package traffic

import (
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/Autumn-27/artex/egress"
)

// A client on a scope relay's credential-free URL (as Chromium uses
// --proxy-server) has the proxy's upstream dials run on the relay's scope, for
// plain HTTP and for an HTTPS tunnel's CONNECT, even when it sends another
// scope's credential through it. A closed relay or a forgotten scope reaches nothing.
func TestScopeRelayCarriesScopeWithoutCredential(t *testing.T) {
	tr, _, proxyURL := startProxy(t)
	spy := &scopeSpy{}
	tr.SetUpstreamDialer(spy.dial)
	target := textServer(t, "ok")

	relayURL, relay, err := tr.StartScopeRelay(tr.ScopedProxyURL("task-r", runScope("task-r")))
	if err != nil || relay == nil {
		t.Fatalf("StartScopeRelay: %v (relay %v)", err, relay)
	}
	t.Cleanup(func() { _ = relay.Close() })
	if u, _ := url.Parse(relayURL); u.User != nil || relayURL == proxyURL || !strings.HasPrefix(relayURL, "http://127.0.0.1:") {
		t.Fatalf("relay URL %q, want a credential-free loopback URL of its own", relayURL)
	}

	other := egress.ScopedProxyURL(relayURL, tr.scopeTokenFor(t, "task-other"))
	for _, u := range []string{relayURL, other} {
		if code, body := getBody(t, proxyClient(t, tr, u), target.URL); code != http.StatusOK || body != "ok" {
			t.Fatalf("via %s: %d %q", u, code, body)
		}
		getBody(t, proxyClient(t, tr, u), "https://"+target.Listener.Addr().String()+"/")
		got := spy.take()
		if len(got) != 2 {
			t.Fatalf("via %s: dials %v, want 2", u, got)
		}
		for _, d := range got {
			if d.run != "task-r" {
				t.Fatalf("via %s: dial %s ran on scope %q, want task-r", u, d.addr, d.run)
			}
		}
	}

	tr.ForgetScope("task-r")
	if code, _ := getBody(t, proxyClient(t, tr, relayURL), target.URL); code == http.StatusOK {
		t.Fatal("a relay of a forgotten scope reached the target")
	}
	_ = relay.Close()
	if code, _ := getBody(t, proxyClient(t, tr, relayURL), target.URL); code == http.StatusOK {
		t.Fatal("a closed relay still forwarded")
	}
	if got := spy.take(); len(got) != 0 {
		t.Fatalf("forgotten/closed relay dialled %v", got)
	}
	// The relay's binding went with its connections: the plain URL is unscoped.
	if code, _ := getBody(t, proxyClient(t, tr, proxyURL), target.URL); code != http.StatusOK {
		t.Fatalf("unscoped client: status %d", code)
	}
	if got := spy.take(); len(got) != 1 || got[0].run != "" {
		t.Fatalf("unscoped client dials %v, want one with no scope", got)
	}
}

// A URL without a scope token needs no relay and keeps the global rules; a URL
// of another proxy is refused, so a caller never hands a client a URL that
// drops its scope; an ended scope's URL gets a relay whose dials are refused.
func TestStartScopeRelayInputs(t *testing.T) {
	tr, _, proxyURL := startProxy(t)
	spy := &scopeSpy{}
	tr.SetUpstreamDialer(spy.dial)
	target := textServer(t, "ok")

	if got, relay, err := tr.StartScopeRelay(proxyURL); err != nil || relay != nil || got != tr.ProxyAddr() {
		t.Fatalf("plain URL: %q %v %v, want ProxyAddr, no relay", got, relay, err)
	}
	for _, bad := range []string{"http://127.0.0.1:1", "::nope", egress.ScopedProxyURL("http://10.0.0.1:8080", egress.ScopeTokenPrefix+"x")} {
		if _, relay, err := tr.StartScopeRelay(bad); err == nil || relay != nil {
			t.Fatalf("StartScopeRelay(%q) accepted", bad)
		}
	}
	endedURL, relay, err := tr.StartScopeRelay(tr.EndedScopeURL())
	if err != nil || relay == nil {
		t.Fatalf("ended scope: %v", err)
	}
	t.Cleanup(func() { _ = relay.Close() })
	if code, _ := getBody(t, proxyClient(t, tr, endedURL), target.URL); code == http.StatusOK {
		t.Fatal("an ended scope's relay reached the target")
	}
	if got := spy.take(); len(got) != 0 {
		t.Fatalf("ended scope's relay dialled %v", got)
	}
}

// scopeTokenFor registers key and returns its token, for a test that sends a
// credential of its own.
func (tr *Traffic) scopeTokenFor(t *testing.T, key string) string {
	t.Helper()
	u, err := url.Parse(tr.ScopedProxyURL(key, runScope(key)))
	if err != nil || u.User == nil {
		t.Fatalf("no token for %s", key)
	}
	pass, _ := u.User.Password()
	return pass
}

// When a relay closes under an open connection, no binding naming a live scope
// is left, and the proxy's disconnect removes whatever is (no leak). The
// tombstone itself (endedRelayToken, refused at the gate) lives only between
// the relay's close and the proxy's disconnect, too brief to observe reliably.
func TestScopeRelayCloseLeavesRefusedBinding(t *testing.T) {
	tr, _, _ := startProxy(t)
	_, relay, err := tr.StartScopeRelay(tr.ScopedProxyURL("task-t", runScope("task-t")))
	if err != nil {
		t.Fatal(err)
	}
	c, err := net.Dial("tcp", strings.TrimPrefix(relay.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	bound := func() (string, bool) {
		var key, tok string
		tr.sink.relayed.Range(func(k, v any) bool { key, tok = k.(string), v.(string); return false })
		return tok, key != ""
	}
	waitFor(t, "the relay binding", func() bool { tok, ok := bound(); return ok && tok != endedRelayToken })
	_ = relay.Close()
	waitFor(t, "the ended binding", func() bool { tok, ok := bound(); return !ok || tok == endedRelayToken })
	if tok, ok := bound(); ok {
		if _, known := tr.scopes.lookup(tok); known {
			t.Fatalf("ended binding %q names an active scope", tok)
		}
	}
	waitFor(t, "the binding to go", func() bool { _, ok := bound(); return !ok })
}
