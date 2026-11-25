package traffic

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync"
	"testing"

	"github.com/Autumn-27/artex/egress"
	"github.com/Autumn-27/artex/intercept"
)

// runKey is the context key the test ScopeFunc sets, standing in for
// intercept.WithTargetScope.
type runKey struct{}

func runScope(name string) ScopeFunc {
	return func(ctx context.Context) context.Context { return context.WithValue(ctx, runKey{}, name) }
}

// scopeSpy records, for each upstream dial, the address and the run scope its
// context carries ("" for none).
type scopeSpy struct {
	mu    sync.Mutex
	dials []scopedDialRec
}

type scopedDialRec struct{ addr, run string }

func (s *scopeSpy) dial(ctx context.Context, network, addr string) (net.Conn, error) {
	run, _ := ctx.Value(runKey{}).(string)
	s.mu.Lock()
	s.dials = append(s.dials, scopedDialRec{addr, run})
	s.mu.Unlock()
	return intercept.DialRefusingMetadata(ctx, network, addr)
}

func (s *scopeSpy) take() []scopedDialRec {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.dials
	s.dials = nil
	return out
}

func textServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, body) }))
	t.Cleanup(srv.Close)
	return srv
}

func getBody(t *testing.T, c *http.Client, u string) (int, string) {
	t.Helper()
	res, err := c.Get(u)
	if err != nil {
		return 0, err.Error()
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(b)
}

// A client using a run's scoped proxy URL has the proxy's upstream dials run on
// that run's scope, for plain HTTP and for the CONNECT of an HTTPS tunnel, paced
// or not; a client on the plain URL keeps the global rules only.
func TestScopedProxyDialsCarryRunScope(t *testing.T) {
	tr, _, proxyURL := startProxy(t)
	spy := &scopeSpy{}
	tr.SetUpstreamDialer(spy.dial)
	target := textServer(t, "ok")
	scoped := tr.ScopedProxyURL("task-1", runScope("task-1"))
	if scoped == proxyURL {
		t.Fatalf("scoped URL %q carries no credential", scoped)
	}

	for _, u := range []string{scoped, egress.MarkProxyURL(scoped)} {
		if code, body := getBody(t, proxyClient(t, tr, u), target.URL); code != http.StatusOK || body != "ok" {
			t.Fatalf("via %s: %d %q", u, code, body)
		}
		// The TLS handshake may fail (httptest's certificate); the dial is what counts.
		getBody(t, proxyClient(t, tr, u), "https://"+target.Listener.Addr().String()+"/")
		got := spy.take()
		if len(got) != 2 {
			t.Fatalf("via %s: dials %v, want 2", u, got)
		}
		for _, d := range got {
			if d.run != "task-1" {
				t.Fatalf("via %s: dial %s ran on scope %q, want task-1", u, d.addr, d.run)
			}
		}
	}

	if code, _ := getBody(t, proxyClient(t, tr, proxyURL), target.URL); code != http.StatusOK {
		t.Fatalf("unscoped client: status %d", code)
	}
	if got := spy.take(); len(got) != 1 || got[0].run != "" {
		t.Fatalf("unscoped client dials %v, want one with no scope", got)
	}
}

// A client naming a scope that is not registered (the run ended, or a made-up
// token) is refused before any dial: it never falls back to the global rules.
func TestScopedProxyUnknownScopeRefused(t *testing.T) {
	tr, _, proxyURL := startProxy(t)
	spy := &scopeSpy{}
	tr.SetUpstreamDialer(spy.dial)
	target := textServer(t, "ok")
	ended := tr.ScopedProxyURL("task-2", runScope("task-2"))
	tr.ForgetScope("task-2")
	madeUp := egress.ScopedProxyURL(proxyURL, egress.ScopeTokenPrefix+"0000")
	for _, u := range []string{ended, madeUp} {
		if code, body := getBody(t, proxyClient(t, tr, u), target.URL); code == http.StatusOK {
			t.Fatalf("via %s: reached the target: %q", u, body)
		}
		getBody(t, proxyClient(t, tr, u), "https://"+target.Listener.Addr().String()+"/")
	}
	if got := spy.take(); len(got) != 0 {
		t.Fatalf("unknown scopes dialled %v", got)
	}
	if code, _ := getBody(t, proxyClient(t, tr, proxyURL), target.URL); code != http.StatusOK {
		t.Fatalf("an unscoped client was refused too: %d", code)
	}
}

// go-mitmproxy v1.9.2 sends every plain-HTTP request of a client connection
// over the upstream its first request dialled. A keep-alive request to a second
// host must reach that host, and be recorded under it.
func TestProxyKeepAliveHostChange(t *testing.T) {
	p := &probe{}
	tr, _, proxyURL := startProxy(t, p)
	spy := &scopeSpy{}
	tr.SetUpstreamDialer(spy.dial)
	a, b := textServer(t, "A"), textServer(t, "B")
	client := proxyClient(t, tr, proxyURL) // keep-alive on: one proxy connection
	for _, step := range []struct{ url, want string }{{a.URL + "/1", "A"}, {b.URL + "/2", "B"}, {a.URL + "/3", "A"}} {
		if code, body := getBody(t, client, step.url); code != http.StatusOK || body != step.want {
			t.Fatalf("GET %s: %d %q, want %q", step.url, code, body, step.want)
		}
	}
	p.mu.Lock()
	ca, cb := p.conn["GET "+a.URL+"/1"], p.conn["GET "+b.URL+"/2"]
	p.mu.Unlock()
	if ca == "" || ca != cb {
		t.Fatalf("the requests did not share a proxy connection (%q, %q): the test does not exercise keep-alive", ca, cb)
	}
	var sawB bool
	for _, d := range spy.take() {
		sawB = sawB || d.addr == b.Listener.Addr().String()
	}
	if !sawB {
		t.Fatal("no upstream dial to the second host")
	}
}

// The SOCKS5 listener takes a run's scope token as the RFC 1929 password: the
// dial runs on the scope; an ended token is refused (RFC 1929 0x01) without a dial.
func TestSOCKS5ScopedCredential(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			_ = c.Close()
		}
	}()
	host, portStr, _ := net.SplitHostPort(ln.Addr().String())
	port, _ := strconv.Atoi(portStr)

	tr, err := Open(t.TempDir(), "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer tr.Close()
	spy := &scopeSpy{}
	addr, err := tr.StartSOCKS5(SOCKS5Config{Addr: "127.0.0.1:0", DialContext: spy.dial, OnRecord: func(RawTCPCapture) {}})
	if err != nil {
		t.Fatal(err)
	}
	tok := func(raw string) string {
		u, err := url.Parse(raw)
		if err != nil || u.User == nil {
			t.Fatalf("no credential in %q", raw)
		}
		p, _ := u.User.Password()
		return p
	}
	live := tok(tr.ScopedProxyURL("task-3", runScope("task-3")))
	ended := tok(tr.ScopedProxyURL("task-4", runScope("task-4")))
	tr.ForgetScope("task-4")

	for _, user := range []string{egress.ScopeUser, egress.PaceUser} {
		c, rep := socks5DialUserPass(t, addr, user, live, host, port)
		if rep != socks5RepSuccess {
			t.Fatalf("%s with a live scope: rep 0x%02x", user, rep)
		}
		_ = c.Close()
	}
	if got := spy.take(); len(got) != 2 || got[0].run != "task-3" || got[1].run != "task-3" {
		t.Fatalf("scoped SOCKS5 dials %v, want two on task-3", got)
	}
	// An ended scope's token is refused at authentication, not downgraded.
	c, status := socks5UserPassAuth(t, addr, egress.ScopeUser, ended)
	_ = c.Close()
	if status != socks5UserPassFail {
		t.Fatalf("ended scope: auth status 0x%02x, want 0x01", status)
	}
	if got := spy.take(); len(got) != 0 {
		t.Fatalf("an ended scope dialled %v", got)
	}
}
