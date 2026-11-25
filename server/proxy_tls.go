package server

import (
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log"
	"net/url"

	"github.com/Autumn-27/artex/egress"
	"github.com/Autumn-27/artex/traffic"
)

// TLS on the worker→control recording-proxy hop (E7, c-677166;
// docs/DISTRIBUTED.md "Transport"). Agent tools speak plain HTTP to their
// proxy, so a worker node runs a loopback relay (traffic.TLSRelay) that its
// agents use as the proxy, with their scope token, and that dials the control
// node's proxy TLS front (traffic.ServeTLSFront) pinned to ARTEX_CONTROL_CA.
// Plain-HTTP target requests and the token then cross the network inside TLS.
// The relay adds and checks nothing: the scope decision stays with the control
// node's proxy, which treats every front connection as a remote client.

// ServeProxyTLS serves the recording proxy's TLS front on addr with cfg (the
// control API's certificate) until Close. It needs the recording proxy.
func (m *Manager) ServeProxyTLS(addr string, cfg *tls.Config) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.traffic == nil {
		return errors.New("the proxy TLS front needs the recording proxy (-proxy)")
	}
	if m.proxyFront != nil {
		return errors.New("the proxy TLS front is already running")
	}
	f, err := m.traffic.ServeTLSFront(addr, cfg)
	if err != nil {
		return fmt.Errorf("proxy TLS front on %s: %w", addr, err)
	}
	m.proxyFront = f
	log.Printf("[traffic] recording proxy TLS front listening on %s", f.Addr)
	return nil
}

// controlProxyBase returns the proxy URL a worker node's agents use for the
// control node's recording proxy at controlProxy (ARTEX_CONTROL_PROXY), and the
// relay behind it to close at shutdown (nil when there is none).
//
//   - https://host:port: a loopback TLS relay to it, trusting only the CA in
//     caFile when set (else the system roots) and checking the certificate
//     against serverName (empty: host); the agents get the relay's
//     http://127.0.0.1:<port>.
//   - http://host:port: used as is, with a WARNING: the token and plain-HTTP
//     target traffic cross the network in clear text.
func controlProxyBase(controlProxy, caFile, serverName string) (string, io.Closer, error) {
	u, err := url.Parse(controlProxy)
	if err != nil || u.Host == "" {
		return "", nil, fmt.Errorf("control proxy URL %q does not parse", egress.WithoutProxyCredential(controlProxy))
	}
	switch u.Scheme {
	case "http":
		log.Printf("WARNING: worker node reaches the control recording proxy at %s over plain HTTP: plain-HTTP target traffic and the task scope tokens cross the network in clear text; serve the proxy TLS front (-proxy-tls / ARTEX_PROXY_TLS_ADDR on the control node) and set ARTEX_CONTROL_PROXY=https://…", u.Host)
		return controlProxy, nil, nil
	case "https":
	default:
		return "", nil, fmt.Errorf("control proxy URL %q must be http:// or https://", egress.WithoutProxyCredential(controlProxy))
	}
	if u.Port() == "" {
		return "", nil, fmt.Errorf("control proxy URL %q needs an explicit port", egress.WithoutProxyCredential(controlProxy))
	}
	cfg := &tls.Config{MinVersion: tls.VersionTLS12, ServerName: serverName}
	if caFile != "" {
		pool, err := controlCAPool(caFile)
		if err != nil {
			return "", nil, err
		}
		cfg.RootCAs = pool
	}
	r, err := traffic.StartTLSRelay(u.Host, cfg)
	if err != nil {
		return "", nil, fmt.Errorf("control proxy TLS relay: %w", err)
	}
	log.Printf("[traffic] worker agents reach the control recording proxy %s over TLS through %s", u.Host, r.Addr)
	return r.URL, r, nil
}
