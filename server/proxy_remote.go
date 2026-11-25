package server

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/Autumn-27/artex/egress"
	"github.com/Autumn-27/artex/intercept"
	"github.com/Autumn-27/artex/traffic"
)

// Worker nodes and the recording proxy (E7, docs/DISTRIBUTED.md). A worker
// node has no capture store of its own: its agents' target traffic goes
// through the control node's recording proxy, the same path as a local
// worker's, so it is recorded on the control node, passes the upstream gate
// (asset block rules, metadata deny), the task's TargetScope (pins, approvals)
// and the noise governor. The pieces:
//
//   - control: remoteScopeResolver resolves a scope token a worker minted (its
//     hash is in proxy_scope_tokens) to the control node's own loaded task, with
//     the same ScopeFunc TaskProxyAddr hands a local worker; publishProxyCACert
//     puts the MITM CA certificate (never its key) in settings;
//   - worker: fetchProxyCACert writes that certificate to a local file, the
//     worker's ProxyCACert; remoteTaskProxy mints and registers a task's token
//     and returns the control proxy URL carrying it.
//
// Every failure yields a URL whose dials the control proxy refuses
// (endedProxyURL), never a token-less URL (a non-loopback client without a
// token is refused, traffic.RequireScopeForRemoteClients) and never "" (no
// proxy: a direct dial, unrecorded and outside the gate).

// settingCaptureProxyCA is the settings key of the control node's MITM CA
// certificate (PEM). It is public material and is stored unsealed: it is not
// in secretSettingKeys (db/secrets.go), and only the certificate is written.
const settingCaptureProxyCA = "capture_proxy_ca_pem"

// proxySettings is the part of *db.DB the CA helpers use.
type proxySettings interface {
	GetSetting(key string) (string, bool, error)
	SetSetting(key, value string) error
}

// proxyTokenStore is the part of *db.DB remoteTaskProxy uses.
type proxyTokenStore interface {
	RegisterProxyScopeToken(taskID int64, token, nodeID string) error
	ForgetNodeProxyScopeTokens(taskID int64, nodeID string) error
}

// remoteScopeResolver resolves a worker-minted scope token: lookup maps it to a
// task ID (the database), task returns the task when this node has it loaded.
// A lookup error, an unknown token and a task that is not loaded all refuse.
func remoteScopeResolver(lookup func(token string) (int64, bool, error), task func(id string) (*Task, bool)) traffic.RemoteScopeResolver {
	return func(token string) (string, traffic.ScopeFunc, bool) {
		taskID, ok, err := lookup(token)
		if err != nil {
			log.Printf("[traffic] remote scope token lookup failed (%v); refusing the connection", err)
			return "", nil, false
		}
		if !ok {
			return "", nil, false
		}
		id := strconv.FormatInt(taskID, 10)
		t, loaded := task(id)
		if !loaded || t == nil {
			return "", nil, false
		}
		return id, func(ctx context.Context) context.Context {
			return intercept.WithTargetScope(ctx, t.targetScope(0))
		}, true
	}
}

// enableRemoteProxyScopes makes this (control) node's recording proxy resolve
// worker-minted scope tokens and refuse token-less non-loopback clients, and
// publishes its CA certificate for the workers. A no-op without a proxy.
func (m *Manager) enableRemoteProxyScopes() error {
	if m.traffic == nil {
		return nil
	}
	m.traffic.SetRemoteScopeResolver(remoteScopeResolver(m.pg.LookupProxyScopeToken, m.Task))
	return publishProxyCACert(m.pg, m.traffic.CACertPath())
}

// publishProxyCACert stores the PEM certificate at certPath in settings. The
// file must hold certificates only: a private key block is refused, so a wrong
// path can never publish the CA key.
func publishProxyCACert(st proxySettings, certPath string) error {
	raw, err := os.ReadFile(certPath)
	if err != nil {
		return fmt.Errorf("read the capture proxy CA certificate: %w", err)
	}
	pemCert, err := certificatesOnly(raw)
	if err != nil {
		return fmt.Errorf("capture proxy CA certificate %s: %w", certPath, err)
	}
	return st.SetSetting(settingCaptureProxyCA, string(pemCert))
}

// fetchProxyCACert writes the control node's published CA certificate to
// dir/control-proxy-ca.pem (0644: the agents' executor reads it) and returns
// the path, the worker's ProxyCACert. The file is replaced atomically.
func fetchProxyCACert(st proxySettings, dir string) (string, error) {
	v, ok, err := st.GetSetting(settingCaptureProxyCA)
	if err != nil {
		return "", fmt.Errorf("read the capture proxy CA certificate: %w", err)
	}
	if !ok || v == "" {
		return "", errors.New("the control node has not published its capture proxy CA certificate")
	}
	pemCert, err := certificatesOnly([]byte(v))
	if err != nil {
		return "", fmt.Errorf("published capture proxy CA certificate: %w", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	f, err := os.CreateTemp(dir, ".control-proxy-ca-*")
	if err != nil {
		return "", err
	}
	tmp := f.Name()
	defer os.Remove(tmp) // a no-op once renamed
	if _, err := f.Write(pemCert); err != nil {
		f.Close()
		return "", err
	}
	if err := f.Chmod(0o644); err != nil {
		f.Close()
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "control-proxy-ca.pem")
	if err := os.Rename(tmp, path); err != nil {
		return "", err
	}
	return path, nil
}

// certificatesOnly returns raw re-encoded as its CA certificate blocks. It
// fails on anything else: no block, a block that is not a CERTIFICATE (a key),
// a certificate that does not parse or is not a CA, or trailing data.
func certificatesOnly(raw []byte) ([]byte, error) {
	var out bytes.Buffer
	rest := raw
	for {
		var b *pem.Block
		b, rest = pem.Decode(rest)
		if b == nil {
			break
		}
		if b.Type != "CERTIFICATE" {
			return nil, fmt.Errorf("holds a %q block; only certificates may be published", b.Type)
		}
		c, err := x509.ParseCertificate(b.Bytes)
		if err != nil {
			return nil, fmt.Errorf("certificate does not parse: %w", err)
		}
		if !c.IsCA {
			return nil, errors.New("certificate is not a CA")
		}
		if err := pem.Encode(&out, &pem.Block{Type: "CERTIFICATE", Bytes: b.Bytes}); err != nil {
			return nil, err
		}
	}
	if len(bytes.TrimSpace(rest)) != 0 {
		return nil, errors.New("holds data that is not PEM")
	}
	if out.Len() == 0 {
		return nil, errors.New("holds no certificate")
	}
	return out.Bytes(), nil
}

// remoteTaskProxy is a worker node's TaskProxyAddr: the control node's
// recording proxy URL carrying a scope token per task, minted here and
// registered (its hash) in the shared database for the control node to resolve.
type remoteTaskProxy struct {
	store  proxyTokenStore
	base   string // control proxy URL, no userinfo
	nodeID string

	mu     sync.Mutex
	tokens map[int64]string
}

// newRemoteTaskProxy checks controlProxyURL (http(s)://host:port, no userinfo,
// no path) so that a scoped URL can always be derived from it.
func newRemoteTaskProxy(store proxyTokenStore, controlProxyURL, nodeID string) (*remoteTaskProxy, error) {
	u, err := url.Parse(controlProxyURL)
	if err != nil {
		return nil, fmt.Errorf("control proxy URL: %w", err)
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" {
		return nil, fmt.Errorf("control proxy URL %q must be http(s)://host:port with no credentials or path", egress.WithoutProxyCredential(controlProxyURL))
	}
	if store == nil {
		return nil, errors.New("control proxy: no token store")
	}
	base := u.Scheme + "://" + u.Host
	return &remoteTaskProxy{store: store, base: base, nodeID: nodeID, tokens: map[int64]string{}}, nil
}

// URL returns the control proxy URL carrying task taskID's scope token. The
// token is minted on first use and registered on every call (idempotent), so a
// row the control node dropped is restored only while the control node still
// has the task loaded (its resolver checks). Any failure returns the
// ended-scope URL: dials refused, never the global rules.
func (p *remoteTaskProxy) URL(taskID int64) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	tok, ok := p.tokens[taskID]
	if !ok {
		t, err := mintScopeToken("")
		if err != nil {
			log.Printf("[traffic] task %d: cannot mint a capture proxy scope token (%v); its dials are refused", taskID, err)
			return endedProxyURL(p.base)
		}
		tok = t
	}
	if err := p.store.RegisterProxyScopeToken(taskID, tok, p.nodeID); err != nil {
		log.Printf("[traffic] task %d: cannot register its capture proxy scope token (%v); its dials are refused", taskID, err)
		return endedProxyURL(p.base)
	}
	p.tokens[taskID] = tok
	return egress.ScopedProxyURL(p.base, tok)
}

// Forget drops task taskID's token on this node and its database row, when the
// task stops running here. The control node drops every node's rows of a task
// whose scope ends (ForgetProxyScopeTokens).
func (p *remoteTaskProxy) Forget(taskID int64) {
	p.mu.Lock()
	_, had := p.tokens[taskID]
	delete(p.tokens, taskID)
	p.mu.Unlock()
	if had {
		if err := p.store.ForgetNodeProxyScopeTokens(taskID, p.nodeID); err != nil {
			log.Printf("[traffic] task %d: forget its capture proxy scope tokens: %v", taskID, err)
		}
	}
}

// Ended is the URL for a client of this node whose run has no scope: every
// dial is refused (a worker node has no token-less proxy path).
func (p *remoteTaskProxy) Ended() string { return endedProxyURL(p.base) }

// mintScopeToken returns egress.ScopeTokenPrefix + infix + 32 hex digits, the
// format of traffic's own tokens (traffic/scope.go).
func mintScopeToken(infix string) (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return egress.ScopeTokenPrefix + infix + hex.EncodeToString(raw), nil
}

// endedProxyURL is base carrying a token that names no scope, so every dial is
// refused, as traffic.EndedScopeURL is for the local proxy.
func endedProxyURL(base string) string {
	tok, err := mintScopeToken("ended-")
	if err != nil {
		tok = egress.ScopeTokenPrefix + "ended"
	}
	return egress.ScopedProxyURL(base, tok)
}
