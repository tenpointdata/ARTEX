package agent

import (
	"testing"
)

// A decomposition re-run after a restart must not duplicate the goals the
// interrupted run already wrote, while other writers (the main agent) keep
// their plain add semantics.
func TestDecomposerSetGoalsSkipsRecordedGoals(t *testing.T) {
	store := guardTaskStore(t)
	first := NewToolSet(store, goalsDecomposerOrigin)
	firstID, err := first.addOneGoal(goalItem{Text: "Obtain an admin session", VulnClass: "auth"})
	if err != nil {
		t.Fatal(err)
	}

	rerun := NewToolSet(store, goalsDecomposerOrigin)
	callGuardTool(t, rerun.setGoals(), `{"goals":[{"text":"  obtain an ADMIN session "},{"text":"Read the flag file"}]}`)
	texts := recordedGoalTexts(store)
	if len(texts) != 2 {
		t.Fatalf("goals after re-decomposition = %q, want the recorded one plus the missing one", texts)
	}
	if id, ok := rerun.existingGoalID("Obtain an admin session"); !ok || id != firstID {
		t.Fatalf("existingGoalID = %d,%v, want %d", id, ok, firstID)
	}

	main := NewToolSet(store, mainAgentOrigin)
	if _, err := main.addOneGoal(goalItem{Text: "Read the flag file"}); err != nil {
		t.Fatal(err)
	}
	if n := len(recordedGoalTexts(store)); n != 3 {
		t.Fatalf("main-agent goal count = %d, want 3 (no dedupe outside the decomposer)", n)
	}
}
