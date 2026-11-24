package proxy

// ARTEX patch: per-host upstream TLS verification. Upstream go-mitmproxy
// v1.9.2 has only the global Options.SslInsecure.

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"

	"github.com/lqqyt2423/go-mitmproxy/internal/helper"
)

// upstreamInsecure reports whether the certificate of host is not verified.
func (proxy *Proxy) upstreamInsecure(host string) bool {
	if f := proxy.Opts.UpstreamInsecure; f != nil {
		return f(host)
	}
	return proxy.Opts.SslInsecure
}

// upstreamTLSConfig returns the client config for a connection to an
// upstream server. Verification is done in VerifyConnection, so that it can
// be decided per host: for host, or when host is "" for the ServerName the
// connection was made with (net/http and gorilla/websocket fill it in).
func (proxy *Proxy) upstreamTLSConfig(host string) *tls.Config {
	return &tls.Config{
		InsecureSkipVerify: true, // replaced by VerifyConnection below
		KeyLogWriter:       helper.GetTlsKeyLogWriter(),
		VerifyConnection: func(cs tls.ConnectionState) error {
			name := host
			if name == "" {
				name = cs.ServerName
			}
			return proxy.verifyUpstream(name, cs)
		},
	}
}

// verifyUpstream applies crypto/tls's default verification (chain to the
// roots, server-auth usage, name) unless the policy exempts host.
func (proxy *Proxy) verifyUpstream(host string, cs tls.ConnectionState) error {
	if proxy.upstreamInsecure(host) {
		return nil
	}
	if host == "" {
		return errors.New("upstream TLS: no server name to verify the certificate against")
	}
	if len(cs.PeerCertificates) == 0 {
		return errors.New("upstream TLS: the server sent no certificate")
	}
	opts := x509.VerifyOptions{
		DNSName:       host,
		Roots:         proxy.Opts.UpstreamRootCAs,
		Intermediates: x509.NewCertPool(),
	}
	for _, c := range cs.PeerCertificates[1:] {
		opts.Intermediates.AddCert(c)
	}
	_, err := cs.PeerCertificates[0].Verify(opts)
	return err
}

// upstreamHost is the name a server's certificate is checked against: the SNI
// the client sent, else the host of the address dialled (an IP target).
func upstreamHost(sni, address string) string {
	if sni != "" {
		return sni
	}
	if h, _, err := net.SplitHostPort(address); err == nil {
		return h
	}
	return address
}
