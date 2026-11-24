package server

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Autumn-27/norma/llm"
)

// terminalTestServer is a Server whose Manager holds one in-memory task, enough
// for the status checks (no PostgreSQL needed: the gate refuses before any
// provider resolution).
func terminalTestServer(status string) (*Server, *Task) {
	task := &Task{ID: "7", Status: status}
	return &Server{m: &Manager{tasks: map[string]*Task{task.ID: task}}}, task
}

// c-7361b4: no goals call may be made for a task once it is terminal.
func TestGoalsRuntimeRefusesTerminalTask(t *testing.T) {
	for _, status := range []string{"done", "failed", "timeout"} {
		s, task := terminalTestServer(status)
		r := &taskLLMRuntime{s: s, taskID: task.ID, agentKey: "goals"}
		if _, _, _, err := r.Complete(context.Background(), llm.CompletionRequest{}); !errors.Is(err, errTaskTerminal) {
			t.Errorf("%s: Complete err = %v, want errTaskTerminal", status, err)
		}
		var streamErr error
		events := 0
		for _, err := range r.Stream(context.Background(), llm.CompletionRequest{}) {
			events++
			streamErr = err
		}
		if events != 1 || !errors.Is(streamErr, errTaskTerminal) {
			t.Errorf("%s: Stream events=%d err=%v, want one errTaskTerminal", status, events, streamErr)
		}
	}
}

func TestTerminalGateScope(t *testing.T) {
	s, task := terminalTestServer("running")
	if err := (&taskLLMRuntime{s: s, taskID: task.ID, agentKey: "goals"}).terminalRefusal(); err != nil {
		t.Fatalf("running task refused: %v", err)
	}
	s, task = terminalTestServer("done")
	// The settle sequence's final planner round and human steering run on
	// terminal tasks by design; only the decomposer is gated.
	for _, key := range []string{"planner", "worker", "mainagent"} {
		if err := (&taskLLMRuntime{s: s, taskID: task.ID, agentKey: key}).terminalRefusal(); err != nil {
			t.Errorf("%s refused on a terminal task: %v", key, err)
		}
	}
}

func TestCancelOnTerminal(t *testing.T) {
	old := terminalWatchInterval
	terminalWatchInterval = 5 * time.Millisecond
	defer func() { terminalWatchInterval = old }()

	s, task := terminalTestServer("running")
	ctx, stop := s.cancelOnTerminal(context.Background(), task.ID)
	defer stop()
	select {
	case <-ctx.Done():
		t.Fatal("cancelled while the task is running")
	case <-time.After(30 * time.Millisecond):
	}
	task.lifecycleMu.Lock()
	task.Status = "done"
	task.lifecycleMu.Unlock()
	select {
	case <-ctx.Done():
		if !errors.Is(context.Cause(ctx), errTaskTerminal) {
			t.Fatalf("cause = %v, want errTaskTerminal", context.Cause(ctx))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("ctx not cancelled after the task turned terminal")
	}
}
