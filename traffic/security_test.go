package traffic

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestResolveListenAddr(t *testing.T) {
	t.Setenv("ARTEX_PROXY_ALLOW_NONLOOPBACK", "")
	for in, want := range map[string]string{
		":8788": "127.0.0.1:8788", "127.0.0.1:1": "127.0.0.1:1", "[::1]:2": "[::1]:2", "localhost:3": "localhost:3",
	} {
		if got, err := ResolveListenAddr(in); err != nil || got != want {
			t.Errorf("%q: got %q, %v want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"0.0.0.0:8788", "192.168.1.5:8788", "example.com:80", "bad"} {
		if _, err := ResolveListenAddr(in); err == nil {
			t.Errorf("%q should be refused", in)
		}
	}
	t.Setenv("ARTEX_PROXY_ALLOW_NONLOOPBACK", "1")
	if _, err := ResolveListenAddr("0.0.0.0:8788"); err != nil {
		t.Errorf("override: %v", err)
	}
}

func TestInsecureUpstreamDefaultsOff(t *testing.T) {
	t.Setenv("ARTEX_CAPTURE_INSECURE_UPSTREAM", "")
	if InsecureUpstream() {
		t.Error("must default to verifying")
	}
	t.Setenv("ARTEX_CAPTURE_INSECURE_UPSTREAM", "1")
	if !InsecureUpstream() {
		t.Error("opt-in ignored")
	}
}

func TestOpenRestrictsIndex(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix permission bits")
	}
	dir := t.TempDir()
	idx := filepath.Join(dir, "_index")
	if err := os.MkdirAll(idx, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(idx, "index.sqlite"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	tr, err := Open(dir, "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer tr.Close()
	for _, p := range []string{"index.sqlite", "index.sqlite-wal", "index.sqlite-shm"} {
		if fi, err := os.Stat(filepath.Join(idx, p)); err == nil && fi.Mode().Perm() != 0o600 {
			t.Errorf("%s mode %o", p, fi.Mode().Perm())
		}
	}
	if fi, _ := os.Stat(idx); fi.Mode().Perm() != 0o700 {
		t.Errorf("_index mode %o", fi.Mode().Perm())
	}
}

// The suite's TLS targets are self-signed httptest servers, so the operator
// opt-in is set for the package; TestInsecureUpstreamDefaultsOff clears it.
func TestMain(m *testing.M) {
	os.Setenv("ARTEX_CAPTURE_INSECURE_UPSTREAM", "1")
	os.Exit(m.Run())
}
