package rsi

import (
	"strings"
	"testing"
)

func TestSlugMatchesHarness(t *testing.T) {
	// eval_suite.slug: lower, [^a-z0-9-]+ → "-", strip "-", first 40.
	for in, want := range map[string]string{
		"Juice-Shop":            "juice-shop",
		"DBEN v1/001":           "dben-v1-001",
		"--x__y--":              "x-y",
		strings.Repeat("a", 50): strings.Repeat("a", 40),
	} {
		if got := Slug(in); got != want {
			t.Fatalf("Slug(%q) = %q, want %q", in, got, want)
		}
	}
	if got := RunID("20261124-1", 1, "planner v2", "dvga"); got != "20261124-1-r1-planner-v2-dvga" {
		t.Fatalf("RunID = %q", got)
	}
}

func ledgerArm(role, arm string, sha map[string]string, lessons []int64, block any, reg SuiteRun, task0 int64) ArmResult {
	raw := map[string]any{"suite_run": reg.SuiteRun, "suite": reg.Suite, "suite_version": reg.SuiteVersion,
		"seed": float64(reg.Seed), "replicates": float64(reg.Replicates), "lessons_block_sha": block}
	a := ArmResult{Stamp: Stamp{Role: role, Arm: arm, Split: reg.Split, PromptSHA: sha, LessonIDs: lessons, Raw: raw}}
	id := task0
	for _, tg := range reg.Targets {
		for rep := 0; rep < reg.Replicates; rep++ {
			p := &Payload{}
			p.Task.ID = TaskID(id)
			a.Records = append(a.Records, Record{Target: tg, Arm: arm, Replicate: rep, RunID: RunID(reg.SuiteRun, rep, arm, tg), Payload: p})
			id++
		}
	}
	return a
}

func ledgerFixture() (SuiteRun, ArmResult, ArmResult, map[int64]TaskArm) {
	reg := SuiteRun{SuiteRun: "run1", Suite: "heldout", SuiteVersion: "v1", Split: "held-out", Targets: []string{"A", "B"}, Seed: 7, Replicates: 2}
	shaB := map[string]string{"goals": "g", "planner": "p0", "worker": "w"}
	shaC := map[string]string{"goals": "g", "planner": "p1", "worker": "w"}
	base := ledgerArm("baseline", "base", shaB, nil, nil, reg, 100)
	cand := ledgerArm("candidate", "cand", shaC, []int64{4, 2}, "blk", reg, 200)
	ledger := map[int64]TaskArm{}
	for _, arm := range []ArmResult{base, cand} {
		for _, r := range arm.Records {
			id := int64(r.Payload.Task.ID)
			l := TaskArm{TaskID: id, TaskName: "bench-x-" + r.RunID, PromptSHA: arm.Stamp.PromptSHA}
			if arm.Stamp.Role == "candidate" {
				l.LessonIDs, l.LessonsBlockSHA = []int64{2, 4}, "blk"
			}
			ledger[id] = l
		}
	}
	return reg, base, cand, ledger
}

func TestCheckRegistrationAndLedger(t *testing.T) {
	reg, base, cand, ledger := ledgerFixture()
	if r := CheckRegistration(base, cand, reg); len(r) != 0 {
		t.Fatalf("clean registration: %v", r)
	}
	if r := CheckLedger(base, cand, ledger); len(r) != 0 {
		t.Fatalf("clean ledger: %v", r)
	}

	cases := map[string]func(reg *SuiteRun, base, cand *ArmResult, ledger map[int64]TaskArm){
		"another suite run": func(reg *SuiteRun, _, _ *ArmResult, _ map[int64]TaskArm) { reg.SuiteRun = "run2" },
		"another seed":      func(reg *SuiteRun, _, _ *ArmResult, _ map[int64]TaskArm) { reg.Seed = 8 },
		"dropped target": func(_ *SuiteRun, _, cand *ArmResult, _ map[int64]TaskArm) {
			cand.Records = cand.Records[:2]
		},
		"unregistered target": func(reg *SuiteRun, _, _ *ArmResult, _ map[int64]TaskArm) { reg.Targets = []string{"A"} },
		"duplicated run": func(_ *SuiteRun, base, _ *ArmResult, _ map[int64]TaskArm) {
			base.Records[1] = base.Records[0]
		},
		"forged run id": func(_ *SuiteRun, base, _ *ArmResult, _ map[int64]TaskArm) { base.Records[0].RunID = "run1-r0-base-b" },
	}
	for name, mutate := range cases {
		reg, base, cand, ledger := ledgerFixture()
		mutate(&reg, &base, &cand, ledger)
		if r := CheckRegistration(base, cand, reg); len(r) == 0 {
			t.Fatalf("%s: registration check passed", name)
		}
	}

	ledgerCases := map[string]func(base, cand *ArmResult, ledger map[int64]TaskArm){
		// The attack: the candidate file claims the candidate body on tasks
		// that actually ran the baseline.
		"task ran the other body": func(_, cand *ArmResult, ledger map[int64]TaskArm) {
			l := ledger[200]
			l.PromptSHA = map[string]string{"goals": "g", "planner": "p0", "worker": "w"}
			ledger[200] = l
		},
		"no ledger row": func(_, _ *ArmResult, ledger map[int64]TaskArm) { delete(ledger, 101) },
		"drifted": func(_, _ *ArmResult, ledger map[int64]TaskArm) {
			l := ledger[100]
			l.Drifted, l.DriftReason = true, "prompt changed"
			ledger[100] = l
		},
		"other lessons injected": func(_, _ *ArmResult, ledger map[int64]TaskArm) {
			l := ledger[201]
			l.LessonIDs = []int64{2}
			ledger[201] = l
		},
		"baseline saw lessons": func(_, _ *ArmResult, ledger map[int64]TaskArm) {
			l := ledger[102]
			l.LessonIDs, l.LessonsBlockSHA = []int64{9}, "x"
			ledger[102] = l
		},
		"another block": func(_, _ *ArmResult, ledger map[int64]TaskArm) {
			l := ledger[203]
			l.LessonsBlockSHA = "other"
			ledger[203] = l
		},
		"task of another target": func(_, _ *ArmResult, ledger map[int64]TaskArm) {
			l := ledger[100]
			l.TaskName = "bench-x-run1-r0-base-b"
			ledger[100] = l
		},
	}
	for name, mutate := range ledgerCases {
		_, base, cand, ledger := ledgerFixture()
		mutate(&base, &cand, ledger)
		if r := CheckLedger(base, cand, ledger); len(r) == 0 {
			t.Fatalf("%s: ledger check passed", name)
		}
	}
	// A crashed run has no task and is not checked against the ledger.
	_, base, cand, ledger = ledgerFixture()
	cand.Records[0].Payload = nil
	delete(ledger, 200)
	if r := CheckLedger(base, cand, ledger); len(r) != 0 {
		t.Fatalf("crashed run: %v", r)
	}
}

func TestSuiteRunValidate(t *testing.T) {
	ok := SuiteRun{SuiteRun: "r", Suite: "s", SuiteVersion: "v", Split: "held-out", Targets: []string{"a"}, Replicates: 1}
	if err := ok.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []SuiteRun{
		{Suite: "s", SuiteVersion: "v", Split: "h", Targets: []string{"a"}, Replicates: 1},
		{SuiteRun: "r", Suite: "s", SuiteVersion: "v", Split: "h", Replicates: 1},
		{SuiteRun: "r", Suite: "s", SuiteVersion: "v", Split: "h", Targets: []string{"a", "a"}, Replicates: 1},
		{SuiteRun: "r", Suite: "s", SuiteVersion: "v", Split: "h", Targets: []string{"a"}},
	} {
		if bad.Validate() == nil {
			t.Fatalf("accepted %+v", bad)
		}
	}
}
