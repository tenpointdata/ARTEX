package server

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Autumn-27/artex/rsi"
)

// RSI gate hardening (card c-0d277f): the server's own records that uploaded
// evidence is checked against.
//
//	register → POST /api/rsi/proposals/{id}/suite-runs  (loop flag on)
//	list     → GET  /api/rsi/proposals/{id}/suite-runs
//
// The arm ledger (task_rsi_arm) is written whenever a task's agents are built.

// rsiMaxSuiteRuns caps registrations per proposal; each backs one upload.
const rsiMaxSuiteRuns = rsiMaxEvidenceUploads

// recordTaskArm writes what this task's agents were built with: the sha of
// the body each RSI role runs and the lessons injected into the planner and
// worker (empty in off and shadow modes, where none are). Failure is logged,
// never fatal: a task without a ledger row simply cannot back evidence.
func (s *Server) recordTaskArm(t *Task, mode string, lessonIDs []int64, block string) {
	pg := s.m.PG()
	if pg == nil || t == nil {
		return
	}
	taskID, err := strconv.ParseInt(t.ID, 10, 64)
	if err != nil {
		return
	}
	arm := rsi.TaskArm{TaskID: taskID, PromptSHA: map[string]string{}, LessonsMode: mode}
	for _, role := range rsi.PromptRoles {
		arm.PromptSHA[role] = rsiLiveSHA(pg, role)
	}
	if mode != "off" && mode != "shadow" && block != "" {
		arm.LessonIDs, arm.LessonsBlockSHA = lessonIDs, rsi.SHA16(block)
	}
	if err := pg.RecordTaskRSIArm(context.Background(), arm); err != nil {
		log.Printf("[rsi] arm ledger write failed for task %s: %v", t.ID, err)
	}
}

// POST /api/rsi/proposals/{id}/suite-runs — body {"suite_run", "suite",
// "suite_version", "split", "targets", "seed", "replicates"}, sent before the
// suite runs (scripts/benchmark: eval_suite.py --rsi-proposal, or
// rsi_submit.py register).
func (s *Server) rsiRegisterSuiteRun(w http.ResponseWriter, r *http.Request) {
	if !s.rsiRequireLoop(w) {
		return
	}
	pg := s.pg(w)
	id, ok := rsiPathID(w, r)
	if pg == nil || !ok {
		return
	}
	var req rsi.SuiteRun
	if err := decode(w, r, &req); err != nil {
		writeErr(w, 400, "invalid request: "+err.Error())
		return
	}
	req.ID, req.ProposalID, req.EvidenceID = 0, id, nil
	req.SuiteRun = strings.TrimSpace(req.SuiteRun)
	if err := req.Validate(); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if req.Split != rsi.DefaultPolicy.RequireSplit {
		writeErr(w, 400, fmt.Sprintf("split %q is not adoption evidence; register a %q suite", req.Split, rsi.DefaultPolicy.RequireSplit))
		return
	}
	out, err := pg.RegisterRSISuiteRun(r.Context(), req, rsiMaxSuiteRuns, s.rsiActor(r))
	if err != nil {
		rsiWriteErr(w, "register suite run", err)
		return
	}
	writeJSON(w, 201, map[string]any{"suite_run": out})
}

// GET /api/rsi/proposals/{id}/suite-runs
func (s *Server) rsiListSuiteRuns(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	id, ok := rsiPathID(w, r)
	if pg == nil || !ok {
		return
	}
	runs, err := pg.RSISuiteRuns(r.Context(), id)
	if err != nil {
		rsiWriteErr(w, "list suite runs", err)
		return
	}
	writeJSON(w, 200, map[string]any{"suite_runs": runs})
}

// rsiBindEvidence adds to v the reasons the uploaded arms are not the
// registered run reg (registered at regAt) as this server recorded it: tasks
// unknown here, older than the registration or already in the evidence
// ledger, tasks whose arm ledger disagrees with the stamp, and records that
// do not match the registration.
func (s *Server) rsiBindEvidence(ctx context.Context, v *rsi.Verdict, base, cand rsi.ArmResult, reg rsi.SuiteRun, regAt time.Time) error {
	pg := s.m.PG()
	v.Reasons = append(v.Reasons, rsi.CheckRegistration(base, cand, reg)...)
	since := regAt.Add(-time.Second)
	if len(v.TaskIDs) > 0 {
		bad, err := pg.RSIEvidenceTasksInvalid(ctx, v.TaskIDs, since)
		if err != nil {
			return err
		}
		if len(bad) > 0 {
			v.Reasons = append(v.Reasons, fmt.Sprintf("tasks %v are unknown here, older than the suite registration, or already back other evidence", bad))
		}
	}
	ledger, err := pg.RSITaskArms(ctx, v.TaskIDs, since)
	if err != nil {
		return err
	}
	v.Reasons = append(v.Reasons, rsi.CheckLedger(base, cand, ledger)...)
	v.Passed = len(v.Reasons) == 0
	return nil
}
