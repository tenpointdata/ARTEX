package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"io"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// `artex node-tls` makes a private CA and a control API certificate it signed,
// for deployments without a certificate manager (docker-compose.distributed.yml,
// separate hosts by hand; docs/DISTRIBUTED.md "Transport"). It writes
//
//	<dir>/tls.crt, <dir>/tls.key  the control node's pair (ARTEX_TLS_CERT/KEY)
//	<ca-dir>/ca.crt                the CA for the workers (ARTEX_CONTROL_CA)
//
// The CA's private key is never written: nothing else can be signed with it,
// and a renewal is a new CA (delete the files, rerun, restart the workers).
// Existing files are kept, so the command is safe to run on every start.

const nodeTLSCommand = "node-tls"

func runNodeTLS(args []string, stdout io.Writer) int {
	fs := flag.NewFlagSet(nodeTLSCommand, flag.ContinueOnError)
	dir := fs.String("dir", "", "directory for tls.crt and tls.key (the control node's pair)")
	caDir := fs.String("ca-dir", "", "directory for ca.crt (mounted on worker nodes); default -dir")
	names := fs.String("names", "localhost", "comma-separated DNS names the certificate covers (the control host as workers address it)")
	ips := fs.String("ips", "127.0.0.1", "comma-separated IP addresses the certificate covers")
	days := fs.Int("days", 3650, "validity in days")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *caDir == "" {
		*caDir = *dir
	}
	if err := writeNodeTLS(*dir, *caDir, splitList(*names), splitList(*ips), time.Duration(*days)*24*time.Hour, stdout); err != nil {
		fmt.Fprintln(os.Stderr, "artex node-tls:", err)
		return 1
	}
	return 0
}

func splitList(s string) []string {
	var out []string
	for _, v := range strings.Split(s, ",") {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}

func writeNodeTLS(dir, caDir string, names, ipList []string, validity time.Duration, stdout io.Writer) error {
	if dir == "" {
		return errors.New("-dir is required")
	}
	if len(names) == 0 && len(ipList) == 0 {
		return errors.New("give at least one -names or -ips entry")
	}
	if validity <= 0 {
		return errors.New("-days must be positive")
	}
	certPath, keyPath, caPath := filepath.Join(dir, "tls.crt"), filepath.Join(dir, "tls.key"), filepath.Join(caDir, "ca.crt")
	if fileExists(certPath) && fileExists(keyPath) && fileExists(caPath) {
		fmt.Fprintf(stdout, "node-tls: %s, %s and %s exist; keeping them\n", certPath, keyPath, caPath)
		return nil
	}
	var ips []net.IP
	for _, s := range ipList {
		ip := net.ParseIP(s)
		if ip == nil {
			return fmt.Errorf("-ips: %q is not an IP address", s)
		}
		ips = append(ips, ip)
	}
	caPEM, certPEM, keyPEM, err := issueNodeTLS(names, ips, validity)
	if err != nil {
		return err
	}
	for _, d := range []string{dir, caDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	// The key is the control node's only; the certificates are public.
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		return err
	}
	if err := os.WriteFile(certPath, certPEM, 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(caPath, caPEM, 0o644); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "node-tls: wrote %s, %s and %s (names %v, ips %v)\n", certPath, keyPath, caPath, names, ipList)
	return nil
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// issueNodeTLS returns a fresh CA certificate and a server certificate and key
// it signed (ECDSA P-256, PEM). The CA key is dropped.
func issueNodeTLS(names []string, ips []net.IP, validity time.Duration) (caPEM, certPEM, keyPEM []byte, err error) {
	now := time.Now()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, nil, err
	}
	caTmpl := &x509.Certificate{
		SerialNumber: randomSerial(), Subject: pkix.Name{CommonName: "ARTEX control CA"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(validity),
		IsCA: true, BasicConstraintsValid: true, MaxPathLenZero: true,
		KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	if err != nil {
		return nil, nil, nil, err
	}
	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		return nil, nil, nil, err
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, nil, err
	}
	cn := "artex-control"
	if len(names) > 0 {
		cn = names[0]
	}
	tmpl := &x509.Certificate{
		SerialNumber: randomSerial(), Subject: pkix.Name{CommonName: cn},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(validity),
		DNSNames: names, IPAddresses: ips,
		KeyUsage:    x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, caCert, &key.PublicKey, caKey)
	if err != nil {
		return nil, nil, nil, err
	}
	kder, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, nil, nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER}),
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: kder}), nil
}

func randomSerial() *big.Int {
	n, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 127))
	if err != nil {
		return big.NewInt(time.Now().UnixNano())
	}
	return n
}
