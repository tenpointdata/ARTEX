package main

import (
	"crypto/tls"
	"strings"
	"testing"
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
