package server

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/Autumn-27/artex/agent"
	"github.com/Autumn-27/artex/db"
)

// Engine hooks for distributed intent workers (E7, docs/DISTRIBUTED.md). Each is
// a no-op for RoleAll, so the single-process deployment is unchanged.

// distributedPoll is how often the control node looks for remote settlements
// and activity, and a worker node re-reads task control state and signals.
const distributedPoll = 2 * time.Second

func (e *Engine) distributed() bool { return e.node.Distributed() }

func taskNumericID(taskID string) (int64, bool) {
	n, err := strconv.ParseInt(taskID, 10, 64)
	return n, err == nil
}

// closeDispatch sets the task's dispatch fence so no node claims another of
// its intents. Called on the first settle (timeout or budget wrap-up) and when
// a delete begins. A write failure is logged: the control node's own loops are
// already stopped by the in-memory flags, and worker nodes fail closed on a
// control-state read error, never on a missing fence alone, so it is retried by
// the drain below.
func (e *Engine) closeDispatch(taskID, reason string) {
	if !e.distributed() || e.m == nil || e.m.pg == nil {
		return
	}
	if n, ok := taskNumericID(taskID); ok {
		if err := e.m.pg.SetTaskDispatchClosed(n, reason); err != nil {
			log.Printf("[engine] task %s dispatch fence %q: %v", taskID, reason, err)
		}
	}
}

// openDispatch clears the fence when a settled run is revived.
func (e *Engine) openDispatch(taskID string) {
	if e.m == nil || e.m.pg == nil {
		return
	}
	if n, ok := taskNumericID(taskID); ok {
		if err := e.m.pg.SetTaskDispatchClosed(n, ""); err != nil {
			log.Printf("[engine] task %s reopen dispatch: %v", taskID, err)
		}
	}
}

// remoteRunning counts the task's live claims held by worker nodes (local runs
// are already in inflight). A read error counts as one, so a drain waits
// (bounded by its grace) instead of settling over live work.
func (e *Engine) remoteRunning(t *Task) int64 {
	if !e.distributed() || e.m == nil || e.m.pg == nil {
		return 0
	}
	n, err := e.m.pg.LiveRemoteRunningIntents(t.ExpID)
	if err != nil {
		log.Printf("[engine] task %s count remote claims: %v", t.ID, err)
		return 1
	}
	return n
}

// revokeRemoteClaims fences the remote holders of the task's running intents
// out: their heartbeat fails within one interval and they stop. Used when a
// settle drain times out and when a task is deleted.
func (e *Engine) revokeRemoteClaims(t *Task) {
	if !e.distributed() || t == nil || t.Store == nil {
		return
	}
	n, err := e.m.pg.RevokeRemoteIntentClaims(t.ExpID)
	if err != nil {
		log.Printf("[engine] task %s revoke remote claims: %v", t.ID, err)
	} else if n > 0 {
		log.Printf("[engine] task %s revoked %d remote intent claim(s)", t.ID, n)
	}
}

// syncBudget raises the ledger to the spend every node recorded in llm_usage.
func (e *Engine) syncBudget(taskID string, l *agent.BudgetLedger) {
	if l == nil || !e.distributed() || e.m == nil || e.m.pg == nil {
		return
	}
	tokens, turns, err := e.m.pg.TaskLLMSpend(taskID)
	if err != nil {
		log.Printf("[budget] task %s sync spend: %v", taskID, err)
		return
	}
	l.SyncSpend(agent.Spend{Tokens: tokens, Turns: turns})
}

// budgetConfig is the run budget this process meters: the environment, or on
// a worker node the configuration the control node published.
func (e *Engine) budgetConfig() (task, intent agent.Budget) {
	if e.node.Role == RoleWorker && e.m != nil && e.m.pg != nil {
		if task, intent, ok := readPublishedBudgets(e.m.pg); ok {
			return task, intent
		}
	}
	return agent.BudgetsFromEnv()
}

// remoteWorkSignal delivers a work control request to the node holding the
// intent's claim. handled is false when the intent is not held by a remote
// node (the caller then reports "no running work" as before).
func (e *Engine) remoteWorkSignal(intentID int64, action, message string) (handled bool, err error) {
	if !e.distributed() || e.m == nil || e.m.pg == nil {
		return false, nil
	}
	taskID, owner, ok, err := e.m.pg.RunningIntentClaim(intentID)
	if err != nil || !ok || !remoteOwner(owner) {
		return false, err
	}
	if _, err := e.m.pg.SendWorkSignal(taskID, intentID, action, message); err != nil {
		return true, err
	}
	log.Printf("[engine] intent #%d: %s sent to %s", intentID, action, owner)
	return true, nil
}

// awaitRemoteSettle waits until a remotely held intent has left 'running'.
func (e *Engine) awaitRemoteSettle(ctx context.Context, intentID int64) error {
	deadline := time.Now().Add(workControlWaitTimeout)
	for {
		_, _, running, err := e.m.pg.RunningIntentClaim(intentID)
		if err == nil && !running {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("waiting for intent %d to settle on its worker node: %w", intentID, context.DeadlineExceeded)
		}
		if sleepCtx(ctx, 500*time.Millisecond) {
			return ctx.Err()
		}
	}
}

// remoteSettleLoop runs on the control node per task: it wakes the planner for
// intents a worker node settled (NotifyDone, as a local worker does) and
// republishes the worker nodes' activity on this node's live stream.
func (e *Engine) remoteSettleLoop(ctx context.Context, t *Task) {
	var since time.Time // zero: the first poll only sets the cursor
	lastAct, err := t.Store.LatestActivityID()
	if err != nil {
		log.Printf("[engine] task %s remote activity cursor: %v", t.ID, err)
	}
	tick := time.NewTicker(distributedPoll)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
		settled, next, err := t.Store.SettledIntentsSince(since)
		if err == nil {
			first := since.IsZero()
			since = next
			if first {
				settled = nil
			}
			for _, si := range settled {
				if remoteOwner(si.Owner) {
					t.NotifyDone(si.ID)
				}
			}
		}
		acts, cursor, err := t.Store.ActivityList(nil, lastAct, 500)
		if err != nil {
			continue
		}
		lastAct = cursor
		for _, a := range acts {
			if remoteOwner(a.Worker) && e.bc != nil {
				e.bc.Publish(t.ID, a)
				e.touch(t.ID)
			}
		}
	}
}

// liveTargetGate is a worker node's target-gate switch: read from the task row
// on every gated call, so a switch made on the control node applies to the
// next tool call here too. A read error reports the gate ON (fail closed).
func liveTargetGate(pg *db.DB, taskID int64) func() bool {
	return func() bool {
		c, err := pg.GetTaskControl(taskID)
		if err != nil || !c.Exists {
			return true
		}
		return c.TargetGate
	}
}
