package traffic

import (
	"bytes"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestResolveListenAddr(t *testing.T) {
	t.Setenv("ARTEX_PROXY_ALLOW_NONLOOPBACK", "")
	t.Setenv(ProxyAuthEnv, "")
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

// The listen-address matrix of the two switches: a non-loopback bind needs
// credentials (quiet) or the explicit accepted-risk opt-out (loud); loopback
// never needs either; a malformed ARTEX_PROXY_AUTH does not count as set.
func TestResolveListenAddrAuthMatrix(t *testing.T) {
	const nonLoop = "0.0.0.0:8788"
	for _, tc := range []struct {
		name, auth, allow string
		loopOK, nonLoopOK bool
		warns             bool
	}{
		{"neither", "", "", true, false, false},
		{"auth", "u:p", "", true, true, false},
		{"allow", "", "1", true, true, true},
		{"both: auth wins, no warning", "u:p", "1", true, true, false},
		{"malformed auth is not auth", "nocolon", "", true, false, false},
		{"empty password is not auth", "u:", "", true, false, false},
		{"allow=0", "", "0", true, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(ProxyAuthEnv, tc.auth)
			t.Setenv("ARTEX_PROXY_ALLOW_NONLOOPBACK", tc.allow)
			var logs bytes.Buffer
			log.SetOutput(&logs)
			t.Cleanup(func() { log.SetOutput(os.Stderr) })

			if _, err := ResolveListenAddr("127.0.0.1:8788"); (err == nil) != tc.loopOK {
				t.Errorf("loopback: err=%v want ok=%v", err, tc.loopOK)
			}
			got, err := ResolveListenAddr(nonLoop)
			if (err == nil) != tc.nonLoopOK {
				t.Fatalf("non-loopback: got %q, %v; want ok=%v", got, err, tc.nonLoopOK)
			}
			if err != nil {
				for _, want := range []string{ProxyAuthEnv, "ARTEX_PROXY_ALLOW_NONLOOPBACK"} {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("refusal %q does not mention %s", err, want)
					}
				}
			}
			if warned := strings.Contains(logs.String(), "WARNING"); warned != tc.warns {
				t.Errorf("warning logged = %v, want %v (log %q)", warned, tc.warns, logs.String())
			}
		})
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
