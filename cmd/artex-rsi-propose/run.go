package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"

	"github.com/Autumn-27/artex/agent"
	"github.com/Autumn-27/artex/rsi"
	"github.com/Autumn-27/artex/rsi/propose"
)

// options are the command-line settings of one run.
type options struct {
	Out      string          // directory for arm files; round-<N> is appended
	Round    int             // -1 = the newest round
	MaxTasks int             // newest closed tasks to read
	Post     bool            // post the drafts; default is a dry run
	Drafter  propose.Drafter // optional model that rewrites bodies (--draft-model)
	Judge    string          // the suite's judge model; the drafter must differ
}

// ErrLoopOff is returned when the server's RSI loop flag is off.
var ErrLoopOff = fmt.Errorf("the RSI loop is off on this server; the owner turns it on with ARTEX_RSI_LOOP=on")

type statusResp struct {
	Loop    string     `json:"loop"`
	Budget  rsi.Budget `json:"budget"`
	MaxOpen int        `json:"max_open"`
}

type taskRow struct {
	ID          string `json:"id"`
	Status      string `json:"status"`
	CompletedAt int64  `json:"completed_unix"`
}

type outcomeResp struct {
	propose.Outcome
	CategoryID int64 `json:"category_id"`
	CompanyID  int64 `json:"company_id"`
}

// result is what a run reports (and writes as plan.json next to the arms).
type result struct {
	propose.Plan
	Note     string   `json:"note,omitempty"`
	Files    []string `json:"files"`
	Posted   []int64  `json:"posted_ids"`
	DryRun   bool     `json:"dry_run"`
	ReadFrom int      `json:"closed_tasks_read"`
}

// run is one proposer pass. The flag is checked on every run, before anything
// is read, and again before posting (RSI.md rule 5).
func run(c *client, opt options, log io.Writer) (*result, error) {
	if err := propose.CheckDrafter(opt.Drafter, opt.Judge); err != nil {
		return nil, err
	}
	st, err := loopStatus(c)
	if err != nil {
		return nil, err
	}
	var props struct {
		Proposals []propose.Prior `json:"proposals"`
	}
	if err := c.get(proposalsPath+"?limit=500", &props); err != nil {
		return nil, err
	}
	maxOpen := st.MaxOpen
	if maxOpen <= 0 {
		maxOpen = 4
	}
	round, slots, note := propose.Slots(st.Budget, maxOpen, props.Proposals, opt.Round)
	res := &result{Plan: propose.Plan{Round: round, Slots: slots, Drafts: []propose.Draft{}, Skipped: []propose.Skip{}},
		Note: note, Files: []string{}, Posted: []int64{}, DryRun: !opt.Post}
	if slots == 0 {
		fmt.Fprintf(log, "no edits left: %s\n", note)
		return res, nil
	}
	tasks, err := closedTasks(c, opt.MaxTasks)
	if err != nil {
		return nil, err
	}
	var gaps struct {
		Gaps []propose.SkillGap `json:"gaps"`
	}
	if err := c.get("/api/skill-gaps?limit=500", &gaps); err != nil {
		return nil, err
	}
	bodies := map[string]string{}
	for _, role := range rsi.PromptRoles {
		var a struct {
			Prompt string `json:"prompt"`
		}
		if err := c.get("/api/agents/"+url.PathEscape(role), &a); err != nil {
			return nil, err
		}
		bodies[role] = a.Prompt
	}
	plan := propose.Build(propose.Input{Round: round, Slots: slots, Tasks: tasks, Bodies: bodies,
		Prior: props.Proposals, Rules: propose.DefaultRules(gaps.Gaps), Markers: agent.CodeOwnedTailMarkers,
		Drafter: opt.Drafter, Judge: opt.Judge}, propose.DefaultConfig)
	res.Plan, res.ReadFrom = plan, len(tasks)
	dir := filepath.Join(opt.Out, "round-"+strconv.Itoa(round))
	for _, d := range plan.Drafts {
		path, err := propose.WriteArm(dir, d, agent.CodeOwnedTailMarkers)
		if err != nil {
			return nil, fmt.Errorf("draft %s: %w", d.Name, err)
		}
		res.Files = append(res.Files, path)
	}
	if opt.Post && len(plan.Drafts) > 0 {
		if _, err := loopStatus(c); err != nil {
			return nil, err
		}
		for _, d := range plan.Drafts {
			var out struct {
				Proposal struct {
					ID int64 `json:"id"`
				} `json:"proposal"`
			}
			req := proposalReq{Round: d.Round, Surface: string(d.Edit.Surface), Target: d.Edit.Target, Body: d.Edit.Body,
				ProposedFrom: d.ProposedFrom, Rationale: d.Rationale}
			if err := c.propose(req, &out); err != nil {
				return res, fmt.Errorf("post %s: %w", d.Name, err)
			}
			res.Posted = append(res.Posted, out.Proposal.ID)
		}
	}
	if len(plan.Drafts) > 0 {
		raw, _ := json.MarshalIndent(res, "", "  ")
		if err := os.WriteFile(filepath.Join(dir, "plan.json"), append(raw, '\n'), 0o644); err != nil {
			return res, err
		}
	}
	return res, nil
}

func loopStatus(c *client) (*statusResp, error) {
	var st statusResp
	if err := c.get("/api/rsi/status", &st); err != nil {
		return nil, err
	}
	if st.Loop != "on" {
		return nil, ErrLoopOff
	}
	return &st, nil
}

// closedTasks reads the newest done/failed tasks with their outcome and lessons.
func closedTasks(c *client, limit int) ([]propose.Task, error) {
	var list struct {
		Tasks []taskRow `json:"tasks"`
	}
	if err := c.get("/api/tasks", &list); err != nil {
		return nil, err
	}
	var rows []taskRow
	for _, t := range list.Tasks {
		if t.Status == "done" || t.Status == "failed" {
			rows = append(rows, t)
		}
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].CompletedAt > rows[j].CompletedAt })
	if limit > 0 && len(rows) > limit {
		rows = rows[:limit]
	}
	out := make([]propose.Task, 0, len(rows))
	for _, t := range rows {
		id := url.PathEscape(t.ID)
		var o outcomeResp
		if err := c.get("/api/tasks/"+id+"/outcome", &o); err != nil {
			return nil, err
		}
		var ls struct {
			Lessons []propose.Lesson `json:"lessons"`
		}
		if err := c.get("/api/tasks/"+id+"/lessons", &ls); err != nil {
			return nil, err
		}
		out = append(out, propose.Task{ID: t.ID, CategoryID: o.CategoryID, CompanyID: o.CompanyID, Outcome: o.Outcome, Lessons: ls.Lessons})
	}
	return out, nil
}
