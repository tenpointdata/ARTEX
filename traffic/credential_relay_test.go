package traffic

import (
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/Autumn-27/artex/egress"
)

// A worker node's browser cannot send the scope token to the control node's
// proxy (Chromium drops --proxy-server's userinfo). A credential relay on the
// worker carries it: a client with no credential, through the relay, reaches
// the target on the token's scope, over plain HTTP (keep-alive) and an HTTPS
// CONNECT tunnel; the client's own Proxy-Authorization is replaced; a token
// that names no scope stays refused. The proxy sees a non-loopback client, as
// it does a worker host.
func TestCredentialRelayCarriesRemoteScope(t *testing.T) {
	t.Setenv("ARTEX_CAPTURE_INSECURE_UPSTREAM", "1") // the TLS target's certificate is self-signed
	tr, _, proxyURL := startProxy(t)
	spy := &scopeSpy{}
	tr.SetUpstreamDialer(spy.dial)
	tr.SetRemoteScopeResolver(newFakeRemote(map[string]string{workerToken(3): "77"}).resolve)
	treatAllRemote(tr)
	plain := textServer(t, "plain")
	secure := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "secure") }))
	t.Cleanup(secure.Close)
	secureURL := strings.Replace(secure.URL, "127.0.0.1", "localhost", 1) // the MITM certificate names hosts, not IPs

	relay, err := StartCredentialRelay(egress.ScopedProxyURL(proxyURL, workerToken(3)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = relay.Close() })
	if strings.Contains(relay.URL, "@") || !strings.HasPrefix(relay.URL, "http://127.0.0.1:") {
		t.Fatalf("relay URL %q: want a credential-free loopback URL", relay.URL)
	}
	c := proxyClient(t, tr, relay.URL)
	for range 2 { // the second request reuses the kept-alive relay connection
		if code, body := getBody(t, c, plain.URL); code != http.StatusOK || body != "plain" {
			t.Fatalf("plain HTTP via the relay: %d %q", code, body)
		}
	}
	if code, body := getBody(t, c, secureURL); code != http.StatusOK || body != "secure" {
		t.Fatalf("HTTPS via the relay: %d %q", code, body)
	}
	got := spy.take()
	if len(got) == 0 {
		t.Fatal("no upstream dial")
	}
	for _, d := range got {
		if d.run != "77" {
			t.Fatalf("dial %s ran on scope %q, want 77", d.addr, d.run)
		}
	}

	// The client's own credential (here an unresolved token, which the proxy
	// would refuse) is replaced by the relay's. A fresh target forces a dial.
	other := textServer(t, "other")
	fu, _ := url.Parse(egress.ScopedProxyURL(proxyURL, workerToken(4)))
	fpass, _ := fu.User.Password()
	forged, _ := http.NewRequest(http.MethodGet, other.URL, nil)
	forged.Header.Set("Proxy-Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(fu.User.Username()+":"+fpass)))
	res, err := proxyClient(t, tr, relay.URL).Do(forged)
	if err != nil || res.StatusCode != http.StatusOK {
		t.Fatalf("a client credential was not replaced: %v %v", res, err)
	}
	res.Body.Close()
	if got := spy.take(); len(got) != 1 || got[0].run != "77" {
		t.Fatalf("dials %v, want one on 77", got)
	}

	ended, err := StartCredentialRelay(egress.ScopedProxyURL(proxyURL, workerToken(4)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ended.Close() })
	if code, body := getBody(t, proxyClient(t, tr, ended.URL), plain.URL); code == http.StatusOK {
		t.Fatalf("an unresolved token reached the target through a relay: %q", body)
	}
	getBody(t, proxyClient(t, tr, ended.URL), secureURL)
	if got := spy.take(); len(got) != 0 {
		t.Fatalf("an unresolved token dialled %v", got)
	}

	// Closing the relay stops it.
	_ = relay.Close()
	if code, _ := getBody(t, proxyClient(t, tr, relay.URL), plain.URL); code == http.StatusOK {
		t.Fatal("a closed relay still served")
	}
}

// A URL that cannot carry a scope gets no relay: no credential, no host, or
// a scheme other than http(s).
func TestCredentialRelayRefusesURLsWithoutCredential(t *testing.T) {
	for _, u := range []string{"http://127.0.0.1:8788", "http://user@127.0.0.1:8788", "socks5://a:b@127.0.0.1:1080", "::bad", "http://a:b@"} {
		if r, err := StartCredentialRelay(u); err == nil {
			_ = r.Close()
			t.Fatalf("StartCredentialRelay(%q): want an error", u)
		}
	}
}

// A real Chrome (ARTEX_TEST_CHROME; skipped otherwise), as a worker node's
// Playwright MCP launches it: through a credential relay to a proxy that sees
// it as a remote client, its dial runs on the token's scope; through a relay
// of a token that names no scope, it is refused.
func TestCredentialRelayRealChrome(t *testing.T) {
	chrome := os.Getenv("ARTEX_TEST_CHROME")
	if chrome == "" {
		t.Skip("ARTEX_TEST_CHROME not set")
	}
	tr, _, proxyURL := startProxy(t)
	spy := &scopeSpy{}
	tr.SetUpstreamDialer(spy.dial)
	tr.SetRemoteScopeResolver(newFakeRemote(map[string]string{workerToken(5): "55"}).resolve)
	treatAllRemote(tr)
	target := textServer(t, "worker-relay-marker")

	relay, err := StartCredentialRelay(egress.ScopedProxyURL(proxyURL, workerToken(5)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = relay.Close() })
	if dom := chromeDump(t, chrome, relay.URL, target.URL+"/relay"); !strings.Contains(dom, "worker-relay-marker") {
		t.Fatalf("Chrome through the credential relay did not load the page: %q", dom)
	}
	if by := dialsByScope(spy.take()); by["55"] == 0 || len(by) != 1 {
		t.Fatalf("Chrome through the credential relay dialled %v, want only scope 55", by)
	}

	ended, err := StartCredentialRelay(egress.ScopedProxyURL(proxyURL, workerToken(6)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ended.Close() })
	if dom := chromeDump(t, chrome, ended.URL, target.URL+"/ended"); strings.Contains(dom, "worker-relay-marker") {
		t.Fatal("Chrome reached the target through a relay of an unresolved token")
	}
	if got := spy.take(); len(got) != 0 {
		t.Fatalf("unresolved token dialled %v", got)
	}
}
