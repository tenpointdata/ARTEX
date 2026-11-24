package traffic

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/Autumn-27/artex/egress"
	mproxy "github.com/lqqyt2423/go-mitmproxy/proxy"
)

// Scoped proxy dials. The upstream gate (upstream_gate.go) applies the global
// egress checks to every dial the recording proxy makes. A run's own rules (a
// task's block rules, its target gate: pins and approvals) need to know whose
// connection it is, and a proxy client says that only through its credential.
// So a run is given a proxy URL carrying a scope token (ScopedProxyURL,
// egress.ScopedProxyURL); the sink binds each client connection that presents
// one to it (bindScope); upstreamFor names the binding to the gate in the
// gate URL's user; and the gate derives the dial's context with the scope's
// ScopeFunc, so intercept.Interceptor.DialSOCKS5 sees the run's TargetScope.
//
// The binding is keyed by the client connection's remote address, the one key
// every request go-mitmproxy hands upstreamFor carries (the CONNECT, each
// request inside an intercepted tunnel, each plain-HTTP request and its
// separate-client retry). A connection without a token stays on the global
// rules, as before; one naming a token that is not registered (a run that
// ended) is refused at the gate.

// ScopeFunc derives the context an upstream dial of a scope runs on. The server
// passes one that adds the task's intercept.TargetScope.
type ScopeFunc func(context.Context) context.Context

// scopeRegistry maps scope keys (task IDs) to their tokens and ScopeFuncs.
type scopeRegistry struct {
	mu      sync.RWMutex
	byKey   map[string]string // key -> token
	byToken map[string]ScopeFunc
}

// token returns key's token, minting it on first use, and sets its ScopeFunc.
func (r *scopeRegistry) token(key string, fn ScopeFunc) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.byKey == nil {
		r.byKey, r.byToken = map[string]string{}, map[string]ScopeFunc{}
	}
	tok, ok := r.byKey[key]
	if !ok {
		raw := make([]byte, 16)
		if _, err := rand.Read(raw); err != nil {
			return "", err
		}
		tok = egress.ScopeTokenPrefix + hex.EncodeToString(raw)
		r.byKey[key] = tok
	}
	r.byToken[tok] = fn
	return tok, nil
}

func (r *scopeRegistry) forget(key string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if tok, ok := r.byKey[key]; ok {
		delete(r.byKey, key)
		delete(r.byToken, tok)
	}
}

func (r *scopeRegistry) lookup(tok string) (ScopeFunc, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	fn, ok := r.byToken[tok]
	return fn, ok && fn != nil
}

// ScopedProxyURL returns the recording proxy's URL carrying key's scope token:
// the upstream dials of a client that uses it run on fn's context. The token is
// stable for key until ForgetScope; a later call replaces fn. On failure (no
// randomness) it returns the plain URL, which keeps the global rules.
func (t *Traffic) ScopedProxyURL(key string, fn ScopeFunc) string {
	tok, err := t.scopes.token(key, fn)
	if err != nil {
		return t.ProxyAddr()
	}
	return egress.ScopedProxyURL(t.ProxyAddr(), tok)
}

// ForgetScope drops key's token. A client still using it is refused from then.
func (t *Traffic) ForgetScope(key string) { t.scopes.forget(key) }

// EndedScopeURL returns the recording proxy's URL carrying a fresh token that
// names no scope, so every upstream dial of a client that uses it is refused.
// It is the URL for a run whose scope has ended: handing it the plain URL
// would downgrade it to the global rules.
func (t *Traffic) EndedScopeURL() string {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		raw = []byte("ended")
	}
	return egress.ScopedProxyURL(t.ProxyAddr(), egress.ScopeTokenPrefix+"ended-"+hex.EncodeToString(raw))
}

// scopeBinding is the scope token a client connection presented.
type scopeBinding struct {
	conn  any // ClientConn.Id, so a stale disconnect never drops a newer binding
	token string
}

// bindScope records the scope token of f's credential, if it has one, for its
// client connection. A request without one leaves the binding as it is: the
// requests inside a tunnel carry no Proxy-Authorization.
func (s *sink) bindScope(f *mproxy.Flow) {
	_, pass, ok := egress.ProxyCredential(f.Request.Header.Get("Proxy-Authorization"))
	if !ok {
		return
	}
	tok, ok := egress.ScopeToken(pass)
	if !ok {
		return
	}
	if key := clientKey(f.ConnContext.ClientConn); key != "" {
		s.scoped.Store(key, scopeBinding{conn: f.ConnContext.ClientConn.Id, token: tok})
	}
}

// unbindScope forgets c's binding when it is still c's.
func (s *sink) unbindScope(c *mproxy.ClientConn) {
	key := clientKey(c)
	if key == "" {
		return
	}
	if v, ok := s.scoped.Load(key); ok && v.(scopeBinding).conn == any(c.Id) {
		s.scoped.CompareAndDelete(key, v)
	}
}

// ClientConnected drops a binding a previous connection from the same address
// may have left.
func (s *sink) ClientConnected(c *mproxy.ClientConn) {
	if key := clientKey(c); key != "" {
		s.scoped.Delete(key)
	}
}

// scopeOf is the scope token bound to the client connection of req ("" for none).
// A scope relay's binding wins over a credential the client sent through it.
func (s *sink) scopeOf(req *http.Request) string {
	if req == nil || req.RemoteAddr == "" {
		return ""
	}
	if v, ok := s.relayed.Load(req.RemoteAddr); ok {
		return v.(string)
	}
	if v, ok := s.scoped.Load(req.RemoteAddr); ok {
		return v.(scopeBinding).token
	}
	return ""
}

func clientKey(c *mproxy.ClientConn) string {
	if c == nil || c.Conn == nil || c.Conn.RemoteAddr() == nil {
		return ""
	}
	return c.Conn.RemoteAddr().String()
}

// unpinHostChange works around go-mitmproxy v1.9.2 pinning a client
// connection's plain-HTTP requests to the upstream connection its first
// request dialled (attacker.initHttpDialFn dials only while ServerConn is
// nil): a later keep-alive request to another host would go to the first
// host's server. Such a request is sent with go-mitmproxy's separate client,
// which dials (through the gate) the host the request names. Requests inside a
// tunnel are left alone: there the CONNECT fixed the server, and a Host header
// (or an absolute-form target, legal inside a TLS tunnel) that differs from it
// is the client's to send; rerouting it would also drop the TLS.
func unpinHostChange(f *mproxy.Flow) {
	raw := f.Request.Raw()
	sc := f.ConnContext.ServerConn
	if raw == nil || sc == nil || f.Request.URL == nil || f.ConnContext.ClientConn.Tls || !absoluteForm(raw.RequestURI) {
		return
	}
	if sc.Address != canonicalAddr(f.Request.URL) {
		f.UseSeparateClient = true
	}
}

// absoluteForm reports whether a request target is absolute-form (RFC 9112
// §3.2.2), as a plain-HTTP request to a proxy is and a request inside a tunnel is not.
func absoluteForm(requestURI string) bool {
	u, err := url.ParseRequestURI(requestURI)
	return err == nil && u.IsAbs() && u.Host != ""
}

// canonicalAddr is host:port of u, with the scheme's default port, as
// go-mitmproxy records ServerConn.Address.
func canonicalAddr(u *url.URL) string {
	port := u.Port()
	if port == "" {
		port = "80"
		if strings.EqualFold(u.Scheme, "https") {
			port = "443"
		}
	}
	return net.JoinHostPort(u.Hostname(), port)
}
