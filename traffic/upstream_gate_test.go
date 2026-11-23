package traffic

import (
	"bufio"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/Autumn-27/artex/intercept"
)

// dialSpy wraps the gate's floor dialer and records each upstream dial.
type dialSpy struct {
	mu      sync.Mutex
	addrs   []string
	blocked []string
}

func (s *dialSpy) dial(ctx context.Context, network, addr string) (net.Conn, error) {
	c, err := intercept.DialRefusingMetadata(ctx, network, addr)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.addrs = append(s.addrs, addr)
	if _, ok := intercept.IsTargetBlocked(err); ok {
		s.blocked = append(s.blocked, addr)
	}
	return c, err
}

func (s *dialSpy) snapshot() (addrs, blocked []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.addrs...), append([]string(nil), s.blocked...)
}

// The audit case: a target answers 302 to the metadata address and the client
// (curl -L) follows it through the recording proxy. The proxy's upstream dial
// to 169.254.169.254 must be refused, not attempted.
func TestProxiedRedirectToMetadataRefused(t *testing.T) {
	tr, _, proxyURL := startProxy(t)
	spy := &dialSpy{}
	tr.SetUpstreamDialer(spy.dial)

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://169.254.169.254/latest/meta-data/iam/", http.StatusFound)
	}))
	defer target.Close()

	// A fresh proxy connection per request, as curl -L opens when the
	// redirect changes host: go-mitmproxy pins a client connection's plain-HTTP
	// requests to the upstream it dialled first.
	client := proxyClient(t, tr, proxyURL)
	client.Transport.(*http.Transport).DisableKeepAlives = true
	res, err := client.Get(target.URL + "/start")
	if err == nil {
		body, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if res.StatusCode == http.StatusOK {
			t.Fatalf("metadata reached through the proxy: %s", body)
		}
	}
	addrs, blocked := spy.snapshot()
	if len(blocked) != 1 || blocked[0] != "169.254.169.254:80" {
		t.Fatalf("gate dials %v, blocked %v; want 169.254.169.254:80 blocked", addrs, blocked)
	}
	if len(addrs) != 2 || !strings.HasPrefix(addrs[0], "127.0.0.1:") {
		t.Fatalf("the target dial did not go through the gate: %v", addrs)
	}
}

// HTTPS (CONNECT) to a metadata address, in its IPv4-mapped IPv6 form, is
// refused at the upstream dial; so is plain HTTP to the NAT64 form.
func TestProxiedMetadataFormsRefused(t *testing.T) {
	tr, _, proxyURL := startProxy(t)
	spy := &dialSpy{}
	tr.SetUpstreamDialer(spy.dial)
	client := proxyClient(t, tr, proxyURL)
	for _, u := range []string{"https://[::ffff:169.254.169.254]/", "http://[64:ff9b::a9fe:a9fe]/", "http://100.100.100.200/"} {
		if res, err := client.Get(u); err == nil {
			res.Body.Close()
			if res.StatusCode == http.StatusOK {
				t.Fatalf("%s reached through the proxy", u)
			}
		}
	}
	if _, blocked := spy.snapshot(); len(blocked) != 3 {
		t.Fatalf("blocked %v, want all three", blocked)
	}
}

// Without SetUpstreamDialer the floor (intercept.DialRefusingMetadata) still
// stands between the proxy and the metadata address.
func TestUpstreamGateFloorRefusesMetadata(t *testing.T) {
	tr, _, proxyURL := startProxy(t)
	res, err := proxyClient(t, tr, proxyURL).Get("http://169.254.169.254/")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode == http.StatusOK {
		t.Fatal("metadata reached through the proxy")
	}
	if got := gateConnect(t, tr.gate.url, "169.254.169.254:80"); got != http.StatusForbidden {
		t.Fatalf("CONNECT to metadata: status %d, want 403", got)
	}
}

// The gate is not an open relay: without its credential it dials nothing.
func TestUpstreamGateRequiresCredential(t *testing.T) {
	tr, _, _ := startProxy(t)
	spy := &dialSpy{}
	tr.SetUpstreamDialer(spy.dial)
	anon := *tr.gate.url
	anon.User = nil
	if got := gateConnect(t, &anon, "example.com:80"); got != http.StatusProxyAuthRequired {
		t.Fatalf("anonymous CONNECT: status %d, want 407", got)
	}
	wrong := *tr.gate.url
	wrong.User = url.UserPassword("artex", "guess")
	if got := gateConnect(t, &wrong, "example.com:80"); got != http.StatusProxyAuthRequired {
		t.Fatalf("wrong credential: status %d, want 407", got)
	}
	if addrs, _ := spy.snapshot(); len(addrs) != 0 {
		t.Fatalf("unauthenticated CONNECT dialled %v", addrs)
	}
}

// gateConnect sends one CONNECT for target to the gate at u and returns the status.
func gateConnect(t *testing.T, u *url.URL, target string) int {
	t.Helper()
	c, err := net.Dial("tcp", u.Host)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	req := &http.Request{Method: http.MethodConnect, URL: &url.URL{Opaque: target}, Host: target, Header: http.Header{}}
	if u.User != nil {
		p, _ := u.User.Password()
		req.SetBasicAuth(u.User.Username(), p)
		req.Header.Set("Proxy-Authorization", req.Header.Get("Authorization"))
		req.Header.Del("Authorization")
	}
	if err := req.Write(c); err != nil {
		t.Fatal(err)
	}
	res, err := http.ReadResponse(bufio.NewReader(c), req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	return res.StatusCode
}
