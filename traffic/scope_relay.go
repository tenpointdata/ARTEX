package traffic

import (
	"errors"
	"io"
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
	mu    sync.Mutex
	done  bool // Close ran; no connection is tracked from then
	conns map[net.Conn]struct{}
}

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
	r := &ScopeRelay{URL: "http://" + ln.Addr().String(), ln: ln, t: t, token: tok, conns: map[net.Conn]struct{}{}}
	go r.serve(u.Host)
	return r.URL, r, nil
}

// Close stops the relay and closes its open connections. Safe on nil.
func (r *ScopeRelay) Close() error {
	if r == nil {
		return nil
	}
	r.once.Do(func() {
		_ = r.ln.Close()
		r.mu.Lock()
		r.done = true
		for c := range r.conns {
			_ = c.Close()
		}
		r.mu.Unlock()
	})
	return nil
}

func (r *ScopeRelay) serve(proxyHost string) {
	for {
		down, err := r.ln.Accept()
		if err != nil {
			return
		}
		go r.relay(down, proxyHost)
	}
}

// track adds c to the open connections; false once the relay is closed.
func (r *ScopeRelay) track(c net.Conn) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.done {
		return false
	}
	r.conns[c] = struct{}{}
	return true
}

func (r *ScopeRelay) untrack(c net.Conn) {
	r.mu.Lock()
	delete(r.conns, c)
	r.mu.Unlock()
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
	if !r.track(down) || !r.track(up) {
		return
	}
	defer r.untrack(down)
	defer r.untrack(up)
	done := make(chan struct{}, 2)
	pipe := func(dst, src net.Conn) {
		_, _ = io.Copy(dst, src)
		if cw, ok := dst.(interface{ CloseWrite() error }); ok {
			_ = cw.CloseWrite()
		}
		done <- struct{}{}
	}
	go pipe(up, down)
	go pipe(down, up)
	<-done
	<-done
}
