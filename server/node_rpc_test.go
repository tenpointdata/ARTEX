package server

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"iter"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Autumn-27/artex/agent"
	"github.com/Autumn-27/artex/db"
	"github.com/Autumn-27/artex/intercept"
	"github.com/Autumn-27/norma/llm"
)

func TestNodeRPCVerifier(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	now := time.Unix(1_800_000_000, 0)
	v := newNodeRPCVerifier(func() []byte { return key })
	v.now = func() time.Time { return now }
	body := []byte(`{"traffic_id":"x"}`)
	req := func(k []byte, at time.Time) *http.Request {
		r := httptest.NewRequest(http.MethodPost, nodeRPCPrefix+"exists", bytes.NewReader(body))
		if err := signNodeRPC(r, k, "node-a", body, at); err != nil {
			t.Fatal(err)
		}
		return r
	}
	ok := req(key, now)
	if node, err := v.verify(ok, body); err != nil || node != "node-a" {
		t.Fatalf("valid request: %q %v", node, err)
	}
	if _, err := v.verify(ok, body); err == nil {
		t.Fatal("a replayed request was accepted")
	}
	if _, err := v.verify(req([]byte("another key, same length........"), now), body); err == nil {
		t.Fatal("a request signed with another key was accepted")
	}
	if _, err := v.verify(req(key, now.Add(-2*nodeRPCSkew)), body); err == nil {
		t.Fatal("a stale request was accepted")
	}
	r := req(key, now)
	if _, err := v.verify(r, []byte(`{"traffic_id":"y"}`)); err == nil {
		t.Fatal("a request whose body changed after signing was accepted")
	}
	r = req(key, now)
	r.URL.Path = nodeRPCPrefix + "bind"
	if _, err := v.verify(r, body); err == nil {
		t.Fatal("a signature for one op was accepted for another")
	}
	if _, err := newNodeRPCVerifier(func() []byte { return nil }).verify(req(key, now), body); err == nil {
		t.Fatal("a verifier without a key accepted a request")
	}
}

func TestControlAPIURL(t *testing.T) {
	for _, c := range []struct{ explicit, proxy, want string }{
		{"", "http://artex-control:8788", "http://artex-control:8787"},
		{"https://ctl.example:9443/", "http://artex-control:8788", "https://ctl.example:9443"},
		{"", "http://[fd00::1]:8788", "http://[fd00::1]:8787"},
	} {
		if got, err := controlAPIURL(c.explicit, c.proxy, false); err != nil || got != c.want {
			t.Errorf("controlAPIURL(%q,%q) = %q %v, want %q", c.explicit, c.proxy, got, err, c.want)
		}
	}
	for _, bad := range []string{"ftp://x:1", "http://u:p@x:1", "http://x:1/api", "http://x:1?a=b"} {
		if _, err := controlAPIURL(bad, "http://c:8788", false); err == nil {
			t.Errorf("controlAPIURL(%q) accepted", bad)
		}
	}
	if got, _ := controlAPIURL("", "http://artex-control:8788", true); got != "https://artex-control:8787" {
		t.Errorf("derived control API with a pinned CA = %q, want https://artex-control:8787", got)
	}
}

// reportProvider is a worker-node model whose first turn on the test task
// reports a finding with the given traffic ref; its next turn ends.
type reportProvider struct {
	taskID string
	input  string
}

func (p *reportProvider) Stream(ctx context.Context, req llm.CompletionRequest) iter.Seq2[llm.StreamEvent, error] {
	scope, _ := intercept.TargetScopeFromContext(ctx)
	events := []llm.StreamEvent{{Type: llm.SETextDelta, Text: "done"}, {Type: llm.SEMessageDelta, StopReason: "end_turn"}}
	if scope.TaskID == p.taskID && !hasToolResult(req) {
		events = []llm.StreamEvent{
			{Type: llm.SEToolUseStart, ToolID: "call-1", ToolName: "report_finding"},
			{Type: llm.SEToolInputJSON, Text: p.input},
			{Type: llm.SEMessageDelta, StopReason: "tool_use"},
		}
	}
	return func(yield func(llm.StreamEvent, error) bool) {
		for _, ev := range events {
			if !yield(ev, nil) {
				return
			}
		}
	}
}

func (p *reportProvider) Complete(ctx context.Context, req llm.CompletionRequest) (llm.Message, string, llm.Usage, error) {
	return accumulateStreamForTest(ctx, p.Stream, req)
}

// E7 follow-up (c-3b781a) done-criterion: an intent run on a worker node
// records a finding whose traffic_refs name an exchange the control node's
// recording proxy captured, and the finding binds that request/response. The
// snapshot bodies are written where the capture is (the control node), the
// worker's traffic tools, evidence reads, prove_goal check and S1 assessment
// all reach them through the signed node RPC, and an unsigned call is refused.
func TestWorkerNodeFindingBindsControlCapture(t *testing.T) {
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
	if len(db.NodeRPCKey()) == 0 {
		t.Fatal("no node RPC key after InitSecrets")
	}
	cs := &Server{m: control, node: NodeConfig{Role: RoleControl}, nodeRPC: newNodeRPCVerifier(db.NodeRPCKey)}
	mux := http.NewServeMux()
	mux.HandleFunc("POST "+nodeRPCPrefix+"{op}", cs.handleNodeRPC)
	// The control API over TLS with the CA pinned on the worker, the deployed
	// default (node_tls.go).
	apiCA := newTestCA(t)
	api := httptest.NewUnstartedServer(cs.requireAuth(mux))
	api.TLS = &tls.Config{Certificates: []tls.Certificate{apiCA.leaf}}
	api.StartTLS()
	defer api.Close()
	apiCAFile := writeCA(t, apiCA)
	pinned, err := controlHTTPClient(api.URL, apiCAFile)
	if err != nil {
		t.Fatal(err)
	}

	pg := control.pg
	if _, err := pg.Exec(`DELETE FROM worker_nodes`); err != nil {
		t.Fatal(err)
	}
	if err := publishBudgets(pg); err != nil {
		t.Fatal(err)
	}
	node := NodeConfig{Role: RoleWorker, NodeID: "node-ev", Host: "node-ev", Workers: 1,
		ControlProxy: control.traffic.ProxyAddr(), ControlAPI: api.URL, ControlCA: apiCAFile}
	worker, err := NewManagerForNode(t.TempDir(), "", node)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = worker.Close() })
	if worker.controlRPC == nil {
		t.Fatal("worker node has no control RPC client")
	}
	// The worker prompt's traffic note keys on the CA; the tools it names must
	// exist whenever the CA does.
	names := map[string]bool{}
	for _, tool := range worker.HostTools() {
		names[tool.Name()] = true
	}
	if worker.ProxyCACert() == "" || !names["traffic_search"] || !names["traffic_get"] {
		t.Fatalf("worker CA %q, host tools %v: want the control CA and the traffic tools together", worker.ProxyCACert(), names)
	}

	created, err := control.CreateTaskWithOptions("e7 evidence", "worker findings carry control captures", db.TaskCreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = control.DeleteTask(created.ID, DeleteTaskOptions{}) })
	task, _ := control.Task(created.ID)
	worker.LoadExisting()
	wt, ok := worker.Task(created.ID)
	if !ok {
		t.Fatal("worker did not load the task")
	}

	// The intent's tool traffic: through the worker's task-scoped proxy URL,
	// recorded on the control node.
	marker := fmt.Sprintf("artex-proof-%d", time.Now().UnixNano())
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("leaked: " + marker + "\n"))
	}))
	defer target.Close()
	pu, _ := url.Parse(worker.TaskProxyAddr(wt))
	hc := &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{Proxy: http.ProxyURL(pu)}}
	resp, err := hc.Get(target.URL + "/secret")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()

	// The worker agent's traffic_search runs on the control node.
	host := strings.TrimPrefix(target.URL, "http://")
	var trafficID string
	waitUntilFor(t, 10*time.Second, "the exchange to be searchable from the worker", func() bool {
		for _, tool := range worker.HostTools() {
			if tool.Name() != "traffic_search" {
				continue
			}
			res, err := tool.Call(context.Background(), json.RawMessage(`{"host":"`+host+`","contains":"secret"}`), nil)
			if err != nil || res.IsError || len(res.Content) == 0 {
				return false
			}
			var rows []struct{ ID string }
			if json.Unmarshal([]byte(res.Content[0].Text), &rows) != nil || len(rows) == 0 {
				return false
			}
			trafficID = rows[0].ID
			return true
		}
		return false
	})

	intentID, err := task.Store.AddIntent(map[string]any{"summary": "prove the leak"}, 1, nil, "planner")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	e := NewEngine(worker)
	e.node = node
	ws := &Server{m: worker, engine: e, ctx: ctx, node: node, chatBusy: map[string]bool{},
		chatCancel: map[string]context.CancelCauseFunc{}, taskAgents: map[string]*taskAgentBundle{}}
	input, _ := json.Marshal(map[string]any{
		"vulnclass": "info-disclosure", "name": "leak", "severity": "high", "summary": "the endpoint leaks " + marker,
		"intent_id": intentID, "evidence": "GET /secret returns " + marker,
		"traffic_refs": []map[string]any{{"traffic_id": trafficID, "role": "proof", "note": "the leaking response"}},
	})
	// Agent traffic binding is an operator setting (off by default); this is
	// the deployment that has it on.
	prevBinding := agent.FindingTrafficBindingEnabled()
	agent.SetFindingTrafficBindingEnabled(func() bool { return true })
	t.Cleanup(func() { agent.SetFindingTrafficBindingEnabled(prevBinding) })
	aw := agent.NewWorker(&reportProvider{taskID: task.ID, input: string(input)}, "test-model", worker.dir, nil, 0, 3)
	aw.SetFindingRecorder(ws.findingEvidence())
	aw.SetFactTraffic(worker.factTraffic())
	e.SetAuthoritativeAgentResolver(func(*Task) (*agent.Planner, *agent.Worker) { return nil, aw })
	go ws.runNodeHeartbeat(ctx)
	ws.startWorkerNode()
	t.Cleanup(func() {
		cancel()
		wctx, done := context.WithTimeout(context.Background(), 30*time.Second)
		defer done()
		_ = e.Wait(wctx)
	})
	waitUntilFor(t, 60*time.Second, "the worker node to run the intent", func() bool {
		st, _ := task.Store.IntentState(intentID)
		return st != "open" && st != "running"
	})
	var owner string
	var findingID int64
	if err := pg.QueryRow(`SELECT COALESCE(owner,'') FROM exploration_nodes WHERE id=$1`, intentID).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	if owner != "node-ev/work#1" {
		t.Fatalf("intent owner = %q, want the worker node", owner)
	}
	if err := pg.QueryRow(`SELECT id FROM findings WHERE task_id=$1 ORDER BY id DESC LIMIT 1`, task.ID).Scan(&findingID); err != nil {
		t.Fatalf("no finding recorded by the worker-node intent: %v", err)
	}
	tr, err := pg.GetFindingTraffic(ctx, findingID)
	if err != nil || len(tr.Bindings) != 1 {
		t.Fatalf("finding %d bindings = %+v (%v), want the one captured exchange", findingID, tr, err)
	}
	b := tr.Bindings[0]
	if b.Snapshot.SourceTrafficID != trafficID || b.Role != "proof" || !strings.HasSuffix(b.Snapshot.URL, "/secret") || b.Snapshot.Status != 200 {
		t.Fatalf("binding = %+v, want the captured GET /secret as proof", b)
	}
	f, _, err := cs.evidenceStore().OpenBody(b.Snapshot, "response")
	if err != nil {
		t.Fatalf("response body not on the control node: %v", err)
	}
	body, _ := readAllClose(f)
	if !strings.Contains(body, marker) {
		t.Fatalf("control evidence body = %q, want the captured response", body)
	}
	if entries, _ := os.ReadDir(filepath.Join(worker.dir, "evidence", "blobs")); len(entries) != 0 {
		t.Fatalf("evidence blobs written on the worker node: %v", entries)
	}

	ev := ws.findingEvidence()
	if ok, err := ev.TrafficExists(ctx, trafficID); err != nil || !ok {
		t.Fatalf("prove_goal capture check from the worker = %v %v", ok, err)
	}
	pv, err := ev.Preview(ctx, bindingPreviewReq{FindingID: findingID, BindingID: b.ID})
	if err != nil {
		t.Fatal(err)
	}
	if raw, _ := json.Marshal(pv.Body); !strings.Contains(string(raw), marker) {
		t.Fatalf("get_finding_traffic from the worker = %s", raw)
	}
	if st, note := ws.assessFindingVerification(ctx, findingID, 3, []string{"leaked: " + marker}); st != db.VerificationVerified {
		t.Fatalf("S1 assessment from the worker = %s (%s), want verified against the control node's body", st, note)
	}
	if st, _ := ws.assessFindingVerification(ctx, findingID, 3, []string{"this line is not in the body at all"}); st == db.VerificationVerified {
		t.Fatal("S1 assessment verified a quote that is not in the bound body")
	}

	// Unsigned and forged calls are refused.
	r, err := pinned.Post(api.URL+nodeRPCPrefix+"exists", "application/json", strings.NewReader(`{"traffic_id":"`+trafficID+`"}`))
	if err != nil {
		t.Fatal(err)
	}
	if r.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unsigned node RPC = %d, want 401", r.StatusCode)
	}
	_ = r.Body.Close()
	forged := newControlRPC(api.URL, "node-ev", func() []byte { return []byte("not the deployment key, 32 bytes") }, pinned)
	var out nodeExistsReply
	if err := forged.call(ctx, "exists", nodeExistsReq{TrafficID: trafficID}, &out); err == nil {
		t.Fatal("a node RPC signed with another key was served")
	}
}

func readAllClose(f *os.File) (string, error) {
	defer f.Close()
	raw, err := io.ReadAll(f)
	return string(raw), err
}

// A reply that is not signed under the deployment key, such as a forged
// "verified" on the path between the nodes, is never accepted.
func TestWorkerRefusesUnsignedReply(t *testing.T) {
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(hdrReplySig, strings.Repeat("0", 64))
		_, _ = w.Write([]byte(`{"status":"verified","note":""}`))
	}))
	defer fake.Close()
	key := []byte("0123456789abcdef0123456789abcdef")
	s := &Server{m: &Manager{controlRPC: newControlRPC(fake.URL, "node-x", func() []byte { return key }, nil)}}
	if st, note := s.assessFindingVerification(context.Background(), 1, 3, []string{"some quote long enough"}); st != db.VerificationNotAssessed {
		t.Fatalf("forged reply gave %s (%s), want not_assessed", st, note)
	}
}
