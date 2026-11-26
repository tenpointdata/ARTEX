package traffic

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Autumn-27/artex/egress"
	"github.com/Autumn-27/artex/intercept"
)

// DialFunc opens an outbound connection, as net.Dialer.DialContext does.
type DialFunc func(ctx context.Context, network, addr string) (net.Conn, error)

// The upstream gate. go-mitmproxy (v1.9.2) dials its upstream itself, with a
// plain net.Dialer, and exposes no dial hook. The server runs as root, so the
// container egress filter (egress.sh, which covers only the executor uid) does
// not apply to those dials: `curl -L` through the proxy following a 302 to
// 169.254.169.254 reached the metadata endpoint. What go-mitmproxy does expose
// is the upstream proxy URL (SetUpstreamProxy), consulted by every upstream
// dial it makes (intercepted, tunnelled, websocket, separate-client). So when
// no operator egress proxy is configured, the recording proxy's upstream is
// this in-process loopback HTTP proxy, which opens each connection with a
// DialFunc that applies the platform's checks (in production
// intercept.Interceptor.DialSOCKS5: asset block rules and the metadata deny,
// dialling only the addresses it checked). It is never a direct dial.
//
// The listener is bound to loopback and requires a per-process random
// credential (the password), so it is not one more unrecorded relay for local
// users. The user names the scope of the dial (scope.go): gateUnscoped for the
// global rules only, else the scope token of the client connection, whose
// ScopeFunc derives the dial's context. A token the registry no longer holds is
// refused.

// gateDialTimeout bounds one upstream dial through the gate.
const gateDialTimeout = 30 * time.Second

// gateUnscoped is the gate user of a dial that names no scope.
const gateUnscoped = "artex"

type upstreamGate struct {
	ln     net.Listener
	srv    *http.Server
	url    *url.URL // http://artex:<secret>@127.0.0.1:<port>
	secret string   // the expected password
	scope  func(token string) (ScopeFunc, bool)
	dial   atomic.Pointer[DialFunc]
	ctx    context.Context
	stop   context.CancelFunc
	conns  sync.WaitGroup
}

// startUpstreamGate listens on a free loopback port and serves CONNECT (and
// absolute-form plain HTTP) with dial. A nil dial is intercept.DialRefusingMetadata.
// scope resolves the scope token a dial names; nil resolves none.
func startUpstreamGate(dial DialFunc, scope func(token string) (ScopeFunc, bool)) (*upstreamGate, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		ln.Close()
		return nil, err
	}
	if scope == nil {
		scope = func(string) (ScopeFunc, bool) { return nil, false }
	}
	secret := hex.EncodeToString(raw)
	ctx, stop := context.WithCancel(context.Background())
	g := &upstreamGate{
		ln:     ln,
		url:    &url.URL{Scheme: "http", User: url.UserPassword(gateUnscoped, secret), Host: ln.Addr().String()},
		secret: secret,
		scope:  scope,
		ctx:    ctx,
		stop:   stop,
	}
	g.setDial(dial)
	g.srv = &http.Server{Handler: g, ReadHeaderTimeout: 30 * time.Second, ErrorLog: log.New(io.Discard, "", 0)}
	go func() { _ = g.srv.Serve(ln) }()
	return g, nil
}

func (g *upstreamGate) setDial(dial DialFunc) {
	if dial == nil {
		dial = intercept.DialRefusingMetadata
	}
	g.dial.Store(&dial)
}

func (g *upstreamGate) close() {
	g.stop()
	_ = g.srv.Close()
	g.conns.Wait()
}

// refuse answers a blocked or failed dial. go-mitmproxy turns the status text
// of a refused CONNECT into its error, so the reason goes there, on one line.
func refuse(w http.ResponseWriter, code int, err error) {
	reason := strings.NewReplacer("\r", " ", "\n", " ").Replace(err.Error())
	if _, blocked := intercept.IsTargetBlocked(err); blocked {
		code = http.StatusForbidden
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Artex-Upstream-Refused", reason)
	w.WriteHeader(code)
	_, _ = io.WriteString(w, reason+"\n")
}

func (g *upstreamGate) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	user, pass, ok := egress.ProxyCredential(r.Header.Get("Proxy-Authorization"))
	if !ok || subtle.ConstantTimeCompare([]byte(pass), []byte(g.secret)) != 1 {
		w.Header().Set("Proxy-Authenticate", `Basic realm="artex-upstream"`)
		http.Error(w, "proxy authentication required", http.StatusProxyAuthRequired)
		return
	}
	dial := *g.dial.Load()
	ctx, cancel := context.WithTimeout(g.ctx, gateDialTimeout)
	defer cancel()
	if user != gateUnscoped {
		fn, known := g.scope(user)
		if !known {
			refuse(w, http.StatusForbidden, errScopeUnknown)
			return
		}
		dial = scopedDial(dial, fn)
	}
	if r.Method != http.MethodConnect {
		g.forward(w, r, dial)
		return
	}
	up, err := dial(ctx, "tcp", r.Host)
	if err != nil {
		// The address is not logged: the traffic log never names a target
		// (rewrite_flow.go). A block is recorded by the interceptor itself.
		if code, blocked := intercept.IsTargetBlocked(err); blocked {
			log.Printf("[traffic] an upstream connection was refused by the egress checks (%s)", code)
		}
		refuse(w, http.StatusBadGateway, err)
		return
	}
	hj, ok := w.(http.Hijacker)
	if !ok {
		up.Close()
		refuse(w, http.StatusInternalServerError, errors.New("connection cannot be hijacked"))
		return
	}
	down, buf, err := hj.Hijack()
	if err != nil {
		up.Close()
		return
	}
	if _, err := io.WriteString(down, "HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
		up.Close()
		down.Close()
		return
	}
	g.conns.Add(1)
	go g.splice(down, buf.Reader, up)
}

// errScopeUnknown refuses a dial whose client named a scope that has ended.
var errScopeUnknown = errors.New("the proxy credential names a run scope that is not active (the run ended); refusing the connection")

// scopedDial is dial on contexts derived by fn.
func scopedDial(dial DialFunc, fn ScopeFunc) DialFunc {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		return dial(fn(ctx), network, addr)
	}
}

// splice copies both ways until either side ends or the gate closes.
func (g *upstreamGate) splice(down net.Conn, pending io.Reader, up net.Conn) {
	defer g.conns.Done()
	done := make(chan struct{}, 2)
	go func() { _, _ = io.Copy(up, io.MultiReader(pending, down)); done <- struct{}{} }()
	go func() { _, _ = io.Copy(down, up); done <- struct{}{} }()
	select {
	case <-done:
	case <-g.ctx.Done():
	}
	down.Close()
	up.Close()
	<-done
}

// forward relays an absolute-form request (go-mitmproxy's separate client sends
// plain-HTTP requests to its upstream proxy this way) over a gated dial.
func (g *upstreamGate) forward(w http.ResponseWriter, r *http.Request, dial DialFunc) {
	if !r.URL.IsAbs() || r.URL.Scheme != "http" {
		refuse(w, http.StatusBadRequest, errors.New("only CONNECT and absolute http:// requests are relayed"))
		return
	}
	out := r.Clone(r.Context())
	out.RequestURI = ""
	out.Header.Del("Proxy-Authorization")
	out.Header.Del("Proxy-Connection")
	tr := &http.Transport{DialContext: dial, DisableCompression: true, DisableKeepAlives: true}
	defer tr.CloseIdleConnections()
	res, err := tr.RoundTrip(out)
	if err != nil {
		refuse(w, http.StatusBadGateway, err)
		return
	}
	defer res.Body.Close()
	for k, vs := range res.Header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(res.StatusCode)
	_, _ = io.Copy(w, res.Body)
}

// SetUpstreamDialer sets how the recording proxy opens its upstream
// connections when no global egress proxy is configured. The server passes
// intercept.Interceptor.DialSOCKS5 (asset block rules and the metadata deny);
// nil restores the floor, intercept.DialRefusingMetadata. Safe at any time;
// it applies to the next connection.
func (t *Traffic) SetUpstreamDialer(dial DialFunc) {
	if t.gate != nil {
		t.gate.setDial(dial)
	}
}

// upstreamFor is go-mitmproxy's upstream proxy: the operator's global egress
// proxy when one is set (it, not this host, reaches the target), else the gate,
// with the scope token of req's client connection as the gate user.
//
// With a global egress proxy the gate is not on the path, and neither the
// global rules nor a run's are applied to the proxy's dial: the operator's
// proxy reaches the target. That is the intent for a remote egress proxy; a
// proxy on this host would reach what this host reaches, metadata included,
// so OPS.md tells operators to run it with its own deny (decision recorded in
// HANDOFF.md).
//
// A non-loopback client is checked first, while RequireScopeForRemoteClients is
// on (scope_remote.go): without an active scope its dial is refused, on either
// path, so the proxy is no open relay for the network it listens on.
func (t *Traffic) upstreamFor(req *http.Request) (*url.URL, error) {
	tok := ""
	if t.sink != nil {
		tok = t.sink.scopeOf(req)
	}
	if err := t.remoteClientRefusal(req, tok); err != nil {
		return nil, err
	}
	if u := t.upstream.Load(); u != nil {
		return u, nil
	}
	if t.gate == nil {
		return nil, errors.New("the upstream gate is not running; refusing a direct dial")
	}
	if tok != "" {
		u := *t.gate.url
		u.User = url.UserPassword(tok, t.gate.secret)
		return &u, nil
	}
	return t.gate.url, nil
}
