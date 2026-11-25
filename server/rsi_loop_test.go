package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"github.com/Autumn-27/artex/agent"
	"github.com/Autumn-27/artex/db"
	"github.com/Autumn-27/artex/rsi"
)

// Every built-in body must itself be inside the search space, or no edit
// derived from it could ever be proposed.
func TestBuiltinPromptBodiesAreProposable(t *testing.T) {
	seeds := agent.BuiltinPromptSeeds()
	for _, role := range rsi.PromptRoles {
		body, ok := seeds[role]
		if !ok {
			t.Fatalf("no built-in seed for %s", role)
		}
		if err := rsi.ValidateEdit(rsi.Edit{Surface: rsi.SurfacePromptBody, Target: role, Body: body}, rsiForbiddenMarkers); err != nil {
			t.Fatalf("%s built-in body refused: %v", role, err)
		}
	}
	// The markers are real: the rendered lesson block carries one.
	block, _ := db.RenderLessons([]db.Lesson{{ID: 1, Kind: "positive", Cue: "c", Action: "a"}}, 1800)
	if err := rsi.ValidateEdit(rsi.Edit{Surface: rsi.SurfacePromptBody, Target: "worker", Body: "x\n" + block}, rsiForbiddenMarkers); err == nil {
		t.Fatal("a body carrying the lesson block must be refused")
	}
}

func TestRSILessonsModes(t *testing.T) {
	for raw, want := range map[string]string{"adopted": "adopted", "BENCH": "bench"} {
		t.Setenv("ARTEX_RSI_LESSONS", raw)
		if got := rsiLessonsMode(); got != want {
			t.Fatalf("%q -> %q", raw, got)
		}
	}
}

type rsiHarness struct {
	t     *testing.T
	s     *Server
	token string
}

func (h rsiHarness) call(method, path string, body any) (int, map[string]any) {
	h.t.Helper()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Authorization", "Bearer "+h.token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.s.Handler().ServeHTTP(rec, req)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

const rsiTestSeed = 7

var rsiTestTargets = []string{"A", "B", "C"}

// rsiArm builds an eval_suite arm file of registered run suiteRun whose runs
// pass the GT entries listed per target, on the given task ids (one per target).
func rsiArm(role, suiteRun string, sha map[string]string, runs map[string][]string, tasks []int64) map[string]any {
	stamp := map[string]any{"role": role, "arm": role, "split": "held-out", "suite": "heldout", "suite_version": "v1",
		"suite_run": suiteRun, "seed": rsiTestSeed, "replicates": 1,
		"proposed_from": []string{}, "prompt_sha": sha, "lesson_ids": []int64{}}
	for _, k := range rsi.InvariantStamps {
		if _, ok := stamp[k]; !ok {
			stamp[k] = "same"
		}
	}
	records := []map[string]any{}
	for i, target := range rsiTestTargets {
		verdicts := []map[string]any{}
		for _, gt := range []string{"GT-1", "GT-2"} {
			ok := false
			for _, p := range runs[target] {
				ok = ok || p == gt
			}
			verdicts = append(verdicts, map[string]any{"gt_id": gt, "detected": ok,
				"gates": map[string]bool{"component": ok, "class": ok, "keywords": ok, "proof": ok}})
		}
		records = append(records, map[string]any{"target": target, "replicate": 0, "run_id": rsi.RunID(suiteRun, 0, role, target),
			"payload": map[string]any{"task_status": "done", "verdicts": verdicts, "task": map[string]any{"id": strconv.FormatInt(tasks[i], 10)}}})
	}
	return map[string]any{"stamp": stamp, "records": records}
}

// rsiRunArm does what the harness and the server do for one arm: make the
// planner run body, create one task per target named after its run id, build
// its ledger row through the real path (recordTaskArm), and finish the task.
func rsiRunArm(t *testing.T, s *Server, plannerID int64, body, role, suiteRun string) []int64 {
	t.Helper()
	pg := s.m.PG()
	if body != "" {
		if cur, _ := pg.CurrentPrompt(plannerID); cur != body {
			if _, err := pg.SavePrompt(plannerID, body, "bench arm "+role, "test"); err != nil {
				t.Fatal(err)
			}
		}
	}
	var ids []int64
	for _, target := range rsiTestTargets {
		task, err := pg.CreateTaskWithOptions("rsi", "goal", db.TaskCreateOptions{Name: "bench-x-" + rsi.RunID(suiteRun, 0, role, target)})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = pg.DeleteTask(task.ID) })
		s.recordTaskArm(&Task{ID: strconv.FormatInt(task.ID, 10)}, rsiLessonsMode(), nil, "")
		if _, err := pg.Exec(`UPDATE tasks SET status='done' WHERE id=$1`, task.ID); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, task.ID)
	}
	return ids
}

func rsiRegister(suiteRun, split string) map[string]any {
	return map[string]any{"suite_run": suiteRun, "suite": "heldout", "suite_version": "v1", "split": split,
		"targets": rsiTestTargets, "seed": rsiTestSeed, "replicates": 1}
}

func TestRSIGateEndToEndPG(t *testing.T) {
	m, err := NewManager(t.TempDir(), "")
	if err != nil {
		db.SkipPGUnavailable(t, err)
	}
	defer m.Close()
	for _, q := range []string{`DELETE FROM rsi_suite_runs`, `DELETE FROM rsi_proposal_evidence`, `ALTER TABLE rsi_adoption_log DISABLE TRIGGER USER`,
		`DELETE FROM rsi_adoption_log`, `ALTER TABLE rsi_adoption_log ENABLE TRIGGER USER`, `DELETE FROM rsi_proposals`} {
		if _, err := m.pg.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	s := New(ctx, m, t.TempDir(), t.TempDir(), t.TempDir())
	defer stopTestServer(t, s, cancel)
	token, err := signJWT(s.jwtKey)
	if err != nil {
		t.Fatal(err)
	}
	h := rsiHarness{t: t, s: s, token: token}
	hash, _ := bcrypt.GenerateFromPassword([]byte("owner-pass"), bcrypt.MinCost)
	oldHash, hadHash, _ := m.pg.GetSetting(s.passwordHashKey())
	if err := m.pg.SetSetting(s.passwordHashKey(), string(hash)); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if hadHash {
			_ = m.pg.SetSetting(s.passwordHashKey(), oldHash)
		}
	}()
	planner, _ := m.pg.GetAgentByKey("planner")
	liveBody, _ := m.pg.CurrentPrompt(planner.ID)
	if liveBody == "" {
		liveBody = agent.BuiltinPromptSeeds()["planner"]
	}
	defer func() { _, _ = m.pg.SavePrompt(planner.ID, liveBody, "test restore", "test") }()

	body := liveBody + "\nPrefer breadth before depth."
	proposal := map[string]any{"round": 0, "surface": "prompt_body", "target": "planner", "body": body,
		"proposed_from": []string{"DBEN-dev-1"}, "rationale": "breadth raises detected on held-out"}

	// Flag off by default: no proposals.
	t.Setenv(rsiLoopFlag, "")
	if code, _ := h.call("POST", "/api/rsi/proposals", proposal); code != 409 {
		t.Fatalf("flag off: propose status %d", code)
	}
	t.Setenv(rsiLoopFlag, "on")
	// Immutable surfaces are refused by the API too.
	if code, _ := h.call("POST", "/api/rsi/proposals", map[string]any{"round": 0, "surface": "tool_allowlist", "body": "x", "rationale": "r"}); code != 400 {
		t.Fatalf("immutable surface: status %d", code)
	}
	assembled := map[string]any{"round": 0, "surface": "prompt_body", "target": "planner",
		"body": liveBody + "\nARTEX_{{/* */}}DATA", "rationale": "r"}
	if code, out := h.call("POST", "/api/rsi/proposals", assembled); code != 400 {
		t.Fatalf("template-assembled marker: %d %v", code, out)
	}
	code, out := h.call("POST", "/api/rsi/proposals", proposal)
	if code != 201 {
		t.Fatalf("propose: %d %v", code, out)
	}
	pid := int64(out["proposal"].(map[string]any)["id"].(float64))
	base := fmt.Sprintf("/api/rsi/proposals/%d", pid)

	run := func(n int) string { return fmt.Sprintf("suite-%d-%d", pid, n) }
	// Evidence needs a registered run; registration needs held-out and one open at a time.
	if code, _ := h.call("POST", base+"/suite-runs", rsiRegister(run(1), "dev")); code != 400 {
		t.Fatalf("dev split registration: %d", code)
	}
	if code, out := h.call("POST", base+"/suite-runs", rsiRegister(run(1), "held-out")); code != 201 {
		t.Fatalf("register: %d %v", code, out)
	}
	if code, _ := h.call("POST", base+"/suite-runs", rsiRegister(run(9), "held-out")); code != 409 {
		t.Fatalf("second open registration: %d", code)
	}
	g0, w0 := rsiLiveSHA(m.pg, "goals"), rsiLiveSHA(m.pg, "worker")
	sha := map[string]string{"goals": g0, "planner": rsi.SHA16(liveBody), "worker": w0}
	candSHA := map[string]string{"goals": g0, "planner": rsi.SHA16(body), "worker": w0}
	upload := func(suiteRun string, baseTasks, candTasks []int64) (int, map[string]any) {
		return h.call("POST", base+"/evidence", map[string]any{
			"baseline":  rsiArm("baseline", suiteRun, sha, map[string][]string{"B": {"GT-1"}}, baseTasks),
			"candidate": rsiArm("candidate", suiteRun, candSHA, map[string][]string{"A": {"GT-1"}, "B": {"GT-1", "GT-2"}}, candTasks)})
	}
	// An unregistered run is refused before anything is stored.
	if code, _ := upload("never-registered", []int64{1, 2, 3}, []int64{4, 5, 6}); code != 409 {
		t.Fatalf("unregistered run: %d", code)
	}
	bt := rsiRunArm(t, s, planner.ID, liveBody, "baseline", run(1))
	ct := rsiRunArm(t, s, planner.ID, body, "candidate", run(1))
	rsiRunArm(t, s, planner.ID, liveBody, "restore", run(0)) // the harness restores the starting body
	code, out = upload(run(1), bt, ct)
	ev, _ := out["evidence"].(map[string]any)
	if code != 200 || ev["passed"] != true {
		t.Fatalf("evidence: %d %v", code, out)
	}
	// The run is consumed: the same files cannot be uploaded again.
	if code, _ := upload(run(1), bt, ct); code != 409 {
		t.Fatalf("replayed run: %d", code)
	}

	// The forgery the arm ledger stops: candidate files claiming the candidate
	// body on tasks that ran the baseline body.
	if code, out := h.call("POST", base+"/suite-runs", rsiRegister(run(2), "held-out")); code != 201 {
		t.Fatalf("register 2: %d %v", code, out)
	}
	bt = rsiRunArm(t, s, planner.ID, liveBody, "baseline", run(2))
	ct = rsiRunArm(t, s, planner.ID, liveBody, "candidate", run(2))
	code, out = upload(run(2), bt, ct)
	ev, _ = out["evidence"].(map[string]any)
	if code != 200 || ev["passed"] != false || !strings.Contains(fmt.Sprint(ev["verdict"]), "ran planner body") {
		t.Fatalf("forged arm: %d %v", code, out)
	}

	if code, out := h.call("POST", base+"/suite-runs", rsiRegister(run(3), "held-out")); code != 201 {
		t.Fatalf("register 3: %d %v", code, out)
	}
	bt = rsiRunArm(t, s, planner.ID, liveBody, "baseline", run(3))
	ct = rsiRunArm(t, s, planner.ID, body, "candidate", run(3))
	rsiRunArm(t, s, planner.ID, liveBody, "restore", run(0))
	code, out = upload(run(3), bt, ct)
	if ev, _ = out["evidence"].(map[string]any); code != 200 || ev["passed"] != true {
		t.Fatalf("fresh evidence: %d %v", code, out)
	}
	evID := ev["id"]
	// A fourth run is over the per-proposal cap.
	if code, _ := h.call("POST", base+"/suite-runs", rsiRegister(run(4), "held-out")); code != 409 {
		t.Fatalf("suite run cap: %d", code)
	}
	if code, out := h.call("GET", base+"/suite-runs", nil); code != 200 || len(out["suite_runs"].([]any)) != 3 {
		t.Fatalf("list suite runs: %d %v", code, out)
	}

	// Adoption needs the owner password; the loop's bearer token is not enough.
	for _, pw := range []string{"", "wrong"} {
		if code, _ := h.call("POST", base+"/adopt", map[string]any{"evidence_id": evID, "password": pw}); code != 403 {
			t.Fatalf("adopt with password %q: %d", pw, code)
		}
	}
	t.Setenv(rsiLoopFlag, "off")
	if code, _ := h.call("POST", base+"/adopt", map[string]any{"evidence_id": evID, "password": "owner-pass"}); code != 409 {
		t.Fatalf("adopt with flag off: %d", code)
	}
	t.Setenv(rsiLoopFlag, "on")
	// Stale evidence: the live prompt is no longer the baseline the suite ran.
	if _, err := m.pg.SavePrompt(planner.ID, liveBody+"\nowner edit", "test", "ui"); err != nil {
		t.Fatal(err)
	}
	if code, _ := h.call("POST", base+"/adopt", map[string]any{"evidence_id": evID, "password": "owner-pass"}); code != 409 {
		t.Fatalf("adopt over a changed live prompt: %d", code)
	}
	if _, err := m.pg.SavePrompt(planner.ID, liveBody, "test", "ui"); err != nil {
		t.Fatal(err)
	}
	if code, out = h.call("POST", base+"/adopt", map[string]any{"evidence_id": evID, "password": "owner-pass"}); code != 200 {
		t.Fatalf("adopt: %d %v", code, out)
	}
	if cur, _ := m.pg.CurrentPrompt(planner.ID); cur != body {
		t.Fatal("adopted body is not active")
	}
	// Rollback stays open with the flag off.
	t.Setenv(rsiLoopFlag, "off")
	if code, out = h.call("POST", base+"/rollback", map[string]any{"reason": "test"}); code != 200 {
		t.Fatalf("rollback: %d %v", code, out)
	}
	if cur, _ := m.pg.CurrentPrompt(planner.ID); cur != liveBody && !(cur == "" && liveBody == agent.BuiltinPromptSeeds()["planner"]) {
		t.Fatal("rollback did not restore the previous prompt")
	}
	code, out = h.call("GET", base, nil)
	if code != 200 || len(out["log"].([]any)) != 9 {
		t.Fatalf("log: %d %v", code, out["log"])
	}
}
