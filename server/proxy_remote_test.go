package server

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Autumn-27/artex/egress"
	"github.com/Autumn-27/artex/traffic"
)

type fakeProxySettings map[string]string

func (f fakeProxySettings) GetSetting(k string) (string, bool, error) {
	v, ok := f[k]
	return v, ok, nil
}

func (f fakeProxySettings) SetSetting(k, v string) error { f[k] = v; return nil }

type fakeTokenStore struct {
	rows   map[string]int64 // token -> task
	fail   error
	forgot []int64
}

func (f *fakeTokenStore) RegisterProxyScopeToken(taskID int64, token, _ string) error {
	if f.fail != nil {
		return f.fail
	}
	if f.rows == nil {
		f.rows = map[string]int64{}
	}
	f.rows[token] = taskID
	return nil
}

func (f *fakeTokenStore) ForgetNodeProxyScopeTokens(taskID int64, _ string) error {
	f.forgot = append(f.forgot, taskID)
	for tok, id := range f.rows {
		if id == taskID {
			delete(f.rows, tok)
		}
	}
	return nil
}

// testCAPEM returns a self-signed CA certificate and its key, PEM-encoded.
func testCAPEM(t *testing.T, isCA bool) (cert, key []byte) {
	t.Helper()
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "test ca"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		IsCA: isCA, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &k.PublicKey, k)
	if err != nil {
		t.Fatal(err)
	}
	kd, err := x509.MarshalECPrivateKey(k)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: kd})
}

// The control node publishes the certificate only; a file holding a key is
// refused; the worker writes it to a readable file it can hand its agents.
func TestProxyCACertPublishFetch(t *testing.T) {
	dir := t.TempDir()
	cert, key := testCAPEM(t, true)
	certPath := filepath.Join(dir, "ca-cert.pem")
	if err := os.WriteFile(certPath, cert, 0o644); err != nil {
		t.Fatal(err)
	}
	st := fakeProxySettings{}
	if err := publishProxyCACert(st, certPath); err != nil {
		t.Fatal(err)
	}
	if got := st[settingCaptureProxyCA]; got != string(cert) {
		t.Fatalf("published %q, want the certificate", got)
	}

	bad := filepath.Join(dir, "with-key.pem")
	if err := os.WriteFile(bad, append(append([]byte{}, cert...), key...), 0o600); err != nil {
		t.Fatal(err)
	}
	st2 := fakeProxySettings{}
	if err := publishProxyCACert(st2, bad); err == nil || st2[settingCaptureProxyCA] != "" {
		t.Fatalf("a file holding a private key was published (err %v)", err)
	}
	leaf, _ := testCAPEM(t, false)
	leafPath := filepath.Join(dir, "leaf.pem")
	_ = os.WriteFile(leafPath, leaf, 0o644)
	if err := publishProxyCACert(fakeProxySettings{}, leafPath); err == nil {
		t.Fatal("a non-CA certificate was published")
	}

	out := filepath.Join(dir, "worker")
	path, err := fetchProxyCACert(st, out)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != string(cert) {
		t.Fatalf("fetched %q (%v)", got, err)
	}
	if fi, _ := os.Stat(path); fi.Mode().Perm() != 0o644 {
		t.Fatalf("CA file mode %v, want 0644", fi.Mode().Perm())
	}
	if _, err := fetchProxyCACert(fakeProxySettings{}, out); err == nil {
		t.Fatal("fetch without a published certificate succeeded")
	}
	if _, err := fetchProxyCACert(fakeProxySettings{settingCaptureProxyCA: string(key)}, out); err == nil {
		t.Fatal("a published key was written as the CA")
	}
}

func scopeTokenOf(t *testing.T, raw string) string {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil || u.User == nil {
		t.Fatalf("URL %q carries no credential", raw)
	}
	if u.User.Username() != egress.ScopeUser {
		t.Fatalf("URL %q user %q, want %q", raw, u.User.Username(), egress.ScopeUser)
	}
	p, _ := u.User.Password()
	tok, ok := egress.ScopeToken(p)
	if !ok {
		t.Fatalf("URL %q carries no scope token", raw)
	}
	return tok
}

// A worker's task URL carries a registered token, stable per task; a failed
// registration yields a refused (ended) URL, never a token-less one.
func TestRemoteTaskProxyURL(t *testing.T) {
	for _, bad := range []string{"", "127.0.0.1:8788", "http://u:p@ctl:8788", "socks5://ctl:1080", "http://ctl:8788/x"} {
		if _, err := newRemoteTaskProxy(&fakeTokenStore{}, bad, "w1"); err == nil {
			t.Errorf("control proxy URL %q accepted", bad)
		}
	}
	store := &fakeTokenStore{}
	p, err := newRemoteTaskProxy(store, "http://artex-control:8788", "w1")
	if err != nil {
		t.Fatal(err)
	}
	u1 := p.URL(5)
	tok := scopeTokenOf(t, u1)
	if !strings.HasPrefix(u1, "http://artex-scope:") || !strings.HasSuffix(u1, "@artex-control:8788") {
		t.Fatalf("URL %q", u1)
	}
	if len(tok) != len(egress.ScopeTokenPrefix)+32 {
		t.Fatalf("token %q is not prefix + 32 hex", tok)
	}
	if store.rows[tok] != 5 {
		t.Fatal("the token was not registered for its task")
	}
	if p.URL(5) != u1 {
		t.Fatal("the token changed between calls for one task")
	}
	if scopeTokenOf(t, p.URL(6)) == tok {
		t.Fatal("two tasks share a token")
	}

	p.Forget(5)
	if _, held := store.rows[tok]; held {
		t.Fatal("Forget left the token registered")
	}

	store.fail = errors.New("database down")
	ended := p.URL(7)
	et := scopeTokenOf(t, ended)
	if !strings.HasPrefix(et, egress.ScopeTokenPrefix+"ended-") {
		t.Fatalf("failed registration URL %q, want an ended-scope token", ended)
	}
	if _, held := store.rows[et]; held {
		t.Fatal("an ended token was registered")
	}
	scopeTokenOf(t, p.Ended())
}

// The control node's resolver refuses a lookup error, an unknown token and a
// task it has not loaded; a loaded task resolves to its ID and a ScopeFunc.
func TestRemoteScopeResolver(t *testing.T) {
	task := &Task{ID: "12"}
	lookupErr := errors.New("db down")
	lookup := func(tok string) (int64, bool, error) {
		switch tok {
		case "scope-err":
			return 0, false, lookupErr
		case "scope-gone":
			return 13, true, nil
		case "scope-ok":
			return 12, true, nil
		}
		return 0, false, nil
	}
	loaded := func(id string) (*Task, bool) {
		if id == task.ID {
			return task, true
		}
		return nil, false
	}
	r := remoteScopeResolver(lookup, loaded)
	for _, tok := range []string{"scope-err", "scope-gone", "scope-unknown"} {
		if _, _, ok := r(tok); ok {
			t.Errorf("token %s resolved", tok)
		}
	}
	key, fn, ok := r("scope-ok")
	if !ok || key != "12" || fn == nil || fn(context.Background()) == nil {
		t.Fatalf("loaded task: key %q ok %v", key, ok)
	}
}

// The recording proxy's real CA file is publishable as is: certificate only.
func TestPublishRecordingProxyCA(t *testing.T) {
	tr, err := traffic.Open(t.TempDir(), "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer tr.Close()
	st := fakeProxySettings{}
	if err := publishProxyCACert(st, tr.CACertPath()); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(st[settingCaptureProxyCA], "PRIVATE") {
		t.Fatal("the published CA holds a private key")
	}
}
