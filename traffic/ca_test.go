package traffic

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func verifyLeaf(t *testing.T, ca interface {
	GetRootCA() *x509.Certificate
}, der []byte, name string) error {
	t.Helper()
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(ca.GetRootCA())
	_, err = leaf.Verify(x509.VerifyOptions{Roots: pool, DNSName: name, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}})
	return err
}

func TestGenerateCAShape(t *testing.T) {
	root, key, err := GenerateCA(CAConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := root.PublicKey.(*ecdsa.PublicKey); !ok || key.Curve != elliptic.P256() {
		t.Fatalf("CA key is %T on %v, want ECDSA P-256", root.PublicKey, key.Curve)
	}
	if !root.IsCA || !root.MaxPathLenZero || root.KeyUsage&x509.KeyUsageCertSign == 0 {
		t.Errorf("not a path-length-0 CA: %+v", root)
	}
	life := root.NotAfter.Sub(time.Now())
	if life < 360*24*time.Hour || life > 366*24*time.Hour {
		t.Errorf("default validity %v, want about one year", life)
	}
	if len(root.PermittedDNSDomains) != 0 {
		t.Errorf("unconstrained by default, got %v", root.PermittedDNSDomains)
	}
	short, _, err := GenerateCA(CAConfig{Validity: 48 * time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	if d := time.Until(short.NotAfter); d > 49*time.Hour || d < 47*time.Hour {
		t.Errorf("configured validity %v, want 48h", d)
	}
	if root.SerialNumber.Cmp(short.SerialNumber) == 0 {
		t.Error("two CAs share a serial")
	}
}

func TestLoadOrCreateCAStoresAndSignsLeaves(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "_ca")
	ca, err := LoadOrCreateCA(dir, CAConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		if fi, err := os.Stat(filepath.Join(dir, caKeyFile)); err != nil || fi.Mode().Perm() != 0o600 {
			t.Errorf("key file: %v %v, want mode 600", fi, err)
		}
		if fi, err := os.Stat(filepath.Join(dir, caCertFile)); err != nil || fi.Mode().Perm()&0o004 == 0 {
			t.Errorf("cert file: %v %v, want world-readable", fi, err)
		}
	}
	for _, name := range []string{"example.com", "a.b.example.org", "127.0.0.1"} {
		c, err := ca.GetCert(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := c.PrivateKey.(*ecdsa.PrivateKey); !ok {
			t.Errorf("leaf key %T, want ECDSA", c.PrivateKey)
		}
		if c.Leaf.PublicKey.(*ecdsa.PublicKey).Equal(ca.GetRootCA().PublicKey) {
			t.Error("a leaf reuses the CA key")
		}
		if ttl := time.Until(c.Leaf.NotAfter); ttl > DefaultCALeafValidity+time.Hour {
			t.Errorf("leaf for %s lives %v, want <= %v", name, ttl, DefaultCALeafValidity)
		}
		if err := verifyLeaf(t, ca, c.Certificate[0], name); err != nil {
			t.Errorf("leaf %s does not chain to the CA: %v", name, err)
		}
	}
	c1, _ := ca.GetCert("example.com")
	c2, _ := ca.GetCert("example.com")
	if c1 != c2 {
		t.Error("a leaf is not cached")
	}

	// A second load returns the same root (the files are kept, not regenerated).
	again, err := LoadOrCreateCA(dir, CAConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if !again.GetRootCA().Equal(ca.GetRootCA()) {
		t.Error("reloading generated a different CA")
	}
	c3, err := again.GetCert("example.com")
	if err != nil {
		t.Fatal(err)
	}
	if err := verifyLeaf(t, ca, c3.Certificate[0], "example.com"); err != nil {
		t.Errorf("a reloaded CA's leaf does not chain to the original: %v", err)
	}
}

func TestCAPermittedDNSConstraints(t *testing.T) {
	ca, err := LoadOrCreateCA(filepath.Join(t.TempDir(), "_ca"), CAConfig{PermittedDNS: []string{"example.com", ".lab.test"}})
	if err != nil {
		t.Fatal(err)
	}
	root := ca.GetRootCA()
	if !root.PermittedDNSDomainsCritical || len(root.PermittedDNSDomains) != 2 {
		t.Fatalf("constraints %v critical=%v", root.PermittedDNSDomains, root.PermittedDNSDomainsCritical)
	}
	for _, ok := range []string{"example.com", "www.example.com", "x.lab.test", "EXAMPLE.com"} {
		c, err := ca.GetCert(ok)
		if err != nil {
			t.Errorf("%s refused: %v", ok, err)
			continue
		}
		if err := verifyLeaf(t, ca, c.Certificate[0], strings.ToLower(ok)); err != nil {
			t.Errorf("%s does not verify under the constrained CA: %v", ok, err)
		}
	}
	for _, bad := range []string{"evil.com", "notexample.com", "lab.test", "example.com.evil.net"} {
		if _, err := ca.GetCert(bad); err == nil {
			t.Errorf("%s was issued outside the constraints", bad)
		}
	}
	// Forcing a leaf outside the constraints (as a stolen key would) fails at the verifier.
	forged := forgeLeaf(t, ca, "evil.com")
	if err := verifyLeaf(t, ca, forged, "evil.com"); err == nil {
		t.Error("a forged out-of-scope leaf verified under a name-constrained CA")
	}
}

// forgeLeaf signs a leaf with the CA's key directly, bypassing GetCert.
func forgeLeaf(t *testing.T, ca interface{ GetRootCA() *x509.Certificate }, name string) []byte {
	t.Helper()
	a := ca.(*artexCA)
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: name},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		DNSNames: []string{name}, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, a.root, &key.PublicKey, a.signer)
	if err != nil {
		t.Fatal(err)
	}
	return der
}

// An existing RSA-2048 CA in go-mitmproxy's format loads unchanged and its
// leaves still chain to it.
func TestLoadExistingRSACAUnchanged(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "_ca")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(42), Subject: pkix.Name{CommonName: "mitmproxy", Organization: []string{"mitmproxy"}},
		NotBefore: time.Now().Add(-48 * time.Hour), NotAfter: time.Now().Add(10 * 24 * time.Hour), // inside the 30-day warning
		BasicConstraintsValid: true, IsCA: true, KeyUsage: x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, _ := x509.MarshalPKCS8PrivateKey(key)
	file := append(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})...)
	path := filepath.Join(dir, caKeyFile)
	if err := os.WriteFile(path, file, 0o600); err != nil {
		t.Fatal(err)
	}

	ca, err := LoadOrCreateCA(dir, CAConfig{PermittedDNS: []string{"example.com"}})
	if err != nil {
		t.Fatal(err)
	}
	if ca.GetRootCA().SerialNumber.Int64() != 42 {
		t.Fatalf("loaded root serial %v, want the existing 42", ca.GetRootCA().SerialNumber)
	}
	if after, _ := os.ReadFile(path); string(after) != string(file) {
		t.Error("the existing CA file was rewritten")
	}
	if _, err := os.Stat(filepath.Join(dir, caCertFile)); err == nil {
		t.Error("loading wrote files beside an existing CA")
	}
	c, err := ca.GetCert("anything.test") // the old root has no constraint, so none is applied
	if err != nil {
		t.Fatal(err)
	}
	if err := verifyLeaf(t, ca, c.Certificate[0], "anything.test"); err != nil {
		t.Errorf("leaf of an RSA CA: %v", err)
	}
	if c.Leaf.NotAfter.After(ca.GetRootCA().NotAfter) {
		t.Error("a leaf outlives its CA")
	}
	if c.Leaf.SignatureAlgorithm != x509.SHA256WithRSA {
		t.Errorf("RSA CA signed with %v", c.Leaf.SignatureAlgorithm)
	}
}

func TestLoadCARefusesMalformedFile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "_ca")
	_ = os.MkdirAll(dir, 0o700)
	path := filepath.Join(dir, caKeyFile)
	if err := os.WriteFile(path, []byte("not a pem"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadOrCreateCA(dir, CAConfig{}); err == nil {
		t.Fatal("a malformed CA file was accepted")
	}
	if b, _ := os.ReadFile(path); string(b) != "not a pem" {
		t.Error("a malformed CA file was overwritten")
	}
}

func TestCAConfigFromEnv(t *testing.T) {
	t.Setenv(CAValidityDaysEnv, "30")
	t.Setenv(CAPermittedDNSEnv, " Example.com, .lab.test ,,")
	cfg := CAConfigFromEnv()
	if cfg.Validity != 30*24*time.Hour || len(cfg.PermittedDNS) != 2 || cfg.PermittedDNS[0] != "example.com" || cfg.PermittedDNS[1] != ".lab.test" {
		t.Fatalf("cfg %+v", cfg)
	}
	t.Setenv(CAValidityDaysEnv, "forever")
	if cfg := CAConfigFromEnv(); cfg.Validity != 0 {
		t.Fatalf("a bad validity must fall back to the default, got %v", cfg.Validity)
	}
}

// Open wires the CA: a fresh data dir gets the new kind, and the proxy still
// signs a handshake with it.
func TestOpenGeneratesECDSACA(t *testing.T) {
	dir := t.TempDir()
	tr, err := Open(dir, "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer tr.Close()
	raw, err := os.ReadFile(tr.CACertPath())
	if err != nil {
		t.Fatal(err)
	}
	blk, _ := pem.Decode(raw)
	root, err := x509.ParseCertificate(blk.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := root.PublicKey.(*ecdsa.PublicKey); !ok {
		t.Fatalf("Open created a %T CA, want ECDSA", root.PublicKey)
	}
	leaf, err := tr.proxy.GetCertificateByCN("example.com")
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(root)
	parsed, _ := x509.ParseCertificate(leaf.Certificate[0])
	if _, err := parsed.Verify(x509.VerifyOptions{Roots: pool, DNSName: "example.com"}); err != nil {
		t.Fatalf("the proxy's leaf does not chain to the CA file: %v", err)
	}
}
