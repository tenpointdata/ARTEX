package main

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strings"

	"github.com/Autumn-27/artex/server"
)

// Environment variables that configure the listener. The flags -tls-cert and
// -tls-key take precedence over the first two.
const (
	envTLSCert       = "ARTEX_TLS_CERT"
	envTLSKey        = "ARTEX_TLS_KEY"
	envAllowInsecure = "ARTEX_ALLOW_INSECURE_BIND"
	envProxyTLSAddr  = "ARTEX_PROXY_TLS_ADDR"
)

// isLoopbackListenAddr reports whether addr (host:port) binds only to the
// loopback interface. An empty host (":8787") binds every interface.
func isLoopbackListenAddr(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip, err := netip.ParseAddr(strings.TrimSuffix(strings.TrimPrefix(host, "["), "]"))
	return err == nil && ip.Unmap().IsLoopback()
}

// checkListenSecurity decides whether the console may listen on addr. TLS
// needs both a certificate and a key. Without TLS, a non-loopback address is
// refused unless allowInsecure is set: the session cookie and the operator's
// traffic would cross the network in clear text.
func checkListenSecurity(addr, certFile, keyFile string, allowInsecure bool) (useTLS bool, warn string, err error) {
	if (certFile == "") != (keyFile == "") {
		return false, "", errors.New("TLS needs both a certificate and a key (-tls-cert/-tls-key or ARTEX_TLS_CERT/ARTEX_TLS_KEY)")
	}
	if certFile != "" {
		return true, "", nil
	}
	if isLoopbackListenAddr(addr) {
		return false, "", nil
	}
	if allowInsecure {
		return false, fmt.Sprintf("listening on %s over plain HTTP (%s=1): the session cookie and all traffic are readable on the network; put a TLS reverse proxy or an SSH tunnel in front", addr, envAllowInsecure), nil
	}
	return false, "", fmt.Errorf("refusing to listen on non-loopback address %s without TLS: set -tls-cert/-tls-key (ARTEX_TLS_CERT/ARTEX_TLS_KEY), bind 127.0.0.1 and use an SSH tunnel or TLS reverse proxy, or set %s=1 to accept plain HTTP", addr, envAllowInsecure)
}

// serverTLSConfig is the TLS configuration for the console listener.
func serverTLSConfig() *tls.Config {
	return &tls.Config{
		MinVersion: tls.VersionTLS12,
		CipherSuites: []uint16{
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
			tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
		},
	}
}

// checkProxyTLS decides whether the recording proxy's TLS front (-proxy-tls)
// may start: it serves the -tls-cert/-tls-key certificate, so it needs both,
// and it fronts this process's recording proxy, so it needs one. A worker node
// runs no proxy of its own.
func checkProxyTLS(frontAddr, proxyAddr, certFile, keyFile string, role server.NodeRole) error {
	if frontAddr == "" {
		return nil
	}
	if role == server.RoleWorker {
		return errors.New("-proxy-tls (ARTEX_PROXY_TLS_ADDR) is for the control node; a worker node reaches the control proxy through ARTEX_CONTROL_PROXY=https://…")
	}
	if proxyAddr == "" {
		return errors.New("-proxy-tls needs the recording proxy (-proxy)")
	}
	if certFile == "" || keyFile == "" {
		return errors.New("-proxy-tls serves the control certificate: set -tls-cert/-tls-key (ARTEX_TLS_CERT/ARTEX_TLS_KEY)")
	}
	if _, _, err := net.SplitHostPort(frontAddr); err != nil {
		return fmt.Errorf("-proxy-tls %q: %w", frontAddr, err)
	}
	return nil
}
