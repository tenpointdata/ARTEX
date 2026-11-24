package traffic

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
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
// credential, so it is not one more unrecorded relay for local users.

// gateDialTimeout bounds one upstream dial through the gate.
const gateDialTimeout = 30 * time.Second

type upstreamGate struct {
	ln    net.Listener
	srv   *http.Server
	url   *url.URL // http://artex:<token>@127.0.0.1:<port>
	auth  string   // expected Proxy-Authorization value
	dial  atomic.Pointer[DialFunc]
	ctx   context.Context
	stop  context.CancelFunc
	conns sync.WaitGroup
}

// startUpstreamGate listens on a free loopback port and serves CONNECT (and
// absolute-form plain HTTP) with dial. A nil dial is intercept.DialRefusingMetadata.
func startUpstreamGate(dial DialFunc) (*upstreamGate, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		ln.Close()
		return nil, err
	}
	user := url.UserPassword("artex", hex.EncodeToString(raw))
	ctx, stop := context.WithCancel(context.Background())
	g := &upstreamGate{
		ln:   ln,
		url:  &url.URL{Scheme: "http", User: user, Host: ln.Addr().String()},
		auth: "Basic " + base64.StdEncoding.EncodeToString([]byte(user.String())),
		ctx:  ctx,
		stop: stop,
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
	if subtle.ConstantTimeCompare([]byte(r.Header.Get("Proxy-Authorization")), []byte(g.auth)) != 1 {
		w.Header().Set("Proxy-Authenticate", `Basic realm="artex-upstream"`)
		http.Error(w, "proxy authentication required", http.StatusProxyAuthRequired)
		return
	}
	dial := *g.dial.Load()
	ctx, cancel := context.WithTimeout(g.ctx, gateDialTimeout)
	defer cancel()
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
// proxy when one is set (it, not this host, reaches the target), else the gate.
//
// Upstream connections are not shared across client connections: go-mitmproxy
// dials one per client connection (TestUpstreamConnectionsNotSharedAcrossClients).
// Its one cross-client pool, the separate client's http.Transport, is used only
// when an addon changes a request's host or scheme or sets UseSeparateClient,
// which no ARTEX addon does, and it keys idle connections by the URL returned
// here. So a per-client scope added later must return a distinct gate URL
// (credential) per scope: that both tells the gate the scope and keeps the
// pool from carrying one scope's connection into another's request.
func (t *Traffic) upstreamFor(*http.Request) (*url.URL, error) {
	if u := t.upstream.Load(); u != nil {
		return u, nil
	}
	if t.gate == nil {
		return nil, errors.New("the upstream gate is not running; refusing a direct dial")
	}
	return t.gate.url, nil
}
