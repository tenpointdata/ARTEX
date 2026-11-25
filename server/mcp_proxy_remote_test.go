package server

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Autumn-27/artex/db"
)

// On a worker node the shared browser MCP row names the control node's
// loopback proxy, which nothing on the worker serves. Its spawn gets a
// credential relay to the control proxy carrying the run's scope token, and the
// worker's copy of the control CA: the browser's dials reach the target on the
// task's scope at the control node. A run with no task scope gets a relay whose
// dials the control node refuses. Another proxy is left alone.
func TestWorkerBrowserMCPRelaysToControlProxy(t *testing.T) {
	control, spy := startScopedProxy(t)
	task := &Task{ID: "42"}
	control.tasks[task.ID] = task
	store := &fakeTokenStore{}
	lookup := func(tok string) (int64, bool, error) { id, ok := store.rows[tok]; return id, ok, nil }
	control.traffic.SetRemoteScopeResolver(remoteScopeResolver(lookup, control.Task))

	rp, err := newRemoteTaskProxy(store, control.traffic.ProxyAddr(), "node-b")
	if err != nil {
		t.Fatal(err)
	}
	worker := &Manager{trafficOn: true, tasks: map[string]*Task{task.ID: task}, remoteProxy: rp, remoteCACert: "/data/control-proxy-ca.pem"}
	// The row as the control node stored it: its own loopback listener (a port
	// the worker does not know) and its own CA path.
	row := &db.MCPServer{Name: browserMCPName, Transport: "stdio", Command: "npx",
		Args: encodeJSON([]string{"@playwright/mcp@latest", "--proxy-server", "http://127.0.0.1:1"}),
		Env:  encodeJSON(map[string]string{"NODE_EXTRA_CA_CERTS": "/control/_ca/ca.pem", "OTHER": "kept"})}
	target := okServer(t)

	via := func(proxy string) int {
		t.Helper()
		u, _ := url.Parse(proxy)
		return statusVia(t, &http.Transport{Proxy: http.ProxyURL(u)}, target.URL)
	}

	spawn, relay, err := worker.scopeMCPProxy(gatedCtx(task, time.Second), row)
	if err != nil || relay == nil {
		t.Fatalf("scopeMCPProxy on a worker: %v (relay %v)", err, relay)
	}
	proxy := spawnProxy(t, spawn)
	if strings.Contains(proxy, "@") || !strings.HasPrefix(proxy, "http://127.0.0.1:") || proxy == "http://127.0.0.1:1" {
		t.Fatalf("spawned --proxy-server %q: want a credential-free loopback relay", proxy)
	}
	env := jsonStrMap(spawn.Env)
	if env["NODE_EXTRA_CA_CERTS"] != worker.remoteCACert || env["OTHER"] != "kept" {
		t.Fatalf("spawned env %v: want the worker's CA copy and the rest kept", env)
	}
	if code := via(proxy); code != http.StatusOK {
		t.Fatalf("GET through the worker relay: %d", code)
	}
	if got := spy.take(); len(got) != 1 || !got[0].ok || got[0].scope.TaskID != task.ID {
		t.Fatalf("control dials %+v; want one on task %s's scope", got, task.ID)
	}
	_ = relay.Close()

	// No task scope: the relay carries the ended-scope token, refused.
	spawn, relay, err = worker.scopeMCPProxy(context.Background(), row)
	if err != nil || relay == nil {
		t.Fatalf("unscoped scopeMCPProxy on a worker: %v (relay %v)", err, relay)
	}
	if code := via(spawnProxy(t, spawn)); code == http.StatusOK {
		t.Fatal("an unscoped worker browser reached the target")
	}
	if got := spy.take(); len(got) != 0 {
		t.Fatalf("an unscoped worker browser dialled %+v", got)
	}
	_ = relay.Close()

	// A proxy that is not the capture proxy is left alone.
	other := &db.MCPServer{Name: "x", Transport: "stdio", Args: encodeJSON([]string{"--proxy-server", "http://corp-proxy:3128"})}
	if got, closer, err := worker.scopeMCPProxy(gatedCtx(task, time.Second), other); err != nil || closer != nil || got != other {
		t.Fatalf("another proxy: %v %v %v; want it unchanged", got, closer, err)
	}
}

func TestWorkerCaptureProxy(t *testing.T) {
	const base = "http://artex-control:8788"
	for v, want := range map[string]bool{
		"http://127.0.0.1:8788":     true,
		"127.0.0.1:9000":            true,
		"http://0.0.0.0:8788":       true,
		"http://localhost:8788":     true,
		"http://[::1]:8788":         true,
		"http://artex-control:8788": true,
		"http://corp-proxy:3128":    false,
		"socks5://127.0.0.1:1080":   false,
		"http://127.0.0.1":          false,
		"":                          false,
	} {
		if got := workerCaptureProxy(v, base); got != want {
			t.Errorf("workerCaptureProxy(%q) = %v, want %v", v, got, want)
		}
	}
}
