package traffic

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// A self-signed target is captured when the trust policy names it, with no
// ARTEX_CAPTURE_INSECURE_UPSTREAM, and refused when it does not.
func TestUpstreamTrustPerHost(t *testing.T) {
	t.Setenv("ARTEX_CAPTURE_INSECURE_UPSTREAM", "")
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "in scope")
	}))
	defer target.Close()
	tr, _, proxyURL := startProxy(t)
	c := proxyClient(t, tr, proxyURL)
	// The client names the target example.com (a SAN of the httptest
	// certificate), so the proxy mints its certificate for that name and checks
	// the upstream one against it.
	c.Transport.(*http.Transport).TLSClientConfig.ServerName = "example.com"

	get := func() (int, error) {
		res, err := c.Get(target.URL + "/")
		if err != nil {
			return 0, err
		}
		defer res.Body.Close()
		_, _ = io.Copy(io.Discard, res.Body)
		return res.StatusCode, nil
	}

	if code, err := get(); err == nil && code == http.StatusOK {
		t.Fatal("a self-signed target outside the trust policy must not be captured")
	}
	tr.SetUpstreamTrust(func(host string) bool { return host == "127.0.0.1" })
	if code, err := get(); err == nil && code == http.StatusOK {
		t.Fatal("a dialled address in scope must not exempt an out-of-scope SNI")
	}
	var asked []string
	tr.SetUpstreamTrust(func(host string) bool {
		asked = append(asked, host)
		return host == "example.com" || host == "127.0.0.1"
	})
	if code, err := get(); err != nil || code != http.StatusOK {
		t.Fatalf("in-scope self-signed target: code %d, err %v (asked %v)", code, err, asked)
	}
	// SNI in scope, dialled address not: no trust (both must be exempt).
	c.CloseIdleConnections()
	tr.SetUpstreamTrust(func(host string) bool { return host == "example.com" })
	if code, err := get(); err == nil && code == http.StatusOK {
		t.Fatal("an in-scope SNI must not exempt an out-of-scope dialled address")
	}
	tr.SetUpstreamTrust(nil)
	c.CloseIdleConnections()
	if code, err := get(); err == nil && code == http.StatusOK {
		t.Fatal("clearing the policy must verify again")
	}
}

// Registries and code hosts are verified whatever the policy or the global
// opt-out says.
func TestProtectedUpstreamHostsAlwaysVerified(t *testing.T) {
	tr := &Traffic{}
	tr.SetUpstreamTrust(func(string) bool { return true })
	t.Setenv("ARTEX_CAPTURE_INSECURE_UPSTREAM", "1")
	for _, h := range []string{"pypi.org", "files.pythonhosted.org", "github.com", "codeload.github.com",
		"objects.githubusercontent.com", "registry.npmjs.org", "proxy.golang.org", "deb.debian.org", "GitHub.com."} {
		if tr.upstreamInsecure(h) {
			t.Errorf("%s must stay verified", h)
		}
	}
	for _, h := range []string{"target.example", "github.com.attacker.net", "notgithub.com", "10.0.0.5"} {
		if !tr.upstreamInsecure(h) {
			t.Errorf("%s: the policy should apply", h)
		}
	}
	if tr.upstreamInsecure("") {
		t.Error("an empty host is never exempt")
	}
	t.Setenv("ARTEX_CAPTURE_INSECURE_UPSTREAM", "")
	tr.SetUpstreamTrust(nil)
	if tr.upstreamInsecure("target.example") {
		t.Error("no policy and no opt-out must verify")
	}
}

// An IP target reached without SNI is checked against the dialled address.
func TestUpstreamTrustIPWithoutSNI(t *testing.T) {
	t.Setenv("ARTEX_CAPTURE_INSECURE_UPSTREAM", "")
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	defer target.Close()
	tr, _, proxyURL := startProxy(t)
	c := proxyClient(t, tr, proxyURL)
	// go-mitmproxy mints its certificate for an IP target without an IP SAN, so
	// this client does not check the proxy's side; the upstream side is the test.
	c.Transport.(*http.Transport).TLSClientConfig.InsecureSkipVerify = true
	var asked []string
	tr.SetUpstreamTrust(func(host string) bool { asked = append(asked, host); return host == "127.0.0.1" })
	res, err := c.Get(target.URL + "/")
	if err != nil {
		t.Fatalf("in-scope IP target: %v (asked %v)", err, asked)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("in-scope IP target: status %d (asked %v)", res.StatusCode, asked)
	}
}
