package rsi

import (
	"encoding/json"
	"fmt"
	"reflect"
)

// The evidence types mirror the per-arm files written by
// scripts/benchmark/eval_suite.py ({"stamp", "records"}), so a suite's output
// is uploaded as is and the gate recomputes every number from the raw records
// instead of trusting a comparison the loop computed itself.

// InvariantStamps must be identical between the arms (same model, seed,
// budget, judge, method and ground truth); suite_stats.INVARIANT_STAMPS.
var InvariantStamps = []string{"method_version", "subject_model", "llm_profiles", "judge_model",
	"task_timeout_seconds", "seed", "suite", "suite_version", "replicates", "ground_truth_versions"}

// Stamp is one arm's stamp: the typed fields the gate reads plus every raw
// field, so invariants are compared on exactly what the harness wrote.
type Stamp struct {
	Role         string            `json:"role"`
	Arm          string            `json:"arm"`
	Split        string            `json:"split"`
	Suite        string            `json:"suite"`
	ProposedFrom []string          `json:"proposed_from"`
	PromptSHA    map[string]string `json:"prompt_sha"`
	LessonIDs    []int64           `json:"lesson_ids"`
	Raw          map[string]any    `json:"-"`
}

// UnmarshalJSON keeps the raw stamp next to the typed fields.
func (s *Stamp) UnmarshalJSON(b []byte) error {
	type plain Stamp
	var p plain
	if err := json.Unmarshal(b, &p); err != nil {
		return err
	}
	var raw map[string]any
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	*s = Stamp(p)
	s.Raw = raw
	return nil
}

// MarshalJSON writes the raw stamp back (the typed fields are a view of it).
func (s Stamp) MarshalJSON() ([]byte, error) {
	if s.Raw != nil {
		return json.Marshal(s.Raw)
	}
	type plain Stamp
	return json.Marshal(plain(s))
}

// JudgeVerdict is one ground-truth entry's grading in a run.
type JudgeVerdict struct {
	GTID     string          `json:"gt_id"`
	Detected bool            `json:"detected"`
	Gates    map[string]bool `json:"gates"`
}

// Payload is the part of run_bench's results.json the gate reads.
type Payload struct {
	TaskStatus string          `json:"task_status"`
	Verdicts   *[]JudgeVerdict `json:"verdicts"` // nil = unjudged (flag-only)
	Result     struct {
		FlagCaptured *bool `json:"flag_captured"`
	} `json:"result"`
	Task struct {
		ID int64 `json:"id"`
	} `json:"task"`
}

// Record is one (target, arm, replicate) run; Payload is nil when it failed.
type Record struct {
	Target    string   `json:"target"`
	Arm       string   `json:"arm"`
	Replicate int      `json:"replicate"`
	RunID     string   `json:"run_id"`
	Payload   *Payload `json:"payload"`
	Error     *string  `json:"error"`
}

// ArmResult is one arm's uploaded file.
type ArmResult struct {
	Stamp   Stamp    `json:"stamp"`
	Records []Record `json:"records"`
}

// Gates are the observation kinds counted per ground-truth entry, plus the
// objective flag oracle (suite_stats.GATES + FLAG_GATE).
var Gates = []string{"detected", "component", "class", "keywords", "proof", "flag_captured"}

// observations returns {"<gt>/<gate>": passed} for one run, {} when it failed.
func (r Record) observations() map[string]bool {
	out := map[string]bool{}
	if r.Payload == nil {
		return out
	}
	if r.Payload.Verdicts != nil {
		for _, v := range *r.Payload.Verdicts {
			out[v.GTID+"/detected"] = v.Detected
			for _, g := range Gates[1:5] {
				out[v.GTID+"/"+g] = v.Gates[g]
			}
		}
	}
	if f := r.Payload.Result.FlagCaptured; f != nil {
		out["flag/flag_captured"] = *f
	}
	return out
}

func stampMismatches(a, b Stamp) []string {
	var out []string
	for _, k := range InvariantStamps {
		if !reflect.DeepEqual(a.Raw[k], b.Raw[k]) {
			out = append(out, fmt.Sprintf("stamp %s differs: baseline=%v candidate=%v", k, a.Raw[k], b.Raw[k]))
		}
	}
	return out
}
