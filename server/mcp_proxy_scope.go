package server

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/Autumn-27/artex/db"
)

// Scoped capture proxy for stdio MCP servers. A stdio MCP is spawned per run
// (wireAgentAugment), but its stored configuration names the plain recording
// proxy: the seeded Playwright browser gets `--proxy-server <ProxyAddr>`
// (syncBrowserMCPProxy), and an operator may point any server at it by flag or
// by HTTP(S)_PROXY. Spawned as stored, its dials would get the global rules
// only. So each spawn has those references replaced by a scope relay of the
// run (traffic.StartScopeRelay): Chromium drops a userinfo from --proxy-server,
// and a relay carries the scope without one. A reference to another proxy is
// left alone: that proxy, not ARTEX, reaches the target.

// proxyEnvKeys are the environment variables a proxy-aware client reads.
var proxyEnvKeys = map[string]bool{"HTTP_PROXY": true, "HTTPS_PROXY": true, "ALL_PROXY": true}

// scopeMCPProxy returns srv as it is to be spawned for the run on ctx, and the
// relay the caller closes with the server (nil when none was needed). With
// capture off, for a non-stdio server, or for one that names no capture proxy,
// srv is returned unchanged. A relay that cannot start is an error: the server
// must not be spawned on a URL that drops its run's scope.
func (m *Manager) scopeMCPProxy(ctx context.Context, srv *db.MCPServer) (*db.MCPServer, io.Closer, error) {
	if srv == nil || srv.Transport != "stdio" || m.traffic == nil || !m.TrafficEnabled() {
		return srv, nil, nil
	}
	capture := m.traffic.ProxyAddr()
	args, env := jsonStrSlice(srv.Args), jsonStrMap(srv.Env)
	if !namesProxy(args, env, capture) {
		return srv, nil, nil
	}
	relayURL, relay, err := m.traffic.StartScopeRelay(m.ScopedProxyAddr(ctx))
	if err != nil {
		return nil, nil, fmt.Errorf("scoped capture proxy: %w", err)
	}
	out := *srv
	args, env = replaceProxy(args, env, capture, relayURL)
	out.Args, out.Env = encodeJSON(args), encodeJSON(env)
	if relay == nil {
		return &out, nil, nil
	}
	return &out, relay, nil
}

// namesProxy reports whether args or env point a client at proxy.
func namesProxy(args []string, env map[string]string, proxy string) bool {
	a, e := replaceProxy(args, env, proxy, "\x00")
	for i := range a {
		if a[i] != args[i] {
			return true
		}
	}
	for k, v := range e {
		if env[k] != v {
			return true
		}
	}
	return false
}

// replaceProxy returns copies of args and env with every --proxy-server value
// ("--proxy-server v" or "--proxy-server=v") and proxy variable that is proxy,
// compared without a credential, replaced by with.
func replaceProxy(args []string, env map[string]string, proxy, with string) ([]string, map[string]string) {
	is := func(v string) bool { return proxyWithoutCredentials(v) == proxy }
	outArgs := append([]string(nil), args...)
	for i := 0; i < len(outArgs); i++ {
		switch a := outArgs[i]; {
		case a == "--proxy-server" && i+1 < len(outArgs):
			if is(outArgs[i+1]) {
				outArgs[i+1] = with
			}
			i++
		case strings.HasPrefix(a, "--proxy-server="):
			if is(strings.TrimPrefix(a, "--proxy-server=")) {
				outArgs[i] = "--proxy-server=" + with
			}
		}
	}
	outEnv := make(map[string]string, len(env))
	for k, v := range env {
		if proxyEnvKeys[strings.ToUpper(k)] && is(v) {
			v = with
		}
		outEnv[k] = v
	}
	return outArgs, outEnv
}
