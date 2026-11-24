package server

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Autumn-27/artex/db"
)

// c-7ad4da: a worker node follows the control node's traffic-capture switch
// within a poll, without a restart: its HostTools, TaskProxyAddr and CA change
// together, and its cached task agent bundles are dropped so they are rebuilt.
func TestWorkerNodeFollowsTrafficCaptureSwitch(t *testing.T) {
	skipWithoutDSN(t)
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	_ = l.Close()
	control, err := NewManagerForNode(t.TempDir(), addr, NodeConfig{Role: RoleControl, NodeID: "ctl", Workers: 0})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = control.Close() })
	if control.traffic == nil {
		t.Skip("recording proxy unavailable in this environment")
	}
	prev := control.TrafficEnabled()
	if err := control.SetTrafficEnabled(true); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = control.SetTrafficEnabled(prev) })
	cs := &Server{m: control, node: NodeConfig{Role: RoleControl}, nodeRPC: newNodeRPCVerifier(db.NodeRPCKey)}
	mux := http.NewServeMux()
	mux.HandleFunc("POST "+nodeRPCPrefix+"{op}", cs.handleNodeRPC)
	api := httptest.NewServer(cs.requireAuth(mux))
	defer api.Close()
	if _, err := control.pg.Exec(`DELETE FROM worker_nodes`); err != nil {
		t.Fatal(err)
	}

	created, err := control.CreateTaskWithOptions("e7 capture switch", "a worker node follows the capture switch", db.TaskCreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = control.DeleteTask(created.ID, DeleteTaskOptions{}) })

	node := NodeConfig{Role: RoleWorker, NodeID: "node-tc", Host: "node-tc", Workers: 1,
		ControlProxy: control.traffic.ProxyAddr(), ControlAPI: api.URL}
	wm, err := NewManagerForNode(t.TempDir(), "", node)
	if err != nil {
		t.Fatal(err)
	}
	wm.LoadExisting()
	wt, ok := wm.Task(created.ID)
	if !ok {
		t.Fatal("worker did not load the task")
	}
	w := startTestNodeOn(t, wm, node, &barrierProvider{taskID: "-"})

	hasTrafficTools := func() bool {
		for _, tool := range wm.HostTools() {
			if tool.Name() == "traffic_search" {
				return true
			}
		}
		return false
	}
	captured := func() bool {
		return hasTrafficTools() && wm.ProxyCACert() != "" &&
			strings.HasPrefix(wm.TaskProxyAddr(wt), "http") && wm.TaskProxyAddr(wt) != wm.GlobalProxy()
	}
	direct := func() bool {
		return !hasTrafficTools() && wm.ProxyCACert() == "" && wm.TaskProxyAddr(wt) == wm.GlobalProxy()
	}
	bundleCached := func() bool {
		w.s.taskAgentMu.Lock()
		defer w.s.taskAgentMu.Unlock()
		return w.s.taskAgents[wt.ID] != nil
	}
	cacheBundle := func() {
		w.s.taskAgentMu.Lock()
		w.s.taskAgents[wt.ID] = &taskAgentBundle{}
		w.s.taskAgentMu.Unlock()
	}
	within := 4 * distributedPoll
	if !captured() {
		t.Fatal("worker started with capture on but its tools/proxy/CA say off")
	}

	cacheBundle()
	if err := control.SetTrafficEnabled(false); err != nil {
		t.Fatal(err)
	}
	waitUntilFor(t, within, "the worker to follow capture off", func() bool { return direct() && !bundleCached() })

	cacheBundle()
	time.Sleep(2 * distributedPoll)
	if !bundleCached() {
		t.Fatal("bundles dropped although the switch did not change")
	}
	if err := control.SetTrafficEnabled(true); err != nil {
		t.Fatal(err)
	}
	waitUntilFor(t, within, "the worker to follow capture on", func() bool { return captured() && !bundleCached() })
}
