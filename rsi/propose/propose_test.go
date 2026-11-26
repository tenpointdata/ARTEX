package propose

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Autumn-27/artex/agent"
	"github.com/Autumn-27/artex/rsi"
)

var bodies = map[string]string{"goals": "You set goals.", "planner": "You plan. {{.Goal}}", "worker": "You work."}

// noisy is a task whose findings were mostly false positives or duplicates.
func noisy(id string, company int64) Task {
	t := Task{ID: id, CompanyID: company}
	t.Outcome.Status = "done"
	t.Outcome.Findings.Total, t.Outcome.Findings.FalsePositive, t.Outcome.Findings.Duplicate = 4, 1, 1
	return t
}

// everything is a task on which every default signal holds.
func everything(id string, company int64) Task {
	t := noisy(id, company)
	t.Outcome.Status = "failed"
	t.Outcome.ToolCalls, t.Outcome.SkillMisses = 30, 3
	t.Outcome.Retests.Inconclusive = 2
	t.Lessons = []Lesson{{ID: 9, Kind: "caution", Support: 3}}
	return t
}

func TestCriticNeedsTransferAcrossScopes(t *testing.T) {
	in := Input{Round: 0, Slots: 4, Bodies: bodies, Rules: DefaultRules(nil), Markers: agent.CodeOwnedTailMarkers,
		Tasks: []Task{noisy("1", 7), noisy("2", 7), noisy("3", 7)}}
	if p := Build(in, DefaultConfig); len(p.Drafts) != 0 {
		t.Fatalf("one company only: want no drafts, got %+v", p.Drafts)
	}
	in.Tasks = append(in.Tasks, noisy("4", 8))
	p := Build(in, DefaultConfig)
	if len(p.Drafts) != 1 || p.Drafts[0].Rule != "worker-precision" {
		t.Fatalf("want the worker-precision draft, got %+v skipped %+v", p.Drafts, p.Skipped)
	}
	d := p.Drafts[0]
	if d.Edit.Target != "worker" || !strings.HasPrefix(d.Edit.Body, bodies["worker"]) || !strings.Contains(d.Edit.Body, GuidanceHeading) {
		t.Fatalf("body must extend the active worker body: %q", d.Edit.Body)
	}
	if strings.Join(d.ProposedFrom, ",") != "task:1,task:2,task:3,task:4" || d.Rationale == "" || d.Scopes != 2 {
		t.Fatalf("proposed_from/rationale: %+v", d)
	}
	// Pruner: the same guidance is not proposed twice, nor re-proposed once active.
	in.Prior = []Prior{{Round: 0, Target: "worker", Body: d.Edit.Body, Status: "rejected"}}
	if p := Build(in, DefaultConfig); len(p.Drafts) != 0 {
		t.Fatal("a guidance already proposed must not be drafted again")
	}
	in.Prior, in.Bodies = nil, map[string]string{"worker": d.Edit.Body}
	if p := Build(in, DefaultConfig); len(p.Drafts) != 0 {
		t.Fatal("guidance already in the active body must not be drafted again")
	}
}

func TestBuildStaysInsideBudget(t *testing.T) {
	gaps := []SkillGap{{AgentKey: "worker", CategoryID: 1, Misses: 2}, {AgentKey: "worker", CategoryID: 2, Misses: 1}}
	tasks := []Task{everything("1", 1), everything("2", 2), everything("3", 3)}
	all := Build(Input{Slots: 99, Tasks: tasks, Bodies: bodies, Rules: DefaultRules(gaps), Markers: agent.CodeOwnedTailMarkers}, DefaultConfig)
	if len(all.Drafts) != 6 {
		t.Fatalf("want every default rule to qualify, got %d: %+v", len(all.Drafts), all.Skipped)
	}
	for round := 0; round < 5; round++ {
		n := rsi.DefaultBudget.Edits(round)
		p := Build(Input{Round: round, Slots: n, Tasks: tasks, Bodies: bodies, Rules: DefaultRules(gaps), Markers: agent.CodeOwnedTailMarkers}, DefaultConfig)
		if len(p.Drafts) != n {
			t.Fatalf("round %d: %d drafts, budget %d", round, len(p.Drafts), n)
		}
		dir := t.TempDir()
		for _, d := range p.Drafts {
			if _, err := WriteArm(dir, d, agent.CodeOwnedTailMarkers); err != nil {
				t.Fatal(err)
			}
		}
		files, _ := filepath.Glob(filepath.Join(dir, "*.json"))
		if len(files) != n {
			t.Fatalf("round %d: %d arm files, budget %d", round, len(files), n)
		}
	}
	if p := Build(Input{Slots: 0, Tasks: tasks, Bodies: bodies, Rules: DefaultRules(gaps)}, DefaultConfig); len(p.Drafts) != 0 {
		t.Fatal("no slots: no drafts")
	}
}

// TestProposerCannotTouchImmutableSurfaces: a rule aimed at any immutable
// surface, a role outside the search space, or a body imitating a code-owned
// tail never becomes a draft, and a forged draft is refused at write time.
func TestProposerCannotTouchImmutableSurfaces(t *testing.T) {
	tasks := []Task{everything("1", 1), everything("2", 2)}
	always := func(Task) bool { return true }
	var rules []Rule
	targets := append(append([]string{}, rsi.ImmutableSurfaces...), rsi.GitReviewedSurfaces...)
	targets = append(targets, "retester", "auto", "pentest", "")
	all := map[string]string{}
	for k, v := range bodies {
		all[k] = v
	}
	for _, s := range targets {
		all[s] = "an existing body"
		rules = append(rules, Rule{ID: "to-" + s, Role: s, Holds: always, Guidance: "Do something else."})
	}
	for i, m := range agent.CodeOwnedTailMarkers {
		rules = append(rules, Rule{ID: "marker-" + string(rune('a'+i)), Role: "worker", Holds: always, Guidance: "Note: " + strings.ToUpper(m) + " applies."})
	}
	p := Build(Input{Slots: 99, Tasks: tasks, Bodies: all, Rules: rules, Markers: agent.CodeOwnedTailMarkers}, DefaultConfig)
	if len(p.Drafts) != 0 {
		t.Fatalf("drafts escaped the search space: %+v", p.Drafts)
	}
	for _, s := range p.Skipped {
		if !strings.Contains(s.Reason, "rsi.ValidateEdit") {
			t.Fatalf("%s skipped for %q, want a ValidateEdit refusal", s.Rule, s.Reason)
		}
	}
	dir := t.TempDir()
	forged := []rsi.Edit{{Surface: rsi.SurfaceLesson, LessonIDs: []int64{1}}}
	for _, s := range targets {
		forged = append(forged, rsi.Edit{Surface: rsi.SurfaceKind(s), Target: "worker", Body: "x"},
			rsi.Edit{Surface: rsi.SurfacePromptBody, Target: s, Body: "x"})
	}
	for _, m := range agent.CodeOwnedTailMarkers {
		forged = append(forged, rsi.Edit{Surface: rsi.SurfacePromptBody, Target: "planner", Body: "ok\n" + m})
	}
	for _, e := range forged {
		_, err := WriteArm(dir, Draft{Name: "forged", Edit: e, ProposedFrom: []string{"task:1"}}, agent.CodeOwnedTailMarkers)
		if !errors.Is(err, rsi.ErrOutsideSearchSpace) {
			t.Fatalf("%+v: want ErrOutsideSearchSpace, got %v", e, err)
		}
	}
	if ents, _ := os.ReadDir(dir); len(ents) != 0 {
		t.Fatalf("refused drafts wrote files: %v", ents)
	}
}

func TestSlots(t *testing.T) {
	b := rsi.DefaultBudget // 4, 2, 1, 1
	cases := []struct {
		prior        []Prior
		want         int
		round, slots int
	}{
		{nil, -1, 0, 4},
		{nil, 1, 1, 0},
		{[]Prior{{Round: 0, Status: "rejected"}, {Round: 0, Status: "adopted"}}, -1, 0, 2},
		{[]Prior{{Round: 1, Status: "rejected"}, {Round: 1, Status: "rejected"}}, -1, 1, 0},
		{[]Prior{{Round: 1, Status: "rejected"}, {Round: 1, Status: "rejected"}}, 2, 2, 1},
		{[]Prior{{Round: 1, Status: "rejected"}}, 0, 0, 0},
		{[]Prior{{Round: 0, Status: "proposed"}, {Round: 0, Status: "passed"}, {Round: 0, Status: "failed"}}, -1, 0, 1},
	}
	for i, c := range cases {
		r, s, note := Slots(b, 4, c.prior, c.want)
		if r != c.round || s != c.slots {
			t.Fatalf("case %d: got round %d slots %d (%s), want %d/%d", i, r, s, note, c.round, c.slots)
		}
	}
}
