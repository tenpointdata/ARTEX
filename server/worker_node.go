package server

import (
	"context"
	"log"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Autumn-27/artex/agent"
	"github.com/Autumn-27/artex/db"
)

// Worker node runtime (E7, docs/DISTRIBUTED.md). A worker node has the same
// Manager, agents, guard hooks, intercept rules and target gate as the control
// node (server.New built them); it runs only intent worker loops. Everything
// the control node decides lives in the database, and this file reads it:
//
//   - which tasks to serve: DispatchableTaskIDs, rescanned every poll;
//   - whether a claim may start: GetTaskControl before every claim (paused,
//     queued, terminal, dispatch fence), plus the run budget the control node
//     published and the absence of a RoleAll node;
//   - what happens to running intents: pause and delete cancel them, a settle
//     fence lets them finish under the deadline / budget wrap-up;
//   - per-intent control (kill, pause, cancel, steer): work_signals.
//
// Every read failure blocks new claims (fail closed). Running intents keep
// heartbeating their lease; the control node can always fence them out by
// revoking the claim (RevokeRemoteIntentClaims).

// controlUnreachablePolls is how many consecutive failed control-state reads
// make a worker node pause a served task locally (about 30 s).
const controlUnreachablePolls = 15

// workerNodeState is the node-wide claim gate.
type workerNodeState struct {
	blocked atomic.Value // string: why this node may not claim ("" = may claim)
	mu      sync.Mutex
	served  map[string]bool // task ids whose worker loops run here
	misses  map[string]int  // consecutive failed control reads per served task
	// budgets is the run budget the control node published, as last read. Once
	// known it is kept across a failed read (the gate still blocks while the
	// registry or the control node cannot be read).
	budgetMu      sync.Mutex
	budgetKnown   bool
	budgetTask    agent.Budget
	budgetIntent  agent.Budget
	sharedEgress  string                // fingerprint of the shared egress settings last applied
	agentSettings agentSettingsFollower // touched by the dispatch loop only
}

func (w *workerNodeState) publishedBudgets() (task, intent agent.Budget, ok bool) {
	w.budgetMu.Lock()
	defer w.budgetMu.Unlock()
	return w.budgetTask, w.budgetIntent, w.budgetKnown
}

func (w *workerNodeState) setPublishedBudgets(task, intent agent.Budget) {
	w.budgetMu.Lock()
	w.budgetTask, w.budgetIntent, w.budgetKnown = task, intent, true
	w.budgetMu.Unlock()
}

func (w *workerNodeState) reason() string {
	r, _ := w.blocked.Load().(string)
	return r
}

func (w *workerNodeState) setBlocked(nodeID, why string) {
	if prev := w.reason(); prev != why {
		w.blocked.Store(why)
		if why != "" {
			log.Printf("[node %s] not claiming: %s", nodeID, why)
		} else {
			log.Printf("[node %s] claiming", nodeID)
		}
	}
}

// startWorkerNode starts the dispatcher and the work signal poller.
func (s *Server) startWorkerNode() {
	if s.m.pg == nil || s.ctx.Err() != nil {
		return
	}
	s.engine.wnode = &workerNodeState{served: map[string]bool{}, misses: map[string]int{}}
	s.engine.wnode.blocked.Store("starting")
	go s.workerDispatchLoop(s.ctx)
	go s.engine.workSignalLoop(s.ctx)
}

func (s *Server) workerDispatchLoop(ctx context.Context) {
	tick := time.NewTicker(distributedPoll)
	defer tick.Stop()
	for {
		s.workerDispatchOnce(ctx)
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

// workerDispatchOnce refreshes the node gate, starts worker loops for newly
// dispatchable tasks, applies pause/settle/delete to served tasks and stops
// the loops of tasks that ended.
func (s *Server) workerDispatchOnce(ctx context.Context) {
	e, pg, w := s.engine, s.m.pg, s.engine.wnode
	if task, intent, ok := readPublishedBudgets(pg); ok {
		w.setPublishedBudgets(task, intent)
	}
	_, _, budgetKnown := w.publishedBudgets()
	allNode, errAll := pg.LiveNodeWithRole(string(RoleAll), s.node.NodeID)
	control, errCtl := pg.LiveNodeWithRole(string(RoleControl), s.node.NodeID)
	egressChanged := s.syncSharedEgress()
	switch {
	case errAll != nil || errCtl != nil:
		w.setBlocked(s.node.NodeID, "node registry unreadable")
	case allNode:
		w.setBlocked(s.node.NodeID, "a live role=all node shares this database (its control state is in memory only); run it as role=control")
	case !control:
		// No deadline coordinator, budget settle or planner is running.
		w.setBlocked(s.node.NodeID, "no live control node")
	case !budgetKnown:
		w.setBlocked(s.node.NodeID, "no run budget published by a control node yet")
	case egressChanged:
		w.setBlocked(s.node.NodeID, "shared capture/proxy settings changed: rebuilding agents")
	default:
		w.setBlocked(s.node.NodeID, "")
	}
	s.followAgentSettings(&w.agentSettings)
	ids, err := pg.DispatchableTaskIDs()
	if err != nil {
		log.Printf("[node %s] list tasks: %v", s.node.NodeID, err)
		ids = nil
	}
	want := map[string]bool{}
	for _, id := range ids {
		want[id] = true
	}
	s.m.LoadExisting() // new tasks into the manager, so TaskStatus/Task see them
	w.mu.Lock()
	for id := range want {
		if !w.served[id] {
			w.served[id] = true
			if t, ok := s.m.Task(id); ok {
				if n, ok := taskNumericID(id); ok {
					t.gateLive = liveTargetGate(pg, n)
				}
				e.RunWorkers(ctx, t)
			}
		}
	}
	served := make([]string, 0, len(w.served))
	for id := range w.served {
		served = append(served, id)
	}
	w.mu.Unlock()
	for _, id := range served {
		if !s.syncServedTask(id) {
			w.mu.Lock()
			delete(w.served, id)
			delete(w.misses, id)
			w.mu.Unlock()
		}
	}
	s.ackDrains()
}

// syncSharedEgress applies a capture-switch or global-proxy change made on the
// control node (its API changes only the control process's memory and the
// settings rows). The task agents are dropped so the next claim builds them on
// the new proxy path. Reports whether anything changed this poll.
func (s *Server) syncSharedEgress() bool {
	fp, err := s.m.reloadSharedEgress()
	w := s.engine.wnode
	if err != nil {
		log.Printf("[node %s] read shared egress settings: %v", s.node.NodeID, err)
		return true // block claims this poll: the path is unknown
	}
	if w.sharedEgress == fp {
		return false
	}
	first := w.sharedEgress == ""
	w.sharedEgress = fp
	if first {
		return false
	}
	log.Printf("[node %s] capture/proxy settings changed on the control node; rebuilding task agents", s.node.NodeID)
	s.invalidateTaskAgents()
	return true
}

// syncServedTask mirrors one served task's control state into this process
// and returns false once its loops were stopped (task gone, deleting, ended).
func (s *Server) syncServedTask(id string) bool {
	e := s.engine
	t, ok := s.m.Task(id)
	n, okID := taskNumericID(id)
	if !ok || !okID {
		e.StopTask(id)
		return false
	}
	c, err := s.m.pg.GetTaskControl(n)
	w := e.wnode
	if err != nil {
		// Claims are blocked by workerClaimAllowed's own read; after a while the
		// running intents stop too, so a pause on the control node cannot be missed
		// for longer than this.
		w.mu.Lock()
		w.misses[id]++
		miss := w.misses[id]
		w.mu.Unlock()
		if miss == controlUnreachablePolls && !e.IsPaused(id) {
			log.Printf("[node %s] task %s: control state unreadable for %d polls, pausing locally: %v", s.node.NodeID, id, miss, err)
			e.Pause(id, agent.AbortControlUnreachable)
		}
		return true
	}
	w.mu.Lock()
	delete(w.misses, id)
	w.mu.Unlock()
	if !c.Exists || c.DispatchClosed == db.DispatchDeleting || db.IsTerminal(c.Status) {
		// Asynchronously: one task's runs unwinding must not hold up the
		// pause/settle sync of the others. Running intents are cancelled; their
		// writes are fenced anyway.
		go func() {
			e.StopTask(id)
			s.m.forgetTaskHandle(id)
		}()
		return false
	}
	t.updateLifecycle(func(st *taskLifecycleState) {
		st.Status, st.Paused, st.Queued, st.DeadlineAt, st.FirstRunAt = c.Status, c.Paused, c.Queued, c.DeadlineAt, c.FirstRunAt
	})
	if c.DeadlineAt > 0 {
		e.deadline.Store(id, c.DeadlineAt)
	}
	// A queued task is parked by the control node with a pause too (admission,
	// LLM unavailable), so its running intents stop here as well.
	hold := c.Paused || c.Queued
	switch {
	case hold && !e.IsPaused(id):
		e.Pause(id, agent.AbortPausedByUser) // running intents stop as on the control node
	case !hold && e.IsPaused(id):
		e.Resume(t)
	}
	switch {
	case c.DispatchClosed == db.DispatchSettling && !e.isSettling(id):
		e.markSettling(id)
		if l := e.budgetFor(id); l != nil {
			l.BeginWrapup() // running intents' meters report no budget left
		}
	case c.DispatchClosed == "" && e.isSettling(id):
		e.settling.Delete(id) // revived: the next ledger is seeded afresh
		e.budgets.Delete(id)
	}
	return true
}

// workerClaimAllowed is the worker node's gate before each claim, read fresh
// from the database. Any failure denies the claim.
func (e *Engine) workerClaimAllowed(t *Task) bool {
	if e.wnode == nil || e.wnode.reason() != "" {
		return false
	}
	n, ok := taskNumericID(t.ID)
	if !ok {
		return false
	}
	c, err := e.m.pg.GetTaskControl(n)
	return err == nil && c.Claimable()
}

// RunWorkers starts only the worker loops of a task (a worker node's Run).
func (e *Engine) RunWorkers(ctx context.Context, t *Task) {
	workers := e.node.localWorkers(e.m.Workers)
	e.deleteMu.RLock()
	if e.IsDeleting(t.ID) {
		e.deleteMu.RUnlock()
		return
	}
	if _, loaded := e.started.LoadOrStore(t.ID, true); loaded {
		e.deleteMu.RUnlock()
		return
	}
	rt := e.registerTaskRoutines(ctx, t.ID, workers)
	e.deleteMu.RUnlock()
	if rt == nil {
		e.started.Delete(t.ID)
		return
	}
	log.Printf("[node %s] serving task %s with %d worker(s)", e.node.NodeID, t.ID, workers)
	for i := 0; i < workers; i++ {
		name := e.node.workerName(i + 1)
		runTaskRoutine(rt, func(loopCtx context.Context) { e.workerLoop(loopCtx, t, name) })
	}
}

// workSignalLoop delivers kill / pause / cancel / steer requests sent by the
// control node to the intents running in this process.
func (e *Engine) workSignalLoop(ctx context.Context) {
	tick := time.NewTicker(distributedPoll)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
		e.workMu.Lock()
		ids := make([]int64, 0, len(e.work))
		for id := range e.work {
			ids = append(ids, id)
		}
		e.workMu.Unlock()
		e.syncServedBudgets() // running intents' meters see the other nodes' spend
		sigs, err := e.m.pg.TakeWorkSignals(ids, e.node.NodeID)
		if err != nil {
			log.Printf("[node %s] work signals: %v", e.node.NodeID, err)
			continue
		}
		for _, sg := range sigs {
			e.applyWorkSignal(sg)
		}
	}
}

func (e *Engine) applyWorkSignal(sg db.WorkSignal) {
	e.workMu.Lock()
	run := e.work[sg.IntentID]
	if run == nil {
		e.workMu.Unlock()
		return // finished meanwhile
	}
	switch sg.Action {
	case db.WorkSignalKill:
		run.cancel(agent.AbortKilledByPlanner)
	case db.WorkSignalPause, db.WorkSignalCancel:
		if run.action == "" {
			run.action = sg.Action
			cause := error(agent.AbortWorkPausedByUser)
			if sg.Action == db.WorkSignalCancel {
				cause = agent.AbortWorkCancelledByUser
			}
			run.cancel(cause)
		}
	case db.WorkSignalSteer:
		e.workMu.Unlock()
		e.steerMu.Lock()
		e.steerBox[sg.IntentID] = append(e.steerBox[sg.IntentID], sg.Message)
		e.steerMu.Unlock()
		log.Printf("[node %s] intent #%d: steer delivered", e.node.NodeID, sg.IntentID)
		return
	}
	e.workMu.Unlock()
	log.Printf("[node %s] intent #%d: %s delivered", e.node.NodeID, sg.IntentID, sg.Action)
}

// WorkerHealthHandler serves a worker node's only endpoint: GET /healthz is 200
// while the database answers and 503 otherwise. It carries no task data.
func (s *Server) WorkerHealthHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		if err := s.m.pg.PingContext(ctx); err != nil {
			http.Error(w, "database unavailable", http.StatusServiceUnavailable)
			return
		}
		state := "claiming"
		if s.engine.wnode != nil && s.engine.wnode.reason() != "" {
			state = "not claiming"
		}
		_, _ = w.Write([]byte("ok " + s.node.NodeID + " " + state + "\n"))
	})
	return mux
}
