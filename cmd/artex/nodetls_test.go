package main

import (
	"crypto/tls"
	"crypto/x509"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// The pair verifies against the CA for every requested name, the CA key is not
// written, the key file is private, and a second run keeps the files.
func TestWriteNodeTLS(t *testing.T) {
	dir, caDir := t.TempDir(), t.TempDir()
	if err := writeNodeTLS(dir, caDir, []string{"artex", "localhost"}, []string{"127.0.0.1"}, 24*time.Hour, io.Discard); err != nil {
		t.Fatal(err)
	}
	pair, err := tls.LoadX509KeyPair(filepath.Join(dir, "tls.crt"), filepath.Join(dir, "tls.key"))
	if err != nil {
		t.Fatal(err)
	}
	leaf, _ := x509.ParseCertificate(pair.Certificate[0])
	caPEM, _ := os.ReadFile(filepath.Join(caDir, "ca.crt"))
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		t.Fatal("ca.crt does not parse")
	}
	for _, name := range []string{"artex", "localhost", "127.0.0.1"} {
		if _, err := leaf.Verify(x509.VerifyOptions{Roots: pool, DNSName: name}); err != nil {
			t.Errorf("verify for %s: %v", name, err)
		}
	}
	if _, err := leaf.Verify(x509.VerifyOptions{Roots: pool, DNSName: "other"}); err == nil {
		t.Error("certificate verified for a name it does not cover")
	}
	if fi, _ := os.Stat(filepath.Join(dir, "tls.key")); fi.Mode().Perm() != 0o600 {
		t.Errorf("tls.key mode %v, want 0600", fi.Mode().Perm())
	}
	entries, _ := os.ReadDir(caDir)
	if len(entries) != 1 {
		t.Errorf("ca dir holds %d files, want only ca.crt", len(entries))
	}
	before, _ := os.ReadFile(filepath.Join(dir, "tls.crt"))
	if err := writeNodeTLS(dir, caDir, []string{"artex"}, nil, time.Hour, io.Discard); err != nil {
		t.Fatal(err)
	}
	if after, _ := os.ReadFile(filepath.Join(dir, "tls.crt")); string(after) != string(before) {
		t.Error("a second run replaced the existing certificate")
	}
}

func TestWriteNodeTLSRefusesBadInput(t *testing.T) {
	for _, c := range []struct {
		dir   string
		names []string
		ips   []string
		days  time.Duration
	}{
		{"", []string{"a"}, nil, time.Hour},
		{t.TempDir(), nil, nil, time.Hour},
		{t.TempDir(), nil, []string{"not-an-ip"}, time.Hour},
		{t.TempDir(), []string{"a"}, nil, 0},
	} {
		if err := writeNodeTLS(c.dir, c.dir, c.names, c.ips, c.days, io.Discard); err == nil {
			t.Errorf("writeNodeTLS(%+v) accepted", c)
		}
	}
}
