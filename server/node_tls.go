package server

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"time"
)

// Transport between worker nodes and the control node's API (E7, c-548e8b;
// docs/DISTRIBUTED.md "Transport"). The node RPC is HMAC-signed both ways, so
// nothing on the path can forge a call or a reply, but its tool/preview
// payloads carry captured target traffic (session cookies, credentials). TLS
// keeps them confidential on the cluster network.
//
// ARTEX_CONTROL_CA pins the control API's certificate authority: the worker
// trusts only the certificates in that file, never the system roots, so a
// publicly valid certificate for the same name does not pass. Without it an
// https:// control API is verified against the system roots, and an http://
// one works but is logged as clear text.

// controlRPCTimeout bounds one node RPC call.
const controlRPCTimeout = 2 * time.Minute

// controlCAPool reads the PEM certificates in caFile. The file must hold at
// least one certificate and nothing else: a private key block is refused, so a
// wrong path cannot be mistaken for the CA.
func controlCAPool(caFile string) (*x509.CertPool, error) {
	raw, err := os.ReadFile(caFile)
	if err != nil {
		return nil, fmt.Errorf("ARTEX_CONTROL_CA: %w", err)
	}
	pemCerts, err := certificatesOnly(raw)
	if err != nil {
		return nil, fmt.Errorf("ARTEX_CONTROL_CA %s: %w", caFile, err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pemCerts) {
		return nil, fmt.Errorf("ARTEX_CONTROL_CA %s: no usable certificate", caFile)
	}
	return pool, nil
}

// controlHTTPClient is the worker's HTTP client for the control API at base
// (scheme://host:port, from controlAPIURL). With caFile it accepts only
// https:// and trusts only that CA.
func controlHTTPClient(base, caFile string) (*http.Client, error) {
	u, err := url.Parse(base)
	if err != nil {
		return nil, err
	}
	if u.Scheme != "https" {
		if caFile != "" {
			return nil, fmt.Errorf("ARTEX_CONTROL_CA is set but ARTEX_CONTROL_API %s is not https://", base)
		}
		log.Printf("WARNING: worker node reaches the control API at %s over plain HTTP: node RPC payloads (captured target traffic) cross the network in clear text; serve the control API with TLS and set ARTEX_CONTROL_API=https://… and ARTEX_CONTROL_CA", base)
		return &http.Client{Timeout: controlRPCTimeout}, nil
	}
	cfg := &tls.Config{MinVersion: tls.VersionTLS12}
	if caFile != "" {
		pool, err := controlCAPool(caFile)
		if err != nil {
			return nil, err
		}
		cfg.RootCAs = pool
	}
	tr, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return nil, errors.New("http.DefaultTransport is not an *http.Transport")
	}
	tr = tr.Clone()
	tr.TLSClientConfig = cfg
	return &http.Client{Timeout: controlRPCTimeout, Transport: tr}, nil
}
