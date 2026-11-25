package server

import (
	"context"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/Autumn-27/artex/db"
)

// The LLM recording switch is held in memory, so a worker node must follow it
// through the agent settings revision: otherwise turning recording on at the
// control node leaves the worker's LLM calls unrecorded (c-6bcdd3).
func TestWorkerNodeFollowsLLMRecordSwitch(t *testing.T) {
	skipWithoutDSN(t)
	control, err := NewManager(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = control.Close() })
	if err := control.SetLLMRecordEnabled(false); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = control.pg.SetBool(settingLLMRecord, false) })
	wm, err := NewManager(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = wm.Close() })
	ws := &Server{m: wm, node: NodeConfig{Role: RoleWorker, NodeID: "node-r"}}
	var f agentSettingsFollower
	ws.followAgentSettings(&f)

	cs := &Server{m: control, node: NodeConfig{Role: RoleControl, NodeID: "ctl"}}
	rec := httptest.NewRecorder()
	cs.putSettings(rec, httptest.NewRequest("PUT", "/api/settings", strings.NewReader(`{"llm_record":true}`)))
	if rec.Code != 200 {
		t.Fatalf("putSettings: %d %s", rec.Code, rec.Body.String())
	}
	if wm.LLMRecordEnabled() {
		t.Fatal("worker node changed the switch before following the revision")
	}
	ws.followAgentSettings(&f)
	if !wm.LLMRecordEnabled() {
		t.Fatal("worker node kept LLM recording off after the control node turned it on")
	}

	// A worker node never bumps the revision itself.
	before, _ := control.pg.SettingsRevision(db.AgentSettingsRevisionKey)
	ws.bumpAgentSettingsRevision()
	if after, _ := control.pg.SettingsRevision(db.AgentSettingsRevisionKey); after != before {
		t.Fatalf("a worker node bumped the agent settings revision %d -> %d", before, after)
	}
}

// An agent's saved settings (max turns, run seconds, web search) are read when
// a task bundle is built. A save on the control node's API must make a worker
// node drop its bundles, so its next claim builds them from the saved row.
func TestWorkerNodeFollowsAgentConfigSave(t *testing.T) {
	skipWithoutDSN(t)
	control, err := NewManager(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = control.Close() })
	pg := control.pg
	td := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	cs := New(ctx, control, td, td, td)
	defer stopTestServer(t, cs, cancel)
	orig, err := pg.GetAgentByKey("worker")
	if err != nil || orig == nil {
		t.Fatalf("worker agent row: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pg.Exec(`UPDATE agents SET max_turns=$1 WHERE key='worker'`, orig.MaxTurns)
	})

	wm, err := NewManager(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = wm.Close() })
	wtd := t.TempDir()
	wctx, wcancel := context.WithCancel(context.Background())
	ws := New(wctx, wm, wtd, wtd, wtd)
	defer stopTestServer(t, ws, wcancel)
	ws.node.Role = RoleWorker

	created, err := control.CreateTaskWithOptions("e7 agent follow", "a worker node follows agent config saves", db.TaskCreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = control.DeleteTask(created.ID, DeleteTaskOptions{}) })
	wm.LoadExisting()
	task, ok := wm.Task(created.ID)
	if !ok {
		t.Fatal("worker manager does not see the task")
	}
	var f agentSettingsFollower
	ws.followAgentSettings(&f)
	first := ws.agentsForTask(task)
	ws.followAgentSettings(&f)
	if ws.agentsForTask(task) != first {
		t.Fatal("worker node dropped its task bundle without a revision bump")
	}

	want := orig.MaxTurns + 7
	req := httptest.NewRequest("PUT", "/api/agents/worker/config", strings.NewReader(`{"max_turns":`+strconv.Itoa(want)+`}`))
	req.SetPathValue("key", "worker")
	rec := httptest.NewRecorder()
	cs.pgSaveAgentConfig(rec, req)
	if rec.Code != 200 {
		t.Fatalf("save agent config: %d %s", rec.Code, rec.Body.String())
	}
	if ws.agentsForTask(task) != first {
		t.Fatal("worker node dropped its task bundle before following the revision")
	}
	ws.followAgentSettings(&f)
	if ws.agentsForTask(task) == first {
		t.Fatal("worker node kept the task bundle built before the agent config save")
	}
	if got := ws.agentMaxTurns("worker"); got != want {
		t.Fatalf("worker node reads max_turns %d, want %d", got, want)
	}
}
