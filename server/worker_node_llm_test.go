package server

import (
	"context"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/Autumn-27/artex/db"
)

// An LLM profile change made through the control node's API reaches a worker
// node: its next intent resolves the new configuration, not its start-up one.
func TestWorkerNodeFollowsLLMProfileChange(t *testing.T) {
	skipWithoutDSN(t)
	control, err := NewManager(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = control.Close() })
	pg := control.pg
	clearProfiles := func() {
		existing, _ := pg.ListProfiles()
		for _, p := range existing {
			_, _ = pg.Exec(`UPDATE llm_profiles SET is_default=false WHERE id=$1`, p.ID)
			_ = pg.DeleteProfile(p.ID)
		}
	}
	clearProfiles()
	t.Cleanup(clearProfiles)
	idA, err := pg.SaveProfile(&db.LLMProfile{Name: "e7-a", Format: "anthropic", Model: "model-a", APIKey: "sk-a"})
	if err != nil {
		t.Fatal(err)
	}
	if err := pg.SetActiveProfile(idA); err != nil {
		t.Fatal(err)
	}

	td := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	cs := New(ctx, control, td, td, td)
	defer stopTestServer(t, cs, cancel)
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

	created, err := control.CreateTaskWithOptions("e7 llm follow", "a worker node follows LLM profile changes", db.TaskCreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = control.DeleteTask(created.ID, DeleteTaskOptions{}) })
	wm.LoadExisting()
	if _, ok := wm.Task(created.ID); !ok {
		t.Fatal("worker manager does not see the task")
	}
	model := func() string {
		t.Helper()
		cfg, ok := (&taskLLMRuntime{s: ws, taskID: created.ID, agentKey: "worker"}).activeCfg()
		if !ok {
			t.Fatal("the worker node resolves no LLM config for the task")
		}
		return cfg.Model
	}
	var f agentSettingsFollower
	ws.followAgentSettings(&f)
	if got := model(); got != "model-a" {
		t.Fatalf("worker node model at start = %q, want model-a", got)
	}

	// Another profile is saved and activated on the control node's API.
	rec := httptest.NewRecorder()
	cs.pgSaveProfile(rec, httptest.NewRequest("POST", "/api/llm/profiles", strings.NewReader(`{"name":"e7-b","format":"anthropic","model":"model-b","api_key":"sk-b"}`)))
	if rec.Code != 200 {
		t.Fatalf("save profile: %d %s", rec.Code, rec.Body.String())
	}
	var idB int64
	if err := pg.QueryRow(`SELECT id FROM llm_profiles WHERE name='e7-b'`).Scan(&idB); err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	cs.pgActivateProfile(rec, httptest.NewRequest("POST", "/api/llm/profiles/activate", strings.NewReader(`{"id":`+strconv.FormatInt(idB, 10)+`}`)))
	if rec.Code != 200 {
		t.Fatalf("activate profile: %d %s", rec.Code, rec.Body.String())
	}
	if got := model(); got != "model-a" {
		t.Fatalf("worker node changed models (%q) before following the revision", got)
	}
	ws.followAgentSettings(&f)
	if got := model(); got != "model-b" {
		t.Fatalf("worker node model after activation = %q, want model-b", got)
	}

	// The active profile is edited: same id, new model.
	rec = httptest.NewRecorder()
	cs.pgSaveProfile(rec, httptest.NewRequest("POST", "/api/llm/profiles", strings.NewReader(`{"id":`+strconv.FormatInt(idB, 10)+`,"name":"e7-b","format":"anthropic","model":"model-b2","api_key":"sk-b"}`)))
	if rec.Code != 200 {
		t.Fatalf("edit profile: %d %s", rec.Code, rec.Body.String())
	}
	ws.followAgentSettings(&f)
	if got := model(); got != "model-b2" {
		t.Fatalf("worker node model after the edit = %q, want model-b2", got)
	}

	// A worker node never bumps the revision itself (it would follow its own bump).
	before, _ := pg.SettingsRevision(db.LLMConfigRevisionKey)
	ws.invalidateProfileAgents()
	if after, _ := pg.SettingsRevision(db.LLMConfigRevisionKey); after != before {
		t.Fatalf("a worker node bumped the LLM config revision %d -> %d", before, after)
	}
}
