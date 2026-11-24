package db

import (
	"errors"
	"strings"
	"testing"

	"github.com/Autumn-27/artex/methodology"
)

func TestCoverageEntryCloses(t *testing.T) {
	cases := []struct {
		outcome, evidence, reason string
		want                      bool
	}{
		{OutcomeReported, "", "", true},
		{OutcomeNoIssueFound, "fact 3", "", true},
		{OutcomeNoIssueFound, " ", "", false}, // a missing_evidence gap never closes an item
		{OutcomeRuledOut, "", "no login", true},
		{OutcomeNotApplicable, "", "", false},
		{OutcomeNeedsFollowUp, "x", "y", false},
	}
	for _, c := range cases {
		if got := CoverageEntryCloses(c.outcome, c.evidence, c.reason); got != c.want {
			t.Errorf("%+v: got %v", c, got)
		}
	}
}

func TestMethodologyCoverage(t *testing.T) {
	d, err := Open(testDSN(t))
	if err != nil {
		SkipPGUnavailable(t, err)
	}
	t.Cleanup(func() { d.Close() })
	as := d.Assets()

	none, err := d.CreateTaskWithOptions("port scan 10.0.0.0/24", "list open services", TaskCreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cleanupErr(t, "task", d.DeleteTask(none.ID)) })
	if got, err := as.MethodologyCoverage(none.ID); err != nil || got == nil || len(got) != 0 {
		t.Fatalf("no methodology applies: %v %v", got, err)
	}

	task, err := d.CreateTaskWithOptions("test https://shop.example.test", "find web vulnerabilities", TaskCreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cleanupErr(t, "task", d.DeleteTask(task.ID)) })
	for _, e := range []CoverageEntry{
		{Surface: "/q", RiskArea: "SQLi", Outcome: OutcomeNoIssueFound, Evidence: "fact 7"},
		{Surface: "/s", RiskArea: "xss", Outcome: OutcomeNoIssueFound}, // missing evidence: not closed
		{Surface: "/c", RiskArea: "wstg-sess-05", Outcome: OutcomeRuledOut, Reason: "no state-changing forms"},
	} {
		if err := as.RecordCoverage(task.ID, e); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := d.AddFinding(task.ID, 0, "SSRF", "ssrf", "high", "s", "e", "w", nil); err != nil {
		t.Fatal(err)
	}
	fp, err := d.AddFinding(task.ID, 0, "idor", "idor", "high", "s", "e", "w", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Exec(`UPDATE findings SET status=$2 WHERE id=$1`, fp, FindingFalsePositive); err != nil {
		t.Fatal(err)
	}
	store := d.Exploration(task.ExplorationID)
	cited, err := store.AddIntent(map[string]any{"summary": "probe the login", IntentMethodologyItemKey: "WSTG-ATHN-03"}, 5, nil, "planner")
	if err != nil {
		t.Fatal(err)
	}
	deleted, err := store.AddIntent(map[string]any{"summary": "gone", IntentMethodologyItemKey: "WSTG-CONF-06"}, 5, nil, "planner")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Exec(`UPDATE exploration_nodes SET state=$2 WHERE id=$1`, deleted, StateIntentDeleted); err != nil {
		t.Fatal(err)
	}

	scores, err := as.MethodologyCoverage(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(scores) != 1 || scores[0].ID != "owasp-wstg" {
		t.Fatalf("want WSTG only: %+v", scores)
	}
	status := map[string]methodology.ItemStatus{}
	for _, it := range scores[0].Items {
		status[it.ID] = it
	}
	want := map[string]string{
		"WSTG-INPV-05": methodology.StatusCovered,    // sqli with evidence
		"WSTG-SESS-05": methodology.StatusCovered,    // by item id, reasoned ruled_out
		"WSTG-INPV-19": methodology.StatusCovered,    // SSRF finding
		"WSTG-INPV-01": methodology.StatusInProgress, // xss without evidence
		"WSTG-ATHZ-04": methodology.StatusOpen,       // false-positive finding proves nothing
		"WSTG-ATHN-03": methodology.StatusInProgress, // cited by an intent
		"WSTG-CONF-06": methodology.StatusOpen,       // cited only by a deleted intent
	}
	for id, st := range want {
		if status[id].Status != st {
			t.Errorf("%s: %s, want %s", id, status[id].Status, st)
		}
	}
	if got := status["WSTG-ATHN-03"].Intents; len(got) != 1 || got[0] != cited {
		t.Errorf("citation intents: %v, want [%d]", got, cited)
	}
	if scores[0].Covered != 3 || scores[0].InProgress != 2 {
		t.Errorf("counts: covered %d in progress %d", scores[0].Covered, scores[0].InProgress)
	}
}

// TestTaskMethodologiesOperatorChoice: a task created with an explicit
// methodology list is checked against exactly that list, whatever its goal says;
// an empty list keeps the automatic selection, and an unknown id is refused.
func TestTaskMethodologiesOperatorChoice(t *testing.T) {
	d, err := Open(testDSN(t))
	if err != nil {
		SkipPGUnavailable(t, err)
	}
	t.Cleanup(func() { d.Close() })
	as := d.Assets()

	ids := func(ms []methodology.Methodology) []string {
		out := []string{}
		for _, m := range ms {
			out = append(out, m.ID)
		}
		return out
	}
	cases := []struct {
		desc, goal string
		chosen     []string
		want       []string
	}{
		// No word selects anything: the operator's choice is the only source.
		{"test 10.0.0.5:8080", "find vulnerabilities", []string{"OWASP API Top10"}, []string{"owasp-api-top10"}},
		// The goal would select WSTG; the explicit choice replaces it (exclusion).
		{"test https://shop.example.test", "find web vulnerabilities", []string{"owasp-api-top10", "owasp-api-top10"}, []string{"owasp-api-top10"}},
		// Empty = automatic, unchanged.
		{"test https://shop.example.test", "find web vulnerabilities", nil, []string{"owasp-wstg"}},
		{"test 10.0.0.5:8080", "find vulnerabilities", []string{}, []string{}},
	}
	for i, c := range cases {
		task, err := d.CreateTaskWithOptions(c.desc, c.goal, TaskCreateOptions{Methodologies: c.chosen})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { cleanupErr(t, "task", d.DeleteTask(task.ID)) })
		stored, err := d.GetTask(task.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(c.chosen) > 0 && (len(stored.Methodologies) != 1 || stored.Methodologies[0] != "owasp-api-top10") {
			t.Errorf("case %d: stored %v, want the canonical id once", i, stored.Methodologies)
		}
		if len(c.chosen) == 0 && (stored.Methodologies == nil || len(stored.Methodologies) != 0) {
			t.Errorf("case %d: stored %#v, want an empty (auto) list", i, stored.Methodologies)
		}
		ms, err := as.TaskMethodologies(task.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got := ids(ms); strings.Join(got, ",") != strings.Join(c.want, ",") {
			t.Errorf("case %d: methodologies %v, want %v", i, got, c.want)
		}
		scores, err := as.MethodologyCoverage(task.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(scores) != len(c.want) || (len(scores) == 1 && scores[0].ID != c.want[0]) {
			t.Errorf("case %d: coverage scores %+v, want %v", i, scores, c.want)
		}
	}

	if _, err := d.CreateTaskWithOptions("x", "y", TaskCreateOptions{Methodologies: []string{"owasp-nope"}}); !errors.Is(err, ErrTaskMethodologyUnknown) {
		t.Fatalf("unknown methodology: %v, want ErrTaskMethodologyUnknown", err)
	}
}

// TestTaskMethodologiesNoneAndPatch: a [none] choice gets no checklist even
// when the goal would select one, "none" beside a methodology is refused, and
// UpdateTask rewrites the choice (none -> chosen -> automatic) so the next
// TaskMethodologies read (the planner's per-round block) follows it.
func TestTaskMethodologiesNoneAndPatch(t *testing.T) {
	d, err := Open(testDSN(t))
	if err != nil {
		SkipPGUnavailable(t, err)
	}
	t.Cleanup(func() { d.Close() })
	as := d.Assets()
	current := func(id int64) string {
		t.Helper()
		ms, err := as.TaskMethodologies(id)
		if err != nil {
			t.Fatal(err)
		}
		out := []string{}
		for _, m := range ms {
			out = append(out, m.ID)
		}
		return strings.Join(out, ",")
	}

	task, err := d.CreateTaskWithOptions("test https://shop.example.test", "find web vulnerabilities",
		TaskCreateOptions{Methodologies: []string{" None "}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cleanupErr(t, "task", d.DeleteTask(task.ID)) })
	if got := strings.Join(task.Methodologies, ","); got != methodology.None {
		t.Fatalf("stored %q, want the canonical sentinel", got)
	}
	if got := current(task.ID); got != "" {
		t.Fatalf("none task got checklists %q despite a matching goal", got)
	}
	if scores, err := as.MethodologyCoverage(task.ID); err != nil || len(scores) != 0 {
		t.Fatalf("none task coverage: %v %v", scores, err)
	}

	if _, err := d.CreateTaskWithOptions("x", "y", TaskCreateOptions{Methodologies: []string{"none", "owasp-wstg"}}); !errors.Is(err, ErrTaskMethodologyUnknown) {
		t.Fatalf("none beside a methodology: %v, want ErrTaskMethodologyUnknown", err)
	}

	steps := []struct {
		set  []string
		want string
	}{
		{[]string{"owasp-api-top10"}, "owasp-api-top10"},
		{[]string{}, "owasp-wstg"}, // back to automatic: the goal selects WSTG again
		{[]string{"none"}, ""},
	}
	for i, s := range steps {
		set := s.set
		updated, err := d.UpdateTask(task.ID, TaskPatch{Methodologies: &set})
		if err != nil || updated == nil {
			t.Fatalf("step %d: %v %v", i, updated, err)
		}
		if got := current(task.ID); got != s.want {
			t.Errorf("step %d: methodologies %q, want %q", i, got, s.want)
		}
	}

	// nil leaves the choice alone; a bad id is refused without changing it.
	name := "renamed"
	if _, err := d.UpdateTask(task.ID, TaskPatch{Name: &name}); err != nil {
		t.Fatal(err)
	}
	bad := []string{"owasp-nope"}
	if _, err := d.UpdateTask(task.ID, TaskPatch{Methodologies: &bad}); !errors.Is(err, ErrTaskMethodologyUnknown) {
		t.Fatalf("bad patch: %v", err)
	}
	if stored, err := d.GetTask(task.ID); err != nil || strings.Join(stored.Methodologies, ",") != methodology.None {
		t.Fatalf("choice after no-op and refused patches: %v %v", stored, err)
	}
}
