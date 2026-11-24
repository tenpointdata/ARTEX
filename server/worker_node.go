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

// workerNodeState is the node-wide claim gate.
type workerNodeState struct {
	blocked atomic.Value // string: why this node may not claim ("" = may claim)
	mu      sync.Mutex
	served  map[string]bool // task ids whose worker loops run here
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
	s.engine.wnode = &workerNodeState{served: map[string]bool{}}
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
	captureErr := s.syncTrafficCapture()
	switch allNode, err := pg.LiveNodeWithRole(string(RoleAll), s.node.NodeID); {
	case captureErr != nil:
		w.setBlocked(s.node.NodeID, "traffic-capture setting unreadable: "+captureErr.Error())
	case err != nil:
		w.setBlocked(s.node.NodeID, "node registry unreadable: "+err.Error())
	case allNode:
		w.setBlocked(s.node.NodeID, "a live role=all node shares this database (its control state is in memory only); run it as role=control")
	default:
		if _, _, ok := readPublishedBudgets(pg); !ok {
			w.setBlocked(s.node.NodeID, "no run budget published by a control node yet")
		} else {
			w.setBlocked(s.node.NodeID, "")
		}
	}
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
			w.mu.Unlock()
		}
	}
}

// syncTrafficCapture follows the control node's traffic-capture switch. When it
// flips, the task agent bundles are dropped (as applyLLM does on the control
// node) so each task's next run is built with the new proxy URL, CA, traffic
// tools (HostTools) and prompt note. An intent already running keeps the
// bundle it started with until it ends. A read error is returned so the caller
// blocks new claims: the node cannot tell whether its traffic must be recorded.
func (s *Server) syncTrafficCapture() error {
	changed, err := s.m.reloadTrafficEnabled()
	if err != nil {
		return err
	}
	if changed {
		log.Printf("[node %s] traffic capture switched %s on the control node; rebuilding task agents", s.node.NodeID, onOff(s.m.TrafficEnabled()))
		s.invalidateTaskAgents()
	}
	return nil
}

func onOff(on bool) string {
	if on {
		return "on"
	}
	return "off"
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
	if err != nil {
		return true // claims are blocked by workerClaimAllowed's own read
	}
	if !c.Exists || c.DispatchClosed == db.DispatchDeleting || db.IsTerminal(c.Status) {
		e.StopTask(id) // cancels running intents; their writes are fenced anyway
		s.m.forgetTaskHandle(id)
		return false
	}
	t.updateLifecycle(func(st *taskLifecycleState) {
		st.Status, st.Paused, st.Queued, st.DeadlineAt, st.FirstRunAt = c.Status, c.Paused, c.Queued, c.DeadlineAt, c.FirstRunAt
	})
	if c.DeadlineAt > 0 {
		e.deadline.Store(id, c.DeadlineAt)
	}
	switch {
	case c.Paused && !e.IsPaused(id):
		e.Pause(id, agent.AbortPausedByUser) // running intents stop as on the control node
	case !c.Paused && e.IsPaused(id):
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
