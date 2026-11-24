package server

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"time"

	"github.com/Autumn-27/norma/llm"
)

// errTaskTerminal is the cause a terminal-gated runtime refuses a call with, and
// the cancel cause of a decomposition aborted because its task ended.
var errTaskTerminal = errors.New("task is in a terminal status")

// terminalGatedAgents are the roles whose LLM calls have no purpose once their
// task is terminal. The goals decomposer only seeds goal nodes for a run that is
// starting; a done/failed/timeout task gains nothing from it (c-7361b4: a looping
// decomposer kept calling a single-slot local model 30+ minutes after its task
// finished, starving every other task). Planner/worker/mainagent are NOT gated:
// the settle sequence's final planner round and human steering run on terminal
// tasks by design, and revival goes through admission which flips the status first.
var terminalGatedAgents = map[string]bool{"goals": true}

// terminalRefusal reports why this runtime must not call the model now, or nil.
func (r *taskLLMRuntime) terminalRefusal() error {
	if !terminalGatedAgents[r.agentKey] || r.s == nil || r.s.m == nil {
		return nil
	}
	if status := r.s.m.TaskStatus(r.taskID); isTerminalStatus(status) {
		return fmt.Errorf("%s call refused for task %s (status %s): %w", r.agentKey, r.taskID, status, errTaskTerminal)
	}
	return nil
}

func refusedStream(err error) iter.Seq2[llm.StreamEvent, error] {
	return func(yield func(llm.StreamEvent, error) bool) { yield(llm.StreamEvent{}, err) }
}

// terminalWatchInterval is how often cancelOnTerminal samples the task status.
var terminalWatchInterval = time.Second

// cancelOnTerminal derives a ctx that is cancelled (cause errTaskTerminal) as soon
// as the task reaches a terminal status, so an in-flight call does not run to its
// output cap after the task ended. The returned stop releases the watcher.
func (s *Server) cancelOnTerminal(parent context.Context, taskID string) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancelCause(parent)
	go func() {
		tick := time.NewTicker(terminalWatchInterval)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				if isTerminalStatus(s.m.TaskStatus(taskID)) {
					cancel(errTaskTerminal)
					return
				}
			}
		}
	}()
	return ctx, func() { cancel(context.Canceled) }
}
