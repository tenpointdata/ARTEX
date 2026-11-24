package server

import (
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/Autumn-27/artex/db"
)

func authorizationBody(notBefore, notAfter time.Time) string {
	return fmt.Sprintf(`{"reference":"SOW-L","scope":[{"kind":"domain","value":"example.com"}],"not_before":%q,"not_after":%q}`,
		notBefore.UTC().Format(time.RFC3339), notAfter.UTC().Format(time.RFC3339))
}

// Storing an artifact turns the loaded task's gate on, and the gate cannot be
// turned off while the task has one (db.ErrTargetGateRequired).
func TestAuthorizationForcesLoadedTaskGatePG(t *testing.T) {
	g := newGateServer(t)
	task := g.newTask(t, db.TaskCreateOptions{})
	if task.targetGate.Load() {
		t.Fatal("a new task starts gated")
	}
	now := time.Now()
	if code, out := g.do(t, "POST", "/api/tasks/"+task.ID+"/authorization", authorizationBody(now.Add(-time.Hour), now.Add(time.Hour))); code != 200 {
		t.Fatalf("store artifact: %d %v", code, out)
	}
	if !task.targetGate.Load() {
		t.Fatal("the loaded task's gate stayed off after an artifact was stored")
	}
	if code, out := g.do(t, "GET", "/api/tasks/"+task.ID, ""); code != 200 || out["target_gate"] != true {
		t.Fatalf("task view: %d target_gate=%v", code, out["target_gate"])
	}
	if code, out := g.do(t, "PATCH", "/api/tasks/"+task.ID, `{"target_gate":false}`); code != 409 {
		t.Fatalf("gate off with an artifact: %d %v, want 409", code, out)
	}
	if !task.targetGate.Load() {
		t.Fatal("the refused patch turned the in-memory gate off")
	}
}

// A task whose artifact ended is paused by the sweep, cannot be resumed until
// a new version is stored, and a revocation pauses it at once.
func TestEndedAuthorizationPausesTaskPG(t *testing.T) {
	g := newGateServer(t)
	task := g.newTask(t, db.TaskCreateOptions{})
	id, _ := strconv.ParseInt(task.ID, 10, 64)
	now := time.Now()
	paused := func() bool {
		t.Helper()
		row, err := g.m.pg.GetTask(id)
		if err != nil || row == nil {
			t.Fatalf("read task: %v", err)
		}
		if row.Paused != task.lifecycleSnapshot().Paused {
			t.Fatalf("stored paused=%v, in memory %v", row.Paused, task.lifecycleSnapshot().Paused)
		}
		return row.Paused
	}
	in, err := db.AuthorizationInput{NotBefore: now.Add(-2 * time.Hour), NotAfter: now.Add(-time.Minute)}.Normalize(nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := g.m.pg.AddAuthorization(id, in, false, "admin"); err != nil {
		t.Fatal(err)
	}
	if paused() {
		t.Fatal("paused before the sweep")
	}
	g.s.pauseEndedAuthorizations()
	if !paused() {
		t.Fatal("the sweep did not pause a task whose authorization expired")
	}
	g.s.pauseEndedAuthorizations() // a paused task is skipped, not paused twice
	if code, out := g.do(t, "POST", "/api/tasks/"+task.ID+"/control", `{"action":"resume"}`); code != 409 {
		t.Fatalf("resume with an expired artifact: %d %v, want 409", code, out)
	}
	if !paused() {
		t.Fatal("the refused resume unpaused the task")
	}
	// A new version in force lets it resume.
	if code, out := g.do(t, "POST", "/api/tasks/"+task.ID+"/authorization", authorizationBody(now.Add(-time.Hour), now.Add(time.Hour))); code != 200 {
		t.Fatalf("renew: %d %v", code, out)
	}
	if code, out := g.do(t, "POST", "/api/tasks/"+task.ID+"/control", `{"action":"resume"}`); code != 200 {
		t.Fatalf("resume after renewal: %d %v", code, out)
	}
	if paused() {
		t.Fatal("still paused after resume")
	}
	// Revocation pauses at once, without waiting for the sweep.
	if code, out := g.do(t, "POST", "/api/tasks/"+task.ID+"/authorization/revoke", `{"note":"stop"}`); code != 200 {
		t.Fatalf("revoke: %d %v", code, out)
	}
	if !paused() {
		t.Fatal("a revocation did not pause the task")
	}
}
