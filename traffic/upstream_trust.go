package traffic

import (
	"strings"
)

// Upstream TLS trust. The recording proxy verifies the certificate of every
// upstream server it connects to, so a tool's package or code download through
// the proxy cannot be tampered with on the way. Pentest targets often present
// self-signed certificates, so verification is skipped for the hosts the trust
// policy names: in production, the hosts an allow rule of an active task names
// (server/upstream_tls.go). The protected hosts below are always verified,
// whatever a rule or ARTEX_CAPTURE_INSECURE_UPSTREAM says: they hold publicly
// trusted certificates, so verifying them never breaks a legitimate capture.

// UpstreamTrustFunc reports whether host (a name or an IP, no port) may be
// reached without verifying its TLS certificate.
type UpstreamTrustFunc func(host string) bool

// protectedUpstreamSuffixes are package registries, code hosts and OS mirrors
// a tool downloads from. A host equal to one, or a subdomain of one, is
// always verified.
var protectedUpstreamSuffixes = []string{
	// Python
	"pypi.org", "pythonhosted.org", "pypi.python.org",
	// JavaScript
	"npmjs.org", "npmjs.com", "yarnpkg.com",
	// Go
	"golang.org", "go.dev", "gopkg.in",
	// Rust, Ruby, PHP, Java, .NET, Perl
	"crates.io", "rubygems.org", "packagist.org", "maven.org", "maven.apache.org",
	"gradle.org", "nuget.org", "cpan.org",
	// Code hosts and container registries
	"github.com", "githubusercontent.com", "githubassets.com", "ghcr.io",
	"gitlab.com", "bitbucket.org", "docker.io", "docker.com", "quay.io",
	// OS package mirrors
	"debian.org", "ubuntu.com", "kali.org", "alpinelinux.org", "fedoraproject.org",
	"archlinux.org", "brew.sh",
}

// ProtectedUpstreamHost reports whether host is a registry, code host or
// mirror whose certificate is always verified.
func ProtectedUpstreamHost(host string) bool {
	h := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
	for _, s := range protectedUpstreamSuffixes {
		if h == s || strings.HasSuffix(h, "."+s) {
			return true
		}
	}
	return false
}

// SetUpstreamTrust sets the policy for which upstream hosts are reached
// without TLS verification; nil verifies every host. Safe at any time; it
// applies to the next handshake.
func (t *Traffic) SetUpstreamTrust(f UpstreamTrustFunc) {
	if f == nil {
		t.trust.Store(nil)
		return
	}
	t.trust.Store(&f)
}

// upstreamInsecure is go-mitmproxy's per-host hook (Options.UpstreamInsecure).
func (t *Traffic) upstreamInsecure(host string) bool {
	if host == "" || ProtectedUpstreamHost(host) {
		return false
	}
	if InsecureUpstream() {
		return true
	}
	if f := t.trust.Load(); f != nil {
		return (*f)(host)
	}
	return false
}
