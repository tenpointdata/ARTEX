package server

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Autumn-27/artex/agent"
	"github.com/Autumn-27/artex/db"
	"github.com/Autumn-27/artex/intercept"
	"github.com/Autumn-27/artex/transcriptsync"
	"github.com/Autumn-27/norma/llm"
	"github.com/Autumn-27/norma/transcript"
)

// c-29ff19 done-criterion: an intent claimed on node A, whose claim is then
// taken from it, resumes on node B from A's transcript (shared through
// PostgreSQL), not from scratch.

// firstNodeProvider runs `echo marker` on the test task's first turn, then
// holds the run (and so the claim) until the node stops.
type firstNodeProvider struct {
	taskID, marker string
	holding        chan struct{}
	once           atomic.Bool
}

func (p *firstNodeProvider) Stream(ctx context.Context, req llm.CompletionRequest) iter.Seq2[llm.StreamEvent, error] {
	scope, _ := intercept.TargetScopeFromContext(ctx)
	events := []llm.StreamEvent{{Type: llm.SETextDelta, Text: "done"}, {Type: llm.SEMessageDelta, StopReason: "end_turn"}}
	switch {
	case scope.TaskID != p.taskID:
	case !hasToolResult(req):
		events = []llm.StreamEvent{
			{Type: llm.SEToolUseStart, ToolID: "call-a", ToolName: "Bash"},
			{Type: llm.SEToolInputJSON, Text: `{"command":"echo ` + p.marker + `"}`},
			{Type: llm.SEMessageDelta, StopReason: "tool_use"},
		}
	default:
		if p.once.CompareAndSwap(false, true) {
			close(p.holding)
		}
		<-ctx.Done()
		return func(yield func(llm.StreamEvent, error) bool) { yield(llm.StreamEvent{}, context.Cause(ctx)) }
	}
	return streamOf(events)
}

func (p *firstNodeProvider) Complete(ctx context.Context, req llm.CompletionRequest) (llm.Message, string, llm.Usage, error) {
	return accumulateStreamForTest(ctx, p.Stream, req)
}

// resumingProvider records whether its first request for the test task
// carries the marker (A's tool result), then ends the run.
type resumingProvider struct {
	taskID, marker string
	sawMarker      atomic.Int32 // 0 unseen, 1 carried, -1 missing
}

func (p *resumingProvider) Stream(ctx context.Context, req llm.CompletionRequest) iter.Seq2[llm.StreamEvent, error] {
	scope, _ := intercept.TargetScopeFromContext(ctx)
	if scope.TaskID == p.taskID {
		seen := int32(-1)
		if b, err := json.Marshal(req.Messages); err == nil && strings.Contains(string(b), p.marker) {
			seen = 1
		}
		p.sawMarker.CompareAndSwap(0, seen)
	}
	return streamOf([]llm.StreamEvent{{Type: llm.SETextDelta, Text: "resumed on B"}, {Type: llm.SEMessageDelta, StopReason: "end_turn"}})
}

func (p *resumingProvider) Complete(ctx context.Context, req llm.CompletionRequest) (llm.Message, string, llm.Usage, error) {
	return accumulateStreamForTest(ctx, p.Stream, req)
}

func streamOf(events []llm.StreamEvent) iter.Seq2[llm.StreamEvent, error] {
	return func(yield func(llm.StreamEvent, error) bool) {
		for _, ev := range events {
			if !yield(ev, nil) {
				return
			}
		}
	}
}

// startSharingWorkerNode is startTestWorkerNode with a transcript store and
// the PostgreSQL mirror, as agentsForTask wires them on a distributed node.
func startSharingWorkerNode(t *testing.T, nodeID string, p llm.Provider) *testNode {
	t.Helper()
	m, err := NewManager(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	e := NewEngine(m)
	e.node = NodeConfig{Role: RoleWorker, NodeID: nodeID, Host: nodeID, Workers: 1}
	s := &Server{m: m, engine: e, ctx: ctx, node: e.node, chatBusy: map[string]bool{},
		chatCancel: map[string]context.CancelCauseFunc{}, taskAgents: map[string]*taskAgentBundle{}}
	worker := agent.NewWorker(p, "test-model", m.dir, transcript.NewStore(filepath.Join(m.dir, "transcripts")), 0, 6)
	worker.SetTranscriptMirror(&transcriptsync.Mirror{Backend: m.pg.SessionTranscripts(), Interval: 100 * time.Millisecond})
	e.SetAuthoritativeAgentResolver(func(*Task) (*agent.Planner, *agent.Worker) { return nil, worker })
	go s.runNodeHeartbeat(ctx)
	s.startWorkerNode()
	n := &testNode{m: m, e: e, s: s, cancel: cancel}
	t.Cleanup(func() { n.stop(); _ = m.Close() })
	return n
}

func (n *testNode) stop() {
	n.cancel()
	wctx, done := context.WithTimeout(context.Background(), 30*time.Second)
	defer done()
	_ = n.e.Wait(wctx)
}

func TestIntentResumesOnAnotherNodeFromSharedTranscript(t *testing.T) {
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
	keepControlLive(t, pg)
	created, err := control.CreateTaskWithOptions("c-29ff19 resume", "an intent resumes across nodes", db.TaskCreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = control.DeleteTask(created.ID, DeleteTaskOptions{}) })
	task, _ := control.Task(created.ID)
	marker := fmt.Sprintf("resume-marker-%d", time.Now().UnixNano())
	session := ""

	// Node A claims the intent, runs a tool, and holds the claim.
	pa := &firstNodeProvider{taskID: task.ID, marker: marker, holding: make(chan struct{})}
	a := startSharingWorkerNode(t, "node-a", pa)
	intent, err := task.Store.AddIntent(map[string]any{"summary": "resume across nodes"}, 1, nil, "planner")
	if err != nil {
		t.Fatal(err)
	}
	session = agent.WorkerSessionID(task.ExpID, intent)
	select {
	case <-pa.holding:
	case <-time.After(30 * time.Second):
		t.Fatal("node-a never reached its second turn")
	}
	waitUntilFor(t, 10*time.Second, "node-a's tool result in the shared transcript", func() bool {
		lines, _ := pg.SessionTranscripts().Lines(context.Background(), session)
		return strings.Contains(string(joinLines(lines)), marker)
	})

	// Kill node A's claim: the control node revokes it (as on a drain timeout
	// or delete) and node A goes away. Its local transcript goes with it.
	if n, err := pg.RevokeRemoteIntentClaims(task.ExpID); err != nil || n != 1 {
		t.Fatalf("revoke node-a's claim = %d, %v", n, err)
	}
	a.stop()
	if st, _ := task.Store.IntentState(intent); st != "open" {
		t.Fatalf("intent after node-a's claim was revoked = %q, want open", st)
	}

	// Node B claims it and resumes from A's transcript.
	pb := &resumingProvider{taskID: task.ID, marker: marker}
	startSharingWorkerNode(t, "node-b", pb)
	waitUntilFor(t, 60*time.Second, "node-b to settle the intent", func() bool {
		st, _ := task.Store.IntentState(intent)
		return st != "open" && st != "running"
	})
	if got := pb.sawMarker.Load(); got != 1 {
		t.Fatalf("node-b's first request carried node-a's history = %d, want 1 (resumed, not restarted)", got)
	}
	var owner string
	if err := pg.QueryRow(`SELECT COALESCE(owner,'') FROM exploration_nodes WHERE id=$1`, intent).Scan(&owner); err != nil || owner != "node-b/work#1" {
		t.Fatalf("intent owner = %q (%v), want node-b/work#1", owner, err)
	}
	waitUntilFor(t, 10*time.Second, "node-b's reply after node-a's history in the shared transcript", func() bool {
		all := string(joinLines(mustLines(t, pg, session)))
		i, j := strings.Index(all, marker), strings.Index(all, "resumed on B")
		return i >= 0 && j > i
	})
}

func mustLines(t *testing.T, pg *db.DB, session string) [][]byte {
	t.Helper()
	lines, err := pg.SessionTranscripts().Lines(context.Background(), session)
	if err != nil {
		t.Fatal(err)
	}
	return lines
}

func joinLines(lines [][]byte) []byte {
	var out []byte
	for _, l := range lines {
		out = append(append(out, l...), '\n')
	}
	return out
}
