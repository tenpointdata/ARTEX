package server

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
)

func TestHostPolicyAllows(t *testing.T) {
	p, err := ParseAllowedHosts(" Artex.Example.com , *.lab.test ,")
	if err != nil {
		t.Fatal(err)
	}
	for host, want := range map[string]bool{
		"localhost:8787": true, "LOCALHOST": true, "artex.localhost:8787": true,
		"127.0.0.1:8787": true, "127.9.9.9": true, "[::1]:8787": true, "192.168.1.5:8787": true,
		"10.0.0.7": true, "[fe80::1]": true,
		"artex.example.com": true, "artex.example.com.:443": true, "a.lab.test": true, "x.y.lab.test:1": true,
		"lab.test": false, "evil.example.com": false, "attacker.test:8787": false, "": false,
		"localhost.evil.test": false, "example.org": false,
	} {
		if got := p.allows(host); got != want {
			t.Errorf("allows(%q) = %v, want %v", host, got, want)
		}
	}
	if (HostPolicy{}).allows("artex.example.com") {
		t.Error("the zero policy must allow only loopback names and IP literals")
	}
}

func TestParseAllowedHostsRejectsWildcardAndJunk(t *testing.T) {
	for _, bad := range []string{"*", "*.", "a.*.b", "host:8787", "http://x", "a b"} {
		if _, err := ParseAllowedHosts(bad); err == nil {
			t.Errorf("ParseAllowedHosts(%q) accepted", bad)
		}
	}
}

func hostGuardReached(s *Server, r *http.Request) (*httptest.ResponseRecorder, bool) {
	reached := false
	w := httptest.NewRecorder()
	s.hostGuard(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { reached = true })).ServeHTTP(w, r)
	return w, reached
}

func TestHostGuardRefusesRebindingHost(t *testing.T) {
	s := &Server{}
	r := httptest.NewRequest("POST", "http://rebind.attacker.test:8787/api/auth/login", nil)
	w, reached := hostGuardReached(s, r)
	if reached || w.Code != http.StatusMisdirectedRequest || !strings.Contains(w.Body.String(), allowedHostsEnv) {
		t.Fatalf("status=%d reached=%v body=%s", w.Code, reached, w.Body.String())
	}
	if _, reached := hostGuardReached(s, httptest.NewRequest("GET", "http://localhost:8787/api/health", nil)); !reached {
		t.Fatal("localhost was refused")
	}
}

func TestHostGuardForwardedHostOnlyFromTrustedProxy(t *testing.T) {
	p, _ := ParseAllowedHosts("artex.example.com")
	s := &Server{allowedHosts: p, trustedProxies: []netip.Prefix{netip.MustParsePrefix("10.0.0.1/32")}}
	req := func(peer, host, xfh string) *http.Request {
		r := httptest.NewRequest("GET", "/api/tasks", nil)
		r.RemoteAddr, r.Host = peer, host
		if xfh != "" {
			r.Header.Set("X-Forwarded-Host", xfh)
		}
		return r
	}
	// A trusted proxy that rewrites Host to its upstream name is judged by X-Forwarded-Host.
	if _, ok := hostGuardReached(s, req("10.0.0.1:5", "artex:8787", "artex.example.com")); !ok {
		t.Error("allowed X-Forwarded-Host from a trusted proxy was refused")
	}
	if _, ok := hostGuardReached(s, req("10.0.0.1:5", "127.0.0.1:8787", "evil.test")); ok {
		t.Error("disallowed X-Forwarded-Host from a trusted proxy got through")
	}
	if _, ok := hostGuardReached(s, req("10.0.0.1:5", "127.0.0.1", "artex.example.com, evil.test")); ok {
		t.Error("every X-Forwarded-Host value must be allowed")
	}
	// From an untrusted peer the header is ignored, both ways.
	if _, ok := hostGuardReached(s, req("192.0.2.4:5", "evil.test", "artex.example.com")); ok {
		t.Error("an untrusted peer chose its host through X-Forwarded-Host")
	}
	if _, ok := hostGuardReached(s, req("192.0.2.4:5", "localhost:8787", "evil.test")); !ok {
		t.Error("an untrusted peer's X-Forwarded-Host must be ignored")
	}
}

func TestHandlerWiresHostGuard(t *testing.T) {
	h := (&Server{}).Handler()
	for _, path := range []string{"/", "/api/health", "/api/auth/login"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", "http://evil.test"+path, nil))
		if w.Code != http.StatusMisdirectedRequest || w.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Fatalf("%s: status=%d headers=%v", path, w.Code, w.Header())
		}
	}
}
