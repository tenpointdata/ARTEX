package server

import (
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Autumn-27/artex/db"
)

// E7 proxy path through the real Manager wiring: a worker node's task proxy URL
// names the control node's recording proxy with a token the control proxy
// resolves through the database, the worker gets the control CA, a forged
// token is refused, and deleting the task on the control node ends the token.
func TestWorkerNodeUsesControlRecordingProxy(t *testing.T) {
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

	worker, err := NewManagerForNode(t.TempDir(), "", NodeConfig{Role: RoleWorker, NodeID: "node-p", Workers: 1,
		ControlProxy: control.traffic.ProxyAddr()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = worker.Close() })
	if _, err := NewManagerForNode(t.TempDir(), "", NodeConfig{Role: RoleWorker, NodeID: "node-q", Workers: 1}); err == nil {
		t.Fatal("a worker node without ARTEX_CONTROL_PROXY started")
	}

	created, err := control.CreateTaskWithOptions("e7 proxy", "worker traffic through the control proxy", db.TaskCreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	worker.LoadExisting()
	wt, ok := worker.Task(created.ID)
	if !ok {
		t.Fatal("worker did not load the task")
	}
	proxyURL := worker.TaskProxyAddr(wt)
	pu, err := url.Parse(proxyURL)
	if err != nil || pu.User == nil || !strings.HasPrefix(pu.Host, "127.0.0.1:") {
		t.Fatalf("worker task proxy = %q, want the control proxy with a scope token", proxyURL)
	}
	if ca := worker.ProxyCACert(); ca == "" {
		t.Fatal("worker has no control CA certificate")
	} else if b, err := os.ReadFile(ca); err != nil || !strings.Contains(string(b), "BEGIN CERTIFICATE") || strings.Contains(string(b), "PRIVATE KEY") {
		t.Fatalf("worker CA file %s: %v", ca, err)
	}
	if ended := worker.ProxyAddr(); ended == control.traffic.ProxyAddr() {
		t.Fatal("a worker's unscoped proxy address is the plain control proxy")
	}
	n, _ := strconv.ParseInt(created.ID, 10, 64)
	if id, found, err := control.pg.LookupProxyScopeToken(pu.User.Username()); err != nil || !found || id != n {
		if p, _ := pu.User.Password(); p != "" {
			id, found, err = control.pg.LookupProxyScopeToken(p)
		}
		if err != nil || !found || id != n {
			t.Fatalf("token lookup = %d %v %v, want task %d", id, found, err, n)
		}
	}

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) }))
	defer target.Close()
	do := func(proxy string) int {
		t.Helper()
		p, _ := url.Parse(proxy)
		c := &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{Proxy: http.ProxyURL(p)}}
		resp, err := c.Get(target.URL)
		if err != nil {
			t.Logf("GET via %s: %v", p.Redacted(), err)
			return -1
		}
		defer resp.Body.Close()
		return resp.StatusCode
	}
	forged := *pu
	forged.User = url.UserPassword("x", "scope-00000000000000000000000000000000")
	if code := do(forged.String()); code == 200 {
		t.Fatal("a forged scope token reached the target through the control proxy")
	}

	before, _ := control.traffic.Count()
	code := do(proxyURL)
	after, _ := control.traffic.Count()
	if code != 200 || after != before+1 {
		t.Fatalf("valid worker token: status %d, recorded exchanges %d -> %d; want 200 and one recorded on the control node", code, before, after)
	}

	if _, err := control.DeleteTask(created.ID, DeleteTaskOptions{}); err != nil {
		t.Fatal(err)
	}
	var left int
	if err := control.pg.QueryRow(`SELECT count(*) FROM proxy_scope_tokens WHERE task_id=$1`, n).Scan(&left); err != nil || left != 0 {
		t.Fatalf("scope tokens left after delete = %d (%v)", left, err)
	}
	if code := do(proxyURL); code == 200 {
		t.Fatal("the deleted task's worker token still reaches the target")
	}
}
