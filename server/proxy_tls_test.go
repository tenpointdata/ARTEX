package server

import (
	"bufio"
	"crypto/tls"
	"io"
	"net"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// tlsLineServer stands in for the control proxy's TLS front: over TLS with
// ca's certificate it answers each line with "got <line>".
func tlsLineServer(t *testing.T, ca testCA) string {
	t.Helper()
	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{ca.leaf}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				line, err := bufio.NewReader(c).ReadString('\n')
				if err != nil {
					return
				}
				_, _ = io.WriteString(c, "got "+line)
			}()
		}
	}()
	return ln.Addr().String()
}

// roundTrip sends a line through the proxy URL's host and returns the answer.
func roundTrip(t *testing.T, proxyURL string) (string, error) {
	t.Helper()
	u, err := url.Parse(proxyURL)
	if err != nil {
		t.Fatal(err)
	}
	c, err := net.DialTimeout("tcp", u.Host, 5*time.Second)
	if err != nil {
		return "", err
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := io.WriteString(c, "hello\n"); err != nil {
		return "", err
	}
	return bufio.NewReader(c).ReadString('\n')
}

// An https:// control proxy is reached through a loopback relay pinned to
// ARTEX_CONTROL_CA: the agents get a plain http://127.0.0.1 URL, and a front
// with a certificate from another CA gets nothing.
func TestControlProxyBaseTLSRelay(t *testing.T) {
	ca := newTestCA(t)
	front := tlsLineServer(t, ca)
	base, relay, err := controlProxyBase("https://"+front, writeCA(t, ca), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { relay.Close() })
	if !strings.HasPrefix(base, "http://127.0.0.1:") || relay == nil {
		t.Fatalf("base %q relay %v, want a loopback relay", base, relay)
	}
	if got, err := roundTrip(t, base); err != nil || got != "got hello\n" {
		t.Fatalf("through the relay: %q, %v", got, err)
	}

	// The certificate is checked against the TLS name when one is set: this
	// one names only 127.0.0.1, so "artex-control" fails.
	named, relayN, err := controlProxyBase("https://"+front, writeCA(t, ca), "artex-control")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { relayN.Close() })
	if got, err := roundTrip(t, named); err == nil && got != "" {
		t.Fatalf("a certificate without the TLS name answered %q", got)
	}

	other := tlsLineServer(t, newTestCA(t))
	base2, relay2, err := controlProxyBase("https://"+other, writeCA(t, ca), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { relay2.Close() })
	if got, err := roundTrip(t, base2); err == nil && got != "" {
		t.Fatalf("a front signed by another CA answered %q", got)
	}
}

func TestControlProxyBaseInputs(t *testing.T) {
	// Plain HTTP stays as it was (with a WARNING), with no relay.
	if base, relay, err := controlProxyBase("http://artex:8788", "", ""); err != nil || base != "http://artex:8788" || relay != nil {
		t.Fatalf("http control proxy: %q %v %v", base, relay, err)
	}
	for _, bad := range []string{"https://artex", "socks5://artex:1080", "://"} {
		if _, relay, err := controlProxyBase(bad, "", ""); err == nil {
			relay.Close()
			t.Fatalf("controlProxyBase(%q) accepted", bad)
		}
	}
	if _, _, err := controlProxyBase("https://artex:8789", filepath.Join(t.TempDir(), "missing.pem"), ""); err == nil {
		t.Fatal("a missing ARTEX_CONTROL_CA was accepted")
	}
}
