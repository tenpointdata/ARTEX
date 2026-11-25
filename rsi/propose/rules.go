package propose

// ARTEX's default critic rules. Each is a per-task signal over the outcome
// card and the task's lessons, and a fixed guidance paragraph written here,
// never copied from a task. Tune by editing this table, not the critic.

func precisionLow(t Task) bool {
	f := t.Outcome.Findings
	return f.Total >= 2 && float64(f.FalsePositive+f.Duplicate) >= 0.4*float64(f.Total)
}

func retestsInconclusive(t Task) bool {
	r := t.Outcome.Retests
	total := r.Reproduced + r.Fixed + r.Inconclusive
	return total >= 1 && float64(r.Inconclusive) >= 0.5*float64(total)
}

func zeroYield(t Task) bool {
	return t.Outcome.Findings.Confirmed == 0 && t.Outcome.ToolCalls >= 20
}

func failedEmpty(t Task) bool {
	return t.Outcome.Status == "failed" && t.Outcome.Findings.Confirmed == 0
}

func recurringCaution(t Task) bool {
	for _, l := range t.Lessons {
		if (l.Kind == "negative" || l.Kind == "caution") && l.Support >= 2 {
			return true
		}
	}
	return false
}

func skillMissed(t Task) bool { return t.Outcome.SkillMisses >= 2 }

// DefaultRules returns the rule table. gaps decides which roles get the
// skill-miss rule: a role qualifies when its misses span at least two task
// categories, so the gap is a platform pattern rather than one target's quirk.
func DefaultRules(gaps []SkillGap) []Rule {
	rules := []Rule{
		{ID: "worker-precision", Role: "worker", Gate: "precision (false positives, duplicates)", Holds: precisionLow,
			Why: "many reported findings were later marked false positive or duplicate.",
			Guidance: "Before reporting a finding, reproduce it once more from a clean request and confirm the response proves the impact. " +
				"If the same weakness on the same endpoint and parameter was already reported in this task, extend that finding instead of filing a new one."},
		{ID: "worker-reproducible", Role: "worker", Gate: "retest reproduced", Holds: retestsInconclusive,
			Why: "retests of this task's findings were mostly inconclusive.",
			Guidance: "Record every finding so another run can replay it: the exact request, the preconditions (session, state, timing), " +
				"and the observable that proves it. Prefer a minimal request that still shows the effect."},
		{ID: "planner-breadth", Role: "planner", Gate: "detected (recall)", Holds: zeroYield,
			Why: "many tool calls produced no confirmed finding.",
			Guidance: "When several intents against one surface yield nothing, stop deepening it: map the remaining attack surface, " +
				"then switch to a different class of weakness or a different entry point before spending more effort."},
		{ID: "planner-avoid-repeats", Role: "planner", Gate: "cost per confirmed finding", Holds: recurringCaution,
			Why: "tasks kept producing cautionary lessons that had already been seen on other tasks.",
			Guidance: "Before planning an approach, check whether it already failed against similar assets in this task; " +
				"if it did, change the technique or the target instead of retrying it unchanged."},
		{ID: "goals-verifiable", Role: "goals", Gate: "task completion", Holds: failedEmpty,
			Why: "tasks failed without a confirmed finding.",
			Guidance: "State each goal as an outcome that can be checked from captured evidence, with a clear stop condition, " +
				"so the planner can tell progress from activity."},
	}
	cats := map[string]map[int64]bool{}
	for _, g := range gaps {
		if g.Misses <= 0 {
			continue
		}
		if cats[g.AgentKey] == nil {
			cats[g.AgentKey] = map[int64]bool{}
		}
		cats[g.AgentKey][g.CategoryID] = true
	}
	for _, role := range []string{"goals", "planner", "worker"} {
		if len(cats[role]) < 2 {
			continue
		}
		rules = append(rules, Rule{ID: role + "-skill-gaps", Role: role, Gate: "tool errors and cost", Holds: skillMissed,
			Why: "the role kept asking for skills the platform does not provide, across several task categories.",
			Guidance: "Request only skills that are listed as available to you. When none fits, carry on with the built-in tools " +
				"and say in the result which capability was missing."})
	}
	return rules
}
