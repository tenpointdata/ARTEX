package traffic

import (
	"bufio"
	"bytes"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Autumn-27/artex/egress"
)

func TestParseProxyAuth(t *testing.T) {
	for in, want := range map[string]ProxyCreds{
		"user:pw":      {"user", "pw"},
		"user:p:w:x":   {"user", "p:w:x"}, // split at the first colon
		"u:pass word":  {"u", "pass word"},
		"admin:s3cr3t": {"admin", "s3cr3t"},
	} {
		if got, err := ParseProxyAuth(in); err != nil || got != want {
			t.Errorf("%q: got %+v, %v want %+v", in, got, err, want)
		}
	}
	for _, in := range []string{"", "hunter2", ":hunter2", "hunter2:", ":"} {
		_, err := ParseProxyAuth(in)
		if err == nil {
			t.Errorf("%q should be rejected", in)
		} else if strings.Contains(err.Error(), "hunter2") {
			t.Errorf("error %q echoes the value", err)
		}
	}
}

func TestOpenRejectsMalformedProxyAuth(t *testing.T) {
	for _, bad := range []string{"nocolon", ":pw", "user:"} {
		t.Setenv(ProxyAuthEnv, bad)
		tr, err := Open(t.TempDir(), "127.0.0.1:0")
		if err == nil {
			tr.Close()
			t.Fatalf("%q: Open should fail at start-up", bad)
		}
		if !strings.Contains(err.Error(), ProxyAuthEnv) || !strings.Contains(err.Error(), "user:password") {
			t.Errorf("%q: unclear error %q", bad, err)
		}
	}
}

func TestProxyAddrCarriesCredentialsOnlyWhenAuthIsSet(t *testing.T) {
	t.Setenv(ProxyAuthEnv, "")
	plain, err := Open(t.TempDir(), ":0")
	if err != nil {
		t.Fatal(err)
	}
	defer plain.Close()
	if got := plain.ProxyAddr(); strings.Contains(got, "@") || got != plain.ProxyDisplayAddr() {
		t.Errorf("no auth: ProxyAddr %q / display %q", got, plain.ProxyDisplayAddr())
	}

	t.Setenv(ProxyAuthEnv, "op:p@ss/w:rd")
	tr, err := Open(t.TempDir(), "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer tr.Close()
	u, err := url.Parse(tr.ProxyAddr())
	if err != nil {
		t.Fatal(err)
	}
	if pw, _ := u.User.Password(); u.User.Username() != "op" || pw != "p@ss/w:rd" || u.Host != "127.0.0.1:0" {
		t.Errorf("ProxyAddr %q does not round-trip the credentials", tr.ProxyAddr())
	}
	if d := tr.ProxyDisplayAddr(); d != "http://127.0.0.1:0" || strings.Contains(d, "ss") {
		t.Errorf("display address %q must not carry credentials", d)
	}
}

// proxyViaURL is a client that reaches the proxy as userinfo says (empty = no
// Proxy-Authorization at all). It does not follow redirects and trusts the CA.
func proxyViaURL(t *testing.T, tr *Traffic, user, pass string) *http.Client {
	t.Helper()
	base, err := url.Parse(tr.ProxyDisplayAddr())
	if err != nil {
		t.Fatal(err)
	}
	if user != "" {
		base.User = url.UserPassword(user, pass)
	}
	return proxyClient(t, tr, base.String())
}

func TestProxyRequiresBasicAuth(t *testing.T) {
	t.Setenv(ProxyAuthEnv, "operator:s3cr3t")
	var mu sync.Mutex
	var seenAuth []string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seenAuth = append(seenAuth, r.Header.Get("Proxy-Authorization"))
		mu.Unlock()
		_, _ = io.WriteString(w, "target-ok")
	})
	plain := httptest.NewServer(handler)
	defer plain.Close()
	tlsSrv := httptest.NewTLSServer(handler)
	defer tlsSrv.Close()
	// The MITM leaf for a bare IP has no IP SAN: address the TLS target by name.
	tlsURL := strings.Replace(tlsSrv.URL, "127.0.0.1", "localhost", 1)

	var logs bytes.Buffer
	log.SetOutput(io.MultiWriter(&logs, os.Stderr))
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	tr, _, _ := startProxy(t)

	type result struct {
		status int
		body   string
		err    error
	}
	do := func(c *http.Client, target string) result {
		resp, err := c.Get(target)
		if err != nil {
			return result{err: err}
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return result{status: resp.StatusCode, body: string(b)}
	}

	for _, tc := range []struct {
		name       string
		user, pass string
		ok         bool
	}{
		{"no credentials", "", "", false},
		{"wrong password", "operator", "nope", false},
		{"wrong user", "intruder", "s3cr3t", false},
		{"prefix of the password", "operator", "s3cr3", false},
		{"right credentials", "operator", "s3cr3t", true},
		{"pace marker with the right password", egress.PaceUser, "s3cr3t", true},
		{"pace marker with the default marker password", egress.PaceUser, egress.PaceUser, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := proxyViaURL(t, tr, tc.user, tc.pass)

			// Plain HTTP: absolute-form request straight to the proxy.
			r := do(c, plain.URL+"/p")
			if tc.ok && (r.err != nil || r.status != 200 || r.body != "target-ok") {
				t.Errorf("plain: %+v, want proxied", r)
			}
			if !tc.ok && (r.err != nil || r.status != http.StatusProxyAuthRequired) {
				t.Errorf("plain: %+v, want 407", r)
			}
			// HTTPS: the CONNECT itself is authenticated.
			r = do(c, tlsURL+"/s")
			if tc.ok && (r.err != nil || r.status != 200 || r.body != "target-ok") {
				t.Errorf("connect: %+v, want proxied", r)
			}
			if !tc.ok && (r.err == nil || !strings.Contains(r.err.Error(), "Proxy Authentication Required")) {
				t.Errorf("connect: %+v, want a 407 refusal", r)
			}
		})
	}

	// The 407 carries the challenge, and nothing that was refused reached a target.
	resp, err := proxyViaURL(t, tr, "", "").Get(plain.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if ch := resp.Header.Get("Proxy-Authenticate"); !strings.HasPrefix(ch, "Basic") {
		t.Errorf("Proxy-Authenticate = %q", ch)
	}
	mu.Lock()
	defer mu.Unlock()
	// 2 allowed cases x (plain + tls) = 4 target hits; the credentials never went on.
	if len(seenAuth) != 4 {
		t.Errorf("target saw %d requests, want 4 (refused ones must not be forwarded)", len(seenAuth))
	}
	for _, a := range seenAuth {
		if a != "" {
			t.Errorf("Proxy-Authorization %q reached the target", a)
		}
	}
	if strings.Contains(logs.String(), "s3cr3t") {
		t.Error("the proxy password was logged")
	}
}

// Raw wire check: a bare CONNECT and a bare absolute-form request without the
// header both get 407 before anything is dialed.
func TestProxyRawRequestsWithoutCredentialsGet407(t *testing.T) {
	t.Setenv(ProxyAuthEnv, "operator:s3cr3t")
	tr, _, _ := startProxy(t)
	addr := strings.TrimPrefix(tr.ProxyDisplayAddr(), "http://")
	for name, req := range map[string]string{
		"connect": "CONNECT example.invalid:443 HTTP/1.1\r\nHost: example.invalid:443\r\n\r\n",
		"plain":   "GET http://example.invalid/ HTTP/1.1\r\nHost: example.invalid\r\nProxy-Authorization: Bearer abc\r\n\r\n",
		"garbage": "GET http://example.invalid/ HTTP/1.1\r\nHost: example.invalid\r\nProxy-Authorization: Basic !!!\r\n\r\n",
	} {
		c, err := net.DialTimeout("tcp", addr, 3*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		_ = c.SetDeadline(time.Now().Add(5 * time.Second))
		_, _ = io.WriteString(c, req)
		resp, err := http.ReadResponse(bufio.NewReader(c), nil)
		c.Close()
		if err != nil || resp.StatusCode != http.StatusProxyAuthRequired {
			t.Errorf("%s: %v %v, want 407", name, resp, err)
		}
	}
}

func TestSOCKS5RequiresCredentialsWhenAuthIsSet(t *testing.T) {
	t.Setenv(ProxyAuthEnv, "operator:s3cr3t")
	tr, err := Open(t.TempDir(), "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer tr.Close()
	addr, err := tr.StartSOCKS5(SOCKS5Config{Addr: "127.0.0.1:0"})
	if err != nil {
		t.Fatal(err)
	}

	greet := func(methods ...byte) (net.Conn, [2]byte) {
		c, err := net.DialTimeout("tcp", addr, 3*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		_ = c.SetDeadline(time.Now().Add(5 * time.Second))
		_, _ = c.Write(append([]byte{socks5Version, byte(len(methods))}, methods...))
		var sel [2]byte
		_, _ = io.ReadFull(c, sel[:])
		return c, sel
	}
	userPass := func(c net.Conn, user, pass string) byte {
		msg := append([]byte{socks5UserPassVer, byte(len(user))}, user...)
		msg = append(append(msg, byte(len(pass))), pass...)
		_, _ = c.Write(msg)
		var rep [2]byte
		_, _ = io.ReadFull(c, rep[:])
		return rep[1]
	}

	// No-auth is refused even from loopback.
	c, sel := greet(socks5AuthNone)
	c.Close()
	if sel != [2]byte{socks5Version, socks5AuthNoAccept} {
		t.Errorf("no-auth greeting answered %v, want [5 255]", sel)
	}
	for name, tc := range map[string]struct {
		user, pass string
		want       byte
	}{
		"right":                  {"operator", "s3cr3t", 0x00},
		"pace marker + password": {egress.PaceUser, "s3cr3t", 0x00},
		"wrong password":         {"operator", "nope", 0x01},
		"wrong user":             {"x", "s3cr3t", 0x01},
		"empty":                  {"", "", 0x01},
		"default pace marker":    {egress.PaceUser, egress.PaceUser, 0x01},
	} {
		c, sel := greet(socks5AuthNone, socks5AuthUserPass)
		if sel != [2]byte{socks5Version, socks5AuthUserPass} {
			t.Fatalf("%s: method selection %v", name, sel)
		}
		if got := userPass(c, tc.user, tc.pass); got != tc.want {
			t.Errorf("%s: auth status 0x%02x, want 0x%02x", name, got, tc.want)
		}
		c.Close()
	}
}

func TestSOCKS5RefusesNonLoopbackBindWithoutAuthOrOptOut(t *testing.T) {
	t.Setenv(ProxyAuthEnv, "")
	t.Setenv("ARTEX_PROXY_ALLOW_NONLOOPBACK", "")
	tr, err := Open(t.TempDir(), "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer tr.Close()
	if addr, err := tr.StartSOCKS5(SOCKS5Config{Addr: "0.0.0.0:0"}); err == nil {
		t.Errorf("SOCKS5 bound %s on a non-loopback address with no authentication", addr)
	}
}
