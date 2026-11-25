package server

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// testCA is a CA and a 127.0.0.1 server certificate it signed.
type testCA struct {
	cert *x509.Certificate
	leaf tls.Certificate
}

func newTestCA(t *testing.T) testCA {
	t.Helper()
	caKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	caTmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "test control CA"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true,
		BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	caCert, _ := x509.ParseCertificate(caDER)
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: "artex-control"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		IPAddresses: []net.IP{net.IPv4(127, 0, 0, 1)}, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		KeyUsage: x509.KeyUsageDigitalSignature}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, caCert, &key.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	return testCA{cert: caCert, leaf: tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}}
}

// signedFactServer is a control API stand-in over TLS that answers the "fact"
// op with a reply signed under key, as the real node RPC does.
func signedFactServer(t *testing.T, key []byte, ca testCA) *httptest.Server {
	t.Helper()
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := []byte(`{"id":"ex-1","ok":true}`)
		w.Header().Set(hdrReplySig, nodeRPCReplySignature(key, r.Header.Get(hdrNodeNonce), http.StatusOK, body))
		_, _ = w.Write(body)
	}))
	srv.TLS = &tls.Config{Certificates: []tls.Certificate{ca.leaf}}
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return srv
}

// writeCA writes ca's certificate as PEM and returns the path.
func writeCA(t *testing.T, ca testCA, extra ...*pem.Block) string {
	t.Helper()
	var b strings.Builder
	_ = pem.Encode(&b, &pem.Block{Type: "CERTIFICATE", Bytes: ca.cert.Raw})
	for _, blk := range extra {
		_ = pem.Encode(&b, blk)
	}
	p := filepath.Join(t.TempDir(), "ca.crt")
	if err := os.WriteFile(p, []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// A worker with ARTEX_CONTROL_CA reaches a control API whose certificate that
// CA signed, and refuses one signed by any other CA, including the system roots.
func TestControlRPCPinnedCA(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	ca, other := newTestCA(t), newTestCA(t)
	ctl := signedFactServer(t, key, ca)

	call := func(caFile string) error {
		hc, err := controlHTTPClient(ctl.URL, caFile)
		if err != nil {
			return err
		}
		_, _, err = newControlRPC(ctl.URL, "node-tls", func() []byte { return key }, hc).ResponseContaining([]string{"x"}, 0, 1)
		return err
	}
	if err := call(writeCA(t, ca)); err != nil {
		t.Fatalf("pinned to the control CA: %v", err)
	}
	if err := call(writeCA(t, other)); err == nil || !strings.Contains(err.Error(), "certificate") {
		t.Fatalf("pinned to another CA: err = %v, want a certificate error", err)
	}
	if err := call(""); err == nil {
		t.Fatal("no pin: a self-signed control certificate passed the system roots")
	}
}

// The CA file must hold CA certificates only, and a pinned CA demands https://.
func TestControlHTTPClientRefusesMisconfig(t *testing.T) {
	ca := newTestCA(t)
	ctl := signedFactServer(t, []byte("0123456789abcdef0123456789abcdef"), ca)
	withKey := writeCA(t, ca, &pem.Block{Type: "PRIVATE KEY", Bytes: []byte("k")})
	if _, err := controlHTTPClient(ctl.URL, withKey); err == nil {
		t.Error("a CA file holding a private key was accepted")
	}
	if _, err := controlHTTPClient(ctl.URL, filepath.Join(t.TempDir(), "missing.crt")); err == nil {
		t.Error("a missing CA file was accepted")
	}
	if _, err := controlHTTPClient("http://artex-control:8787", writeCA(t, ca)); err == nil {
		t.Error("a pinned CA with an http:// control API was accepted")
	}
	if _, err := controlHTTPClient("http://artex-control:8787", ""); err != nil {
		t.Errorf("plain http without a pin (compat) refused: %v", err)
	}
}

func TestNodeConfigReadsControlCA(t *testing.T) {
	t.Setenv("ARTEX_NODE_ID", "w1")
	t.Setenv("ARTEX_CONTROL_PROXY", "http://c:8788")
	t.Setenv("ARTEX_CONTROL_CA", " /tls/ca.crt ")
	c, err := NodeConfigFromEnv(RoleWorker)
	if err != nil || c.ControlCA != "/tls/ca.crt" {
		t.Fatalf("ControlCA = %q, %v", c.ControlCA, err)
	}
}
