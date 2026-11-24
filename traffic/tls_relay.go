package traffic

import (
	"context"
	"crypto/tls"
	"errors"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// TLSRelay is a loopback listener that forwards every connection it accepts
// to a remote address over TLS. A worker node runs one in front of the
// control node's recording proxy (proxy_tls_front.go): its agents use it as an
// ordinary plain-HTTP proxy, with their scope token, and the bytes, token
// included, cross the network inside TLS. It is a byte pipe only: it reads,
// adds and checks nothing, so every scope decision stays with the proxy.
//
// The relay is general: anything that must reach a TLS-only service through a
// client that speaks plain TCP can use it (the browser MCP relay of c-18f7a8).
type TLSRelay struct {
	URL  string // http://127.0.0.1:<port>, the proxy URL for clients (no credential)
	Addr string // 127.0.0.1:<port>

	ln     net.Listener
	once   sync.Once
	conns  *connSet
	dialer *tls.Dialer
	remote string
	logged atomic.Int64 // unix seconds of the last dial-failure log line
}

// maxTLSRelayConns caps the client connections one relay serves at once.
const maxTLSRelayConns = 1024

// tlsRelayDialTimeout bounds the TCP dial and TLS handshake with the remote.
const tlsRelayDialTimeout = 10 * time.Second

// StartTLSRelay listens on a free loopback port and forwards each connection
// to remote (host:port) over TLS with cfg. cfg must verify the remote: it may
// not set InsecureSkipVerify, and with no ServerName the remote's host is used.
// Pin a private CA through cfg.RootCAs.
func StartTLSRelay(remote string, cfg *tls.Config) (*TLSRelay, error) {
	host, _, err := net.SplitHostPort(remote)
	if err != nil || host == "" {
		return nil, errors.New("TLS relay: the remote must be host:port")
	}
	if cfg == nil {
		cfg = &tls.Config{}
	}
	if cfg.InsecureSkipVerify {
		return nil, errors.New("TLS relay: refusing a configuration that skips certificate verification")
	}
	c := cfg.Clone()
	if c.ServerName == "" {
		c.ServerName = host
	}
	if c.MinVersion < tls.VersionTLS12 {
		c.MinVersion = tls.VersionTLS12
	}
	if c.ClientSessionCache == nil {
		c.ClientSessionCache = tls.NewLRUClientSessionCache(64)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	r := &TLSRelay{
		URL: "http://" + ln.Addr().String(), Addr: ln.Addr().String(),
		ln: ln, conns: newConnSet(maxTLSRelayConns), remote: remote,
		dialer: &tls.Dialer{NetDialer: &net.Dialer{Timeout: tlsRelayDialTimeout}, Config: c},
	}
	go r.conns.serveConns(ln, r.relay)
	return r, nil
}

// Close stops the relay and closes its open connections. Safe on nil.
func (r *TLSRelay) Close() error {
	if r == nil {
		return nil
	}
	r.once.Do(func() {
		_ = r.ln.Close()
		r.conns.closeAll()
	})
	return nil
}

func (r *TLSRelay) relay(down net.Conn) {
	defer down.Close()
	ctx, cancel := context.WithTimeout(context.Background(), tlsRelayDialTimeout)
	up, err := r.dialer.DialContext(ctx, "tcp", r.remote)
	cancel()
	if err != nil {
		// A pinning or name mismatch shows up here: say why, once a minute.
		now := time.Now().Unix()
		if last := r.logged.Load(); now-last >= 60 && r.logged.CompareAndSwap(last, now) {
			log.Printf("[traffic] TLS relay to %s: %v", r.remote, err)
		}
		return
	}
	defer up.Close()
	r.conns.splice(down, up)
}
