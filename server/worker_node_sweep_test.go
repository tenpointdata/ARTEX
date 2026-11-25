package server

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/Autumn-27/artex/agent"
	"github.com/Autumn-27/artex/db"
)

// E7 card c-1bbeb6: a worker node removes its local files of a task deleted
// or archived on the control node, and keeps those of a live task and of a
// retired task whose loops still run on the node.
func TestWorkerNodeSweepsRetiredTaskFiles(t *testing.T) {
	skipWithoutDSN(t)
	t.Setenv("ARTEX_ARCHIVE_STORAGE", "local")
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
	// The control server only runs the archive job here: its context is
	// cancelled before New, so it registers no node and starts no loop.
	cctx, ccancel := context.WithCancel(context.Background())
	ccancel()
	cs := New(cctx, control, t.TempDir(), t.TempDir(), t.TempDir())
	cs.archiveWG.Wait()
	cs.ctx = context.Background()

	tasks := map[string]*Task{}
	for _, name := range []string{"deleted", "archived", "live"} {
		created, err := control.CreateTaskWithOptions("e7 sweep "+name, "worker node sweeps "+name, db.TaskCreateOptions{})
		if err != nil {
			t.Fatal(err)
		}
		task, _ := control.Task(created.ID)
		tasks[name] = task
		t.Cleanup(func() {
			n, _ := taskNumericID(task.ID)
			_, _ = pg.Exec(`DELETE FROM task_archives WHERE task_id=$1`, n)
			_, _ = control.DeleteTask(task.ID, DeleteTaskOptions{})
			_ = pg.DeleteTask(n)
		})
	}

	node := startTestWorkerNode(t, "node-sweep", &holdProvider{taskID: "none", started: make(chan struct{})})
	// Each task's intent runs on the worker node, which serves all three.
	paths := map[string][]string{}
	for name, task := range tasks {
		intent, err := task.Store.AddIntent(map[string]any{"summary": "sweep " + name}, 1, nil, "planner")
		if err != nil {
			t.Fatal(err)
		}
		waitUntilFor(t, 30*time.Second, name+" intent to run on the worker node", func() bool {
			st, _ := task.Store.IntentState(intent)
			return st == "done"
		})
		var owner string
		if err := pg.QueryRow(`SELECT owner FROM exploration_nodes WHERE id=$1`, intent).Scan(&owner); err != nil || owner != "node-sweep/work#1" {
			t.Fatalf("%s intent owner %q (%v), want node-sweep/work#1", name, owner, err)
		}
		paths[name] = plantTaskFiles(t, node.m.dir, task, intent)
	}
	// A transcript of an exploration no task has named and a file of no
	// exploration are never touched.
	var standalone int64
	if err := pg.QueryRow(`INSERT INTO explorations(description, goal) VALUES ('', 'no task') RETURNING id`).Scan(&standalone); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = pg.Exec(`DELETE FROM explorations WHERE id=$1`, standalone) })
	other := filepath.Join(node.m.dir, "transcripts", fmt.Sprintf("exp%d-goals.jsonl", standalone))
	stray := filepath.Join(node.m.dir, "transcripts", "session-abc.jsonl")
	for _, p := range []string{other, stray} {
		writeFixture(t, p)
	}

	if _, err := control.DeleteTask(tasks["deleted"].ID, DeleteTaskOptions{}); err != nil {
		t.Fatal(err)
	}
	archivedID, _ := taskNumericID(tasks["archived"].ID)
	if err := pg.SetPaused(archivedID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := pg.QueueTaskArchive(archivedID); err != nil {
		t.Fatal(err)
	}
	if err := cs.runOneTaskArchiveJob(); err != nil {
		t.Fatalf("archive: %v", err)
	}
	waitUntilFor(t, 30*time.Second, "the worker node to stop serving the retired tasks", func() bool {
		return !node.e.Started(tasks["deleted"].ID) && !node.e.Started(tasks["archived"].ID)
	})

	// A retired task whose loops still run here is left alone. The node has
	// already dropped its handle, so its exploration is unknown: no transcript
	// is removed in this pass, only the deleted task's run and spill
	// directories.
	node.e.started.Store(tasks["archived"].ID, true)
	if _, err := node.s.sweepRetiredTaskFiles(); err != nil {
		t.Fatal(err)
	}
	assertPaths(t, "archived task still running here", paths["archived"], true)
	assertPaths(t, "deleted task directories", paths["deleted"][:2], false)
	assertPaths(t, "transcripts while a running task's exploration is unknown", paths["deleted"][2:], true)
	node.e.started.Delete(tasks["archived"].ID)

	n, err := node.s.sweepRetiredTaskFiles()
	if err != nil {
		t.Fatal(err)
	}
	if want := len(paths["archived"]) + 2; n < want {
		t.Fatalf("second sweep removed %d paths, want at least %d", n, want)
	}
	assertPaths(t, "archived task", paths["archived"], false)
	assertPaths(t, "deleted task", paths["deleted"], false)
	assertPaths(t, "live task", paths["live"], true)
	assertPaths(t, "unrelated files", []string{other, stray}, true)
	if names, _ := dirNames(filepath.Join(node.m.dir, ".delete-staging")); len(names) != 0 {
		t.Fatalf("staging left behind: %v", names)
	}
}

// plantTaskFiles writes, on a node's data directory, the local files a run
// leaves: the run directory, the spill directory, a transcript and a
// sidechain directory. It returns the paths the sweep owns.
func plantTaskFiles(t *testing.T, dataDir string, task *Task, intent int64) []string {
	t.Helper()
	runDir := filepath.Join(dataDir, "tasks", task.ID)
	spill := agent.TaskSpillDir(dataDir, task.ID)
	session := fmt.Sprintf("exp%d-worker-i%d", task.ExpID, intent)
	transcript := filepath.Join(dataDir, "transcripts", session+".jsonl")
	sidechain := filepath.Join(dataDir, "transcripts", session)
	writeFixture(t, filepath.Join(runDir, "run.log"))
	writeFixture(t, filepath.Join(spill, "out-"+strconv.FormatInt(intent, 10)+".txt"))
	writeFixture(t, transcript)
	writeFixture(t, filepath.Join(sidechain, "sidechain.jsonl"))
	return []string{runDir, spill, transcript, sidechain}
}

func writeFixture(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func assertPaths(t *testing.T, what string, paths []string, exist bool) {
	t.Helper()
	for _, p := range paths {
		_, err := os.Lstat(p)
		if exist && err != nil {
			t.Fatalf("%s: %s was removed (%v)", what, p, err)
		}
		if !exist && !os.IsNotExist(err) {
			t.Fatalf("%s: %s still exists (%v)", what, p, err)
		}
	}
}
