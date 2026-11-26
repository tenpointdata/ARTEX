package traffic

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	mproxy "github.com/lqqyt2423/go-mitmproxy/proxy"
)

// startRelay starts a relay of a fresh scope key and closes it with the test.
func startRelay(t *testing.T, tr *Traffic, key string) *ScopeRelay {
	t.Helper()
	_, relay, err := tr.StartScopeRelay(tr.ScopedProxyURL(key, runScope(key)))
	if err != nil || relay == nil {
		t.Fatalf("StartScopeRelay(%s): %v (relay %v)", key, err, relay)
	}
	t.Cleanup(func() { _ = relay.Close() })
	return relay
}

// dialsByScope groups recorded dials by the scope they ran on.
func dialsByScope(got []scopedDialRec) map[string]int {
	out := map[string]int{}
	for _, d := range got {
		out[d.run]++
	}
	return out
}

// Two relays of different scopes live at once: every connection of each,
// plain HTTP and CONNECT, interleaved across goroutines, dials on its own
// relay's scope and never on the other's or on none.
func TestScopeRelaysConcurrentScopes(t *testing.T) {
	tr, _, _ := startProxy(t)
	spy := &scopeSpy{}
	tr.SetUpstreamDialer(spy.dial)
	target := textServer(t, "ok")
	relays := map[string]*ScopeRelay{"task-a": startRelay(t, tr, "task-a"), "task-b": startRelay(t, tr, "task-b")}

	const perScope = 8
	var wg sync.WaitGroup
	errs := make(chan string, 4*perScope)
	for key, r := range relays {
		for i := 0; i < perScope; i++ {
			wg.Add(1)
			go func(key, relayURL string, i int) {
				defer wg.Done()
				// A client per request: each is its own relay and proxy connection.
				c := proxyClient(t, tr, relayURL)
				c.Transport.(*http.Transport).DisableKeepAlives = true
				u := fmt.Sprintf("%s/%s/%d", target.URL, key, i)
				if i%2 == 1 {
					u = fmt.Sprintf("https://%s/%s/%d", target.Listener.Addr(), key, i)
				}
				res, err := c.Get(u)
				if err != nil {
					if i%2 == 0 { // the https target is plain HTTP: only the dial matters
						errs <- fmt.Sprintf("%s: %v", u, err)
					}
					return
				}
				_, _ = io.Copy(io.Discard, res.Body)
				res.Body.Close()
			}(key, r.URL, i)
		}
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		t.Error(e)
	}
	got := dialsByScope(spy.take())
	if got["task-a"] != perScope || got["task-b"] != perScope || len(got) != 2 {
		t.Fatalf("dials by scope %v, want %d on each of task-a and task-b and none elsewhere", got, perScope)
	}
}

// A keep-alive connection through a relay that changes host goes through
// go-mitmproxy's separate client (unpinHostChange): that dial, too, runs on
// the relay's scope, and is refused once the scope is forgotten.
func TestScopeRelayKeepAliveHostChange(t *testing.T) {
	p := &probe{}
	tr, _, _ := startProxy(t, p)
	spy := &scopeSpy{}
	tr.SetUpstreamDialer(spy.dial)
	a, b := textServer(t, "A"), textServer(t, "B")
	relay := startRelay(t, tr, "task-k")
	client := proxyClient(t, tr, relay.URL) // keep-alive: one relay connection
	for _, step := range []struct{ url, want string }{{a.URL + "/1", "A"}, {b.URL + "/2", "B"}} {
		if code, body := getBody(t, client, step.url); code != http.StatusOK || body != step.want {
			t.Fatalf("GET %s: %d %q, want %q", step.url, code, body, step.want)
		}
	}
	p.mu.Lock()
	ca, cb := p.conn["GET "+a.URL+"/1"], p.conn["GET "+b.URL+"/2"]
	p.mu.Unlock()
	if ca == "" || ca != cb {
		t.Fatalf("the requests did not share a proxy connection (%q, %q): the test does not exercise the host change", ca, cb)
	}
	got := spy.take()
	var sawB bool
	for _, d := range got {
		if d.run != "task-k" {
			t.Fatalf("dial %s ran on scope %q, want task-k", d.addr, d.run)
		}
		sawB = sawB || d.addr == b.Listener.Addr().String()
	}
	if !sawB {
		t.Fatalf("no dial to the second host in %v", got)
	}

	// The scope is checked at dial time, so ForgetScope also closes the
	// connections bound to it (TestForgetScopeClosesBoundConnections covers
	// the pinned host). A host change needs a new dial, and is refused.
	tr.ForgetScope("task-k")
	if code, _ := getBody(t, client, b.URL+"/3"); code == http.StatusOK {
		t.Fatal("a keep-alive host change of a forgotten scope reached its target")
	}
	if got := spy.take(); len(got) != 0 {
		t.Fatalf("forgotten scope dialled %v", got)
	}
}

// A WebSocket (wss, a CONNECT tunnel the proxy intercepts and upgrades) through
// a relay dials on the relay's scope and carries messages both ways.
func TestScopeRelayWebSocket(t *testing.T) {
	tr, _, _ := startProxy(t)
	spy := &scopeSpy{}
	tr.SetUpstreamDialer(spy.dial)
	srv := echoWSServer(t)
	relay := startRelay(t, tr, "task-w")
	target := "wss://" + strings.TrimPrefix(localhostURL(srv.URL), "https://") + "/ws"
	c, _, err := wsDialer(t, tr, relay.URL).Dial(target, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer closeWS(c)
	if err := c.WriteMessage(1, []byte("hi")); err != nil {
		t.Fatal(err)
	}
	if _, msg, err := c.ReadMessage(); err != nil || string(msg) != "echo:hi" {
		t.Fatalf("read %q %v", msg, err)
	}
	got := spy.take()
	if len(got) == 0 {
		t.Fatal("no upstream dial for the WebSocket")
	}
	for _, d := range got {
		if d.run != "task-w" {
			t.Fatalf("WebSocket dial %s ran on scope %q, want task-w", d.addr, d.run)
		}
	}
}

// holdRequest blocks a flow's Request hook (after the request is read, before
// the upstream dial) until release is closed, and reports the scope the
// proxy would dial it on at that point.
type holdRequest struct {
	mproxy.BaseAddon
	s       *sink
	held    chan struct{}
	release chan struct{}
	scope   chan string
}

func (h *holdRequest) Request(f *mproxy.Flow) {
	close(h.held)
	<-h.release
	h.scope <- h.s.scopeOf(f.Request.Raw())
}

// A request the proxy has already read when its relay closes is not dialled on
// the global rules: the binding of its connection is the refused tombstone
// until the proxy drops the connection, and no dial is made. Removing the
// tombstone (deleting the binding on relay end) makes this fail.
func TestScopeRelayTombstoneRefusesInFlightRequest(t *testing.T) {
	h := &holdRequest{held: make(chan struct{}), release: make(chan struct{}), scope: make(chan string, 1)}
	tr, s, _ := startProxy(t, h)
	h.s = s
	spy := &scopeSpy{}
	tr.SetUpstreamDialer(spy.dial)
	target := textServer(t, "ok")
	relay := startRelay(t, tr, "task-h")

	c, err := net.Dial("tcp", strings.TrimPrefix(relay.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if _, err := fmt.Fprintf(c, "GET %s/ HTTP/1.1\r\nHost: %s\r\n\r\n", target.URL, target.Listener.Addr()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-h.held:
	case <-time.After(10 * time.Second):
		t.Fatal("the proxy never read the request")
	}
	_ = relay.Close()
	// The proxy cannot drop the connection while its handler is held, so the
	// binding the relay leaves is observable: it must be the tombstone.
	waitFor(t, "the tombstone", func() bool {
		var tok string
		s.relayed.Range(func(_, v any) bool { tok = v.(string); return false })
		return tok == endedRelayToken
	})
	close(h.release)
	if got := <-h.scope; got != endedRelayToken {
		t.Fatalf("the held request would dial on scope %q, want the tombstone", got)
	}
	_, _ = io.Copy(io.Discard, bufio.NewReader(c)) // until the proxy is done with it
	if got := spy.take(); len(got) != 0 {
		t.Fatalf("a request of a closed relay dialled %v", got)
	}
	waitFor(t, "the binding to go", func() bool {
		n := 0
		s.relayed.Range(func(_, _ any) bool { n++; return true })
		return n == 0
	})
}

// A relay serves at most maxRelayConns client connections at once: one over
// the cap is closed unserved, and its slot is usable again once one ends.
func TestScopeRelayConnectionCap(t *testing.T) {
	tr, _, _ := startProxy(t)
	spy := &scopeSpy{}
	tr.SetUpstreamDialer(spy.dial)
	target := textServer(t, "ok")
	relay := startRelay(t, tr, "task-c")
	for i := 0; i < cap(relay.slots)-1; i++ { // all but one slot taken
		relay.slots <- struct{}{}
	}
	addr := strings.TrimPrefix(relay.URL, "http://")
	held, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the last slot", func() bool { return len(relay.slots) == cap(relay.slots) })

	over, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer over.Close()
	_ = over.SetReadDeadline(time.Now().Add(5 * time.Second))
	if n, err := over.Read(make([]byte, 1)); err != io.EOF || n != 0 {
		t.Fatalf("a connection over the cap read %d, %v; want it closed", n, err)
	}

	held.Close()
	waitFor(t, "the slot to free", func() bool { return len(relay.slots) < cap(relay.slots) })
	c := proxyClient(t, tr, relay.URL)
	if code, body := getBody(t, c, target.URL); code != http.StatusOK || body != "ok" {
		t.Fatalf("after a slot freed: %d %q", code, body)
	}
	if got := dialsByScope(spy.take()); got["task-c"] != 1 || len(got) != 1 {
		t.Fatalf("dials %v, want one on task-c", got)
	}
}
