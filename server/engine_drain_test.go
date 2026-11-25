package server

import (
	"context"
	"testing"
	"time"

	"github.com/Autumn-27/artex/db"
)

// drainFixture is a control node (manager, engine and a server for the delete
// path) and one task, with the registry cleared, a run budget published and a
// live control row, so worker nodes claim.
type drainFixture struct {
	control *Manager
	ce      *Engine
	cs      *Server
	task    *Task
}

func newDrainFixture(t *testing.T, bound time.Duration) *drainFixture {
	t.Helper()
	skipWithoutDSN(t)
	prev := remoteDrainAckBound
	remoteDrainAckBound = bound
	t.Cleanup(func() { remoteDrainAckBound = prev })
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
	ce := NewEngine(control)
	ce.node = NodeConfig{Role: RoleControl, NodeID: "ctl", Workers: 0}
	cs := &Server{m: control, engine: ce, ctx: context.Background(), node: ce.node, chatBusy: map[string]bool{},
		chatCancel: map[string]context.CancelCauseFunc{}, taskAgents: map[string]*taskAgentBundle{}}
	created, err := control.CreateTaskWithOptions("e7 drain ack", "a delete waits for the worker nodes", db.TaskCreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = control.DeleteTask(created.ID, DeleteTaskOptions{}) })
	task, _ := control.Task(created.ID)
	return &drainFixture{control: control, ce: ce, cs: cs, task: task}
}

// drain runs the delete path's barrier and wait, and reports how long it took.
func (f *drainFixture) drain(t *testing.T) time.Duration {
	t.Helper()
	start := time.Now()
	if !f.ce.BeginDelete(f.task.ID) {
		t.Fatal("BeginDelete refused")
	}
	ctx, cancel := context.WithTimeout(context.Background(), taskDeleteDrainTimeout)
	defer cancel()
	if err := f.cs.waitTaskQuiescent(ctx, f.task.ID); err != nil {
		t.Fatalf("waitTaskQuiescent: %v", err)
	}
	return time.Since(start)
}

// A delete waits for the worker node that runs one of the task's intents to
// stop it and acknowledge the fence, and no longer than that.
func TestDeleteWaitsForWorkerNodeDrainAck(t *testing.T) {
	f := newDrainFixture(t, 9*time.Second) // never reached: the ack ends the wait
	p := &holdProvider{taskID: f.task.ID, started: make(chan struct{})}
	node := startTestWorkerNode(t, "node-drain", p)
	if _, err := f.task.Store.AddIntent(map[string]any{"summary": "held"}, 1, nil, "planner"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-p.started:
	case <-time.After(30 * time.Second):
		t.Fatal("the worker node never started the intent")
	}

	took := f.drain(t)
	if took >= remoteDrainAckBound {
		t.Fatalf("drain took %s: it ran into the bound instead of ending on the ack", took)
	}
	if node.e.hasRuntime(f.task.ID) {
		t.Fatal("drain ended while the worker node still had the task's routines")
	}
	n, _ := taskNumericID(f.task.ID)
	if missing, err := f.control.pg.UnackedLiveWorkerNodes(n); err != nil || len(missing) != 0 {
		t.Fatalf("unacked live worker nodes after the drain = %v (%v)", missing, err)
	}

	// The acks answer one fence: reopening dispatch clears them.
	f.ce.AbortDelete(f.task.ID, false)
	if missing, err := f.control.pg.UnackedLiveWorkerNodes(n); err != nil || len(missing) != 1 {
		t.Fatalf("unacked live worker nodes after the fence was lifted = %v (%v), want node-drain", missing, err)
	}
}

// A live registry row whose node never answers (gone, partitioned) holds the
// delete for the bound, then the delete goes on.
func TestDeleteDrainWaitsForTheBoundWhenAWorkerNodeIsGone(t *testing.T) {
	const bound = 1500 * time.Millisecond
	f := newDrainFixture(t, bound)

	// No worker node: nothing to wait for.
	if took := f.drain(t); took >= bound {
		t.Fatalf("drain with no worker node took %s", took)
	}
	f.ce.AbortDelete(f.task.ID, false)

	gone := db.WorkerNode{NodeID: "node-gone", Role: string(RoleWorker), Host: "gone"}
	if err := f.control.pg.HeartbeatNode(gone, true); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.control.pg.RemoveNode(gone.NodeID) })
	took := f.drain(t)
	if took < bound {
		t.Fatalf("drain took %s with an unanswered live worker node, want at least the %s bound", took, bound)
	}
	if took > bound+2*time.Second {
		t.Fatalf("drain took %s, well past the %s bound", took, bound)
	}
}
