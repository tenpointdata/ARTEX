// Package propose is the Loop C critic (RSI.md rule 6): it reads what closed
// live tasks produced and drafts prompt-body edits expected to transfer.
//
// Live tasks propose, benchmarks adopt (rule 1): a draft here is only a
// candidate. It is bounded by the round's remaining budget, every draft passes
// rsi.ValidateEdit before it leaves this package, and nothing here can adopt,
// reject or change a live prompt. The package has no HTTP, database or model
// dependency; cmd/artex-rsi-propose is the adapter that feeds it and posts.
//
// A rule is a per-task signal plus a fixed, code-owned guidance paragraph. The
// critic keeps a rule only when its signal holds on several closed tasks that
// span several scopes (companies or task categories), so an edit that fits one
// target is pruned before it is ever proposed. Text taken from live tasks
// (lesson cues, skill names, finding titles) never enters a drafted body: live
// output is untrusted, so drafts cite task ids and counts only. An optional
// Drafter (drafter.go) may rewrite the body instead of appending; it is given
// the same code-owned guidance and counts, never task text, and its output
// passes the same checks.
package propose

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Autumn-27/artex/rsi"
)

// Outcome is the subset of GET /api/tasks/{id}/outcome the rules read.
type Outcome struct {
	Status   string `json:"status"`
	Findings struct {
		Total         int `json:"total"`
		Confirmed     int `json:"confirmed"`
		FalsePositive int `json:"false_positive"`
		Duplicate     int `json:"duplicate"`
	} `json:"findings"`
	Retests struct {
		Reproduced   int `json:"reproduced"`
		Fixed        int `json:"fixed"`
		Inconclusive int `json:"inconclusive"`
	} `json:"retests"`
	ToolCalls   int `json:"tool_calls"`
	SkillMisses int `json:"skill_misses"`
}

// Lesson is the subset of a task's lesson the rules read (no cue or action text).
type Lesson struct {
	ID      int64  `json:"id"`
	Kind    string `json:"kind"`
	Support int    `json:"support"`
}

// SkillGap is the subset of GET /api/skill-gaps the rules read.
type SkillGap struct {
	AgentKey   string `json:"agent_key"`
	CategoryID int64  `json:"category_id"`
	Misses     int    `json:"misses"`
}

// Task is one closed live task as the critic sees it.
type Task struct {
	ID         string
	CategoryID int64
	CompanyID  int64
	Outcome    Outcome
	Lessons    []Lesson
}

// Scope is what "a different target" means for transfer: the company the task
// ran for, else its category.
func (t Task) Scope() string {
	if t.CompanyID != 0 {
		return fmt.Sprintf("company:%d", t.CompanyID)
	}
	return fmt.Sprintf("category:%d", t.CategoryID)
}

// Rule is one critic rule: a per-task signal and the guidance it proposes.
type Rule struct {
	ID       string
	Role     string // one of rsi.PromptRoles; anything else is refused by ValidateEdit
	Gate     string // the benchmark gate the edit is expected to move
	Holds    func(Task) bool
	Guidance string // code-owned text appended to the role's body
	Why      string
}

// Prior is an existing proposal, from GET /api/rsi/proposals.
type Prior struct {
	Round     int    `json:"round"`
	Target    string `json:"target"`
	Body      string `json:"body"`
	Status    string `json:"status"`
	Rationale string `json:"rationale"`
}

// Config holds the transfer thresholds.
type Config struct {
	MinTasks  int     // distinct tasks the signal must hold on
	MinScopes int     // distinct scopes those tasks must span
	MinShare  float64 // share of closed tasks the signal must hold on
}

// DefaultConfig: at least 2 tasks across 2 scopes, and a quarter of the tasks.
var DefaultConfig = Config{MinTasks: 2, MinScopes: 2, MinShare: 0.25}

// Input is everything one planning pass reads.
type Input struct {
	Round   int
	Slots   int               // edits left in the round (budget minus used, capped by open slots)
	Tasks   []Task            // closed live tasks
	Bodies  map[string]string // role → active editable body
	Prior   []Prior
	Rules   []Rule
	Markers []string // code-owned tail markers (agent.CodeOwnedTailMarkers)
	Drafter Drafter  // optional: a model rewrites the body; nil appends the fixed guidance
	Judge   string   // the suite's judge model; a drafter must differ from it (CheckDrafter)
}

// Draft is one candidate edit, ready to write as an arm file and to post.
type Draft struct {
	Name         string   `json:"name"`
	Round        int      `json:"round"`
	Rule         string   `json:"rule"`
	Edit         rsi.Edit `json:"edit"`
	ProposedFrom []string `json:"proposed_from"`
	Rationale    string   `json:"rationale"`
	Support      int      `json:"support"`
	Of           int      `json:"of"`
	Scopes       int      `json:"scopes"`
	DraftedBy    string   `json:"drafted_by,omitempty"` // drafting model; empty = fixed guidance
}

// Skip records a rule the critic or the pruner dropped, and why.
type Skip struct {
	Rule   string `json:"rule"`
	Reason string `json:"reason"`
}

// Plan is the outcome of one pass: at most Slots drafts.
type Plan struct {
	Round   int     `json:"round"`
	Slots   int     `json:"slots"`
	Drafts  []Draft `json:"drafts"`
	Skipped []Skip  `json:"skipped"`
}

// GuidanceHeading opens every appended guidance paragraph.
const GuidanceHeading = "## Field guidance"

// Build runs the critic and the pruner and returns at most in.Slots drafts.
func Build(in Input, cfg Config) Plan {
	plan := Plan{Round: in.Round, Slots: max(in.Slots, 0), Drafts: []Draft{}, Skipped: []Skip{}}
	var cands []Draft
	rules := map[string]Rule{}
	for _, r := range in.Rules {
		rules[r.ID] = r
		d, why := critique(r, in, cfg)
		if why != "" {
			plan.Skipped = append(plan.Skipped, Skip{Rule: r.ID, Reason: why})
			continue
		}
		cands = append(cands, d)
	}
	// Strongest evidence first; ties by rule id so a run is reproducible.
	sort.SliceStable(cands, func(i, j int) bool {
		if cands[i].Support != cands[j].Support {
			return cands[i].Support > cands[j].Support
		}
		return cands[i].Rule < cands[j].Rule
	})
	for _, d := range cands {
		if len(plan.Drafts) >= plan.Slots {
			plan.Skipped = append(plan.Skipped, Skip{Rule: d.Rule, Reason: fmt.Sprintf("round %d budget exhausted", in.Round)})
			continue
		}
		if in.Drafter != nil { // drafted in rank order, so a refused body frees its slot
			var why string
			if d, why = redraft(d, rules[d.Rule], in); why != "" {
				plan.Skipped = append(plan.Skipped, Skip{Rule: d.Rule, Reason: why})
				continue
			}
		}
		plan.Drafts = append(plan.Drafts, d)
	}
	return plan
}

// critique returns the rule's draft, or the reason it does not qualify.
func critique(r Rule, in Input, cfg Config) (Draft, string) {
	if r.Holds == nil || strings.TrimSpace(r.Guidance) == "" {
		return Draft{}, "rule has no signal or no guidance"
	}
	var from []string
	scopes := map[string]bool{}
	for _, t := range in.Tasks {
		if r.Holds(t) {
			from = append(from, "task:"+t.ID)
			scopes[t.Scope()] = true
		}
	}
	n := len(in.Tasks)
	switch {
	case len(from) < cfg.MinTasks:
		return Draft{}, fmt.Sprintf("signal on %d of %d tasks, under %d", len(from), n, cfg.MinTasks)
	case len(scopes) < cfg.MinScopes:
		return Draft{}, fmt.Sprintf("signal on %d scope(s), under %d: may only fit one target", len(scopes), cfg.MinScopes)
	case float64(len(from)) < cfg.MinShare*float64(n):
		return Draft{}, fmt.Sprintf("signal on %d of %d tasks, under %.0f%%", len(from), n, cfg.MinShare*100)
	}
	cur := in.Bodies[r.Role]
	if strings.TrimSpace(cur) == "" {
		return Draft{}, fmt.Sprintf("role %q has no stored body to extend", r.Role)
	}
	guidance := strings.TrimSpace(r.Guidance)
	if strings.Contains(guidance, "{{") {
		return Draft{}, "guidance must not carry template actions"
	}
	if strings.Contains(cur, guidance) {
		return Draft{}, "the active body already carries this guidance"
	}
	for _, p := range in.Prior {
		if p.Target == r.Role && strings.Contains(p.Body, guidance) {
			return Draft{}, fmt.Sprintf("already proposed (status %s)", p.Status)
		}
	}
	edit := rsi.Edit{Surface: rsi.SurfacePromptBody, Target: r.Role,
		Body: strings.TrimRight(cur, "\n") + "\n\n" + GuidanceHeading + "\n" + guidance + "\n"}
	if err := rsi.ValidateEdit(edit, in.Markers); err != nil {
		return Draft{}, "refused by rsi.ValidateEdit: " + err.Error()
	}
	return Draft{
		Name: fmt.Sprintf("r%d-%s", in.Round, r.ID), Round: in.Round, Rule: r.ID, Edit: edit,
		ProposedFrom: from, Support: len(from), Of: n, Scopes: len(scopes),
		Rationale: fmt.Sprintf("%s: %s Signal held on %d of %d closed tasks across %d scopes (%s). Expected to move the %s gate; the guidance is generic, not tied to any target.",
			r.Role, r.Why, len(from), n, len(scopes), strings.Join(from, ", "), r.Gate),
	}, ""
}
