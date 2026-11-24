package server

import (
	"crypto/tls"
	"crypto/x509"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	pgdb "github.com/Autumn-27/artex/db"
	"github.com/Autumn-27/artex/traffic"
)

// The whole path the card c-8d54d5 asks for, against a real database: a
// task's allow rules in Postgres -> the server's trust policy -> the capture
// proxy -> a self-signed upstream. No ARTEX_CAPTURE_INSECURE_UPSTREAM.
func TestUpstreamTLSScopeEndToEndPG(t *testing.T) {
	t.Setenv("ARTEX_CAPTURE_INSECURE_UPSTREAM", "")
	dsn, _, err := pgdb.DSN()
	if err != nil {
		pgdb.SkipPGUnavailable(t, err)
	}
	d, err := pgdb.Open(dsn)
	if err != nil {
		pgdb.SkipPGUnavailable(t, err)
	}
	t.Cleanup(func() { d.Close() })
	task, err := d.CreateTask("upstream tls e2e", "goal", nil, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.DeleteTask(task.ID) })

	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "in scope")
	}))
	defer target.Close()

	tr, proxyURL := startServerTestProxy(t)
	now := time.Unix(1000, 0)
	scope := newUpstreamTLSScope(d.Assets())
	scope.now = func() time.Time { return now }
	tr.SetUpstreamTrust(scope.trusts)
	refresh := func() { now = now.Add(upstreamTLSScopeTTL) }

	// get requests the target under the name sni (a SAN of the httptest
	// certificate is example.com), on a fresh client connection each time.
	get := func(sni string) error {
		c := serverTestProxyClient(t, tr, proxyURL, sni)
		defer c.CloseIdleConnections()
		res, err := c.Get(target.URL + "/")
		if err != nil {
			return err
		}
		defer res.Body.Close()
		_, _ = io.Copy(io.Discard, res.Body)
		if res.StatusCode != http.StatusOK {
			return &url.Error{Op: "GET", URL: target.URL, Err: io.ErrUnexpectedEOF}
		}
		return nil
	}

	if get("example.com") == nil {
		t.Fatal("no allow rule yet: the self-signed target must be refused")
	}
	assets := d.Assets()
	for _, r := range [][2]string{{"exact_domain", "example.com"}, {"exact_ip", "127.0.0.1"}, {"exact_domain", "pypi.org"}} {
		if _, err := assets.CreateTaskInterceptRule(task.ID, "allow", r[0], r[1], "", true); err != nil {
			t.Fatal(err)
		}
	}
	refresh()
	if err := get("example.com"); err != nil {
		t.Fatalf("in-scope self-signed target with no env var: %v", err)
	}
	// A registry stays verified even when a task names it and the global
	// opt-out is set.
	t.Setenv("ARTEX_CAPTURE_INSECURE_UPSTREAM", "1")
	if get("pypi.org") == nil {
		t.Fatal("pypi.org must be verified whatever the rules or the opt-out say")
	}
	t.Setenv("ARTEX_CAPTURE_INSECURE_UPSTREAM", "")
	if _, err := d.Exec(`UPDATE tasks SET status='done' WHERE id=$1`, task.ID); err != nil {
		t.Fatal(err)
	}
	refresh()
	if get("example.com") == nil {
		t.Fatal("a finished task's rules must not trust the target")
	}
}

func startServerTestProxy(t *testing.T) (*traffic.Traffic, string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()
	tr, err := traffic.Open(t.TempDir(), addr)
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = tr.Start() }()
	t.Cleanup(func() { _ = tr.Close() })
	for deadline := time.Now().Add(5 * time.Second); ; {
		if c, err := net.Dial("tcp", addr); err == nil {
			c.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("proxy did not start")
		}
		time.Sleep(20 * time.Millisecond)
	}
	return tr, "http://" + addr
}

func serverTestProxyClient(t *testing.T, tr *traffic.Traffic, proxyURL, sni string) *http.Client {
	t.Helper()
	pem, err := os.ReadFile(tr.CACertPath())
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		t.Fatal("no CA in", tr.CACertPath())
	}
	u, _ := url.Parse(proxyURL)
	return &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{
		Proxy: http.ProxyURL(u), TLSClientConfig: &tls.Config{RootCAs: pool, ServerName: sni}}}
}
