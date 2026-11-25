package traffic

import (
	"bufio"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Autumn-27/artex/egress"
)

// fakeRemote stands in for the control node's database-backed resolver: token
// -> scope key, each resolving to runScope(key) while the key is active.
type fakeRemote struct {
	mu     sync.Mutex
	tokens map[string]string
	ended  map[string]bool
	calls  atomic.Int32
}

func newFakeRemote(tokens map[string]string) *fakeRemote {
	return &fakeRemote{tokens: tokens, ended: map[string]bool{}}
}

func (f *fakeRemote) resolve(tok string) (string, ScopeFunc, bool) {
	f.calls.Add(1)
	f.mu.Lock()
	defer f.mu.Unlock()
	key, ok := f.tokens[tok]
	if !ok || f.ended[key] {
		return "", nil, false
	}
	return key, runScope(key), true
}

func (f *fakeRemote) end(key string) {
	f.mu.Lock()
	f.ended[key] = true
	f.mu.Unlock()
}

// workerToken is a token minted the way a worker node mints one.
func workerToken(n int) string {
	return egress.ScopeTokenPrefix + "worker" + strconv.Itoa(n) + "0000000000000000000000000"
}

// treatAllRemote makes every client of tr count as non-loopback, standing in
// for a worker host reaching the proxy over the network.
func treatAllRemote(tr *Traffic) {
	notLoopback := func(string) bool { return false }
	tr.isLoopback.Store(&notLoopback)
}

// setClock replaces the remote cache's clock.
func setClock(tr *Traffic, now func() time.Time) {
	tr.remote.mu.Lock()
	tr.remote.now = now
	tr.remote.mu.Unlock()
}

// A token the proxy did not mint is resolved remotely: the dial runs on the
// scope the resolver returns, for plain HTTP and for an HTTPS CONNECT, and a
// positive resolution is cached; one the resolver refuses is refused with no
// dial, never downgraded to the global rules.
func TestRemoteScopeTokenResolved(t *testing.T) {
	tr, _, proxyURL := startProxy(t)
	spy := &scopeSpy{}
	tr.SetUpstreamDialer(spy.dial)
	target := textServer(t, "ok")
	remote := newFakeRemote(map[string]string{workerToken(1): "42"})
	tr.SetRemoteScopeResolver(remote.resolve)
	treatAllRemote(tr)

	scoped := egress.ScopedProxyURL(proxyURL, workerToken(1))
	for _, u := range []string{scoped, egress.MarkProxyURL(scoped)} {
		if code, body := getBody(t, proxyClient(t, tr, u), target.URL); code != http.StatusOK || body != "ok" {
			t.Fatalf("via %s: %d %q", u, code, body)
		}
		getBody(t, proxyClient(t, tr, u), "https://"+target.Listener.Addr().String()+"/")
		got := spy.take()
		if len(got) != 2 {
			t.Fatalf("via %s: dials %v, want 2", u, got)
		}
		for _, d := range got {
			if d.run != "42" {
				t.Fatalf("via %s: dial %s ran on scope %q, want 42", u, d.addr, d.run)
			}
		}
	}
	if n := remote.calls.Load(); n != 1 {
		t.Fatalf("resolver asked %d times within the TTL, want 1 (cached)", n)
	}

	unknown := egress.ScopedProxyURL(proxyURL, workerToken(2))
	if code, body := getBody(t, proxyClient(t, tr, unknown), target.URL); code == http.StatusOK {
		t.Fatalf("an unresolved token reached the target: %q", body)
	}
	getBody(t, proxyClient(t, tr, unknown), "https://"+target.Listener.Addr().String()+"/")
	if got := spy.take(); len(got) != 0 {
		t.Fatalf("an unresolved token dialled %v", got)
	}
}

// keepAliveConn opens a raw proxy connection carrying proxyURL's credential and
// proves its upstream is open (two requests on one pinned upstream).
func keepAliveConn(t *testing.T, proxyURL, target string) (net.Conn, *bufio.Reader, string) {
	t.Helper()
	host, auth := basicAuth(t, proxyURL)
	conn, err := net.Dial("tcp", host)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	br := bufio.NewReader(conn)
	for i := range 2 {
		if code, err := keepAliveGet(conn, br, target+"/", auth); err != nil || code != http.StatusOK {
			t.Fatalf("request %d: %d %v", i, code, err)
		}
	}
	return conn, br, auth
}

// assertClosed checks that the next keep-alive request on conn does not reach
// the target and that the proxy closed the connection (no timeout).
func assertClosed(t *testing.T, conn net.Conn, br *bufio.Reader, target, auth string, hits *atomic.Int32) {
	t.Helper()
	before := hits.Load()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	code, err := keepAliveGet(conn, br, target+"/", auth)
	if err == nil && code == http.StatusOK {
		t.Fatal("a keep-alive request to the pinned host reached it after the scope ended")
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		t.Fatalf("the connection stayed open after the scope ended: %v", err)
	}
	if got := hits.Load(); got != before {
		t.Fatalf("target saw %d requests after the scope ended", got-before)
	}
}

// ForgetScope closes the client connections bound to a remote token resolved to
// the key, also once the cache entry is past its TTL, and a reconnect with the
// token is refused when the resolver no longer resolves it.
func TestForgetScopeClosesRemoteBoundConnections(t *testing.T) {
	for _, stale := range []bool{false, true} {
		t.Run("stale="+strconv.FormatBool(stale), func(t *testing.T) {
			tr, _, proxyURL := startProxy(t)
			target, hits := countingServer(t)
			remote := newFakeRemote(map[string]string{workerToken(3): "7"})
			tr.SetRemoteScopeResolver(remote.resolve)
			var now atomic.Int64
			now.Store(time.Now().UnixNano())
			setClock(tr, func() time.Time { return time.Unix(0, now.Load()) })
			scoped := egress.ScopedProxyURL(proxyURL, workerToken(3))
			conn, br, auth := keepAliveConn(t, scoped, target)
			if stale {
				now.Add(int64(2 * remoteScopeTTL))
			}
			remote.end("7") // the control node ends the task: tokens gone from the database
			tr.ForgetScope("7")
			assertClosed(t, conn, br, target, auth, hits)
			if code, _ := getBody(t, proxyClient(t, tr, scoped), target); code == http.StatusOK {
				t.Fatal("a reconnect with the forgotten remote token reached the target")
			}
		})
	}
}

// A cached remote token whose re-check fails after the TTL is dropped, and its
// open client connections are closed with it.
func TestRemoteScopeRecheckFailureCloses(t *testing.T) {
	tr, _, proxyURL := startProxy(t)
	target, hits := countingServer(t)
	remote := newFakeRemote(map[string]string{workerToken(4): "8"})
	tr.SetRemoteScopeResolver(remote.resolve)
	var now atomic.Int64
	now.Store(time.Now().UnixNano())
	setClock(tr, func() time.Time { return time.Unix(0, now.Load()) })
	scoped := egress.ScopedProxyURL(proxyURL, workerToken(4))
	conn, br, auth := keepAliveConn(t, scoped, target)

	remote.end("8")
	now.Add(int64(2 * remoteScopeTTL))
	if code, _ := getBody(t, proxyClient(t, tr, scoped), target); code == http.StatusOK {
		t.Fatal("a token whose scope ended passed its re-check")
	}
	assertClosed(t, conn, br, target, auth, hits)
}

// A resolution in flight when ForgetScope runs for its key is refused and not
// cached: a dial racing the end of the scope cannot re-open it.
func TestRemoteScopeResolutionRacingForget(t *testing.T) {
	tr, _, _ := startProxy(t)
	entered, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	tr.SetRemoteScopeResolver(func(tok string) (string, ScopeFunc, bool) {
		if calls.Add(1) == 1 {
			close(entered)
			<-release
		}
		return "9", runScope("9"), true
	})
	tok := workerToken(5)
	done := make(chan bool)
	go func() {
		_, ok := tr.resolveScope(tok)
		done <- ok
	}()
	<-entered
	tr.ForgetScope("9")
	close(release)
	if <-done {
		t.Fatal("a resolution racing ForgetScope was accepted")
	}
	if _, ok := tr.resolveScope(tok); !ok {
		t.Fatal("a later resolution (the resolver says active) was refused")
	}
	if n := calls.Load(); n != 2 {
		t.Fatalf("resolver calls %d, want 2: the racing result must not have been cached", n)
	}
}

// With RequireScopeForRemoteClients on (the default once a resolver is set), a
// non-loopback client without a token is refused with no dial, also when a
// global egress proxy would carry the dial past the gate; with a token it is
// served. Loopback clients keep the global rules.
func TestRemoteClientWithoutScopeRefused(t *testing.T) {
	tr, _, proxyURL := startProxy(t)
	spy := &scopeSpy{}
	tr.SetUpstreamDialer(spy.dial)
	target := textServer(t, "ok")
	tr.SetRemoteScopeResolver(newFakeRemote(map[string]string{workerToken(6): "10"}).resolve)

	// Loopback, token-less: served as before.
	if code, _ := getBody(t, proxyClient(t, tr, proxyURL), target.URL); code != http.StatusOK {
		t.Fatalf("a loopback client without a token was refused: %d", code)
	}
	if got := spy.take(); len(got) != 1 || got[0].run != "" {
		t.Fatalf("loopback dials %v, want one unscoped", got)
	}

	treatAllRemote(tr)
	for _, u := range []string{proxyURL, egress.MarkProxyURL(proxyURL)} {
		if code, body := getBody(t, proxyClient(t, tr, u), target.URL); code == http.StatusOK {
			t.Fatalf("a remote client without a token via %s reached the target: %q", u, body)
		}
		getBody(t, proxyClient(t, tr, u), "https://"+target.Listener.Addr().String()+"/")
	}
	if got := spy.take(); len(got) != 0 {
		t.Fatalf("a remote client without a token dialled %v", got)
	}
	scoped := egress.ScopedProxyURL(proxyURL, workerToken(6))
	if code, _ := getBody(t, proxyClient(t, tr, scoped), target.URL); code != http.StatusOK {
		t.Fatalf("a remote client with a resolved token was refused: %d", code)
	}
	if got := spy.take(); len(got) != 1 || got[0].run != "10" {
		t.Fatalf("remote scoped dials %v, want one on 10", got)
	}

	// The global egress proxy path bypasses the gate: the refusal comes first.
	if err := tr.SetUpstreamProxy("http://127.0.0.1:1"); err != nil {
		t.Fatal(err)
	}
	req := &http.Request{RemoteAddr: "10.1.2.3:40000", URL: &url.URL{Scheme: "http", Host: "example.test"}}
	if _, err := tr.upstreamFor(req); !errors.Is(err, ErrScopeRequired) {
		t.Fatalf("upstreamFor of a remote token-less client with an egress proxy: %v, want ErrScopeRequired", err)
	}
	_ = tr.SetUpstreamProxy("")

	// The option, turned off, restores the global rules for remote clients.
	tr.RequireScopeForRemoteClients(false)
	if code, _ := getBody(t, proxyClient(t, tr, proxyURL), target.URL); code != http.StatusOK {
		t.Fatalf("with the option off a remote token-less client was refused: %d", code)
	}
}

// The real address check: loopback addresses pass, others and junk do not.
func TestIsLoopbackAddr(t *testing.T) {
	for addr, want := range map[string]bool{
		"127.0.0.1:5000": true, "[::1]:5000": true, "127.9.9.9:1": true,
		"10.0.0.5:5000": false, "[fd00::1]:5000": false, "": false, "garbage": false,
	} {
		if got := isLoopbackAddr(addr); got != want {
			t.Errorf("isLoopbackAddr(%q) = %v, want %v", addr, got, want)
		}
	}
}

// The SOCKS5 listener resolves a remote token too, and refuses a non-loopback
// client whose credential names no scope while the option is on.
func TestSOCKS5RemoteScope(t *testing.T) {
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
	tr.SetRemoteScopeResolver(newFakeRemote(map[string]string{workerToken(7): "11"}).resolve)
	treatAllRemote(tr)

	c, rep := socks5DialUserPass(t, addr, egress.ScopeUser, workerToken(7), host, port)
	if rep != socks5RepSuccess {
		t.Fatalf("remote token: rep 0x%02x", rep)
	}
	_ = c.Close()
	if got := spy.take(); len(got) != 1 || got[0].run != "11" {
		t.Fatalf("SOCKS5 remote dials %v, want one on 11", got)
	}
	// The bare pace marker passes the loopback listener's open-mode auth, then
	// the remote-client gate refuses its CONNECT (0x02).
	if c, rep := socks5DialUserPass(t, addr, egress.PaceUser, "", host, port); rep != socks5RepNotAllowed {
		_ = c.Close()
		t.Fatalf("remote client with the bare pace marker: rep 0x%02x, want 0x02", rep)
	}
	// A credential that is no live scope token, and a token-shaped one the
	// resolver does not hold, are refused at authentication (RFC 1929 0x01).
	for _, pass := range []string{"not-a-token", workerToken(8)} {
		c, status := socks5UserPassAuth(t, addr, egress.PaceUser, pass)
		_ = c.Close()
		if status != socks5UserPassFail {
			t.Fatalf("remote client with password %q: auth status 0x%02x, want 0x01", pass, status)
		}
	}
	if got := spy.take(); len(got) != 0 {
		t.Fatalf("refused SOCKS5 clients dialled %v", got)
	}
}
