package server

import (
	"strconv"
	"testing"

	"github.com/Autumn-27/artex/db"
)

// reloadTask opens a second Manager on the same database, as a restarted
// process would, and returns its freshly loaded handle for id.
func reloadTask(t *testing.T, id string) *Task {
	t.Helper()
	m, err := NewManager(t.TempDir(), "")
	if err != nil {
		db.SkipPGUnavailable(t, err)
	}
	t.Cleanup(func() { _ = m.Close() })
	m.LoadExisting()
	task, ok := m.Task(id)
	if !ok {
		t.Fatalf("task %s not reloaded", id)
	}
	return task
}

func TestRestoreReDecomposesTaskInterruptedDuringRoundZero(t *testing.T) {
	m, err := NewManager(t.TempDir(), "")
	if err != nil {
		db.SkipPGUnavailable(t, err)
	}
	defer m.Close()
	task, err := m.CreateTask("restart during round 0", "find the admin panel", nil, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = m.DeleteTask(task.ID, DeleteTaskOptions{}) }()
	// startTaskEngine wrote its round-0 marker, then the process died before any goal.
	if _, err := task.Store.AppendActivity(db.Activity{Worker: "planner", Kind: "round", Summary: goalDecompositionRoundSummary}); err != nil {
		t.Fatal(err)
	}

	reloaded := reloadTask(t, task.ID)
	if mode, ok := restoreRunMode(reloaded); !ok || mode != "bootstrap" {
		t.Fatalf("interrupted decomposition restored as mode=%q ok=%v, want bootstrap", mode, ok)
	}
	s := newAdmissionTestServer(m, nil)
	if got := s.resumeAdmissionMode(reloaded); got != "bootstrap" {
		t.Fatalf("resumeAdmissionMode=%q, want bootstrap", got)
	}

	// A paused task keeps idling; resuming it re-derives bootstrap.
	reloaded.updateLifecycle(func(st *taskLifecycleState) { st.Paused = true })
	if mode, ok := restoreRunMode(reloaded); !ok || mode != "resume" {
		t.Fatalf("paused task restored as mode=%q ok=%v, want resume", mode, ok)
	}
	reloaded.updateLifecycle(func(st *taskLifecycleState) { st.Paused = false })

	// Queued tasks wait for FIFO admission, which keeps their own mode.
	reloaded.updateLifecycle(func(st *taskLifecycleState) { st.Queued = true })
	if _, ok := restoreRunMode(reloaded); ok {
		t.Fatal("queued task must not be started by restore")
	}
}

func TestRestoreKeepsGoalNodesAndDeliberatelyGoallessTasks(t *testing.T) {
	m, err := NewManager(t.TempDir(), "")
	if err != nil {
		db.SkipPGUnavailable(t, err)
	}
	defer m.Close()
	s := newAdmissionTestServer(m, nil)

	withGoal, err := m.CreateTask("has goals", "enumerate subdomains", nil, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = m.DeleteTask(withGoal.ID, DeleteTaskOptions{}) }()
	if _, err := withGoal.Store.AddNode(db.KindGoal, map[string]any{"text": "enumerate subdomains"}, 0, "met", "system", nil); err != nil {
		t.Fatal(err)
	}

	// Every goal deleted by the human after planning ran: goalless by design.
	goalless, err := m.CreateTask("goals deleted", "probe login", nil, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = m.DeleteTask(goalless.ID, DeleteTaskOptions{}) }()
	for _, summary := range []string{goalDecompositionRoundSummary, "Planning round 1"} {
		if _, err := goalless.Store.AppendActivity(db.Activity{Worker: "planner", Kind: "round", Summary: summary}); err != nil {
			t.Fatal(err)
		}
	}
	gid, err := goalless.Store.AddNode(db.KindGoal, map[string]any{"text": "probe login"}, 0, "open", "system", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := goalless.Store.DeleteGoal(gid); err != nil {
		t.Fatal(err)
	}

	for _, id := range []string{withGoal.ID, goalless.ID} {
		reloaded := reloadTask(t, id)
		if mode, ok := restoreRunMode(reloaded); !ok || mode != "resume" {
			t.Fatalf("task %s restored as mode=%q ok=%v, want resume", id, mode, ok)
		}
		if got := s.resumeAdmissionMode(reloaded); got != "resume" {
			t.Fatalf("task %s resumeAdmissionMode=%q, want resume", id, got)
		}
	}

	// Terminal tasks are never restarted.
	n, _ := strconv.ParseInt(goalless.ID, 10, 64)
	if err := m.pg.SetStatus(n, "done"); err != nil {
		t.Fatal(err)
	}
	if _, ok := restoreRunMode(reloadTask(t, goalless.ID)); ok {
		t.Fatal("terminal task must not be restored")
	}
}
