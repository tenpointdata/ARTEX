package db

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/Autumn-27/artex/rsi"
)

func rsiLedgerTask(t *testing.T, d *DB, name, status string) int64 {
	t.Helper()
	task, err := d.CreateTaskWithOptions("rsi ledger", "goal", TaskCreateOptions{Name: name})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Exec(`UPDATE tasks SET status=$2 WHERE id=$1`, task.ID, status); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.DeleteTask(task.ID) })
	return task.ID
}

func rsiArmRow(t *testing.T, d *DB, id int64) rsi.TaskArm {
	t.Helper()
	got, err := d.RSITaskArms(context.Background(), []int64{id}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	a, ok := got[id]
	if !ok {
		t.Fatalf("task %d has no ledger row", id)
	}
	return a
}

// The ledger records the first build; a different rebuild while the task is
// live, or a prompt pointer move on an RSI role, marks it drifted; a finished
// task's row never changes.
func TestRSITaskArmLedgerPG(t *testing.T) {
	d := rsiTestDB(t)
	ctx := context.Background()
	shaA := map[string]string{"goals": "g", "planner": "pA", "worker": "w"}
	shaB := map[string]string{"goals": "g", "planner": "pB", "worker": "w"}

	live := rsiLedgerTask(t, d, "bench-x-run-live", "running")
	if err := d.RecordTaskRSIArm(ctx, rsi.TaskArm{TaskID: live, PromptSHA: shaA, LessonsMode: "bench", LessonIDs: []int64{5, 3, 5}, LessonsBlockSHA: "blk"}); err != nil {
		t.Fatal(err)
	}
	a := rsiArmRow(t, d, live)
	if a.TaskName != "bench-x-run-live" || a.PromptSHA["planner"] != "pA" || fmt.Sprint(a.LessonIDs) != "[3 5]" || a.LessonsBlockSHA != "blk" || a.Drifted {
		t.Fatalf("first build: %+v", a)
	}
	// The same arm rebuilt is not drift.
	if err := d.RecordTaskRSIArm(ctx, rsi.TaskArm{TaskID: live, PromptSHA: shaA, LessonsMode: "bench", LessonIDs: []int64{3, 5}, LessonsBlockSHA: "blk"}); err != nil {
		t.Fatal(err)
	}
	if rsiArmRow(t, d, live).Drifted {
		t.Fatal("an identical rebuild must not drift")
	}
	if err := d.RecordTaskRSIArm(ctx, rsi.TaskArm{TaskID: live, PromptSHA: shaB, LessonsMode: "bench", LessonIDs: []int64{3, 5}, LessonsBlockSHA: "blk"}); err != nil {
		t.Fatal(err)
	}
	if a := rsiArmRow(t, d, live); !a.Drifted || a.PromptSHA["planner"] != "pA" {
		t.Fatalf("a different rebuild while live must drift and keep the first record: %+v", a)
	}

	done := rsiLedgerTask(t, d, "bench-x-run-done", "running")
	if err := d.RecordTaskRSIArm(ctx, rsi.TaskArm{TaskID: done, PromptSHA: shaA, LessonsMode: "off"}); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Exec(`UPDATE tasks SET status='done' WHERE id=$1`, done); err != nil {
		t.Fatal(err)
	}
	if err := d.RecordTaskRSIArm(ctx, rsi.TaskArm{TaskID: done, PromptSHA: shaB, LessonsMode: "off"}); err != nil {
		t.Fatal(err)
	}
	if a := rsiArmRow(t, d, done); a.Drifted || a.PromptSHA["planner"] != "pA" {
		t.Fatalf("a rebuild after the task ended must change nothing: %+v", a)
	}

	// A prompt pointer move on an RSI role drifts every live task, not finished ones.
	running := rsiLedgerTask(t, d, "bench-x-run-2", "running")
	if err := d.RecordTaskRSIArm(ctx, rsi.TaskArm{TaskID: running, PromptSHA: shaA, LessonsMode: "off"}); err != nil {
		t.Fatal(err)
	}
	planner := mustAgentID(t, d, "planner")
	_, before := rsiAgentBody(t, d, "planner")
	t.Cleanup(func() { _, _ = d.SavePrompt(planner, before, "test restore", "test") })
	if _, err := d.SavePrompt(planner, "planner edited mid-task {{.Goal}}", "test", "ui"); err != nil {
		t.Fatal(err)
	}
	if a := rsiArmRow(t, d, running); !a.Drifted || a.DriftReason == "" {
		t.Fatalf("pointer move while live must drift: %+v", a)
	}
	if rsiArmRow(t, d, done).Drifted {
		t.Fatal("a finished task must not drift on a later prompt change")
	}
	// The freshness bound filters older tasks out.
	got, err := d.RSITaskArms(ctx, []int64{live, done}, time.Now().Add(time.Hour))
	if err != nil || len(got) != 0 {
		t.Fatalf("since filter: %v %v", got, err)
	}
}

func TestRSISuiteRunRegistrationPG(t *testing.T) {
	d := rsiTestDB(t)
	ctx := context.Background()
	p, err := d.CreateRSIProposal(ctx, RSIProposal{Round: 0, Surface: "lesson", LessonIDs: []int64{1}, Rationale: "r"}, oneEdit, 10)
	if err != nil {
		t.Fatal(err)
	}
	stamp := fmt.Sprint(time.Now().UnixNano())
	reg := func(name string, max int) (*rsi.SuiteRun, error) {
		return d.RegisterRSISuiteRun(ctx, rsi.SuiteRun{ProposalID: p.ID, SuiteRun: name + stamp, Suite: "heldout", SuiteVersion: "v1",
			Split: "held-out", Targets: []string{"A", "B"}, Seed: 7, Replicates: 2}, max, "loop")
	}
	r1, err := reg("s1-", 2)
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(r1.Targets) != "[A B]" || r1.Seed != 7 || r1.Replicates != 2 || r1.EvidenceID != nil {
		t.Fatalf("stored: %+v", r1)
	}
	if _, err := reg("s2-", 2); !errors.Is(err, ErrRSIConflict) {
		t.Fatalf("second open registration: want conflict, got %v", err)
	}
	open, at, err := d.OpenRSISuiteRun(ctx, p.ID, "s1-"+stamp)
	if err != nil || open.ID != r1.ID || at.IsZero() {
		t.Fatalf("open run: %+v %v", open, err)
	}
	if _, _, err := d.OpenRSISuiteRun(ctx, p.ID, "unregistered"); !errors.Is(err, ErrRSIConflict) {
		t.Fatalf("unregistered run: %v", err)
	}
	// Evidence must consume an open registration of this proposal.
	if _, err := d.AddRSIEvidence(ctx, p.ID, r1.ID+1000, false, []byte(`{}`), []byte(`{}`), []byte(`{}`), nil, 10, "loop"); !errors.Is(err, ErrRSIConflict) {
		t.Fatalf("evidence on an unknown registration: %v", err)
	}
	ev, err := d.AddRSIEvidence(ctx, p.ID, r1.ID, false, []byte(`{}`), []byte(`{}`), []byte(`{}`), nil, 10, "loop")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := d.OpenRSISuiteRun(ctx, p.ID, "s1-"+stamp); !errors.Is(err, ErrRSIConflict) {
		t.Fatalf("a consumed run is not open: %v", err)
	}
	if _, err := d.AddRSIEvidence(ctx, p.ID, r1.ID, false, []byte(`{}`), []byte(`{}`), []byte(`{}`), nil, 10, "loop"); !errors.Is(err, ErrRSIConflict) {
		t.Fatalf("a registration backs one upload: %v", err)
	}
	if _, err := reg("s1-", 5); !errors.Is(err, ErrRSIConflict) {
		t.Fatalf("a suite_run id is registered once: %v", err)
	}
	if _, err := reg("s2-", 2); err != nil {
		t.Fatal(err)
	}
	runs, err := d.RSISuiteRuns(ctx, p.ID)
	if err != nil || len(runs) != 2 || runs[0].EvidenceID == nil || *runs[0].EvidenceID != ev.ID {
		t.Fatalf("runs: %+v %v", runs, err)
	}
	ev2, err := d.AddRSIEvidence(ctx, p.ID, runs[1].ID, false, []byte(`{}`), []byte(`{}`), []byte(`{}`), nil, 10, "loop")
	if err != nil || ev2 == nil {
		t.Fatal(err)
	}
	if _, err := reg("s3-", 2); !errors.Is(err, ErrRSIConflict) {
		t.Fatalf("registration cap: %v", err)
	}
	if err := d.RejectRSIProposal(ctx, p.ID, "owner", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := reg("s4-", 10); !errors.Is(err, ErrRSIConflict) {
		t.Fatalf("a closed proposal takes no runs: %v", err)
	}
}

// A graduated lesson that loses its last evidence is retired, and a later
// extraction can revive it as a candidate; both land on the adopted
// proposal's trail.
func TestRSIAdoptedLessonRetireReviveLoggedPG(t *testing.T) {
	d := rsiTestDB(t)
	ctx := context.Background()
	key := "rsi-retire-" + t.Name() + fmt.Sprint(time.Now().UnixNano())
	id, err := upsertLessonQ(ctx, d, key, "positive", "sqli", "Confirmed sqli", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Exec(`DELETE FROM rsi_lessons WHERE id=$1`, id) })
	if _, err := d.Exec(`INSERT INTO rsi_lesson_evidence(lesson_id,source_kind,source_id) VALUES($1,'finding',424242)`, id); err != nil {
		t.Fatal(err)
	}
	p, err := d.CreateRSIProposal(ctx, RSIProposal{Round: 0, Surface: "lesson", LessonIDs: []int64{id}, Rationale: "r"}, oneEdit, 10)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.AdoptRSIProposal(ctx, p.ID, passEvidence(t, d, p.ID, true), "owner", nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := retractLessonEvidenceQ(ctx, d, "finding", 424242, "positive", ""); err != nil {
		t.Fatal(err)
	}
	var status string
	if err := d.QueryRow(`SELECT status FROM rsi_lessons WHERE id=$1`, id).Scan(&status); err != nil || status != "retired" {
		t.Fatalf("lesson status %q %v", status, err)
	}
	if _, err := upsertLessonQ(ctx, d, key, "positive", "sqli", "Confirmed sqli", 0, 0); err != nil {
		t.Fatal(err)
	}
	if err := d.QueryRow(`SELECT status FROM rsi_lessons WHERE id=$1`, id).Scan(&status); err != nil || status != "candidate" {
		t.Fatalf("revived status %q %v", status, err)
	}
	lg, err := d.RSILog(ctx, p.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	var actions []string
	for _, e := range lg {
		actions = append(actions, e.Action)
		if e.Action == "lesson_retire" && (e.Actor != "system" || string(e.Detail) == "{}") {
			t.Fatalf("retire row: %+v", e)
		}
	}
	if fmt.Sprint(actions) != "[propose register evidence adopt lesson_retire lesson_revive]" {
		t.Fatalf("log %v", actions)
	}
	// A candidate lesson (not graduated) retiring logs nothing.
	other, err := upsertLessonQ(ctx, d, key+"-other", "positive", "xss", "Confirmed xss", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Exec(`DELETE FROM rsi_lessons WHERE id=$1`, other) })
	if err := retireLessonsIfEmptyQ(ctx, d, []int64{other}); err != nil {
		t.Fatal(err)
	}
	if lg2, _ := d.RSILog(ctx, 0, 1000); len(lg2) != len(lg) {
		t.Fatalf("a non-adopted lesson retiring wrote %d log rows", len(lg2)-len(lg))
	}
}
