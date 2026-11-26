package propose

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/Autumn-27/artex/rsi"
)

// Drafter is an optional model that rewrites a role's body for a rule that
// already passed the transfer filter (Input.Drafter; nil keeps the fixed,
// appended guidance). It only proposes text: whatever it returns goes through
// the same checks as a code-written draft (checkDrafted, rsi.ValidateEdit here
// and again in WriteArm), and a refused body is skipped, never repaired.
type Drafter interface {
	// Model names the drafting model; it must differ from the suite's judge.
	Model() string
	// Draft returns the full new body for req.Role.
	Draft(req DraftRequest) (string, error)
}

// DraftRequest is everything a drafter is given. It holds code-owned text (the
// rule's why, gate and guidance), the role's active body (owner-adopted, not
// live task output) and counts. No task id, lesson cue, finding title or other
// live task text is in it, so live output cannot steer the drafted body.
type DraftRequest struct {
	Rule     string
	Role     string
	Gate     string
	Why      string
	Guidance string
	Body     string // the active editable body to rewrite
	Support  int    // tasks the signal held on
	Of       int    // closed tasks read
	Scopes   int    // distinct scopes those tasks span
}

// ErrDrafterIsJudge: the drafting model is (or may be) the suite's judge.
var ErrDrafterIsJudge = errors.New("the drafting model must not be the benchmark judge")

// CheckDrafter keeps the suite's judge/subject independence: the model that
// writes a candidate body must not be the model that grades it. An unknown
// judge cannot be checked, so it is refused too.
func CheckDrafter(d Drafter, judge string) error {
	if d == nil {
		return nil
	}
	m := modelKey(d.Model())
	if m == "" {
		return errors.New("the drafter names no model")
	}
	j := modelKey(judge)
	if j == "" {
		return fmt.Errorf("%w: no judge model given, so independence cannot be checked", ErrDrafterIsJudge)
	}
	if m == j {
		return fmt.Errorf("%w: both are %q", ErrDrafterIsJudge, judge)
	}
	return nil
}

// modelKey folds a model name for comparison: case, surrounding space and a
// provider prefix ("openrouter/deepseek-chat" is "deepseek-chat").
func modelKey(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if i := strings.LastIndex(s, "/"); i >= 0 {
		s = s[i+1:]
	}
	return s
}

var templateAction = regexp.MustCompile(`\{\{.*?\}\}`)

// actions lists the distinct template actions in a body, in order.
func actions(body string) []string {
	seen := map[string]bool{}
	var out []string
	for _, a := range templateAction.FindAllString(body, -1) {
		a = strings.Join(strings.Fields(a), " ")
		if !seen[a] {
			seen[a] = true
			out = append(out, a)
		}
	}
	sort.Strings(out)
	return out
}

// checkDrafted refuses a model-written body that the fixed path could never
// produce: one that adds, drops or alters a template action (the code binds
// those, so a new one could pull task data into the system prompt), carries a
// stray delimiter, or is the active body unchanged. rsi.ValidateEdit (markers,
// role, size) runs after this, as for every draft.
func checkDrafted(cur, body string) string {
	if strings.TrimSpace(body) == "" {
		return "drafter returned an empty body"
	}
	have, want := actions(body), actions(cur)
	if strings.Join(have, "\x00") != strings.Join(want, "\x00") {
		return fmt.Sprintf("drafted body changes the template actions (has %v, the active body has %v)", have, want)
	}
	rest := templateAction.ReplaceAllString(body, "")
	if strings.Contains(rest, "{{") || strings.Contains(rest, "}}") {
		return "drafted body carries a stray template delimiter"
	}
	if strings.TrimSpace(body) == strings.TrimSpace(cur) {
		return "drafter returned the active body unchanged"
	}
	return ""
}

// redraft replaces d's appended body with the drafter's rewrite of the same
// role's active body. d already passed the transfer filter and rsi.ValidateEdit,
// so only a role inside the search space ever reaches the drafter. why is
// non-empty when the rewrite is refused; the caller then drops the rule.
func redraft(d Draft, r Rule, in Input) (Draft, string) {
	if err := CheckDrafter(in.Drafter, in.Judge); err != nil {
		return d, err.Error()
	}
	tag := ruleTag(r.ID)
	for _, p := range in.Prior {
		if p.Target == r.Role && strings.Contains(p.Rationale, tag) {
			return d, fmt.Sprintf("a drafted body for this rule was already proposed (status %s)", p.Status)
		}
	}
	cur := in.Bodies[r.Role]
	out, err := in.Drafter.Draft(DraftRequest{Rule: r.ID, Role: r.Role, Gate: r.Gate, Why: r.Why,
		Guidance: strings.TrimSpace(r.Guidance), Body: cur, Support: d.Support, Of: d.Of, Scopes: d.Scopes})
	if err != nil {
		return d, "drafter failed: " + err.Error()
	}
	if why := checkDrafted(cur, out); why != "" {
		return d, why
	}
	body := strings.TrimRight(out, "\n") + "\n"
	for _, p := range in.Prior {
		if p.Target == r.Role && strings.TrimSpace(p.Body) == strings.TrimSpace(body) {
			return d, fmt.Sprintf("drafted body already proposed (status %s)", p.Status)
		}
	}
	edit := d.Edit
	edit.Body = body
	if err := rsi.ValidateEdit(edit, in.Markers); err != nil {
		return d, "drafted body refused by rsi.ValidateEdit: " + err.Error()
	}
	d.Edit, d.DraftedBy = edit, in.Drafter.Model()
	d.Rationale = strings.TrimSuffix(d.Rationale, "the guidance is generic, not tied to any target.") +
		fmt.Sprintf("the body was rewritten by %s from the code-owned guidance, given counts only, no task text %s.", d.DraftedBy, tag)
	return d, ""
}

// ruleTag marks a drafted proposal's rationale with its rule, so the pruner
// can tell a later round that the rule was already drafted: a rewrite need not
// contain the guidance text the fixed path's pruner looks for.
func ruleTag(id string) string { return "[drafted rule " + id + "]" }
