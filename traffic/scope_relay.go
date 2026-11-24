package traffic

import (
	"errors"
	"net"
	"net/url"
	"sync"
	"time"

	"github.com/Autumn-27/artex/egress"
)

// Scope relays. A scope token rides on the proxy URL's userinfo (scope.go),
// which a client sends as Proxy-Authorization. Some clients cannot carry it:
// Chromium drops the userinfo from --proxy-server and sends credentials only
// after a 407 challenge, which the recording proxy never issues. A relay is a
// loopback listener that stands for one token: every connection it accepts is
// forwarded to the recording proxy and bound to the token by the address the
// proxy sees, so the client needs no credential. It only adds the scope's
// rules, as the token does; a token that names no active scope is refused at
// the gate, as with the credential.

// ScopeRelay is a running relay; Close stops it and its connections.
type ScopeRelay struct {
	URL string // credential-free proxy URL for the client

	ln    net.Listener
	t     *Traffic
	token string
	once  sync.Once
	conns *connSet // open connections, at most maxRelayConns clients at once
}

// maxRelayConns caps the client connections one relay serves at once; a
// connection over it is closed unserved. Chromium keeps at most 256 sockets
// per profile (and 6 per proxy host per pool), so a browser never meets the
// cap; it bounds what a runaway or hostile client of the loopback port can
// hold open (two sockets and two goroutines a connection).
const maxRelayConns = 256

// StartScopeRelay returns a relay for the scope token in proxyURL, a URL that
// ScopedProxyURL, EndedScopeURL (or the server's ScopedProxyAddr) returned. A
// URL without a scope token needs no relay: it is returned without its
// credential and a nil relay, and keeps the global rules. A URL that is not
// the recording proxy's is an error, as is a relay that cannot listen: the
// caller must not fall back to a URL that drops the scope.
func (t *Traffic) StartScopeRelay(proxyURL string) (string, *ScopeRelay, error) {
	u, err := url.Parse(proxyURL)
	if err != nil || u.Host == "" {
		return "", nil, errors.New("scope relay: the proxy URL does not parse")
	}
	if t.sink == nil || egress.WithoutProxyCredential(proxyURL) != t.ProxyAddr() {
		return "", nil, errors.New("scope relay: the URL is not the recording proxy's")
	}
	pass, _ := u.User.Password()
	tok, ok := egress.ScopeToken(pass)
	if !ok {
		return t.ProxyAddr(), nil, nil
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", nil, err
	}
	r := &ScopeRelay{URL: "http://" + ln.Addr().String(), ln: ln, t: t, token: tok, conns: newConnSet(maxRelayConns)}
	go r.conns.serveConns(ln, func(down net.Conn) { r.relay(down, u.Host) })
	return r.URL, r, nil
}

// Close stops the relay and closes its open connections. Safe on nil.
func (r *ScopeRelay) Close() error {
	if r == nil {
		return nil
	}
	r.once.Do(func() {
		_ = r.ln.Close()
		r.conns.closeAll()
	})
	return nil
}

// relayDialTimeout bounds a relay's dial of the recording proxy (loopback).
const relayDialTimeout = 5 * time.Second

// endedRelayToken is the binding left for a relay connection that ended: a
// token no scope is registered under, so a dial named by it is refused.
const endedRelayToken = egress.ScopeTokenPrefix + "relay-ended"

func (r *ScopeRelay) relay(down net.Conn, proxyHost string) {
	defer down.Close()
	up, err := net.DialTimeout("tcp", proxyHost, relayDialTimeout)
	if err != nil {
		return
	}
	defer up.Close()
	// Bind before a byte is sent: the proxy reads no request from up, so it
	// makes no upstream dial for it, until the copy below starts.
	// When the relay ends the binding becomes a tombstone, not a gap: the
	// proxy may have read a request it has not dialled for yet, and that dial
	// must be refused, not made on the global rules. The proxy's disconnect of
	// the connection (sink.ClientDisconnected) drops the entry; if it came first,
	// the swap finds nothing and nothing is left behind.
	key := up.LocalAddr().String()
	r.t.sink.relayed.Store(key, r.token)
	defer r.t.sink.relayed.CompareAndSwap(key, r.token, endedRelayToken)
	r.conns.splice(down, up)
}
