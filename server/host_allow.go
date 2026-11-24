package server

import (
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// HostPolicy is the Host-header allowlist (DNS-rebinding defence). A rebinding
// page is served from the attacker's name and then re-points that name at
// ARTEX, so the browser sends the attacker's name as Host. Refusing names the
// operator did not list stops the page from reading the API or burning the
// login rate limit. Always allowed:
//   - IP literals. A browser sends an IP literal only when the page itself was
//     loaded from that IP, so the origin is the IP and no foreign name is
//     involved: IP-literal Hosts are never a rebinding vector. Allowing them
//     keeps LAN access (http://192.168.1.5:8787) and Docker port publishing
//     working without listing interface addresses.
//   - localhost and *.localhost, which resolve to loopback only (RFC 6761).
//
// Any other name must be listed in ARTEX_ALLOWED_HOSTS. The zero value allows
// only the defaults above.
type HostPolicy struct {
	exact    map[string]bool
	suffixes []string // ".example.com" for "*.example.com"
}

// builtinHostNames are allowed besides IP literals and *.localhost. A var so
// the package tests can add httptest's default host, example.com.
var builtinHostNames = map[string]bool{"localhost": true}

// allowedHostsEnv names the setting in messages to the operator.
const allowedHostsEnv = "ARTEX_ALLOWED_HOSTS"

// ParseAllowedHosts parses ARTEX_ALLOWED_HOSTS: comma-separated host names,
// each an exact name ("artex.example.com") or "*.suffix" for every subdomain
// of suffix (not suffix itself). Ports are not part of an entry. A bare "*" is
// refused: it would switch the defence off.
func ParseAllowedHosts(s string) (HostPolicy, error) {
	p := HostPolicy{exact: map[string]bool{}}
	for _, f := range strings.Split(s, ",") {
		f = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(f)), ".")
		if f == "" {
			continue
		}
		name := strings.TrimPrefix(f, "*.")
		if name == "" || strings.ContainsAny(name, "*:/ []") {
			return HostPolicy{}, fmt.Errorf("%q is not a host name or *.suffix pattern", f)
		}
		if name != f {
			p.suffixes = append(p.suffixes, "."+name)
		} else {
			p.exact[name] = true
		}
	}
	return p, nil
}

// allows reports whether host (a Host header value, port optional) is allowed.
func (p HostPolicy) allows(host string) bool {
	h := strings.TrimSpace(host)
	if hh, _, err := net.SplitHostPort(h); err == nil {
		h = hh
	}
	h = strings.TrimSuffix(strings.ToLower(strings.Trim(h, "[]")), ".")
	if h == "" {
		return false
	}
	if _, err := netip.ParseAddr(h); err == nil {
		return true
	}
	if builtinHostNames[h] || strings.HasSuffix(h, ".localhost") || p.exact[h] {
		return true
	}
	for _, suf := range p.suffixes {
		if strings.HasSuffix(h, suf) {
			return true
		}
	}
	return false
}

// WithAllowedHosts sets the Host allowlist (see HostPolicy).
func WithAllowedHosts(p HostPolicy) Option {
	return func(s *Server) { s.allowedHosts = p }
}

// requestHosts are the names the browser used to reach us. From a trusted
// proxy (the same rule as requestIsSecure) that is X-Forwarded-Host when the
// proxy sets it, since the proxy may rewrite Host to its upstream name; every
// listed value must pass. Otherwise it is Host.
func (s *Server) requestHosts(r *http.Request) []string {
	if xfh := r.Header.Values("X-Forwarded-Host"); len(xfh) > 0 {
		if addr, err := netip.ParseAddr(peerIP(r)); err == nil && isTrusted(addr, s.trustedProxies) {
			return strings.Split(strings.Join(xfh, ","), ",")
		}
	}
	return []string{r.Host}
}

// hostGuard refuses, with 421, a request whose Host is not allowed.
func (s *Server) hostGuard(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, host := range s.requestHosts(r) {
			if !s.allowedHosts.allows(host) {
				name := strings.TrimSpace(host)
				if len(name) > 255 {
					name = name[:255]
				}
				writeErr(w, http.StatusMisdirectedRequest, fmt.Sprintf(
					"host %q is not allowed (DNS-rebinding protection). ARTEX answers to localhost, IP addresses and the names in %s; to use this name, add it to %s (comma-separated, *.example.com matches subdomains) and restart",
					name, allowedHostsEnv, allowedHostsEnv))
				return
			}
		}
		h.ServeHTTP(w, r)
	})
}
