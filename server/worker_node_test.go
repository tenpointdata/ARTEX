package server

import (
	"context"
	"fmt"
	"iter"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Autumn-27/artex/agent"
	"github.com/Autumn-27/artex/db"
	"github.com/Autumn-27/artex/intercept"
	"github.com/Autumn-27/norma/llm"
)

// E7 done-criterion: two worker nodes (separate processes in production; here
// separate Manager/Engine/Interceptor instances sharing only the database)
// execute one task's intents, and each runs them through the same gating: the
// task's Guard, an intercept rule added on the control node after the workers
// started, and the target-gate switch flipped on the control node.

// barrierProvider is a worker model for one node. On its first turn for the
// test task it waits until every node has arrived (so each node holds one
// intent at a time and both nodes must work), then calls Bash with command;
// on the next turn it ends. Turns of any other task end at once.
type barrierProvider struct {
	taskID, command string
	arrive          func() <-chan struct{}
}

func (p *barrierProvider) Stream(ctx context.Context, req llm.CompletionRequest) iter.Seq2[llm.StreamEvent, error] {
	scope, _ := intercept.TargetScopeFromContext(ctx)
	events := []llm.StreamEvent{{Type: llm.SETextDelta, Text: "done"}, {Type: llm.SEMessageDelta, StopReason: "end_turn"}}
	if scope.TaskID == p.taskID && !hasToolResult(req) {
		select {
		case <-p.arrive():
		case <-ctx.Done():
		case <-time.After(30 * time.Second):
		}
		events = []llm.StreamEvent{
			{Type: llm.SEToolUseStart, ToolID: "call-1", ToolName: "Bash"},
			{Type: llm.SEToolInputJSON, Text: `{"command":"` + p.command + `"}`},
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

func (p *barrierProvider) Complete(ctx context.Context, req llm.CompletionRequest) (llm.Message, string, llm.Usage, error) {
	return accumulateStreamForTest(ctx, p.Stream, req)
}

// hasToolResult reports whether the conversation already carries a tool result
// (the run is past its first turn).
func hasToolResult(req llm.CompletionRequest) bool {
	for _, m := range req.Messages {
		for _, b := range m.Content {
			if b.Type == llm.BlockToolResult {
				return true
			}
		}
	}
	return false
}

// barrier opens once n parties have arrived.
type barrier struct {
	mu   sync.Mutex
	n    int
	open chan struct{}
}

func newBarrier(n int) *barrier { return &barrier{n: n, open: make(chan struct{})} }

func (b *barrier) arrive() <-chan struct{} {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.n--; b.n == 0 {
		close(b.open)
	}
	return b.open
}

type testNode struct {
	m      *Manager
	e      *Engine
	s      *Server
	cancel context.CancelFunc
}

func startTestWorkerNode(t *testing.T, nodeID string, p llm.Provider) *testNode {
	t.Helper()
	m, err := NewManager(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	return startTestNodeOn(t, m, NodeConfig{Role: RoleWorker, NodeID: nodeID, Host: nodeID, Workers: 1}, p)
}

// startTestNodeOn runs m as the worker node node, with p as its worker model.
// It closes m when the test ends.
func startTestNodeOn(t *testing.T, m *Manager, node NodeConfig, p llm.Provider) *testNode {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	e := NewEngine(m)
	e.node = node
	s := &Server{m: m, engine: e, ctx: ctx, node: e.node, chatBusy: map[string]bool{},
		chatCancel: map[string]context.CancelCauseFunc{}, taskAgents: map[string]*taskAgentBundle{}}
	worker := agent.NewWorker(p, "test-model", m.dir, nil, 0, 3)
	e.SetAuthoritativeAgentResolver(func(*Task) (*agent.Planner, *agent.Worker) { return nil, worker })
	go s.runNodeHeartbeat(ctx)
	s.startWorkerNode()
	n := &testNode{m: m, e: e, s: s, cancel: cancel}
	t.Cleanup(func() {
		cancel()
		wctx, done := context.WithTimeout(context.Background(), 30*time.Second)
		defer done()
		_ = e.Wait(wctx)
		_ = m.Close()
	})
	return n
}

func TestTwoWorkerNodesShareOneTaskWithIdenticalGating(t *testing.T) {
	skipWithoutDSN(t)
	control, err := NewManager(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = control.Close() })
	pg := control.pg
	// A role=all row left by another test would (correctly) stop the nodes.
	if _, err := pg.Exec(`DELETE FROM worker_nodes`); err != nil {
		t.Fatal(err)
	}
	if err := publishBudgets(pg); err != nil {
		t.Fatal(err)
	}
	created, err := control.CreateTaskWithOptions("e7 two nodes", "two worker nodes share one task", db.TaskCreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = control.DeleteTask(created.ID, DeleteTaskOptions{}) })
	task, _ := control.Task(created.ID)
	marker := fmt.Sprintf("e7-gate-%d", time.Now().UnixNano())
	intents := make([]int64, 2)
	for i := range intents {
		if intents[i], err = task.Store.AddIntent(map[string]any{"summary": fmt.Sprintf("probe %d", i)}, 1, nil, "planner"); err != nil {
			t.Fatal(err)
		}
	}

	// The workers start before the rule exists: they must pick it up from the
	// shared revision, not from a cache loaded at start.
	b := newBarrier(3) // both nodes and this test, once the rule and the gate are in place
	nodes := map[string]*testNode{}
	for _, id := range []string{"node-b", "node-c"} {
		nodes[id] = startTestWorkerNode(t, id, &barrierProvider{taskID: task.ID, command: "echo " + marker, arrive: b.arrive})
	}
	rule, err := pg.CreateInterceptRule("deny "+marker, "tool_input", "string", marker, "deny", "", 1000020, true, false, 0, "deny")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pg.DeleteInterceptRule(rule.ID); _, _ = pg.BumpInterceptRulesRevision() })
	if err := control.interceptor.Invalidate(); err != nil {
		t.Fatal(err)
	}
	// The gate switch made on the control node is what the nodes' scopes read.
	if _, err := control.UpdateTaskMetadata(task.ID, db.TaskPatch{TargetGate: ptr(true)}); err != nil {
		t.Fatal(err)
	}

	b.arrive()
	waitUntilFor(t, 60*time.Second, "both intents to settle", func() bool {
		for _, id := range intents {
			if st, _ := task.Store.IntentState(id); st == "open" || st == "running" {
				return false
			}
		}
		return true
	})
	owners := map[string]bool{}
	for _, id := range intents {
		var owner string
		if err := pg.QueryRow(`SELECT COALESCE(owner,'') FROM exploration_nodes WHERE id=$1`, id).Scan(&owner); err != nil {
			t.Fatal(err)
		}
		owners[owner] = true
	}
	if !owners["node-b/work#1"] || !owners["node-c/work#1"] {
		t.Fatalf("intent owners = %v, want one intent run by each node", owners)
	}
	for id, n := range nodes {
		nt, ok := n.m.Task(task.ID)
		if !ok {
			t.Fatalf("%s does not serve the task", id)
		}
		var blocked []string
		for _, a := range nt.Guard.Audit() {
			if a.Tool == "Bash" && a.Action == "block" {
				blocked = append(blocked, a.Reason)
			}
		}
		if len(blocked) != 1 {
			t.Fatalf("%s: Bash blocks = %q, want the control node's rule to block its one call", id, blocked)
		}
		if !nt.targetScope(0).Enabled() {
			t.Fatalf("%s: the target gate switched on at the control node reads off", id)
		}
	}
	nodesRows, err := pg.ListNodes()
	if err != nil {
		t.Fatal(err)
	}
	live := 0
	for _, r := range nodesRows {
		if r.Live && r.Role == "worker" && strings.HasPrefix(r.NodeID, "node-") {
			live++
		}
	}
	if live != 2 {
		t.Fatalf("registry = %+v, want two live worker nodes", nodesRows)
	}
}

func ptr[T any](v T) *T { return &v }

func waitUntilFor(t *testing.T, d time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(d)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// holdProvider blocks the first run of the test task until its context is
// cancelled (so the control node can kill it); every other run ends at once.
type holdProvider struct {
	taskID  string
	held    atomic.Bool
	started chan struct{}
}

func (p *holdProvider) Stream(ctx context.Context, _ llm.CompletionRequest) iter.Seq2[llm.StreamEvent, error] {
	scope, _ := intercept.TargetScopeFromContext(ctx)
	if scope.TaskID == p.taskID && p.held.CompareAndSwap(false, true) {
		close(p.started)
		<-ctx.Done()
		return func(yield func(llm.StreamEvent, error) bool) { yield(llm.StreamEvent{}, context.Cause(ctx)) }
	}
	return func(yield func(llm.StreamEvent, error) bool) {
		for _, ev := range []llm.StreamEvent{{Type: llm.SETextDelta, Text: "done"}, {Type: llm.SEMessageDelta, StopReason: "end_turn"}} {
			if !yield(ev, nil) {
				return
			}
		}
	}
}

func (p *holdProvider) Complete(ctx context.Context, req llm.CompletionRequest) (llm.Message, string, llm.Usage, error) {
	return accumulateStreamForTest(ctx, p.Stream, req)
}

// A worker node follows the control node's decisions that live in the
// database: a planner kill of an intent it runs, a task pause, and the absence
// of a published run budget (it claims nothing rather than run unmetered).
func TestWorkerNodeFollowsControlState(t *testing.T) {
	skipWithoutDSN(t)
	control, err := NewManager(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = control.Close() })
	pg := control.pg
	if _, err := pg.Exec(`DELETE FROM worker_nodes`); err != nil {
		t.Fatal(err)
	}
	if err := publishBudgets(pg); err != nil {
		t.Fatal(err)
	}
	ce := NewEngine(control)
	ce.node = NodeConfig{Role: RoleControl, NodeID: "ctl", Workers: 0}
	created, err := control.CreateTaskWithOptions("e7 control state", "a worker node follows the control node", db.TaskCreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = control.DeleteTask(created.ID, DeleteTaskOptions{}) })
	task, _ := control.Task(created.ID)
	p := &holdProvider{taskID: task.ID, started: make(chan struct{})}
	startTestWorkerNode(t, "node-k", p)
	state := func(id int64) string { st, _ := task.Store.IntentState(id); return st }

	// Kill: the intent runs on node-k; the control engine has no local handle.
	held, err := task.Store.AddIntent(map[string]any{"summary": "held"}, 1, nil, "planner")
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-p.started:
	case <-time.After(30 * time.Second):
		t.Fatal("node-k never started the intent")
	}
	if err := ce.KillWork(held); err != nil {
		t.Fatalf("control KillWork of a remote intent: %v", err)
	}
	waitUntilFor(t, 30*time.Second, "the killed intent to leave running", func() bool { return state(held) != "running" })
	var consumer string
	if err := pg.QueryRow(`SELECT consumed_by FROM work_signals WHERE intent_id=$1 AND action='kill'`, held).Scan(&consumer); err != nil || consumer != "node-k" {
		t.Fatalf("kill signal consumed by %q (%v), want node-k", consumer, err)
	}

	// Pause, set through the control node's manager: no new claim.
	if err := control.SetTaskPaused(task.ID, true); err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * distributedPoll)
	paused, err := task.Store.AddIntent(map[string]any{"summary": "after pause"}, 1, nil, "planner")
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(3 * distributedPoll)
	if st := state(paused); st != "open" {
		t.Fatalf("intent of a paused task is %q on the worker node, want open", st)
	}
	if err := control.SetTaskPaused(task.ID, false); err != nil {
		t.Fatal(err)
	}
	waitUntilFor(t, 30*time.Second, "the intent to run after resume", func() bool { return state(paused) == "done" })

	// No published budget: fail closed.
	if _, err := pg.Exec(`DELETE FROM settings WHERE key=$1`, settingDistributedBudgets); err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * distributedPoll)
	unmetered, err := task.Store.AddIntent(map[string]any{"summary": "no budget"}, 1, nil, "planner")
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(3 * distributedPoll)
	if st := state(unmetered); st != "open" {
		t.Fatalf("intent claimed with no published budget: %q", st)
	}
	if err := publishBudgets(pg); err != nil {
		t.Fatal(err)
	}
	waitUntilFor(t, 30*time.Second, "the intent to run once the budget is published", func() bool { return state(unmetered) == "done" })
}
