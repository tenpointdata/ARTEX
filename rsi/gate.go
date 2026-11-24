package rsi

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
)

// Policy is the adoption bar a comparison must clear.
type Policy struct {
	RequireSplit        string  // the suite split adoption evidence must come from
	MinJudgedRunsPerArm int     // runs with a payload, per arm
	MaxFailedFraction   float64 // failed runs / attempted runs, per arm
	// MinTargetsImproved is the pruner (RSI.md rule 6): the paired gain must
	// be spread over at least this many distinct held-out targets, so an edit
	// that only fits one task is rejected.
	MinTargetsImproved int
}

// DefaultPolicy is the bar the server applies.
var DefaultPolicy = Policy{RequireSplit: "held-out", MinJudgedRunsPerArm: 2, MaxFailedFraction: 0.25, MinTargetsImproved: 2}

// Rate is passes over a denominator; never quoted without n.
type Rate struct {
	Passes int `json:"passes"`
	N      int `json:"n"`
}

// GateRow is one gate's unpaired rates and paired discordance.
type GateRow struct {
	Gate      string `json:"gate"`
	Baseline  Rate   `json:"baseline"`
	Candidate Rate   `json:"candidate"`
	Both      int    `json:"both"`
	Neither   int    `json:"neither"`
	Gained    int    `json:"gained"`
	Lost      int    `json:"lost"`
}

// Regression is a gate the candidate lost on net, attributed to the edited
// module (RSI.md rule 7) and to the targets where the loss happened.
type Regression struct {
	Module  string   `json:"module"`
	Gate    string   `json:"gate"`
	Gained  int      `json:"gained"`
	Lost    int      `json:"lost"`
	Targets []string `json:"targets"`
}

// Verdict is the gate's deterministic result over one comparison.
type Verdict struct {
	Passed          bool         `json:"passed"`
	Module          string       `json:"module"`
	Reasons         []string     `json:"reasons"`
	Gates           []GateRow    `json:"gates"`
	Regressions     []Regression `json:"regressions"`
	TargetsImproved []string     `json:"targets_improved"`
	Targets         []string     `json:"targets"`
	TaskIDs         []int64      `json:"task_ids"`
	Runs            [2][2]int    `json:"runs"` // [baseline, candidate] x [attempted, failed]
}

// Binding ties a comparison to one edit: the candidate arm must have run
// exactly that edit and nothing else, the baseline nothing evolved.
type Binding struct {
	Edit         Edit
	ProposedFrom []string
}

type pairKey struct {
	target    string
	replicate int
	obs       string
}

// Evaluate recomputes the comparison from the raw records and applies the
// policy. Every failed check adds a reason; Passed means there are none.
func Evaluate(base, cand ArmResult, b Binding, p Policy) Verdict {
	edit := b.Edit.Normalize()
	v := Verdict{Module: edit.Module()}
	fail := func(format string, args ...any) { v.Reasons = append(v.Reasons, fmt.Sprintf(format, args...)) }

	if base.Stamp.Role != "baseline" || cand.Stamp.Role != "candidate" {
		fail("arm roles are %q/%q, want baseline/candidate", base.Stamp.Role, cand.Stamp.Role)
	}
	for _, k := range InvariantStamps {
		_, okB := base.Stamp.Raw[k]
		_, okC := cand.Stamp.Raw[k]
		if !okB || !okC {
			fail("stamp %s is missing", k)
		}
	}
	for _, m := range stampMismatches(base.Stamp, cand.Stamp) {
		fail("%s", m)
	}
	for _, s := range []Stamp{base.Stamp, cand.Stamp} {
		if s.Split != p.RequireSplit {
			fail("%s arm ran on split %q; adoption needs %q", s.Role, s.Split, p.RequireSplit)
		}
	}
	checkBinding(base.Stamp, cand.Stamp, edit, fail)

	targets := map[string]bool{}
	tasks := map[int64]bool{}
	for i, arm := range []ArmResult{base, cand} {
		judged := 0
		for _, r := range arm.Records {
			targets[r.Target] = true
			v.Runs[i][0]++
			if r.Payload == nil {
				v.Runs[i][1]++
				continue
			}
			if len(r.observations()) > 0 {
				judged++
			}
			id := int64(r.Payload.Task.ID)
			if id <= 0 {
				fail("run %s has no task id", r.RunID)
			} else if tasks[id] {
				fail("task %d appears in more than one run", id)
			} else {
				tasks[id] = true
				v.TaskIDs = append(v.TaskIDs, id)
			}
		}
		role := [2]string{"baseline", "candidate"}[i]
		if judged < p.MinJudgedRunsPerArm {
			fail("%s has %d completed runs with observations, need %d", role, judged, p.MinJudgedRunsPerArm)
		}
		if att := v.Runs[i][0]; att > 0 && float64(v.Runs[i][1])/float64(att) > p.MaxFailedFraction {
			fail("%s failed %d of %d runs, over the %.0f%% limit", role, v.Runs[i][1], att, p.MaxFailedFraction*100)
		}
	}
	for t := range targets {
		v.Targets = append(v.Targets, t)
	}
	sort.Strings(v.Targets)
	sort.Slice(v.TaskIDs, func(i, j int) bool { return v.TaskIDs[i] < v.TaskIDs[j] })
	proposed := append(append([]string{}, b.ProposedFrom...), cand.Stamp.ProposedFrom...)
	for _, t := range proposed {
		if targets[t] {
			fail("target %s was used to propose the edit and is in the evaluation set (RSI.md rule 1)", t)
		}
	}

	paired(&v, base, cand, fail, p)
	v.Passed = len(v.Reasons) == 0
	return v
}

// checkBinding proves the candidate ran exactly the edit and the baseline ran
// nothing evolved; together that is the paired evolving/non-evolving rule.
func checkBinding(base, cand Stamp, edit Edit, fail func(string, ...any)) {
	if len(base.LessonIDs) > 0 {
		fail("baseline arm injected lessons %v; the baseline must not evolve", base.LessonIDs)
	}
	for _, role := range PromptRoles {
		bs, cs := base.PromptSHA[role], cand.PromptSHA[role]
		if bs == "" || cs == "" {
			fail("prompt_sha for %s is missing from a stamp", role)
			continue
		}
		if edit.Surface == SurfacePromptBody && role == edit.Target {
			if cs != SHA16(edit.Body) {
				fail("candidate ran %s body %s, not the proposed body %s", role, cs, SHA16(edit.Body))
			}
			if bs == cs {
				fail("baseline and candidate ran the same %s body", role)
			}
		} else if bs != cs {
			fail("%s body differs between arms (%s vs %s); one proposal changes one module", role, bs, cs)
		}
	}
	switch edit.Surface {
	case SurfacePromptBody:
		if len(cand.LessonIDs) > 0 {
			fail("candidate arm injected lessons %v; a prompt proposal changes only its body", cand.LessonIDs)
		}
	case SurfaceLesson:
		if !reflect.DeepEqual(Edit{LessonIDs: cand.LessonIDs}.Normalize().LessonIDs, edit.LessonIDs) {
			fail("candidate injected lessons %v, not the proposed %v", cand.LessonIDs, edit.LessonIDs)
		}
	}
}

// failedRuns are the (target, replicate) runs of an arm that produced no payload.
func failedRuns(arm ArmResult) map[[2]string]bool {
	out := map[[2]string]bool{}
	for _, r := range arm.Records {
		if r.Payload == nil {
			out[[2]string{r.Target, fmt.Sprint(r.Replicate)}] = true
		}
	}
	return out
}

func index(arm ArmResult) map[pairKey]bool {
	out := map[pairKey]bool{}
	for _, r := range arm.Records {
		for k, passed := range r.observations() {
			out[pairKey{r.Target, r.Replicate, k}] = passed
		}
	}
	return out
}

func gateOf(obs string) string { return obs[strings.LastIndex(obs, "/")+1:] }

// paired fills the gate table, attributes regressions and applies the pruner.
func paired(v *Verdict, base, cand ArmResult, fail func(string, ...any), p Policy) {
	bi, ci := index(base), index(cand)
	rows := map[string]*GateRow{}
	for _, g := range Gates {
		rows[g] = &GateRow{Gate: g}
	}
	for k, passed := range bi {
		r := rows[gateOf(k.obs)]
		r.Baseline.N++
		if passed {
			r.Baseline.Passes++
		}
	}
	for k, passed := range ci {
		r := rows[gateOf(k.obs)]
		r.Candidate.N++
		if passed {
			r.Candidate.Passes++
		}
	}
	net := map[string]int{}                // per target, over all gates
	lostAt := map[string]map[string]bool{} // gate -> targets with a lost pair
	pairs := 0
	candFailed := failedRuns(cand)
	for k, bp := range bi {
		cp, ok := ci[k]
		if !ok {
			// A candidate run that crashed where the baseline ran is not
			// dropped: what the baseline passed there counts as lost.
			if !candFailed[[2]string{k.target, fmt.Sprint(k.replicate)}] {
				continue
			}
			cp = false
		}
		pairs++
		g := gateOf(k.obs)
		r := rows[g]
		switch {
		case bp && cp:
			r.Both++
		case !bp && !cp:
			r.Neither++
		case cp:
			r.Gained++
			net[k.target]++
		default:
			r.Lost++
			net[k.target]--
			if lostAt[g] == nil {
				lostAt[g] = map[string]bool{}
			}
			lostAt[g][k.target] = true
		}
	}
	improved := false
	for _, g := range Gates {
		r := rows[g]
		v.Gates = append(v.Gates, *r)
		if r.Gained > r.Lost {
			improved = true
		}
		if r.Lost > r.Gained {
			reg := Regression{Module: v.Module, Gate: g, Gained: r.Gained, Lost: r.Lost}
			for t := range lostAt[g] {
				reg.Targets = append(reg.Targets, t)
			}
			sort.Strings(reg.Targets)
			v.Regressions = append(v.Regressions, reg)
			fail("regression on gate %s attributed to %s: lost %d, gained %d (targets %v)",
				g, v.Module, r.Lost, r.Gained, reg.Targets)
		}
	}
	for t, n := range net {
		if n > 0 {
			v.TargetsImproved = append(v.TargetsImproved, t)
		}
	}
	sort.Strings(v.TargetsImproved)
	if pairs == 0 {
		fail("no paired observations: both arms must be judged on the same targets and replicates")
		return
	}
	if !improved {
		fail("no gate improved on paired observations")
	}
	if len(v.TargetsImproved) < p.MinTargetsImproved {
		fail("gain on %d target(s) %v; the pruner needs at least %d distinct held-out targets",
			len(v.TargetsImproved), v.TargetsImproved, p.MinTargetsImproved)
	}
}
