package traffic

import (
	"bufio"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"
)

// Credential relays. A client that cannot send a proxy credential (Chromium
// drops the userinfo from --proxy-server and answers only a 407 challenge,
// which ARTEX's recording proxy never issues) cannot carry a scope token to a
// proxy on another host. A CredentialRelay is a loopback listener that stands
// for one upstream HTTP proxy URL with a credential: every request it reads
// from a client is forwarded to that proxy with the URL's credential as
// Proxy-Authorization, replacing any the client sent; a CONNECT (or a 101
// upgrade) the proxy accepts becomes a raw tunnel. It is the worker-node
// counterpart of ScopeRelay, which binds a token by connection address and so
// only works in the proxy's own process.
//
// The relay adds the credential and nothing else: what the upstream proxy
// allows for it (the run's scope, approvals, pacing) is decided there. A URL
// without a credential, or that is not http(s), is an error, so a caller never
// gets a relay that silently drops the scope.

// CredentialRelay is a running credential relay; Close stops it and its
// connections.
type CredentialRelay struct {
	URL string // credential-free proxy URL for the client

	*relayConns
	upstream string // host:port
	tls      bool   // https:// upstream
	auth     string // Proxy-Authorization value
}

// credentialRelayDialTimeout bounds a relay's dial of the upstream proxy.
const credentialRelayDialTimeout = 10 * time.Second

// StartCredentialRelay starts a relay for proxyURL, an http(s) proxy URL that
// carries user:password.
func StartCredentialRelay(proxyURL string) (*CredentialRelay, error) {
	u, err := url.Parse(proxyURL)
	if err != nil || u.Host == "" {
		return nil, errors.New("credential relay: the proxy URL does not parse")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, errors.New("credential relay: the proxy URL must be http or https")
	}
	pass, hasPass := u.User.Password()
	if u.User == nil || !hasPass {
		return nil, errors.New("credential relay: the proxy URL carries no credential")
	}
	host := u.Host
	if u.Port() == "" {
		host = net.JoinHostPort(u.Hostname(), map[string]string{"http": "80", "https": "443"}[u.Scheme])
	}
	base, err := listenRelay()
	if err != nil {
		return nil, err
	}
	cred := base64.StdEncoding.EncodeToString([]byte(u.User.Username() + ":" + pass))
	r := &CredentialRelay{URL: base.url(), relayConns: base, upstream: host, tls: u.Scheme == "https", auth: "Basic " + cred}
	go r.serve(r.relay)
	return r, nil
}

// Close stops the relay and closes its open connections. Safe on nil.
func (r *CredentialRelay) Close() error {
	if r != nil {
		r.close()
	}
	return nil
}

func (r *CredentialRelay) dial() (net.Conn, error) {
	d := &net.Dialer{Timeout: credentialRelayDialTimeout}
	if r.tls {
		host, _, _ := net.SplitHostPort(r.upstream)
		return tls.DialWithDialer(d, "tcp", r.upstream, &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12})
	}
	return d.Dial("tcp", r.upstream)
}

// relay serves one client connection over one upstream connection: request by
// request (keep-alive) until either side closes, or as a raw tunnel once the
// proxy accepts a CONNECT or an upgrade.
func (r *CredentialRelay) relay(down net.Conn) {
	defer down.Close()
	up, err := r.dial()
	if err != nil {
		return
	}
	defer up.Close()
	if !r.track(down) || !r.track(up) {
		return
	}
	defer r.untrack(down)
	defer r.untrack(up)
	downR, upR := bufio.NewReader(down), bufio.NewReader(up)
	for {
		req, err := http.ReadRequest(downR)
		if err != nil {
			return
		}
		tunnel, keep := r.forward(req, down, up, upR)
		if tunnel {
			pipeTunnel(down, downR, up, upR)
			return
		}
		if !keep {
			return
		}
	}
}

// forward sends req upstream with the credential and copies the response (and
// any 1xx before it) back. tunnel reports a CONNECT or upgrade the proxy
// accepted; keep whether the connection may carry another request.
func (r *CredentialRelay) forward(req *http.Request, down, up net.Conn, upR *bufio.Reader) (tunnel, keep bool) {
	req.Header.Set("Proxy-Authorization", r.auth)
	if _, ok := req.Header["User-Agent"]; !ok {
		req.Header["User-Agent"] = []string{""} // send none, as the client did (Write would add Go's)
	}
	var err error
	if req.Method == http.MethodConnect {
		err = req.Write(up) // "CONNECT host:port"
	} else {
		err = req.WriteProxy(up) // absolute-form request line
	}
	if err != nil {
		return false, false
	}
	for {
		resp, err := http.ReadResponse(upR, req)
		if err != nil {
			return false, false
		}
		if (req.Method == http.MethodConnect && resp.StatusCode/100 == 2) || resp.StatusCode == http.StatusSwitchingProtocols {
			if writeHead(down, resp) != nil {
				return false, false
			}
			return true, false
		}
		err = resp.Write(down)
		resp.Body.Close()
		if err != nil {
			return false, false
		}
		if resp.StatusCode/100 == 1 {
			continue // 100 Continue and the like: the final response follows
		}
		return false, !resp.Close && !req.Close && req.Method != http.MethodConnect
	}
}

// writeHead writes a tunnel-opening response's status line and headers only:
// what follows on the connection is the tunnel, not a body.
func writeHead(w io.Writer, resp *http.Response) error {
	bw := bufio.NewWriter(w)
	if _, err := io.WriteString(bw, "HTTP/1.1 "+resp.Status+"\r\n"); err != nil {
		return err
	}
	if err := resp.Header.Write(bw); err != nil {
		return err
	}
	if _, err := io.WriteString(bw, "\r\n"); err != nil {
		return err
	}
	return bw.Flush()
}

// pipeTunnel copies both ways until both directions end, starting with what
// the readers already buffered.
func pipeTunnel(down net.Conn, downR *bufio.Reader, up net.Conn, upR *bufio.Reader) {
	done := make(chan struct{}, 2)
	pipe := func(dst net.Conn, src io.Reader) {
		_, _ = io.Copy(dst, src)
		if cw, ok := dst.(interface{ CloseWrite() error }); ok {
			_ = cw.CloseWrite()
		} else {
			_ = dst.Close()
		}
		done <- struct{}{}
	}
	go pipe(up, downR)
	go pipe(down, upR)
	<-done
	<-done
}
