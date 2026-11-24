package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/Autumn-27/artex/agent"
	"github.com/Autumn-27/artex/db"
	"github.com/Autumn-27/artex/rsi"
)

// RSI Loop C benchmark gate and Loop B lesson graduation (RSI.md). The HTTP
// adapter over package rsi (search space, budget, gate) and db (proposals,
// evidence, adoption log). Live tasks propose, held-out benchmarks adopt:
//
//	propose  → POST /api/rsi/proposals                (loop flag on, budget)
//	evidence → POST /api/rsi/proposals/{id}/evidence  (loop flag on; the server's verdict)
//	adopt    → POST /api/rsi/proposals/{id}/adopt     (loop flag on; owner password)
//	reject   → POST /api/rsi/proposals/{id}/reject    (always)
//	rollback → POST /api/rsi/proposals/{id}/rollback  (always; a pointer change)

// rsiLoopFlag is the feature flag, off by default and read on every request
// (RSI.md rule 5). Turning it on is an owner decision made in the environment.
const rsiLoopFlag = "ARTEX_RSI_LOOP"

func rsiLoopOn() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv(rsiLoopFlag)), "on")
}

// rsiForbiddenMarkers are strings of the code-owned prompt tails. A proposed
// body that contains one could imitate or pre-empt a tail, so it is refused.
var rsiForbiddenMarkers = []string{
	"## Prior evidence from earlier tasks", // lesson block header (db.RenderLessons)
	"ARTEX_DATA",                           // data envelope marker stem
	"Data envelope (code-owned)",
	"Extra duty: register the test asset scope", // goals scope tail (agent/goals.go)
	"[Noise profile: ",                          // noise profile tail (agent/noise_profile.go)
}

const (
	// rsiMaxOpen caps proposals that are open at once (proposed, passed, failed).
	rsiMaxOpen = 4
	// rsiMaxEvidenceUploads caps evidence uploads per proposal, so a proposal
	// cannot be re-run until a noisy pass appears.
	rsiMaxEvidenceUploads = 3
)

func (s *Server) rsiRequireLoop(w http.ResponseWriter) bool {
	if !rsiLoopOn() {
		writeErr(w, 409, "the RSI loop is off; the owner turns it on with "+rsiLoopFlag+"=on")
		return false
	}
	return true
}

func (s *Server) rsiActor(r *http.Request) string {
	if a := s.approver(r); a != "" {
		return a
	}
	return "api"
}

func rsiWriteErr(w http.ResponseWriter, op string, err error) {
	switch {
	case errors.Is(err, db.ErrRSINotFound):
		writeErr(w, 404, err.Error())
	case errors.Is(err, db.ErrRSIConflict):
		writeErr(w, 409, err.Error())
	default:
		log.Printf("[rsi] %s: %v", op, err)
		writeErr(w, 500, op+" failed")
	}
}

func rsiPathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeErr(w, 400, "invalid proposal id")
		return 0, false
	}
	return id, true
}

// GET /api/rsi/status — the flag, the search space and the budget.
func (s *Server) rsiStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{
		"loop":                 map[bool]string{true: "on", false: "off"}[rsiLoopOn()],
		"flag":                 rsiLoopFlag,
		"lessons_mode":         rsiLessonsMode(),
		"editable_surfaces":    map[string]any{string(rsi.SurfacePromptBody): rsi.PromptRoles, string(rsi.SurfaceLesson): "candidate lesson ids"},
		"git_reviewed":         rsi.GitReviewedSurfaces,
		"immutable_surfaces":   rsi.ImmutableSurfaces,
		"budget":               rsi.DefaultBudget,
		"budget_by_round":      []int{rsi.DefaultBudget.Edits(0), rsi.DefaultBudget.Edits(1), rsi.DefaultBudget.Edits(2), rsi.DefaultBudget.Edits(3)},
		"policy":               rsi.DefaultPolicy,
		"adoption_requires":    "owner password, loop on, newest evidence passed, baseline equals the live prompt",
		"rollback_always_open": true,
	})
}

type rsiProposalReq struct {
	Round        int      `json:"round"`
	Surface      string   `json:"surface"`
	Target       string   `json:"target"`
	Body         string   `json:"body"`
	LessonIDs    []int64  `json:"lesson_ids"`
	ProposedFrom []string `json:"proposed_from"`
	Rationale    string   `json:"rationale"`
}

// POST /api/rsi/proposals
func (s *Server) rsiCreateProposal(w http.ResponseWriter, r *http.Request) {
	if !s.rsiRequireLoop(w) {
		return
	}
	pg := s.pg(w)
	if pg == nil {
		return
	}
	var req rsiProposalReq
	if err := decode(w, r, &req); err != nil {
		writeErr(w, 400, "invalid request: "+err.Error())
		return
	}
	edit := rsi.Edit{Surface: rsi.SurfaceKind(req.Surface), Target: req.Target, Body: req.Body, LessonIDs: req.LessonIDs}.Normalize()
	if err := rsi.ValidateEdit(edit, rsiForbiddenMarkers); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if strings.TrimSpace(req.Rationale) == "" {
		writeErr(w, 400, "a proposal needs a rationale: what it should change, and on which gates")
		return
	}
	switch edit.Surface {
	case rsi.SurfacePromptBody:
		a, err := pg.GetAgentByKey(edit.Target)
		if err != nil || a == nil {
			writeErr(w, 400, "agent "+edit.Target+" is not available")
			return
		}
		vars, _ := pg.PromptVars(a.ID)
		if bad := validateTemplate(edit.Body, withGlobalVars(vars)); bad != "" {
			writeErr(w, 400, bad)
			return
		}
		// Template actions can assemble a marker from pieces: check the rendering too.
		rendered, err := renderPrompt(edit.Body, withGlobalVars(vars), nil)
		if err != nil {
			writeErr(w, 400, "template does not render: "+err.Error())
			return
		}
		if err := rsi.CheckMarkers(rendered, rsiForbiddenMarkers); err != nil {
			writeErr(w, 400, "rendered body: "+err.Error())
			return
		}
	case rsi.SurfaceLesson:
		lessons, err := pg.LessonsByIDs(r.Context(), edit.LessonIDs)
		if err != nil {
			rsiWriteErr(w, "lesson lookup", err)
			return
		}
		if len(lessons) != len(edit.LessonIDs) {
			writeErr(w, 400, "every lesson id must name a lesson that is not retired")
			return
		}
		for _, l := range lessons {
			if l.Status != "candidate" {
				writeErr(w, 400, fmt.Sprintf("lesson %d is %s; only candidates graduate", l.ID, l.Status))
				return
			}
		}
	}
	from, _ := json.Marshal(nonNilStrings(req.ProposedFrom))
	p := db.RSIProposal{Round: req.Round, Surface: string(edit.Surface), Target: edit.Target, Body: edit.Body,
		LessonIDs: edit.LessonIDs, ProposedFrom: from, Rationale: strings.TrimSpace(req.Rationale), CreatedBy: s.rsiActor(r)}
	if edit.Surface == rsi.SurfacePromptBody {
		p.BodySHA = rsi.SHA16(edit.Body)
	}
	out, err := pg.CreateRSIProposal(r.Context(), p, rsi.DefaultBudget.Edits, rsiMaxOpen)
	if err != nil {
		rsiWriteErr(w, "create proposal", err)
		return
	}
	writeJSON(w, 201, map[string]any{"proposal": out})
}

func nonNilStrings(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}

// GET /api/rsi/proposals
func (s *Server) rsiListProposals(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	ps, err := pg.ListRSIProposals(r.Context(), limit)
	if err != nil {
		rsiWriteErr(w, "list proposals", err)
		return
	}
	writeJSON(w, 200, map[string]any{"proposals": ps, "loop": rsiLoopOn()})
}

// GET /api/rsi/proposals/{id}
func (s *Server) rsiGetProposal(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	id, ok := rsiPathID(w, r)
	if pg == nil || !ok {
		return
	}
	p, ev, lg, err := pg.GetRSIProposal(r.Context(), id)
	if err != nil {
		rsiWriteErr(w, "get proposal", err)
		return
	}
	writeJSON(w, 200, map[string]any{"proposal": p, "evidence": ev, "log": lg})
}

// GET /api/rsi/log
func (s *Server) rsiLog(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	lg, err := pg.RSILog(r.Context(), 0, limit)
	if err != nil {
		rsiWriteErr(w, "read log", err)
		return
	}
	writeJSON(w, 200, map[string]any{"log": lg})
}

// POST /api/rsi/proposals/{id}/evidence — body {"baseline": <arm file>, "candidate": <arm file>}.
// The server recomputes the verdict from the raw records; a comparison the
// loop computed is never trusted.
func (s *Server) rsiAddEvidence(w http.ResponseWriter, r *http.Request) {
	if !s.rsiRequireLoop(w) {
		return
	}
	pg := s.pg(w)
	id, ok := rsiPathID(w, r)
	if pg == nil || !ok {
		return
	}
	var req struct {
		Baseline  json.RawMessage `json:"baseline"`
		Candidate json.RawMessage `json:"candidate"`
	}
	if err := decode(w, r, &req); err != nil {
		writeErr(w, 400, "invalid request: "+err.Error())
		return
	}
	var base, cand rsi.ArmResult
	if err := json.Unmarshal(req.Baseline, &base); err != nil {
		writeErr(w, 400, "baseline: "+err.Error())
		return
	}
	if err := json.Unmarshal(req.Candidate, &cand); err != nil {
		writeErr(w, 400, "candidate: "+err.Error())
		return
	}
	p, _, _, err := pg.GetRSIProposal(r.Context(), id)
	if err != nil {
		rsiWriteErr(w, "get proposal", err)
		return
	}
	var from []string
	_ = json.Unmarshal(p.ProposedFrom, &from)
	v := rsi.Evaluate(base, cand, rsi.Binding{
		Edit:         rsi.Edit{Surface: rsi.SurfaceKind(p.Surface), Target: p.Target, Body: p.Body, LessonIDs: p.LessonIDs},
		ProposedFrom: from,
	}, rsi.DefaultPolicy)
	// Evidence must be runs this instance made after the proposal existed.
	if len(v.TaskIDs) > 0 {
		bad, err := pg.RSIEvidenceTasksInvalid(r.Context(), v.TaskIDs, p.CreatedAt.Add(-time.Second))
		if err != nil {
			rsiWriteErr(w, "check evidence tasks", err)
			return
		}
		if len(bad) > 0 {
			v.Reasons = append(v.Reasons, fmt.Sprintf("tasks %v are unknown here, older than the proposal, or already back other evidence", bad))
			v.Passed = false
		}
	}
	vj, _ := json.Marshal(v)
	ev, err := pg.AddRSIEvidence(r.Context(), id, v.Passed, vj, req.Baseline, req.Candidate, v.TaskIDs, rsiMaxEvidenceUploads, s.rsiActor(r))
	if err != nil {
		rsiWriteErr(w, "store evidence", err)
		return
	}
	writeJSON(w, 200, map[string]any{"evidence": map[string]any{"id": ev.ID, "passed": v.Passed, "verdict": v}})
}

// ownerStepUp re-checks the owner password for an adoption, under the login
// rate limit. A bearer token alone (what a script holds) cannot adopt.
func (s *Server) ownerStepUp(w http.ResponseWriter, r *http.Request, password string) bool {
	ip := clientIP(r, s.trustedProxies)
	if wait := s.loginLimit.allow(ip); wait > 0 {
		writeErr(w, 429, fmt.Sprintf("too many password attempts, try again in %d seconds", int(wait/time.Second)+1))
		return false
	}
	hash, ok, err := s.m.PG().GetSetting(s.passwordHashKey())
	if err != nil {
		writeErr(w, 503, errDataSourceUnavailable)
		return false
	}
	if !ok || hash == "" || password == "" || bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		writeErr(w, 403, "adoption is an owner action: the owner password is required")
		return false
	}
	s.loginLimit.reset(ip)
	return true
}

// POST /api/rsi/proposals/{id}/adopt — body {"evidence_id": N, "password": "..."}.
func (s *Server) rsiAdopt(w http.ResponseWriter, r *http.Request) {
	if !s.rsiRequireLoop(w) {
		return
	}
	pg := s.pg(w)
	id, ok := rsiPathID(w, r)
	if pg == nil || !ok {
		return
	}
	var req struct {
		EvidenceID int64  `json:"evidence_id"`
		Password   string `json:"password"`
	}
	if err := decode(w, r, &req); err != nil || req.EvidenceID <= 0 {
		writeErr(w, 400, "body needs evidence_id and password")
		return
	}
	if !s.ownerStepUp(w, r, req.Password) {
		return
	}
	p, ev, _, err := pg.GetRSIProposal(r.Context(), id)
	if err != nil {
		rsiWriteErr(w, "get proposal", err)
		return
	}
	arms, err := rsiEvidenceArms(pg, r, ev, req.EvidenceID)
	if err != nil {
		rsiWriteErr(w, "read evidence", err)
		return
	}
	// Roles the proposal does not edit must still run what the suite ran.
	for _, role := range rsi.PromptRoles {
		if role == p.Target {
			continue
		}
		if live := rsiLiveSHA(pg, role); live != arms[0].Stamp.PromptSHA[role] {
			writeErr(w, 409, fmt.Sprintf("%s changed since the suite ran (%s, evidence %s); re-run the suite",
				role, live, arms[0].Stamp.PromptSHA[role]))
			return
		}
	}
	out, err := pg.AdoptRSIProposal(r.Context(), id, req.EvidenceID, s.rsiActor(r), func(live db.RSIPromptState) error {
		body := live.Body
		if !live.Stored {
			body = agent.BuiltinPromptSeeds()[p.Target]
		}
		want := arms[0].Stamp.PromptSHA[p.Target]
		if rsi.SHA16(body) != want {
			return fmt.Errorf("%w: the evidence baseline ran %s body %s, but %s is live now; re-run the suite",
				db.ErrRSIConflict, p.Target, want, rsi.SHA16(body))
		}
		return nil
	}, func(current []db.Lesson) error {
		// The lessons must render now to the block the candidate arm ran.
		block, rendered := db.RenderLessons(current, 1800)
		want, _ := arms[1].Stamp.Raw["lessons_block_sha"].(string)
		if len(rendered) != len(p.LessonIDs) || block == "" || rsi.SHA16(block) != want {
			return fmt.Errorf("%w: the lessons changed since the suite ran (block %s, evidence %s); re-run the suite",
				db.ErrRSIConflict, rsi.SHA16(block), want)
		}
		return nil
	})
	if err != nil {
		rsiWriteErr(w, "adopt", err)
		return
	}
	log.Printf("[rsi] proposal %d adopted by %s (evidence %d)", id, s.rsiActor(r), req.EvidenceID)
	writeJSON(w, 200, map[string]any{"proposal": out})
}

// rsiEvidenceArms returns the [baseline, candidate] arms of one evidence row of
// this proposal.
func rsiEvidenceArms(pg *db.DB, r *http.Request, ev []db.RSIEvidence, evidenceID int64) ([2]rsi.ArmResult, error) {
	var out [2]rsi.ArmResult
	for _, e := range ev {
		if e.ID != evidenceID {
			continue
		}
		base, cand, err := pg.RSIEvidenceArms(r.Context(), evidenceID)
		if err != nil {
			return out, err
		}
		if err := json.Unmarshal(base, &out[0]); err != nil {
			return out, err
		}
		return out, json.Unmarshal(cand, &out[1])
	}
	return out, fmt.Errorf("%w: evidence %d does not belong to this proposal", db.ErrRSIConflict, evidenceID)
}

// rsiLiveSHA is the sha of the body role runs now (its stored prompt, else the
// built-in default), the value the harness stamps.
func rsiLiveSHA(pg *db.DB, role string) string {
	body := ""
	if a, err := pg.GetAgentByKey(role); err == nil && a != nil {
		body, _ = pg.CurrentPrompt(a.ID)
	}
	if body == "" {
		body = agent.BuiltinPromptSeeds()[role]
	}
	return rsi.SHA16(body)
}

func (s *Server) rsiReasonAction(w http.ResponseWriter, r *http.Request, op string,
	act func(pg *db.DB, id int64, actor, reason string) (any, error)) {
	pg := s.pg(w)
	id, ok := rsiPathID(w, r)
	if pg == nil || !ok {
		return
	}
	var req struct {
		Reason string `json:"reason"`
	}
	_ = decode(w, r, &req)
	out, err := act(pg, id, s.rsiActor(r), strings.TrimSpace(req.Reason))
	if err != nil {
		rsiWriteErr(w, op, err)
		return
	}
	writeJSON(w, 200, map[string]any{"proposal": out})
}

// POST /api/rsi/proposals/{id}/reject — open with the flag off too.
func (s *Server) rsiReject(w http.ResponseWriter, r *http.Request) {
	s.rsiReasonAction(w, r, "reject", func(pg *db.DB, id int64, actor, reason string) (any, error) {
		if err := pg.RejectRSIProposal(r.Context(), id, actor, reason); err != nil {
			return nil, err
		}
		p, _, _, err := pg.GetRSIProposal(r.Context(), id)
		return p, err
	})
}

// POST /api/rsi/proposals/{id}/rollback — open with the flag off too, so an
// adoption can always be reversed.
func (s *Server) rsiRollback(w http.ResponseWriter, r *http.Request) {
	s.rsiReasonAction(w, r, "rollback", func(pg *db.DB, id int64, actor, reason string) (any, error) {
		return pg.RollbackRSIProposal(r.Context(), id, actor, reason)
	})
}
