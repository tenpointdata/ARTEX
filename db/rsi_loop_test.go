package db

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Autumn-27/artex/rsi"
)

func rsiTestDB(t *testing.T) *DB {
	t.Helper()
	d, err := Open(testDSN(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	ctx := context.Background()
	for _, q := range []string{`DELETE FROM rsi_suite_runs`, `DELETE FROM rsi_proposal_evidence`, `ALTER TABLE rsi_adoption_log DISABLE TRIGGER USER`,
		`DELETE FROM rsi_adoption_log`, `ALTER TABLE rsi_adoption_log ENABLE TRIGGER USER`, `DELETE FROM rsi_proposals`} {
		if _, err := d.ExecContext(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	return d
}

func rsiAgentBody(t *testing.T, d *DB, key string) (int, string) {
	t.Helper()
	a, err := d.GetAgentByKey(key)
	if err != nil || a == nil {
		t.Fatalf("agent %s: %v", key, err)
	}
	var ver int
	var body string
	err = d.QueryRow(`SELECT COALESCE(p.version,0), COALESCE(p.template_text,'') FROM agents a
		LEFT JOIN agent_prompts p ON p.id=a.current_prompt_id WHERE a.id=$1`, a.ID).Scan(&ver, &body)
	if err != nil {
		t.Fatal(err)
	}
	return ver, body
}

func oneEdit(int) int { return 1 }

var rsiRunSeq atomic.Int64

// openSuiteRun returns the proposal's open suite registration, registering
// one when there is none (every upload consumes one).
func openSuiteRun(t *testing.T, d *DB, proposalID int64) int64 {
	t.Helper()
	var id int64
	err := d.QueryRow(`SELECT id FROM rsi_suite_runs WHERE proposal_id=$1 AND evidence_id IS NULL`, proposalID).Scan(&id)
	if err == nil {
		return id
	}
	reg, err := d.RegisterRSISuiteRun(context.Background(), rsi.SuiteRun{ProposalID: proposalID,
		SuiteRun: fmt.Sprintf("test-%d-%d", time.Now().UnixNano(), rsiRunSeq.Add(1)), Suite: "heldout", SuiteVersion: "v1",
		Split: "held-out", Targets: []string{"A", "B"}, Seed: 1, Replicates: 1}, 100, "loop")
	if err != nil {
		t.Fatal(err)
	}
	return reg.ID
}

func passEvidence(t *testing.T, d *DB, id int64, passed bool) int64 {
	t.Helper()
	ev, err := d.AddRSIEvidence(context.Background(), id, openSuiteRun(t, d, id), passed, []byte(`{"passed":true}`), []byte(`{"stamp":{}}`), []byte(`{"stamp":{}}`), nil, 10, "loop")
	if err != nil {
		t.Fatal(err)
	}
	return ev.ID
}

func TestRSIPromptLifecyclePG(t *testing.T) {
	d := rsiTestDB(t)
	ctx := context.Background()
	if _, err := d.SavePrompt(mustAgentID(t, d, "planner"), "planner v-live {{.Goal}}", "test", "test"); err != nil {
		t.Fatal(err)
	}
	beforeVer, _ := rsiAgentBody(t, d, "planner")

	p, err := d.CreateRSIProposal(ctx, RSIProposal{Round: 0, Surface: "prompt_body", Target: "planner",
		Body: "planner candidate {{.Goal}}", BodySHA: "x", Rationale: "r", CreatedBy: "loop"}, oneEdit, 10)
	if err != nil {
		t.Fatal(err)
	}
	// Budget and forward-only rounds.
	if _, err := d.CreateRSIProposal(ctx, RSIProposal{Round: 0, Surface: "lesson", LessonIDs: []int64{1}}, oneEdit, 10); !errors.Is(err, ErrRSIConflict) {
		t.Fatalf("over budget: want conflict, got %v", err)
	}
	if _, err := d.CreateRSIProposal(ctx, RSIProposal{Round: 2, Surface: "lesson", LessonIDs: []int64{1}}, oneEdit, 10); !errors.Is(err, ErrRSIConflict) {
		t.Fatalf("skipping a round: want conflict, got %v", err)
	}
	if _, err := d.CreateRSIProposal(ctx, RSIProposal{Round: 1, Surface: "lesson", LessonIDs: []int64{1}}, oneEdit, 1); !errors.Is(err, ErrRSIConflict) {
		t.Fatalf("open cap: want conflict, got %v", err)
	}
	if _, err := d.CreateRSIProposal(ctx, RSIProposal{Round: 1, Surface: "lesson", LessonIDs: []int64{1}}, oneEdit, 10); err != nil {
		t.Fatal(err)
	}
	if _, err := d.CreateRSIProposal(ctx, RSIProposal{Round: 0, Surface: "lesson", LessonIDs: []int64{1}}, oneEdit, 10); !errors.Is(err, ErrRSIConflict) {
		t.Fatalf("closed round: want conflict, got %v", err)
	}
	// No adoption without passing evidence; failed evidence blocks too.
	if _, err := d.AdoptRSIProposal(ctx, p.ID, 1, "owner", func(RSIPromptState) error { return nil }, nil); !errors.Is(err, ErrRSIConflict) {
		t.Fatalf("adopt without evidence: %v", err)
	}
	failed := passEvidence(t, d, p.ID, false)
	if _, err := d.AdoptRSIProposal(ctx, p.ID, failed, "owner", func(RSIPromptState) error { return nil }, nil); !errors.Is(err, ErrRSIConflict) {
		t.Fatalf("adopt failed evidence: %v", err)
	}
	old := passEvidence(t, d, p.ID, true)
	ev := passEvidence(t, d, p.ID, true)
	if _, err := d.AdoptRSIProposal(ctx, p.ID, old, "owner", func(RSIPromptState) error { return nil }, nil); !errors.Is(err, ErrRSIConflict) {
		t.Fatalf("adopt stale evidence id: %v", err)
	}
	// The live-baseline check sees the active prompt and can refuse.
	stale := fmt.Errorf("%w: stale", ErrRSIConflict)
	var seen RSIPromptState
	if _, err := d.AdoptRSIProposal(ctx, p.ID, ev, "owner", func(s RSIPromptState) error { seen = s; return stale }, nil); !errors.Is(err, ErrRSIConflict) {
		t.Fatalf("stale baseline: %v", err)
	}
	if seen.Body != "planner v-live {{.Goal}}" || !seen.Stored {
		t.Fatalf("checkLive saw %+v", seen)
	}
	if v, _ := rsiAgentBody(t, d, "planner"); v != beforeVer {
		t.Fatal("refused adoption changed the prompt")
	}

	got, err := d.AdoptRSIProposal(ctx, p.ID, ev, "owner", func(RSIPromptState) error { return nil }, nil)
	if err != nil {
		t.Fatal(err)
	}
	ver, body := rsiAgentBody(t, d, "planner")
	if got.Status != "adopted" || body != "planner candidate {{.Goal}}" || ver != beforeVer+1 ||
		*got.PromptVersionBefore != beforeVer || *got.PromptVersionAfter != ver {
		t.Fatalf("adopt: %+v ver=%d body=%q", got, ver, body)
	}
	if _, err := d.AddRSIEvidence(ctx, p.ID, 0, true, []byte(`{}`), []byte(`{}`), []byte(`{}`), nil, 10, "loop"); !errors.Is(err, ErrRSIConflict) {
		t.Fatalf("evidence after adoption: %v", err)
	}
	if err := d.RejectRSIProposal(ctx, p.ID, "owner", "x"); !errors.Is(err, ErrRSIConflict) {
		t.Fatalf("reject after adoption: %v", err)
	}

	// Rollback is a pointer change back to the previous version.
	rb, err := d.RollbackRSIProposal(ctx, p.ID, "owner", "regressed live")
	if err != nil {
		t.Fatal(err)
	}
	ver, body = rsiAgentBody(t, d, "planner")
	if rb.Status != "rolled_back" || ver != beforeVer || body != "planner v-live {{.Goal}}" {
		t.Fatalf("rollback: %+v ver=%d body=%q", rb, ver, body)
	}
	var versions int
	if err := d.QueryRow(`SELECT count(*) FROM agent_prompts WHERE agent_id=$1 AND version=$2`, mustAgentID(t, d, "planner"), beforeVer+1).Scan(&versions); err != nil || versions != 1 {
		t.Fatalf("adopted version must stay in history: %d %v", versions, err)
	}
	if _, err := d.RollbackRSIProposal(ctx, p.ID, "owner", ""); !errors.Is(err, ErrRSIConflict) {
		t.Fatalf("second rollback: %v", err)
	}

	lg, err := d.RSILog(ctx, p.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	var actions []string
	for _, e := range lg {
		actions = append(actions, e.Action)
	}
	if fmt.Sprint(actions) != "[propose register evidence register evidence register evidence adopt rollback]" {
		t.Fatalf("log %v", actions)
	}
	if _, err := d.Exec(`UPDATE rsi_adoption_log SET actor='forged'`); err == nil {
		t.Fatal("adoption log must be append-only")
	}
	if _, err := d.Exec(`DELETE FROM rsi_adoption_log`); err == nil {
		t.Fatal("adoption log must be append-only")
	}
}

func TestRSIRollbackRefusesAfterOwnerEditPG(t *testing.T) {
	d := rsiTestDB(t)
	ctx := context.Background()
	p, err := d.CreateRSIProposal(ctx, RSIProposal{Round: 0, Surface: "prompt_body", Target: "worker",
		Body: "worker candidate", Rationale: "r"}, oneEdit, 10)
	if err != nil {
		t.Fatal(err)
	}
	ev := passEvidence(t, d, p.ID, true)
	if _, err := d.AdoptRSIProposal(ctx, p.ID, ev, "owner", func(RSIPromptState) error { return nil }, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := d.SavePrompt(mustAgentID(t, d, "worker"), "owner edit", "", "ui"); err != nil {
		t.Fatal(err)
	}
	if _, err := d.RollbackRSIProposal(ctx, p.ID, "owner", ""); !errors.Is(err, ErrRSIConflict) {
		t.Fatalf("rollback over an owner edit: %v", err)
	}
	if _, body := rsiAgentBody(t, d, "worker"); body != "owner edit" {
		t.Fatalf("owner edit overwritten: %q", body)
	}
}

func TestRSILessonGraduationPG(t *testing.T) {
	d := rsiTestDB(t)
	ctx := context.Background()
	var a, b int64
	for i, dst := range []*int64{&a, &b} {
		if err := d.QueryRow(`INSERT INTO rsi_lessons(dedupe_key,kind,cue,action) VALUES($1,'positive','xss','Confirmed xss') RETURNING id`,
			fmt.Sprintf("rsi-test-%d-%s", i, t.Name())).Scan(dst); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { d.Exec(`DELETE FROM rsi_lessons WHERE id=ANY($1::bigint[])`, []int64{a, b}) })
	p, err := d.CreateRSIProposal(ctx, RSIProposal{Round: 0, Surface: "lesson", LessonIDs: []int64{a, b}, Rationale: "r"}, oneEdit, 10)
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(p.LessonIDs) != fmt.Sprint([]int64{a, b}) {
		t.Fatalf("lesson ids %v", p.LessonIDs)
	}
	ev := passEvidence(t, d, p.ID, true)
	if _, err := d.AdoptRSIProposal(ctx, p.ID, ev, "owner", nil, nil); err != nil {
		t.Fatal(err)
	}
	got, err := d.LessonsByIDs(ctx, []int64{a, b})
	if err != nil || len(got) != 2 || got[0].Status != "adopted" || got[1].Status != "adopted" {
		t.Fatalf("graduated: %+v %v", got, err)
	}
	if _, err := d.RollbackRSIProposal(ctx, p.ID, "owner", ""); err != nil {
		t.Fatal(err)
	}
	got, _ = d.LessonsByIDs(ctx, []int64{a, b})
	if got[0].Status != "candidate" || got[1].Status != "candidate" {
		t.Fatalf("rolled back: %+v", got)
	}
}

func TestRSIEvidenceLedgerAndCapPG(t *testing.T) {
	d := rsiTestDB(t)
	ctx := context.Background()
	p, err := d.CreateRSIProposal(ctx, RSIProposal{Round: 0, Surface: "lesson", LessonIDs: []int64{1}, Rationale: "r"}, oneEdit, 10)
	if err != nil {
		t.Fatal(err)
	}
	add := func(passed bool, tasks []int64, cap int) error {
		_, err := d.AddRSIEvidence(ctx, p.ID, openSuiteRun(t, d, p.ID), passed, []byte(`{}`), []byte(`{}`), []byte(`{}`), tasks, cap, "loop")
		return err
	}
	if err := add(true, []int64{9001, 9002}, 5); err != nil {
		t.Fatal(err)
	}
	if err := add(true, []int64{9002, 9003}, 5); !errors.Is(err, ErrRSIConflict) {
		t.Fatalf("replayed task: want conflict, got %v", err)
	}
	if err := add(false, []int64{9002}, 5); err != nil {
		t.Fatalf("failing evidence may name a used task: %v", err)
	}
	if err := add(false, nil, 2); !errors.Is(err, ErrRSIConflict) {
		t.Fatalf("upload cap: want conflict, got %v", err)
	}
	bad, err := d.RSIEvidenceTasksInvalid(ctx, []int64{9001}, time.Time{})
	if err != nil || len(bad) != 1 {
		t.Fatalf("ledger task must be invalid for new evidence: %v %v", bad, err)
	}
}

func mustAgentID(t *testing.T, d *DB, key string) int64 {
	t.Helper()
	a, err := d.GetAgentByKey(key)
	if err != nil || a == nil {
		t.Fatalf("agent %s: %v", key, err)
	}
	return a.ID
}
