package main

import (
	"crypto/tls"
	"fmt"
	"log"
	"os"
	"sync"
	"time"
)

// certReloader serves the listener's certificate and reloads it when the
// certificate or key file changes, so a renewed certificate (cert-manager, a
// rotated Kubernetes Secret) is picked up without a restart. A reload that
// fails keeps the last good pair and logs.
type certReloader struct {
	certFile, keyFile string
	every             time.Duration

	mu        sync.Mutex
	cert      *tls.Certificate
	certMod   time.Time
	keyMod    time.Time
	lastCheck time.Time
}

// newCertReloader loads the pair once; an error here stops start-up.
func newCertReloader(certFile, keyFile string, every time.Duration) (*certReloader, error) {
	r := &certReloader{certFile: certFile, keyFile: keyFile, every: every}
	if err := r.load(); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *certReloader) load() error {
	cm, km, err := r.mtimes()
	if err != nil {
		return err
	}
	c, err := tls.LoadX509KeyPair(r.certFile, r.keyFile)
	if err != nil {
		return fmt.Errorf("load TLS certificate: %w", err)
	}
	r.cert, r.certMod, r.keyMod = &c, cm, km
	return nil
}

func (r *certReloader) mtimes() (time.Time, time.Time, error) {
	cs, err := os.Stat(r.certFile)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	ks, err := os.Stat(r.keyFile)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	return cs.ModTime(), ks.ModTime(), nil
}

// GetCertificate is tls.Config.GetCertificate. It looks at the files at most
// once per r.every.
func (r *certReloader) GetCertificate(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if now := time.Now(); now.Sub(r.lastCheck) >= r.every {
		r.lastCheck = now
		if cm, km, err := r.mtimes(); err != nil {
			log.Printf("[tls] stat certificate: %v (keeping the loaded one)", err)
		} else if !cm.Equal(r.certMod) || !km.Equal(r.keyMod) {
			if err := r.load(); err != nil {
				log.Printf("[tls] reload certificate: %v (keeping the loaded one)", err)
			} else {
				log.Printf("[tls] reloaded certificate %s", r.certFile)
			}
		}
	}
	return r.cert, nil
}
