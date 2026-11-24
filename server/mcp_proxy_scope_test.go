package server

import (
	"context"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/Autumn-27/artex/db"
	"github.com/Autumn-27/artex/intercept"
)

// browserRow is the seeded browser MCP as syncBrowserMCPProxy stores it with
// capture on: the plain recording-proxy URL in --proxy-server.
func browserRow(m *Manager, extraEnv map[string]string) *db.MCPServer {
	env := map[string]string{"NODE_EXTRA_CA_CERTS": "/ca.pem"}
	for k, v := range extraEnv {
		env[k] = v
	}
	return &db.MCPServer{Name: browserMCPName, Transport: "stdio", Command: "npx",
		Args: encodeJSON([]string{"@playwright/mcp@latest", "--headless", "--proxy-server", m.ProxyAddr()}),
		Env:  encodeJSON(env)}
}

// spawnProxy returns the --proxy-server value srv would be spawned with.
func spawnProxy(t *testing.T, srv *db.MCPServer) string {
	t.Helper()
	args := jsonStrSlice(srv.Args)
	for i, a := range args {
		if a == "--proxy-server" && i+1 < len(args) {
			return args[i+1]
		}
	}
	t.Fatalf("no --proxy-server in %v", args)
	return ""
}

// The browser MCP spawned for a conversation turn or a task run has its
// --proxy-server replaced by a credential-free relay of that run's scope, so
// the proxy's upstream dials of its browser run under the run's rules, with no
// wait. The relay goes with the run; a deleted conversation's relay is refused.
func TestBrowserMCPProxyDialsCarryRunScope(t *testing.T) {
	m, spy := startScopedProxy(t)
	target := okServer(t)
	task := &Task{ID: "task-b"}
	m.tasks[task.ID] = task

	for name, run := range map[string]struct {
		ctx  context.Context
		want func(intercept.TargetScope) bool
	}{
		"conversation": {conversationCtx(11), func(s intercept.TargetScope) bool { return s.ConversationID == 11 && s.TaskID == "" }},
		"task":         {gatedCtx(task, intercept.DefaultTargetWaitBound), func(s intercept.TargetScope) bool { return s.TaskID == task.ID }},
	} {
		row := browserRow(m, nil)
		spawn, relay, err := m.scopeMCPProxy(run.ctx, row)
		if err != nil || relay == nil {
			t.Fatalf("%s: scopeMCPProxy: %v (relay %v)", name, err, relay)
		}
		proxy := spawnProxy(t, spawn)
		if u, _ := url.Parse(proxy); u.User != nil || proxy == m.ProxyAddr() {
			t.Fatalf("%s: spawned with %q, want a credential-free relay URL", name, proxy)
		}
		if spawnProxy(t, row) != m.ProxyAddr() {
			t.Fatalf("%s: the stored row was modified", name)
		}
		if !reflect.DeepEqual(jsonStrMap(spawn.Env), jsonStrMap(row.Env)) {
			t.Fatalf("%s: env changed to %v", name, jsonStrMap(spawn.Env))
		}
		if code := statusVia(t, proxyTransport(t, proxy), target.URL); code != http.StatusOK {
			t.Fatalf("%s: status %d", name, code)
		}
		got := spy.take()
		if len(got) != 1 || !got[0].ok || !run.want(got[0].scope) || got[0].scope.WaitBound != 0 {
			t.Fatalf("%s: dials %+v, want one on the run's scope with no wait", name, got)
		}
		_ = relay.Close()
		if code := statusVia(t, proxyTransport(t, proxy), target.URL); code == http.StatusOK {
			t.Fatalf("%s: a closed relay still reached the target", name)
		}
	}

	spawn, relay, err := m.scopeMCPProxy(conversationCtx(12), browserRow(m, nil))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = relay.Close() })
	m.forgetConversationScope(12)
	if code := statusVia(t, proxyTransport(t, spawnProxy(t, spawn)), target.URL); code == http.StatusOK {
		t.Fatal("a deleted conversation's browser relay reached the target")
	}
	if got := spy.take(); len(got) != 0 {
		t.Fatalf("closed/forgotten relays dialled %v", got)
	}
}

// Proxy variables naming the capture proxy are relayed too; another proxy, a
// server that names none, a non-stdio server, capture off, and a run with no
// scope (global rules, the stored URL) are left as stored.
func TestScopeMCPProxyLeavesOtherServers(t *testing.T) {
	m, _ := startScopedProxy(t)
	ctx := conversationCtx(21)

	row := browserRow(m, map[string]string{"https_proxy": m.ProxyAddr(), "HTTP_PROXY": "http://corp:3128"})
	row.Args = encodeJSON([]string{"--proxy-server=" + m.ProxyAddr()})
	spawn, relay, err := m.scopeMCPProxy(ctx, row)
	if err != nil || relay == nil {
		t.Fatalf("flag form: %v", err)
	}
	defer relay.Close()
	args, env := jsonStrSlice(spawn.Args), jsonStrMap(spawn.Env)
	relayURL := strings.TrimPrefix(args[0], "--proxy-server=")
	if relayURL == m.ProxyAddr() || env["https_proxy"] != relayURL || env["HTTP_PROXY"] != "http://corp:3128" {
		t.Fatalf("args %v env %v: want the capture proxy relayed, the corporate proxy kept", args, env)
	}

	unscoped, relay2, err := m.scopeMCPProxy(context.Background(), browserRow(m, nil))
	if err != nil || relay2 != nil || spawnProxy(t, unscoped) != m.ProxyAddr() {
		t.Fatalf("no scope: %q %v %v, want the stored URL", spawnProxy(t, unscoped), relay2, err)
	}

	other := &db.MCPServer{Transport: "stdio", Args: encodeJSON([]string{"--proxy-server", "http://corp:3128"})}
	httpSrv := &db.MCPServer{Transport: "http", URL: "http://mcp.local", Args: encodeJSON([]string{"--proxy-server", m.ProxyAddr()})}
	for name, srv := range map[string]*db.MCPServer{"other proxy": other, "http transport": httpSrv} {
		if got, closer, err := m.scopeMCPProxy(ctx, srv); err != nil || closer != nil || got != srv {
			t.Fatalf("%s: rewritten (%v, %v)", name, closer, err)
		}
	}
	m.trafficOn = false
	plain := browserRow(m, nil)
	if got, closer, err := m.scopeMCPProxy(ctx, plain); err != nil || closer != nil || got != plain {
		t.Fatalf("capture off: rewritten (%v, %v)", closer, err)
	}
}

// A proxy setting names the capture proxy in any of the forms a client
// accepts (no scheme, a loopback name, a credential), and only on its port.
func TestSameProxyForms(t *testing.T) {
	const capture = "http://127.0.0.1:8788"
	for v, want := range map[string]bool{
		capture: true, "127.0.0.1:8788": true, "localhost:8788": true, "http://LOCALHOST:8788": true,
		"http://u:p@127.0.0.1:8788": true, "http://[::1]:8788": true, " http://127.0.0.1:8788 ": true,
		"http://127.0.0.1:8789": false, "http://10.0.0.1:8788": false, "socks5://127.0.0.1:8788": false,
		"127.0.0.1": false, "": false, "http=127.0.0.1:8788;https=127.0.0.1:8788": false,
	} {
		if got := sameProxy(v, capture); got != want {
			t.Errorf("sameProxy(%q) = %v, want %v", v, got, want)
		}
	}
	args := []string{"--proxy-server", "http=127.0.0.1:8788;https=127.0.0.1:8788"}
	if namesProxy(args, nil, capture) || !mentionsProxyPort(args, nil, capture) {
		t.Error("a per-scheme list must be reported, not rewritten")
	}
	env := map[string]string{"PLAYWRIGHT_MCP_PROXY_SERVER": "localhost:8788"}
	if _, e := replaceProxy(nil, env, capture, "R"); e["PLAYWRIGHT_MCP_PROXY_SERVER"] != "R" {
		t.Errorf("PLAYWRIGHT_MCP_PROXY_SERVER not relayed: %v", e)
	}
}
