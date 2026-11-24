package traffic

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"io"
	"math/big"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Autumn-27/artex/egress"
)

// testCA is a throwaway CA and a server certificate it signed for 127.0.0.1.
type testCA struct {
	pool   *x509.CertPool
	server tls.Certificate
}

func newTestCA(t *testing.T) testCA {
	t.Helper()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	caTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "test CA"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	caCert, _ := x509.ParseCertificate(caDER)
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: "control"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, caCert, &key.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(caCert)
	return testCA{pool: pool, server: tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}}
}

// tap is a TCP forwarder that keeps every byte it carries in either direction:
// the packet capture of the worker→control hop.
type tap struct {
	ln   net.Listener
	mu   sync.Mutex
	seen bytes.Buffer
}

func startTap(t *testing.T, to string) *tap {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	tp := &tap{ln: ln}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				up, err := net.Dial("tcp", to)
				if err != nil {
					return
				}
				defer up.Close()
				done := make(chan struct{}, 2)
				cp := func(dst, src net.Conn) {
					_, _ = io.Copy(io.MultiWriter(dst, tp), src)
					done <- struct{}{}
				}
				go cp(up, c)
				go cp(c, up)
				<-done
			}()
		}
	}()
	return tp
}

func (tp *tap) Write(p []byte) (int, error) {
	tp.mu.Lock()
	defer tp.mu.Unlock()
	return tp.seen.Write(p)
}

func (tp *tap) bytes() []byte {
	tp.mu.Lock()
	defer tp.mu.Unlock()
	return append([]byte(nil), tp.seen.Bytes()...)
}

// frontPath is the worker→control path: a TLS relay (worker) → tap (the
// network) → TLS front (control) → recording proxy.
func frontPath(t *testing.T, tr *Traffic, ca testCA, relayCfg *tls.Config) (*TLSRelay, *tap) {
	t.Helper()
	front, err := tr.ServeTLSFront("127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{ca.server}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { front.Close() })
	tp := startTap(t, front.Addr)
	relay, err := StartTLSRelay(tp.ln.Addr().String(), relayCfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { relay.Close() })
	return relay, tp
}

// Through the relay and the front a scoped plain-HTTP request reaches the
// target on its scope, and the hop carries neither the request line nor the
// token in clear text. Every hop is on loopback here, yet a token-less client
// is refused: the front's connections are remote clients to the gate.
func TestTLSFrontCarriesScopeEncrypted(t *testing.T) {
	tr, _, _ := startProxy(t)
	spy := &scopeSpy{}
	tr.SetUpstreamDialer(spy.dial)
	target := textServer(t, "ok")
	tr.SetRemoteScopeResolver(newFakeRemote(map[string]string{workerToken(7): "11"}).resolve)
	tr.RequireScopeForRemoteClients(false) // the front must turn it back on
	ca := newTestCA(t)
	relay, tp := frontPath(t, tr, ca, &tls.Config{RootCAs: ca.pool})

	scoped := egress.ScopedProxyURL(relay.URL, workerToken(7))
	if code, body := getBody(t, proxyClient(t, tr, scoped), target.URL+"/secret-path"); code != http.StatusOK || body != "ok" {
		t.Fatalf("scoped request through the front: %d %q", code, body)
	}
	if got := spy.take(); len(got) != 1 || got[0].run != "11" {
		t.Fatalf("dials %v, want one on 11", got)
	}
	// HTTPS targets work too (CONNECT, then the proxy's MITM inside the TLS hop).
	httpsTarget := "https://" + target.Listener.Addr().String() + "/"
	getBody(t, proxyClient(t, tr, scoped), httpsTarget)
	if got := spy.take(); len(got) != 1 || got[0].run != "11" {
		t.Fatalf("CONNECT dials %v, want one on 11", got)
	}

	wire := tp.bytes()
	if len(wire) == 0 {
		t.Fatal("the tap saw no bytes")
	}
	for _, clear := range []string{"secret-path", "GET http", "Proxy-Authorization", workerToken(7), "CONNECT "} {
		if bytes.Contains(wire, []byte(clear)) {
			t.Fatalf("the worker→control hop carries %q in clear text", clear)
		}
	}

	for _, u := range []string{relay.URL, egress.MarkProxyURL(relay.URL)} {
		if code, body := getBody(t, proxyClient(t, tr, u), target.URL); code == http.StatusOK {
			t.Fatalf("a token-less client through the front via %s reached the target: %q", u, body)
		}
	}
	unknown := egress.ScopedProxyURL(relay.URL, workerToken(8))
	if code, _ := getBody(t, proxyClient(t, tr, unknown), target.URL); code == http.StatusOK {
		t.Fatal("a client naming an unknown scope through the front reached the target")
	}
	if got := spy.take(); len(got) != 0 {
		t.Fatalf("refused clients dialled %v", got)
	}
}

// A relay that does not trust the front's CA never delivers a byte to the
// proxy: the handshake fails and the client's request fails.
func TestTLSRelayRefusesUnpinnedFront(t *testing.T) {
	tr, _, _ := startProxy(t)
	spy := &scopeSpy{}
	tr.SetUpstreamDialer(spy.dial)
	target := textServer(t, "ok")
	tr.SetRemoteScopeResolver(newFakeRemote(map[string]string{workerToken(9): "12"}).resolve)
	other := newTestCA(t)
	relay, tp := frontPath(t, tr, newTestCA(t), &tls.Config{RootCAs: other.pool})

	scoped := egress.ScopedProxyURL(relay.URL, workerToken(9))
	if code, _ := getBody(t, proxyClient(t, tr, scoped), target.URL); code == http.StatusOK {
		t.Fatal("a relay pinned to another CA reached the target")
	}
	if got := spy.take(); len(got) != 0 {
		t.Fatalf("dials %v through an unpinned front", got)
	}
	if bytes.Contains(tp.bytes(), []byte(workerToken(9))) {
		t.Fatal("the token crossed the hop in clear text")
	}
}

func TestTLSFrontAndRelayInputs(t *testing.T) {
	tr, _, _ := startProxy(t)
	if _, err := tr.ServeTLSFront("127.0.0.1:0", &tls.Config{}); err == nil {
		t.Fatal("a front without a certificate started")
	}
	if _, err := tr.ServeTLSFront("127.0.0.1:0", nil); err == nil {
		t.Fatal("a front with no TLS config started")
	}
	if _, err := StartTLSRelay("127.0.0.1:1", &tls.Config{InsecureSkipVerify: true}); err == nil || !strings.Contains(err.Error(), "verification") {
		t.Fatalf("a relay skipping verification: %v", err)
	}
	if _, err := StartTLSRelay("no-port", nil); err == nil {
		t.Fatal("a relay to a remote with no port started")
	}
	for in, want := range map[string]string{
		"http://0.0.0.0:8788":   "127.0.0.1:8788",
		"http://127.0.0.1:8788": "127.0.0.1:8788",
		"http://[::]:8788":      "127.0.0.1:8788",
		"http://10.0.0.5:8788":  "10.0.0.5:8788",
	} {
		if got, err := loopbackDialHost(in); err != nil || got != want {
			t.Fatalf("loopbackDialHost(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
}
