package propose

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/Autumn-27/artex/rsi"
)

// Arm is the candidate arm file scripts/benchmark/arms.py loads: one role's
// body from a template file next to it, every other role "current". The extra
// fields (round, rule, rationale, body_sha) are ignored by arms.py and kept for
// the owner's review and for rsi_submit.py.
type Arm struct {
	Name         string                       `json:"name"`
	ProposedFrom []string                     `json:"proposed_from"`
	Prompts      map[string]map[string]string `json:"prompts"`
	Round        int                          `json:"round"`
	Rule         string                       `json:"rule"`
	Target       string                       `json:"target"`
	BodySHA      string                       `json:"body_sha"`
	Rationale    string                       `json:"rationale"`
	DraftedBy    string                       `json:"drafted_by,omitempty"`
}

var safeName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,80}$`)

// WriteArm validates the draft again (a forged or edited draft is refused
// here too) and writes <name>.json and <name>.tmpl into dir.
func WriteArm(dir string, d Draft, markers []string) (string, error) {
	if err := rsi.ValidateEdit(d.Edit, markers); err != nil {
		return "", err
	}
	if d.Edit.Surface != rsi.SurfacePromptBody {
		return "", fmt.Errorf("%w: the proposer drafts prompt bodies only", rsi.ErrOutsideSearchSpace)
	}
	if !safeName.MatchString(d.Name) {
		return "", fmt.Errorf("draft name %q is not a safe file name", d.Name)
	}
	if len(d.ProposedFrom) == 0 {
		return "", fmt.Errorf("draft %s has no proposed_from tasks", d.Name)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	tmpl := d.Name + ".tmpl"
	if err := os.WriteFile(filepath.Join(dir, tmpl), []byte(d.Edit.Body), 0o644); err != nil {
		return "", err
	}
	arm := Arm{Name: d.Name, ProposedFrom: d.ProposedFrom, Round: d.Round, Rule: d.Rule, Target: d.Edit.Target,
		BodySHA: rsi.SHA16(d.Edit.Body), Rationale: d.Rationale, DraftedBy: d.DraftedBy,
		Prompts: map[string]map[string]string{d.Edit.Target: {"source": "file", "path": tmpl}}}
	raw, err := json.MarshalIndent(arm, "", "  ")
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, d.Name+".json")
	return path, os.WriteFile(path, append(raw, '\n'), 0o644)
}
