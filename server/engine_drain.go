package server

import (
	"log"
	"time"
)

// Delete/archive drain across nodes (E7, c-1d3e0a; docs/DISTRIBUTED.md).
// BeginDelete sets the task's 'deleting' fence and revokes the remote claims.
// Every worker node answers the fence with an ack once it holds no runs of the
// task (ackDrains). The control node's delete or archive waits for an ack from
// every live worker node before removing the task's data, up to
// remoteDrainAckBound. A node whose registry row went stale is not waited for:
// it has not reached the database for NodeTTL, so it is not writing to it.

// remoteDrainAckBound bounds the wait for worker nodes' acks. It is inside
// taskDeleteDrainTimeout, so a node that never answers delays a delete by the
// bound and the delete then goes on (logged), as before the acks existed. A
// variable so tests can shorten it.
var remoteDrainAckBound = 8 * time.Second

// remoteDrainRecheck is how often the wait re-reads the acks (waitTaskQuiescent
// ticks much faster for the local work).
const remoteDrainRecheck = 250 * time.Millisecond

// remoteDrain is one delete or archive's wait for the worker nodes' acks.
type remoteDrain struct {
	s      *Server
	taskID string
	n      int64
	bound  time.Time
	next   time.Time
	done   bool
}

// beginRemoteDrain starts the wait for the worker nodes' acks of taskID's
// fence; nil (always quiet) outside a distributed deployment. A claim that
// landed between BeginDelete and now is revoked here.
func (s *Server) beginRemoteDrain(taskID string) *remoteDrain {
	if s.engine == nil || !s.engine.distributed() {
		return nil
	}
	if t, ok := s.m.Task(taskID); ok {
		s.engine.revokeRemoteClaims(t)
	}
	n, ok := taskNumericID(taskID)
	if !ok {
		return nil
	}
	return &remoteDrain{s: s, taskID: taskID, n: n, bound: time.Now().Add(remoteDrainAckBound)}
}

// quiet reports whether the worker nodes are done with the task: every live
// worker node acked the fence, or the bound passed. A read error is not quiet
// (the bound still ends the wait).
func (d *remoteDrain) quiet(now time.Time) bool {
	if d == nil || d.done {
		return true
	}
	if now.Before(d.next) {
		return false
	}
	d.next = now.Add(remoteDrainRecheck)
	missing, err := d.s.m.pg.UnackedLiveWorkerNodes(d.n)
	if err == nil && len(missing) == 0 {
		d.done = true
		return true
	}
	if now.Before(d.bound) {
		return false
	}
	d.done = true
	if err != nil {
		log.Printf("[engine] task %s drain: worker node acks unreadable for %s; going on: %v", d.taskID, remoteDrainAckBound, err)
	} else {
		log.Printf("[engine] task %s drain: no ack from worker node(s) %v within %s; going on", d.taskID, missing, remoteDrainAckBound)
	}
	return true
}

// ackDrains answers the 'deleting' fences this worker node has not acked yet.
// A task still served here, or whose runs are still unwinding (StopTask has
// not returned), is acked on a later poll. It runs in the dispatch loop, the
// only place a task starts being served on this node, and the dispatch list
// leaves fenced tasks out, so no run can start between the check and the ack.
func (s *Server) ackDrains() {
	e, pg := s.engine, s.m.pg
	ids, err := pg.TasksAwaitingDrainAck(s.node.NodeID)
	if err != nil {
		log.Printf("[node %s] read delete fences: %v", s.node.NodeID, err)
		return
	}
	for _, id := range ids {
		e.wnode.mu.Lock()
		served := e.wnode.served[id]
		e.wnode.mu.Unlock()
		n, ok := taskNumericID(id)
		if served || e.hasRuntime(id) || !ok {
			continue
		}
		if _, err := pg.AckTaskDrain(n, s.node.NodeID); err != nil {
			log.Printf("[node %s] task %s: ack the delete fence: %v", s.node.NodeID, id, err)
		}
	}
}

// hasRuntime reports whether taskID's routines are registered here (StopTask
// removes them only after joining them).
func (e *Engine) hasRuntime(taskID string) bool {
	e.runtimeMu.Lock()
	defer e.runtimeMu.Unlock()
	return e.runtimes[taskID] != nil
}
