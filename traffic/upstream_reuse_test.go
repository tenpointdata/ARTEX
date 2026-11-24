package traffic

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// connCounter counts the distinct TCP connections a test target accepts.
type connCounter struct {
	mu    sync.Mutex
	conns map[string]bool
}

func (c *connCounter) handler(w http.ResponseWriter, r *http.Request) {
	c.mu.Lock()
	if c.conns == nil {
		c.conns = map[string]bool{}
	}
	c.conns[r.RemoteAddr] = true
	c.mu.Unlock()
	_, _ = io.WriteString(w, "ok")
}

func (c *connCounter) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.conns)
}

// Scope rules are applied when the upstream gate dials (upstream_gate.go). A
// rule therefore holds for a request only if no upstream connection dialled
// for one client connection carries a request of another. go-mitmproxy v1.9.2
// keeps one upstream connection per client connection (ConnContext.ServerConn,
// dialled through the gate by getUpstreamConn); the only pool it shares across
// clients is the attacker's separate client, used when an addon changes a
// request's host or scheme or sets Flow.UseSeparateClient, which no ARTEX addon
// does. This test pins that: two client connections, each sending several
// keep-alive requests to the same HTTP and HTTPS targets, cause one gate dial
// and one target connection per client connection and target, never one shared.
// If a go-mitmproxy upgrade starts pooling upstreams across clients, the
// counts drop to one and this fails.
func TestUpstreamConnectionsNotSharedAcrossClients(t *testing.T) {
	plainSeen, tlsSeen := &connCounter{}, &connCounter{}
	plain := httptest.NewServer(http.HandlerFunc(plainSeen.handler))
	defer plain.Close()
	tlsSrv := httptest.NewTLSServer(http.HandlerFunc(tlsSeen.handler))
	defer tlsSrv.Close()
	// The MITM leaf for a bare IP has no IP SAN: address the TLS target by name.
	tlsURL := strings.Replace(tlsSrv.URL, "127.0.0.1", "localhost", 1)

	tr, _, proxyURL := startProxy(t)
	spy := &dialSpy{}
	tr.SetUpstreamDialer(spy.dial)

	// Each http.Client has its own Transport, so its own client connection to
	// the proxy: as two tasks under two scopes would.
	clients := []*http.Client{proxyClient(t, tr, proxyURL), proxyClient(t, tr, proxyURL)}
	for range 3 {
		for _, c := range clients {
			get(t, c, plain.URL+"/p")
			get(t, c, tlsURL+"/s")
		}
	}

	if n := plainSeen.count(); n != len(clients) {
		t.Fatalf("plain target saw %d connections for %d client connections; want one each", n, len(clients))
	}
	if n := tlsSeen.count(); n != len(clients) {
		t.Fatalf("TLS target saw %d connections for %d client connections; want one each", n, len(clients))
	}
	addrs, _ := spy.snapshot()
	byAddr := map[string]int{}
	for _, a := range addrs {
		byAddr[a]++
	}
	plainAddr := strings.TrimPrefix(plain.URL, "http://")
	tlsAddr := strings.TrimPrefix(tlsURL, "https://")
	if byAddr[plainAddr] != len(clients) || byAddr[tlsAddr] != len(clients) {
		t.Fatalf("gate dials %v; want %d to %s and %d to %s", addrs, len(clients), plainAddr, len(clients), tlsAddr)
	}
}
