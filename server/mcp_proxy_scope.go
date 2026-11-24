package server

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/url"
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
// PLAYWRIGHT_MCP_PROXY_SERVER is @playwright/mcp's spelling of --proxy-server.
var proxyEnvKeys = map[string]bool{"HTTP_PROXY": true, "HTTPS_PROXY": true, "ALL_PROXY": true, "PLAYWRIGHT_MCP_PROXY_SERVER": true}

// scopeMCPProxy returns srv as it is to be spawned for the run on ctx, and the
// relay the caller closes with the server (nil when none was needed). With
// capture off, for a non-stdio server, or for one that names no capture proxy,
// srv is returned unchanged. A relay that cannot start is an error: the server
// must not be spawned on a URL that drops its run's scope.
func (m *Manager) scopeMCPProxy(ctx context.Context, srv *db.MCPServer) (*db.MCPServer, io.Closer, error) {
	if srv != nil && srv.Transport == "stdio" && m.traffic == nil && m.remoteProxy != nil && m.TrafficEnabled() {
		return m.scopeMCPProxyRemote(ctx, srv) // worker node (mcp_proxy_remote.go)
	}
	if srv == nil || srv.Transport != "stdio" || m.traffic == nil || !m.TrafficEnabled() {
		return srv, nil, nil
	}
	capture := m.traffic.ProxyAddr()
	args, env := jsonStrSlice(srv.Args), jsonStrMap(srv.Env)
	if !namesProxy(args, env, capture) {
		if mentionsProxyPort(args, env, capture) {
			log.Printf("[mcp] %s: a proxy setting names the capture proxy's port in a form that cannot be rewritten; its dials get the global rules only (use --proxy-server %s)", srv.Name, capture)
		}
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
	return namesProxyFunc(args, env, func(v string) bool { return sameProxy(v, proxy) })
}

// namesProxyFunc reports whether args or env point a client at a proxy that is accepts.
func namesProxyFunc(args []string, env map[string]string, is func(string) bool) bool {
	a, e := replaceProxyFunc(args, env, is, "\x00")
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
	return replaceProxyFunc(args, env, func(v string) bool { return sameProxy(v, proxy) }, with)
}

// replaceProxyFunc is replaceProxy for every proxy value is accepts.
func replaceProxyFunc(args []string, env map[string]string, is func(string) bool, with string) ([]string, map[string]string) {
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

// sameProxy reports whether the proxy setting v names proxy: the same port and
// the same host, a loopback name or address standing for any other. A value
// without a scheme ("127.0.0.1:8788", as Chromium accepts) is read as http.
func sameProxy(v, proxy string) bool {
	v = strings.TrimSpace(v)
	if v == "" {
		return false
	}
	if !strings.Contains(v, "://") {
		v = "http://" + v
	}
	a, errA := url.Parse(v)
	b, errB := url.Parse(proxy)
	if errA != nil || errB != nil || a.Port() == "" || a.Port() != b.Port() || !strings.EqualFold(a.Scheme, b.Scheme) {
		return false
	}
	return strings.EqualFold(a.Hostname(), b.Hostname()) || (isLoopbackHost(a.Hostname()) && isLoopbackHost(b.Hostname()))
}

func isLoopbackHost(h string) bool {
	if strings.EqualFold(h, "localhost") {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && (ip.IsLoopback() || ip.IsUnspecified())
}

// mentionsProxyPort reports whether a proxy flag or variable mentions proxy's
// port without naming it in a form replaceProxy understands (Chromium's
// "http=…;https=…" list, say), so the operator can be told.
func mentionsProxyPort(args []string, env map[string]string, proxy string) bool {
	u, err := url.Parse(proxy)
	if err != nil || u.Port() == "" {
		return false
	}
	port := ":" + u.Port()
	for i, a := range args {
		if strings.HasPrefix(a, "--proxy-server=") && strings.Contains(a, port) {
			return true
		}
		if a == "--proxy-server" && i+1 < len(args) && strings.Contains(args[i+1], port) {
			return true
		}
	}
	for k, v := range env {
		if proxyEnvKeys[strings.ToUpper(k)] && strings.Contains(v, port) {
			return true
		}
	}
	return false
}
