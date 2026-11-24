package main

import (
	"crypto/tls"
	"strings"
	"testing"

	"github.com/Autumn-27/artex/server"
)

func TestIsLoopbackListenAddr(t *testing.T) {
	for addr, want := range map[string]bool{
		"127.0.0.1:8787": true, "localhost:8787": true, "[::1]:8787": true,
		"0.0.0.0:8787": false, ":8787": false, "192.168.1.5:8787": false, "[::]:8787": false, "bad": false,
	} {
		if got := isLoopbackListenAddr(addr); got != want {
			t.Errorf("isLoopbackListenAddr(%q) = %v, want %v", addr, got, want)
		}
	}
}

func TestCheckListenSecurity(t *testing.T) {
	if _, _, err := checkListenSecurity("0.0.0.0:8787", "", "", false); err == nil || !strings.Contains(err.Error(), "refusing") {
		t.Fatalf("non-loopback without TLS must be refused, got %v", err)
	}
	if useTLS, warn, err := checkListenSecurity("0.0.0.0:8787", "", "", true); err != nil || useTLS || warn == "" {
		t.Fatalf("override must warn: tls=%v warn=%q err=%v", useTLS, warn, err)
	}
	if useTLS, _, err := checkListenSecurity("0.0.0.0:8787", "c.pem", "k.pem", false); err != nil || !useTLS {
		t.Fatalf("TLS on a public bind must pass: tls=%v err=%v", useTLS, err)
	}
	if _, _, err := checkListenSecurity("127.0.0.1:8787", "c.pem", "", false); err == nil {
		t.Fatal("cert without key must fail")
	}
	if useTLS, warn, err := checkListenSecurity("127.0.0.1:8787", "", "", false); err != nil || useTLS || warn != "" {
		t.Fatalf("loopback plain HTTP must pass quietly: %v %q %v", useTLS, warn, err)
	}
}

func TestServerTLSConfigMinVersion(t *testing.T) {
	if serverTLSConfig().MinVersion < tls.VersionTLS12 {
		t.Fatal("minimum TLS version below 1.2")
	}
}

func TestCheckProxyTLS(t *testing.T) {
	cases := []struct {
		name                    string
		front, proxy, cert, key string
		role                    server.NodeRole
		ok                      bool
	}{
		{"off", "", "", "", "", server.RoleControl, true},
		{"control with cert", ":8789", "127.0.0.1:8788", "c.pem", "k.pem", server.RoleControl, true},
		{"single process with cert", "127.0.0.1:8789", "127.0.0.1:8788", "c.pem", "k.pem", server.RoleAll, true},
		{"worker", ":8789", "", "c.pem", "k.pem", server.RoleWorker, false},
		{"no proxy", ":8789", "", "c.pem", "k.pem", server.RoleControl, false},
		{"no cert", ":8789", "127.0.0.1:8788", "", "", server.RoleControl, false},
		{"cert without key", ":8789", "127.0.0.1:8788", "c.pem", "", server.RoleControl, false},
		{"bad address", "8789", "127.0.0.1:8788", "c.pem", "k.pem", server.RoleControl, false},
	}
	for _, c := range cases {
		err := checkProxyTLS(c.front, c.proxy, c.cert, c.key, c.role)
		if (err == nil) != c.ok {
			t.Errorf("%s: err = %v, want ok=%v", c.name, err, c.ok)
		}
	}
}
