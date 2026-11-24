package traffic

import (
	"crypto/tls"
	"errors"
	"net"
	"net/url"
	"sync"
	"time"
)

// TLS front for the recording proxy (E7, c-677166; docs/DISTRIBUTED.md
// "Transport"). Proxy clients (curl, nuclei, sqlmap, Chromium) speak plain
// HTTP to their proxy, so a worker node's agents cannot reach the control
// node's proxy over TLS themselves. Instead a worker-side relay (tls_relay.go)
// takes their plain connections on loopback and dials this front over TLS;
// the front terminates TLS and hands the bytes to the recording proxy over
// loopback. Plain-HTTP target requests and the scope token
// (Proxy-Authorization) then never cross the network in clear text.
//
// The front's connection to the proxy comes from loopback, which the gate
// would otherwise treat as a local, trusted client (token-less dials allowed
// on the global rules). Each one is marked fronted before a byte is relayed,
// so the proxy treats it as the remote client it carries: it must name an
// active scope (scope_remote.go). Starting a front turns on
// RequireScopeForRemoteClients, so a front is never an open relay.

// ProxyTLSFront is a running TLS front; Close stops it and its connections.
type ProxyTLSFront struct {
	Addr string // the address it listens on

	ln    net.Listener
	t     *Traffic
	once  sync.Once
	conns *connSet
}

// maxFrontConns caps the client connections a front serves at once (every
// worker node's agents together); one over it is closed unserved.
const maxFrontConns = 4096

// frontHandshakeTimeout bounds a client's TLS handshake, so a peer that opens
// a connection and sends nothing holds no slot for long.
const frontHandshakeTimeout = 10 * time.Second

// ServeTLSFront listens on addr with cfg (which must provide a certificate:
// Certificates or GetCertificate) and relays each TLS client to the recording
// proxy as a remote client. It turns on RequireScopeForRemoteClients.
func (t *Traffic) ServeTLSFront(addr string, cfg *tls.Config) (*ProxyTLSFront, error) {
	if t.sink == nil {
		return nil, errors.New("proxy TLS front: the recording proxy is not running")
	}
	if cfg == nil || (len(cfg.Certificates) == 0 && cfg.GetCertificate == nil && cfg.GetConfigForClient == nil) {
		return nil, errors.New("proxy TLS front: no certificate")
	}
	proxyHost, err := loopbackDialHost(t.ProxyAddr())
	if err != nil {
		return nil, err
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	t.RequireScopeForRemoteClients(true)
	f := &ProxyTLSFront{Addr: ln.Addr().String(), ln: ln, t: t, conns: newConnSet(maxFrontConns)}
	go f.conns.serveConns(tls.NewListener(ln, cfg.Clone()), func(c net.Conn) { f.relay(c, proxyHost) })
	return f, nil
}

// Close stops the front and closes its open connections. Safe on nil.
func (f *ProxyTLSFront) Close() error {
	if f == nil {
		return nil
	}
	f.once.Do(func() {
		_ = f.ln.Close()
		f.conns.closeAll()
	})
	return nil
}

func (f *ProxyTLSFront) relay(c net.Conn, proxyHost string) {
	defer c.Close()
	tc, ok := c.(*tls.Conn)
	if !ok {
		return
	}
	// Handshake before dialling the proxy: a peer that cannot complete it
	// never reaches the proxy.
	_ = tc.SetDeadline(time.Now().Add(frontHandshakeTimeout))
	if err := tc.Handshake(); err != nil {
		return
	}
	_ = tc.SetDeadline(time.Time{})
	up, err := net.DialTimeout("tcp", proxyHost, relayDialTimeout)
	if err != nil {
		return
	}
	defer up.Close()
	// Mark before a byte is sent: the proxy reads no request from up until
	// the splice starts. The mark outlives this relay until the proxy's
	// disconnect of the connection (sink.ClientDisconnected) drops it, so a
	// request the proxy read but has not dialled for yet is still judged as a
	// remote client's. Should the proxy drop the connection before the mark is
	// stored, the mark stays behind and only makes a later loopback client on
	// the same port remote: it fails closed.
	f.t.sink.fronted.Store(up.LocalAddr().String(), struct{}{})
	f.conns.splice(tc, up)
}

// loopbackDialHost is the host:port to dial the proxy at proxyURL from this
// host: an unspecified bind address (0.0.0.0, ::) is dialled on loopback.
func loopbackDialHost(proxyURL string) (string, error) {
	u, err := url.Parse(proxyURL)
	if err != nil || u.Host == "" {
		return "", errors.New("proxy TLS front: the recording proxy address does not parse")
	}
	host, port, err := net.SplitHostPort(u.Host)
	if err != nil {
		return "", err
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsUnspecified() {
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, port), nil
}
