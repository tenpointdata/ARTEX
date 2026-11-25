package traffic

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"log"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	mcert "github.com/lqqyt2423/go-mitmproxy/cert"
)

// The MITM certificate authority. go-mitmproxy's own CA (cert.SelfSignCA) makes
// an RSA-2048 root valid for three years, lets it sign anything, and reuses the
// root key as every leaf's key. ARTEX supplies its own cert.CA through
// Options.NewCaFunc:
//
//   - a new root is ECDSA P-256, valid one year by default (ARTEX_CA_VALIDITY_DAYS),
//     path-length 0 (it signs leaves only) and limited to the serverAuth EKU;
//   - leaves get a fresh P-256 key each and are valid for days, not a year;
//   - optionally the root carries critical DNS name constraints
//     (ARTEX_CA_PERMITTED_DNS). A MITM CA must sign arbitrary hosts, so this is
//     off by default; where an engagement's domains are known, scope the CA to
//     them: a stolen key then cannot impersonate other sites to a trusting client;
//   - an existing root file is loaded unchanged (RSA or ECDSA, PKCS#8 / PKCS#1 /
//     SEC1) so installed trust keeps working; it only draws a warning when it is
//     RSA-2048 or near expiry. To rotate, stop ARTEX, delete the files named in
//     the warning (mitmproxy-ca*.pem/.cer in the traffic _ca directory), start
//     again, and re-install the new certificate where it is trusted.
//
// File names and formats match go-mitmproxy's (key and certificate together in
// mitmproxy-ca.pem, certificate alone in mitmproxy-ca-cert.pem / .cer) so
// CACertPath, the isolation rules and existing installs are unchanged.

const (
	// CAValidityDaysEnv sets how long a newly generated root is valid (default 365).
	CAValidityDaysEnv = "ARTEX_CA_VALIDITY_DAYS"
	// CAPermittedDNSEnv is a comma-separated list of DNS names a newly generated
	// root is constrained to (itself and its subdomains; ".example.com" means
	// subdomains only). Empty means unconstrained. IP-address leaves are not
	// constrained by it.
	CAPermittedDNSEnv = "ARTEX_CA_PERMITTED_DNS"

	DefaultCAValidity     = 365 * 24 * time.Hour
	DefaultCALeafValidity = 7 * 24 * time.Hour
	caExpiryWarn          = 30 * 24 * time.Hour
	caLeafCacheMax        = 1024
	caKeyFile             = "mitmproxy-ca.pem"
	caCertFile            = "mitmproxy-ca-cert.pem"
	caCerFile             = "mitmproxy-ca-cert.cer"
)

// CAConfig tunes a newly generated CA; a loaded CA keeps what its files say.
type CAConfig struct {
	Validity     time.Duration // root lifetime (default DefaultCAValidity)
	LeafValidity time.Duration // leaf lifetime (default DefaultCALeafValidity)
	PermittedDNS []string      // critical DNS name constraints on the root; empty = none
}

// CAConfigFromEnv reads ARTEX_CA_VALIDITY_DAYS and ARTEX_CA_PERMITTED_DNS.
func CAConfigFromEnv() CAConfig {
	var cfg CAConfig
	if raw := strings.TrimSpace(os.Getenv(CAValidityDaysEnv)); raw != "" {
		if d, err := strconv.Atoi(raw); err == nil && d >= 1 && d <= 3650 {
			cfg.Validity = time.Duration(d) * 24 * time.Hour
		} else {
			log.Printf("[traffic/ca] %s=%q is not a number of days in 1..3650; using %d", CAValidityDaysEnv, raw, int(DefaultCAValidity.Hours()/24))
		}
	}
	for _, d := range strings.Split(os.Getenv(CAPermittedDNSEnv), ",") {
		if d = strings.ToLower(strings.TrimSpace(d)); d != "" {
			cfg.PermittedDNS = append(cfg.PermittedDNS, d)
		}
	}
	return cfg
}

func (c CAConfig) withDefaults() CAConfig {
	if c.Validity <= 0 {
		c.Validity = DefaultCAValidity
	}
	if c.LeafValidity <= 0 {
		c.LeafValidity = DefaultCALeafValidity
	}
	return c
}

// GenerateCA returns a new self-signed root and its key per cfg (see above).
// It touches no files.
func GenerateCA(cfg CAConfig) (*x509.Certificate, *ecdsa.PrivateKey, error) {
	cfg = cfg.withDefaults()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	serial, err := newSerial()
	if err != nil {
		return nil, nil, err
	}
	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   "ARTEX MITM CA " + hex.EncodeToString(serial.Bytes()[:3]),
			Organization: []string{"ARTEX"},
		},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(cfg.Validity),
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            0,
		MaxPathLenZero:        true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	if len(cfg.PermittedDNS) > 0 {
		tmpl.PermittedDNSDomains = cfg.PermittedDNS
		tmpl.PermittedDNSDomainsCritical = true
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, nil, err
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, nil, err
	}
	return cert, key, nil
}

func newSerial() (*big.Int, error) {
	return rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 127))
}

// artexCA is a cert.CA that signs short-lived P-256 leaves with a loaded or
// generated root.
type artexCA struct {
	root    *x509.Certificate
	signer  crypto.Signer
	leafTTL time.Duration

	mu    sync.Mutex
	cache map[string]*tls.Certificate
}

// NewCAFunc returns the go-mitmproxy Options.NewCaFunc that loads the CA in dir
// or, when dir has none, generates one per cfg and stores it (key file 0600).
func NewCAFunc(dir string, cfg CAConfig) func() (mcert.CA, error) {
	return func() (mcert.CA, error) { return LoadOrCreateCA(dir, cfg) }
}

// LoadOrCreateCA loads dir's mitmproxy-ca.pem unchanged, or generates and
// stores a new CA when it does not exist. An unreadable or malformed file is
// an error: it is never overwritten.
func LoadOrCreateCA(dir string, cfg CAConfig) (mcert.CA, error) {
	cfg = cfg.withDefaults()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	keyPath := filepath.Join(dir, caKeyFile)
	data, err := os.ReadFile(keyPath)
	switch {
	case err == nil:
		root, signer, err := parseCAFile(data)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", keyPath, err)
		}
		warnExistingCA(keyPath, root, signer, cfg)
		return &artexCA{root: root, signer: signer, leafTTL: cfg.LeafValidity, cache: map[string]*tls.Certificate{}}, nil
	case !errors.Is(err, os.ErrNotExist):
		return nil, err
	}
	root, key, err := GenerateCA(cfg)
	if err != nil {
		return nil, err
	}
	if err := storeCA(dir, root, key); err != nil {
		return nil, err
	}
	log.Printf("[traffic/ca] generated a new MITM CA (ECDSA P-256, valid until %s) in %s; install %s where clients must trust it",
		root.NotAfter.Format("2006-01-02"), dir, filepath.Join(dir, caCertFile))
	return &artexCA{root: root, signer: key, leafTTL: cfg.LeafValidity, cache: map[string]*tls.Certificate{}}, nil
}

// storeCA writes the key and certificate (0600, via a temp file and rename so a
// crash never leaves a half-written key) and the certificate alone (0644).
func storeCA(dir string, root *x509.Certificate, key *ecdsa.PrivateKey) error {
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return err
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: root.Raw})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	if err := writeFileAtomic(filepath.Join(dir, caKeyFile), append(keyPEM, certPEM...), 0o600); err != nil {
		return err
	}
	if err := writeFileAtomic(filepath.Join(dir, caCertFile), certPEM, 0o644); err != nil {
		return err
	}
	return writeFileAtomic(filepath.Join(dir, caCerFile), certPEM, 0o644)
}

func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".ca-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if err := f.Chmod(perm); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// parseCAFile reads go-mitmproxy's layout: the private key block, then the
// certificate block. RSA and ECDSA keys are accepted in PKCS#8, PKCS#1 or SEC1.
func parseCAFile(data []byte) (*x509.Certificate, crypto.Signer, error) {
	keyBlock, rest := pem.Decode(data)
	if keyBlock == nil {
		return nil, nil, errors.New("no private key block")
	}
	certBlock, _ := pem.Decode(rest)
	if certBlock == nil {
		return nil, nil, errors.New("no certificate block")
	}
	var key crypto.Signer
	if k, err := x509.ParsePKCS8PrivateKey(keyBlock.Bytes); err == nil {
		s, ok := k.(crypto.Signer)
		if !ok {
			return nil, nil, fmt.Errorf("unsupported private key type %T", k)
		}
		key = s
	} else if k, err := x509.ParsePKCS1PrivateKey(keyBlock.Bytes); err == nil {
		key = k
	} else if k, err := x509.ParseECPrivateKey(keyBlock.Bytes); err == nil {
		key = k
	} else {
		return nil, nil, errors.New("unrecognized private key encoding")
	}
	root, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return nil, nil, err
	}
	return root, key, nil
}

// warnExistingCA logs what an operator should rotate: a weak or expiring root.
func warnExistingCA(path string, root *x509.Certificate, key crypto.Signer, cfg CAConfig) {
	rotate := "to rotate: stop ARTEX, delete " + strings.Join([]string{caKeyFile, caCertFile, caCerFile}, ", ") +
		" in " + filepath.Dir(path) + ", restart, and re-install the new certificate"
	if k, ok := key.(*rsa.PrivateKey); ok && k.N.BitLen() <= 2048 {
		log.Printf("[traffic/ca] WARNING the MITM CA is RSA-%d, generated by an older version; kept unchanged so installed trust works; %s", k.N.BitLen(), rotate)
	}
	if left := time.Until(root.NotAfter); left < caExpiryWarn {
		log.Printf("[traffic/ca] WARNING the MITM CA expires %s (in %d days); %s", root.NotAfter.Format("2006-01-02"), int(left.Hours()/24), rotate)
	}
	if len(cfg.PermittedDNS) > 0 && len(root.PermittedDNSDomains) == 0 {
		log.Printf("[traffic/ca] %s applies only to a newly generated CA; the existing one is unconstrained; %s", CAPermittedDNSEnv, rotate)
	}
}

func (c *artexCA) GetRootCA() *x509.Certificate { return c.root }

// GetCert returns a leaf for commonName, cached until a day before it expires.
// A root with DNS name constraints refuses a name outside them: the client
// would reject the leaf anyway, and the error says why.
func (c *artexCA) GetCert(commonName string) (*tls.Certificate, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if cert, ok := c.cache[commonName]; ok && time.Until(leafNotAfter(cert)) > 24*time.Hour {
		return cert, nil
	}
	cert, err := c.leaf(commonName)
	if err != nil {
		return nil, err
	}
	if len(c.cache) >= caLeafCacheMax {
		c.cache = map[string]*tls.Certificate{}
	}
	c.cache[commonName] = cert
	return cert, nil
}

func leafNotAfter(c *tls.Certificate) time.Time {
	if c.Leaf != nil {
		return c.Leaf.NotAfter
	}
	return time.Time{}
}

func (c *artexCA) leaf(commonName string) (*tls.Certificate, error) {
	ip := net.ParseIP(commonName)
	if ip == nil && len(c.root.PermittedDNSDomains) > 0 && !dnsPermitted(commonName, c.root.PermittedDNSDomains) {
		return nil, fmt.Errorf("mitm ca: %q is outside the CA's permitted DNS names", commonName)
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	serial, err := newSerial()
	if err != nil {
		return nil, err
	}
	now := time.Now()
	notAfter := now.Add(c.leafTTL)
	if notAfter.After(c.root.NotAfter) {
		notAfter = c.root.NotAfter
	}
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: commonName, Organization: []string{"ARTEX"}},
		NotBefore:    now.Add(-24 * time.Hour), // clock-skew allowance for tools in other VMs
		NotAfter:     notAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	if ip != nil {
		tmpl.IPAddresses = []net.IP{ip}
	} else {
		tmpl.DNSNames = []string{commonName}
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, c.root, &key.PublicKey, c.signer)
	if err != nil {
		return nil, err
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, err
	}
	return &tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: leaf}, nil
}

// dnsPermitted applies RFC 5280 DNS name constraints as crypto/x509 does: a
// constraint matches the name itself and any subdomain; one with a leading dot
// matches subdomains only.
func dnsPermitted(host string, permitted []string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	for _, p := range permitted {
		p = strings.ToLower(strings.TrimSuffix(p, "."))
		if sub, ok := strings.CutPrefix(p, "."); ok {
			if strings.HasSuffix(host, "."+sub) {
				return true
			}
			continue
		}
		if host == p || strings.HasSuffix(host, "."+p) {
			return true
		}
	}
	return false
}
