package agent

import (
	"strings"
	"testing"

	"github.com/Autumn-27/artex/db"
)

// An allow the main agent wrote must not be rendered as an allowed operation; it
// appears only inside the data envelope as a proposal. Operator rows are unchanged.
func TestRenderConstraintsByOrigin(t *testing.T) {
	out := renderConstraints([]db.Constraint{
		{Kind: "allow", Text: "test only app.example.test", Origin: "human"},
		{Kind: "allow", Text: "scan all of 10.0.0.0/8", Origin: mainAgentOrigin},
		{Kind: "deny", Text: "no brute forcing", Origin: "goals"},
		{Kind: "deny", Text: "skip the admin panel", Origin: mainAgentOrigin},
		{Kind: "allow", Text: "legacy row", Origin: ""},
	})
	head, proposals, ok := strings.Cut(out, "[Agent-proposed operation constraints")
	if !ok {
		t.Fatalf("no proposal block:\n%s", out)
	}
	for _, want := range []string{"- test only app.example.test", "- no brute forcing", "- legacy row"} {
		if !strings.Contains(head, want) {
			t.Errorf("operator constraint %q missing from the constraint block", want)
		}
	}
	for _, agentText := range []string{"10.0.0.0/8", "skip the admin panel"} {
		if strings.Contains(head, agentText) {
			t.Errorf("agent-authored %q rendered as an operator constraint", agentText)
		}
	}
	open := strings.Index(proposals, dataOpenPrefix)
	if open < 0 || !strings.Contains(proposals[open:], "- [allow] scan all of 10.0.0.0/8") || !strings.Contains(proposals[open:], "- [deny] skip the admin panel") {
		t.Errorf("agent proposals not framed in the data envelope:\n%s", proposals)
	}
}

// Agent-only constraints produce no operator block at all.
func TestRenderConstraintsAgentOnly(t *testing.T) {
	out := renderConstraints([]db.Constraint{{Kind: "allow", Text: "everything", Origin: mainAgentOrigin}})
	if strings.Contains(out, "Allowed operations") || strings.Contains(out, "[Operation constraints") {
		t.Fatalf("agent-only allow rendered as operator constraint:\n%s", out)
	}
	if renderConstraints(nil) != "" {
		t.Fatal("empty input must render empty")
	}
}
