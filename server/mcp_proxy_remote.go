package server

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/Autumn-27/artex/db"
	"github.com/Autumn-27/artex/traffic"
)

// Browser MCP on a worker node (E7, docs/DISTRIBUTED.md). The MCP rows are
// shared: the control node's syncBrowserMCPProxy stores its own recording
// proxy, a loopback address, in --proxy-server, and its CA path in
// NODE_EXTRA_CA_CERTS. On a worker node nothing listens there (a worker runs
// no proxy of its own, configureNodeProxy), so a spawn as stored would dial
// nothing. Each spawn instead gets a credential relay of its run
// (traffic.StartCredentialRelay): a loopback listener that forwards to the
// control node's proxy (ARTEX_CONTROL_PROXY) with the run's scope token as
// Proxy-Authorization, which Chromium cannot send itself. The CA path becomes
// the copy of the control node's certificate this node fetched.
//
// A run without a task scope gets the ended-scope URL (ScopedProxyAddr), so
// its relay's dials are refused at the control node, never sent on the global
// rules. A relay that cannot start is an error: the server is not spawned.

// scopeMCPProxyRemote is scopeMCPProxy on a worker node.
func (m *Manager) scopeMCPProxyRemote(ctx context.Context, srv *db.MCPServer) (*db.MCPServer, io.Closer, error) {
	args, env := jsonStrSlice(srv.Args), jsonStrMap(srv.Env)
	is := func(v string) bool { return workerCaptureProxy(v, m.remoteProxy.base) }
	if !namesProxyFunc(args, env, is) {
		return srv, nil, nil
	}
	relay, err := traffic.StartCredentialRelay(m.ScopedProxyAddr(ctx))
	if err != nil {
		return nil, nil, fmt.Errorf("scoped control proxy relay: %w", err)
	}
	args, env = replaceProxyFunc(args, env, is, relay.URL)
	for k := range env {
		if strings.EqualFold(k, "NODE_EXTRA_CA_CERTS") {
			env[k] = m.ProxyCACert()
		}
	}
	out := *srv
	out.Args, out.Env = encodeJSON(args), encodeJSON(env)
	return &out, relay, nil
}

// workerCaptureProxy reports whether a stored proxy value v names the capture
// proxy as seen from a worker node: the control proxy URL itself, or an http
// proxy on a loopback (or unspecified) address, which on a worker can only be
// the control node's own address written into the shared row. Any other proxy
// is left alone: that proxy, not ARTEX, reaches the target.
func workerCaptureProxy(v, controlBase string) bool {
	if sameProxy(v, controlBase) {
		return true
	}
	v = strings.TrimSpace(v)
	if v == "" {
		return false
	}
	if !strings.Contains(v, "://") {
		v = "http://" + v
	}
	u, err := url.Parse(v)
	return err == nil && strings.EqualFold(u.Scheme, "http") && u.Port() != "" && isLoopbackHost(u.Hostname())
}
