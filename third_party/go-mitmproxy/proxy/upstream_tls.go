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
// be decided per host. The certificate is checked against the first non-empty
// name of names, or when there is none the ServerName the connection was made
// with (net/http and gorilla/websocket fill it in; it is "" for an IP). It is
// exempt only when the policy exempts every non-empty name: a client that
// sends one host's SNI while connecting to another gets no trust either one
// lacks.
func (proxy *Proxy) upstreamTLSConfig(names ...string) *tls.Config {
	return &tls.Config{
		InsecureSkipVerify: true, // replaced by VerifyConnection below
		KeyLogWriter:       helper.GetTlsKeyLogWriter(),
		VerifyConnection: func(cs tls.ConnectionState) error {
			var hosts []string
			for _, n := range names {
				if n != "" {
					hosts = append(hosts, n)
				}
			}
			if len(hosts) == 0 && cs.ServerName != "" {
				hosts = []string{cs.ServerName}
			}
			return proxy.verifyUpstream(hosts, cs)
		},
	}
}

// verifyUpstream applies crypto/tls's default verification (chain to the
// roots, server-auth usage, name hosts[0]) unless the policy exempts every
// host.
func (proxy *Proxy) verifyUpstream(hosts []string, cs tls.ConnectionState) error {
	if len(hosts) == 0 {
		return errors.New("upstream TLS: no server name to verify the certificate against")
	}
	exempt := true
	for _, h := range hosts {
		exempt = exempt && proxy.upstreamInsecure(h)
	}
	if exempt {
		return nil
	}
	host := hosts[0]
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

// addressHost is the host of a host:port address.
func addressHost(address string) string {
	if h, _, err := net.SplitHostPort(address); err == nil {
		return h
	}
	return address
}
