package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writePair writes a self-signed certificate with the given common name and its key.
func writePair(t *testing.T, dir, cn string, mod time.Time) (string, string) {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: cn},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	kder, _ := x509.MarshalPKCS8PrivateKey(key)
	cp, kp := filepath.Join(dir, "tls.crt"), filepath.Join(dir, "tls.key")
	if err := os.WriteFile(cp, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(kp, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: kder}), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{cp, kp} {
		_ = os.Chtimes(p, mod, mod)
	}
	return cp, kp
}

func servedCN(t *testing.T, r *certReloader) string {
	t.Helper()
	c, err := r.GetCertificate(nil)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(c.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	return leaf.Subject.CommonName
}

// A renewed certificate is served without a restart; a broken renewal keeps
// the last good one.
func TestCertReloader(t *testing.T) {
	dir := t.TempDir()
	base := time.Now().Add(-time.Hour)
	cp, kp := writePair(t, dir, "first", base)
	r, err := newCertReloader(cp, kp, 0)
	if err != nil {
		t.Fatal(err)
	}
	if cn := servedCN(t, r); cn != "first" {
		t.Fatalf("served %q, want first", cn)
	}
	writePair(t, dir, "renewed", base.Add(time.Minute))
	if cn := servedCN(t, r); cn != "renewed" {
		t.Fatalf("after renewal served %q, want renewed", cn)
	}
	if err := os.WriteFile(kp, []byte("garbage"), 0o600); err != nil {
		t.Fatal(err)
	}
	if cn := servedCN(t, r); cn != "renewed" {
		t.Fatalf("after a broken renewal served %q, want the last good (renewed)", cn)
	}
	if _, err := newCertReloader(cp, kp, 0); err == nil {
		t.Fatal("a broken pair at start-up was accepted")
	}
}
