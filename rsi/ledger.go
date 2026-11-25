package rsi

import (
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"
)

// The checks here tie uploaded arm files to what the server itself recorded
// (card c-0d277f): a suite run registered before it ran, and a per-task arm
// ledger written when each task's agents were built. Evaluate proves the arm
// files are internally consistent with the edit; these prove the files
// describe runs this instance actually made, as registered.

// TaskArm is the server's ledger row for one task: what the task ran.
type TaskArm struct {
	TaskID          int64             `json:"task_id"`
	TaskName        string            `json:"task_name"`
	PromptSHA       map[string]string `json:"prompt_sha"`
	LessonsMode     string            `json:"lessons_mode"`
	LessonIDs       []int64           `json:"lesson_ids"`
	LessonsBlockSHA string            `json:"lessons_block_sha"`
	Drifted         bool              `json:"drifted"`
	DriftReason     string            `json:"drift_reason,omitempty"`
}

// SuiteRun is a suite run registered for a proposal before it ran.
type SuiteRun struct {
	ID           int64    `json:"id"`
	ProposalID   int64    `json:"proposal_id"`
	SuiteRun     string   `json:"suite_run"`
	Suite        string   `json:"suite"`
	SuiteVersion string   `json:"suite_version"`
	Split        string   `json:"split"`
	Targets      []string `json:"targets"`
	Seed         int64    `json:"seed"`
	Replicates   int      `json:"replicates"`
	EvidenceID   *int64   `json:"evidence_id,omitempty"`
}

// MaxSuiteTargets bounds a registration's target list.
const MaxSuiteTargets = 64

var slugRe = regexp.MustCompile(`[^a-z0-9-]+`)

// Slug mirrors scripts/benchmark/eval_suite.py slug(), which builds run ids.
func Slug(text string) string {
	s := strings.Trim(slugRe.ReplaceAllString(strings.ToLower(text), "-"), "-")
	if len(s) > 40 {
		s = s[:40]
	}
	return s
}

// RunID is the run id eval_suite gives one (replicate, arm, target) run.
func RunID(suiteRun string, replicate int, arm, target string) string {
	return fmt.Sprintf("%s-r%d-%s-%s", suiteRun, replicate, Slug(arm), Slug(target))
}

// Validate checks a registration request's shape.
func (r SuiteRun) Validate() error {
	switch {
	case strings.TrimSpace(r.SuiteRun) == "" || strings.TrimSpace(r.Suite) == "" || strings.TrimSpace(r.SuiteVersion) == "":
		return fmt.Errorf("suite_run, suite and suite_version are required")
	case r.Split == "":
		return fmt.Errorf("split is required")
	case r.Replicates <= 0 || r.Replicates > 100:
		return fmt.Errorf("replicates must be 1 to 100")
	case len(r.Targets) == 0 || len(r.Targets) > MaxSuiteTargets:
		return fmt.Errorf("targets must list 1 to %d suite target ids", MaxSuiteTargets)
	}
	seen := map[string]bool{}
	for _, t := range r.Targets {
		if strings.TrimSpace(t) == "" || seen[t] {
			return fmt.Errorf("targets must be distinct, non-empty ids")
		}
		seen[t] = true
	}
	return nil
}

// stampNumber reads a JSON number from a raw stamp.
func stampNumber(s Stamp, key string) (int64, bool) {
	f, ok := s.Raw[key].(float64)
	return int64(f), ok && f == float64(int64(f))
}

func stampString(s Stamp, key string) string {
	v, _ := s.Raw[key].(string)
	return v
}

// CheckRegistration proves both arms are the registered suite run: the same
// run id, suite, version, split, seed and replicates, and exactly one record
// per registered (target, replicate) in each arm, each with the run id the
// harness derives for it. A run cannot drop an unfavourable target or
// replicate, nor add one that was not registered.
func CheckRegistration(base, cand ArmResult, reg SuiteRun) []string {
	var out []string
	fail := func(format string, args ...any) { out = append(out, fmt.Sprintf(format, args...)) }
	for _, arm := range []ArmResult{base, cand} {
		s := arm.Stamp
		for _, kv := range [][3]string{
			{"suite_run", stampString(s, "suite_run"), reg.SuiteRun},
			{"suite", stampString(s, "suite"), reg.Suite},
			{"suite_version", stampString(s, "suite_version"), reg.SuiteVersion},
			{"split", s.Split, reg.Split},
		} {
			if kv[1] != kv[2] {
				fail("%s arm %s is %q; the registered run has %q", s.Role, kv[0], kv[1], kv[2])
			}
		}
		if seed, ok := stampNumber(s, "seed"); !ok || seed != reg.Seed {
			fail("%s arm seed is %v; the registered run has %d", s.Role, s.Raw["seed"], reg.Seed)
		}
		if reps, ok := stampNumber(s, "replicates"); !ok || int(reps) != reg.Replicates {
			fail("%s arm replicates is %v; the registered run has %d", s.Role, s.Raw["replicates"], reg.Replicates)
		}
		want := map[string]bool{}
		for _, t := range reg.Targets {
			for rep := 0; rep < reg.Replicates; rep++ {
				want[fmt.Sprintf("%s#%d", t, rep)] = true
			}
		}
		for _, r := range arm.Records {
			key := fmt.Sprintf("%s#%d", r.Target, r.Replicate)
			if !want[key] {
				fail("%s arm has a run of %s replicate %d that is not registered (or appears twice)", s.Role, r.Target, r.Replicate)
				continue
			}
			delete(want, key)
			if id := RunID(reg.SuiteRun, r.Replicate, s.Arm, r.Target); r.RunID != id {
				fail("%s arm run %q should be %q", s.Role, r.RunID, id)
			}
		}
		if len(want) > 0 {
			missing := make([]string, 0, len(want))
			for k := range want {
				missing = append(missing, k)
			}
			sort.Strings(missing)
			fail("%s arm is missing registered runs %v", s.Role, missing)
		}
	}
	return out
}

// CheckLedger proves each judged run's task ran what its arm's stamp claims:
// the prompt sha of every RSI role and the injected lesson set and block, as
// the server recorded them when the task's agents were built, with no prompt
// change while the task was live; and that the task is the run's own (its
// name ends with the run id, which names the target).
func CheckLedger(base, cand ArmResult, ledger map[int64]TaskArm) []string {
	var out []string
	fail := func(format string, args ...any) { out = append(out, fmt.Sprintf(format, args...)) }
	for _, arm := range []ArmResult{base, cand} {
		s := arm.Stamp
		wantLessons := Edit{LessonIDs: s.LessonIDs}.Normalize().LessonIDs
		wantBlock := stampString(s, "lessons_block_sha")
		for _, r := range arm.Records {
			if r.Payload == nil {
				continue
			}
			id := int64(r.Payload.Task.ID)
			l, ok := ledger[id]
			if !ok {
				fail("task %d (%s) has no arm record on this server", id, r.RunID)
				continue
			}
			if l.Drifted {
				fail("task %d did not run one arm: %s", id, l.DriftReason)
			}
			if r.RunID == "" || !strings.HasSuffix(l.TaskName, "-"+r.RunID) {
				fail("task %d is %q, not the task of run %s", id, l.TaskName, r.RunID)
			}
			for _, role := range PromptRoles {
				if got, claim := l.PromptSHA[role], s.PromptSHA[role]; got != claim {
					fail("task %d ran %s body %s; the %s arm claims %s", id, role, got, s.Role, claim)
				}
			}
			if got := (Edit{LessonIDs: l.LessonIDs}).Normalize().LessonIDs; !reflect.DeepEqual(got, wantLessons) {
				fail("task %d injected lessons %v; the %s arm claims %v", id, got, s.Role, wantLessons)
			}
			if l.LessonsBlockSHA != wantBlock {
				fail("task %d lesson block %q; the %s arm claims %q", id, l.LessonsBlockSHA, s.Role, wantBlock)
			}
		}
	}
	return out
}
