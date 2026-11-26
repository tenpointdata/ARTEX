package traffic

import (
	"io"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/Autumn-27/artex/egress"
)

// socks5AuthFixture starts a SOCKS5 listener (cfg.Addr "127.0.0.1:0" unless
// set) with a recording dial spy, plus a TCP target that accepts and closes.
type socks5AuthFixture struct {
	tr   *Traffic
	addr string
	host string
	port int
	spy  *scopeSpy
}

func newSOCKS5AuthFixture(t *testing.T, cfg SOCKS5Config) *socks5AuthFixture {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			_ = c.Close()
		}
	}()
	host, portStr, _ := net.SplitHostPort(ln.Addr().String())
	port, _ := strconv.Atoi(portStr)

	tr, err := Open(t.TempDir(), "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tr.Close() })
	f := &socks5AuthFixture{tr: tr, host: host, port: port, spy: &scopeSpy{}}
	if cfg.Addr == "" {
		cfg.Addr = "127.0.0.1:0"
	}
	cfg.DialContext = f.spy.dial
	cfg.OnRecord = func(RawTCPCapture) {}
	if f.addr, err = tr.StartSOCKS5(cfg); err != nil {
		t.Fatal(err)
	}
	return f
}

// greet sends a greeting offering methods and returns the selected method.
func (f *socks5AuthFixture) greet(t *testing.T, methods ...byte) (net.Conn, byte) {
	t.Helper()
	c, err := net.DialTimeout("tcp", f.addr, 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	_ = c.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := c.Write(append([]byte{socks5Version, byte(len(methods))}, methods...)); err != nil {
		t.Fatal(err)
	}
	var sel [2]byte
	if _, err := io.ReadFull(c, sel[:]); err != nil {
		t.Fatal(err)
	}
	return c, sel[1]
}

func TestSOCKS5OpenLoopbackRejectsWrongCredentials(t *testing.T) {
	f := newSOCKS5AuthFixture(t, SOCKS5Config{})
	for _, cr := range [][2]string{
		{"", ""}, {"alice", "hunter2"}, {egress.PaceUser, "guess"}, {egress.ScopeUser, ""},
		{egress.ScopeUser, egress.ScopeTokenPrefix + "not-registered"},
	} {
		c, status := socks5UserPassAuth(t, f.addr, cr[0], cr[1])
		_ = c.Close()
		if status != socks5UserPassFail {
			t.Errorf("credential %q/%q: status 0x%02x, want 0x01", cr[0], cr[1], status)
		}
	}
	// No-auth from a loopback peer keeps working in this mode.
	c, m := f.greet(t, socks5AuthNone)
	_ = c.Close()
	if m != socks5AuthNone {
		t.Fatalf("no-auth method = 0x%02x, want 0x00", m)
	}
	if got := f.spy.take(); len(got) != 0 {
		t.Fatalf("a refused client dialled %v", got)
	}
}

func TestSOCKS5OperatorCredential(t *testing.T) {
	f := newSOCKS5AuthFixture(t, SOCKS5Config{Username: "op", Password: "s3cret-pass"})

	// Wrong user or password, empty password, and a near-miss are refused.
	for _, cr := range [][2]string{
		{"op", "wrong"}, {"op", ""}, {"other", "s3cret-pass"}, {"op", "s3cret-pass2"}, {"op", "s3cret-pas"},
		{egress.PaceUser, egress.PaceUser}, // the open-mode marker no longer works
	} {
		c, status := socks5UserPassAuth(t, f.addr, cr[0], cr[1])
		_ = c.Close()
		if status != socks5UserPassFail {
			t.Errorf("credential %q/%q: status 0x%02x, want 0x01", cr[0], cr[1], status)
		}
	}
	// No-auth is refused even from loopback.
	c, m := f.greet(t, socks5AuthNone)
	_ = c.Close()
	if m != socks5AuthNoAccept {
		t.Fatalf("no-auth method = 0x%02x, want 0xFF", m)
	}
	if got := f.spy.take(); len(got) != 0 {
		t.Fatalf("a refused client dialled %v", got)
	}

	// The operator credential reaches the global rules, paced via the marker user too.
	for _, user := range []string{"op", egress.PaceUser, egress.ScopeUser} {
		c, rep := socks5DialUserPass(t, f.addr, user, "s3cret-pass", f.host, f.port)
		if rep != socks5RepSuccess {
			t.Fatalf("operator user %q: rep 0x%02x", user, rep)
		}
		_ = c.Close()
	}
	if got := f.spy.take(); len(got) != 3 || got[0].run != "" {
		t.Fatalf("operator dials %v, want three on the global rules", got)
	}

	// A live scope token still names its scope, with no operator credential.
	tok := scopeTokenOf(t, f.tr, "task-auth")
	c2, rep := socks5DialUserPass(t, f.addr, egress.ScopeUser, tok, f.host, f.port)
	if rep != socks5RepSuccess {
		t.Fatalf("scope token: rep 0x%02x", rep)
	}
	_ = c2.Close()
	if got := f.spy.take(); len(got) != 1 || got[0].run != "task-auth" {
		t.Fatalf("scope dials %v, want one on task-auth", got)
	}
	// Forgotten, the token is refused rather than falling back to the operator rules.
	f.tr.ForgetScope("task-auth")
	c3, status := socks5UserPassAuth(t, f.addr, egress.ScopeUser, tok)
	_ = c3.Close()
	if status != socks5UserPassFail {
		t.Fatalf("forgotten token: status 0x%02x, want 0x01", status)
	}
}

// A listener not bound to loopback with no operator credential admits only
// scope-token clients; the policy keys on the bind address, so the check is
// driven through the server value a wildcard bind produces.
func TestSOCKS5NonLoopbackBindWithoutCredentialTokensOnly(t *testing.T) {
	probe, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Skipf("no wildcard bind: %v", err)
	}
	_ = probe.Close()
	f := newSOCKS5AuthFixture(t, SOCKS5Config{Addr: "0.0.0.0:0"})
	_, port, _ := net.SplitHostPort(f.addr)
	f.addr = net.JoinHostPort("127.0.0.1", port)

	c, m := f.greet(t, socks5AuthNone)
	_ = c.Close()
	if m != socks5AuthNoAccept {
		t.Fatalf("no-auth on a wildcard bind = 0x%02x, want 0xFF", m)
	}
	c2, status := socks5UserPassAuth(t, f.addr, egress.PaceUser, egress.PaceUser)
	_ = c2.Close()
	if status != socks5UserPassFail {
		t.Fatalf("pace marker on a wildcard bind: status 0x%02x, want 0x01", status)
	}
	tok := scopeTokenOf(t, f.tr, "task-wild")
	c3, rep := socks5DialUserPass(t, f.addr, egress.ScopeUser, tok, f.host, f.port)
	if rep != socks5RepSuccess {
		t.Fatalf("scope token on a wildcard bind: rep 0x%02x", rep)
	}
	_ = c3.Close()
}

func TestSOCKS5OperatorCredentialFromEnvAndValidation(t *testing.T) {
	t.Setenv(SOCKS5EnabledEnv, "1")
	t.Setenv(SOCKS5UserEnv, "")
	t.Setenv(SOCKS5PasswordEnv, "pw")
	cfg, ok := SOCKS5ConfigFromEnv()
	if !ok || cfg.Username != DefaultSOCKS5User || cfg.Password != "pw" {
		t.Fatalf("env cfg = %+v ok=%v, want default user and password pw", cfg, ok)
	}
	t.Setenv(SOCKS5UserEnv, "me")
	if cfg, _ = SOCKS5ConfigFromEnv(); cfg.Username != "me" {
		t.Fatalf("user = %q, want me", cfg.Username)
	}
	t.Setenv(SOCKS5PasswordEnv, "")
	if cfg, _ = SOCKS5ConfigFromEnv(); cfg.Username != "" || cfg.Password != "" {
		t.Fatalf("a user without a password must configure nothing, got %+v", cfg)
	}
	long := make([]byte, 256)
	for i := range long {
		long[i] = 'a'
	}
	for _, bad := range []SOCKS5Config{
		{Username: "u"}, {Username: "u", Password: string(long)}, {Username: string(long), Password: "p"},
	} {
		if err := bad.validateAuth(); err == nil {
			t.Errorf("validateAuth(%d/%d bytes) accepted", len(bad.Username), len(bad.Password))
		}
	}
}

func TestSecretEqualConstantTimeSemantics(t *testing.T) {
	if !secretEqual("a", "a") || secretEqual("a", "b") || secretEqual("a", "ab") || secretEqual("", "a") {
		t.Fatal("secretEqual wrong")
	}
}

// scopeTokenOf registers a scope for key on tr and returns its token.
func scopeTokenOf(t *testing.T, tr *Traffic, key string) string {
	t.Helper()
	tok, err := tr.scopes.token(key, runScope(key))
	if err != nil {
		t.Fatal(err)
	}
	return tok
}
