package db

import (
	"encoding/json"
	"strings"

	"github.com/Autumn-27/artex/methodology"
)

// Methodology coverage: the task's checklist completion against the testing
// methodologies (methodology package) that apply to it. The operator may choose
// them (tasks.methodologies, set at creation and editable with PATCH, including
// methodology.None to turn checklists off); otherwise they are derived from the
// task's goal and description (methodology.Select). The score reads three existing sources:
//   - coverage_ledger rows (agent_reported), matched by risk_area;
//   - findings (machine_observed), matched by vulnclass, as "reported";
//   - intents whose payload names a methodology_item (planner citations).
// Nothing is derived from absence: an item no source matches stays open.

// IntentMethodologyItemKey is the intent payload key of the methodology item an
// intent advances (agent add_intent writes it; MethodologyCoverage reads it).
const IntentMethodologyItemKey = "methodology_item"

// CoverageEntryCloses reports whether a ledger entry settles its risk area for
// checklist purposes. It mirrors BuildCoverageLedger's gaps: a follow-up never
// closes, and an entry missing its required evidence or reason does not either.
func CoverageEntryCloses(outcome, evidence, reason string) bool {
	switch outcome {
	case OutcomeReported:
		return true
	case OutcomeNoIssueFound:
		return strings.TrimSpace(evidence) != ""
	case OutcomeRuledOut, OutcomeNotApplicable:
		return strings.TrimSpace(reason) != ""
	}
	return false
}

// TaskMethodologies returns the built-in methodologies that apply to a task:
// the operator's choice (tasks.methodologies) when set, none for a [none]
// choice, else those its goal and description select (methodology.Choose).
func (s *AssetStore) TaskMethodologies(taskID int64) ([]methodology.Methodology, error) {
	var goal, desc, chosenJSON string
	if err := s.db.QueryRow(`SELECT COALESCE(goal,''), COALESCE(description,''),
COALESCE(array_to_json(methodologies)::text,'[]') FROM tasks WHERE id=$1`, taskID).Scan(&goal, &desc, &chosenJSON); err != nil {
		return nil, err
	}
	var chosen []string
	if err := json.Unmarshal([]byte(chosenJSON), &chosen); err != nil {
		return nil, err
	}
	all, err := methodology.All()
	if err != nil {
		return nil, err
	}
	return methodology.Choose(all, chosen, goal+"\n"+desc), nil
}

// MethodologyCoverage scores each applicable methodology for a task. It returns
// an empty slice (not nil) when none applies, so the API always carries the field.
func (s *AssetStore) MethodologyCoverage(taskID int64) ([]methodology.Score, error) {
	ms, err := s.TaskMethodologies(taskID)
	if err != nil || len(ms) == 0 {
		return []methodology.Score{}, err
	}
	obs, err := s.methodologyObservations(taskID)
	if err != nil {
		return nil, err
	}
	cites, err := s.methodologyCitations(taskID)
	if err != nil {
		return nil, err
	}
	out := make([]methodology.Score, 0, len(ms))
	for _, m := range ms {
		out = append(out, methodology.Evaluate(m, obs, cites))
	}
	return out, nil
}

func (s *AssetStore) methodologyObservations(taskID int64) ([]methodology.Observation, error) {
	var obs []methodology.Observation
	rows, err := s.db.Query(`SELECT risk_area, outcome, evidence, reason FROM coverage_ledger WHERE task_id=$1`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var ra, outcome, evidence, reason string
		if err := rows.Scan(&ra, &outcome, &evidence, &reason); err != nil {
			return nil, err
		}
		obs = append(obs, methodology.Observation{RiskArea: ra, Outcome: outcome, Closes: CoverageEntryCloses(outcome, evidence, reason)})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// A finding triaged as false positive, duplicate or ignored proves nothing.
	var classes []string
	if err := s.scanStrings(&classes, `SELECT DISTINCT btrim(vulnclass) FROM findings
WHERE task_id=$1 AND btrim(vulnclass)<>'' AND COALESCE(status,'') NOT IN ($2,$3,$4)`, taskID, FindingFalsePositive, FindingDuplicate, FindingIgnored); err != nil {
		return nil, err
	}
	for _, c := range classes {
		obs = append(obs, methodology.Observation{RiskArea: c, Outcome: OutcomeReported, Closes: true})
	}
	return obs, nil
}

func (s *AssetStore) methodologyCitations(taskID int64) ([]methodology.Citation, error) {
	rows, err := s.db.Query(`SELECT n.id, btrim(n.payload->>'`+IntentMethodologyItemKey+`') FROM exploration_nodes n
JOIN tasks t ON t.exploration_id=n.exploration_id
WHERE t.id=$1 AND n.kind=$2 AND n.state<>$3 AND COALESCE(btrim(n.payload->>'`+IntentMethodologyItemKey+`'),'')<>''`,
		taskID, KindIntent, StateIntentDeleted)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []methodology.Citation
	for rows.Next() {
		var c methodology.Citation
		if err := rows.Scan(&c.IntentID, &c.ItemID); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
