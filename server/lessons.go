package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/Autumn-27/artex/agent"
	"github.com/Autumn-27/artex/db"
	"github.com/Autumn-27/artex/rsi"
)

// rsiLessonsMode reads the shadow-mode switch:
//
//	off (default)  no selection, no logging, no prompt change
//	shadow         select lessons and log what would have been shown
//	inject         select, log, and append the block to planner/worker prompts
//	adopted        as inject, but only lessons an owner adopted through the
//	               benchmark gate (graduated lessons; server/rsi_loop.go)
//	bench          inject exactly the pinned set (PUT /api/rsi/bench-lessons),
//	               ignoring scope; for a dedicated benchmark instance only
//
// The flag is read per task-bundle build, so changing it needs a new task or a
// task-agent invalidation, not a restart.
func rsiLessonsMode() string {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("ARTEX_RSI_LESSONS"))) {
	case "shadow":
		return "shadow"
	case "inject":
		return "inject"
	case "adopted":
		return "adopted"
	case "bench":
		return "bench"
	default:
		return "off"
	}
}

// wireTaskLessons selects the lessons visible to a task and, in inject mode,
// attaches the rendered block to the planner and worker. The block is
// code-owned: it is appended after the rendered prompt and cannot be removed
// by editing the prompt in the UI. Usage is logged in both modes so a later
// comparison can tell which tasks saw which lessons.
func (s *Server) wireTaskLessons(t *Task, wk *agent.Worker, pl *agent.Planner) {
	mode := rsiLessonsMode()
	if mode == "off" || t == nil {
		return
	}
	taskID, err := strconv.ParseInt(t.ID, 10, 64)
	if err != nil {
		return
	}
	lessons, err := s.selectTaskLessons(context.Background(), mode, taskID)
	if err != nil {
		log.Printf("[rsi] lesson selection failed for task %s: %v", t.ID, err)
		return
	}
	block, ids := db.RenderLessons(lessons, 1800)
	if block == "" {
		return
	}
	if mode != "shadow" {
		wk.SetLessonsBlock(block)
		pl.SetLessonsBlock(block)
	}
	if err := s.m.PG().LogLessonUsage(context.Background(), taskID, ids, mode); err != nil {
		log.Printf("[rsi] lesson usage log failed for task %s: %v", t.ID, err)
	}
}

// taskLessons returns the lessons visible to one task plus the rendered block.
// Read-only: it never extracts.
func (s *Server) taskLessons(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeErr(w, 400, "invalid task id")
		return
	}
	lessons, err := s.m.PG().TaskLessons(r.Context(), id, 50)
	if errors.Is(err, sql.ErrNoRows) {
		writeErr(w, 404, "task not found")
		return
	}
	if err != nil {
		log.Printf("[rsi] lesson list failed for task %d: %v", id, err)
		writeErr(w, 500, "lesson query failed")
		return
	}
	block, _ := db.RenderLessons(lessons, 1800)
	writeJSON(w, 200, map[string]any{
		"mode":    rsiLessonsMode(),
		"lessons": lessons,
		"block":   block,
		"count":   len(lessons),
	})
}

// extractTaskLessons derives candidate lessons from one task's findings and
// retests. It is idempotent; call it after the task closes (the manager does
// this automatically on terminal status) or manually for a backfill.
func (s *Server) extractTaskLessons(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeErr(w, 400, "invalid task id")
		return
	}
	n, err := s.m.PG().ExtractLessonsForTask(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		writeErr(w, 404, "task not found")
		return
	}
	if err != nil {
		log.Printf("[rsi] lesson extraction failed for task %d: %v", id, err)
		writeErr(w, 500, "lesson extraction failed")
		return
	}
	writeJSON(w, 200, map[string]any{"extracted": n})
}

// selectTaskLessons picks the lessons a task sees in mode.
func (s *Server) selectTaskLessons(ctx context.Context, mode string, taskID int64) ([]db.Lesson, error) {
	switch mode {
	case "adopted":
		return s.m.PG().TaskAdoptedLessons(ctx, taskID, 8)
	case "bench":
		ids, err := benchLessonIDs(s.m.PG())
		if err != nil || len(ids) == 0 {
			return nil, err
		}
		return s.m.PG().LessonsByIDs(ctx, ids)
	}
	return s.m.PG().TaskLessons(ctx, taskID, 8)
}

// benchLessonsKey is the settings row holding the benchmark lesson pin.
const benchLessonsKey = "rsi_bench_lessons"

func benchLessonIDs(pg *db.DB) ([]int64, error) {
	raw, ok, err := pg.GetSetting(benchLessonsKey)
	if err != nil || !ok || raw == "" {
		return []int64{}, err
	}
	ids := []int64{}
	if err := json.Unmarshal([]byte(raw), &ids); err != nil {
		return nil, err
	}
	return ids, nil
}

// benchLessonsState answers the pinned ids and the sha of the block they
// render to, the value an arm stamps.
func (s *Server) benchLessonsState(w http.ResponseWriter, r *http.Request) {
	ids, err := benchLessonIDs(s.m.PG())
	if err != nil {
		writeErr(w, 500, "bench lesson read failed")
		return
	}
	lessons, err := s.m.PG().LessonsByIDs(r.Context(), ids)
	if err != nil {
		writeErr(w, 500, "bench lesson read failed")
		return
	}
	block, _ := db.RenderLessons(lessons, 1800)
	sha := ""
	if block != "" {
		sha = rsi.SHA16(block)
	}
	writeJSON(w, 200, map[string]any{"lesson_ids": ids, "block_sha": sha})
}

func (s *Server) benchLessonsGuard(w http.ResponseWriter) bool {
	if rsiLessonsMode() != "bench" {
		writeErr(w, 409, "the lesson pin works only on a benchmark instance with ARTEX_RSI_LESSONS=bench")
		return false
	}
	return s.pg(w) != nil
}

// GET /api/rsi/bench-lessons
func (s *Server) getBenchLessons(w http.ResponseWriter, r *http.Request) {
	if s.benchLessonsGuard(w) {
		s.benchLessonsState(w, r)
	}
}

// PUT /api/rsi/bench-lessons — body {"lesson_ids": [...]}. Every id must be a
// lesson that is not retired, and the whole set must render into the block
// (the block's character cap would silently drop the tail otherwise).
func (s *Server) putBenchLessons(w http.ResponseWriter, r *http.Request) {
	if !s.benchLessonsGuard(w) {
		return
	}
	var req struct {
		LessonIDs []int64 `json:"lesson_ids"`
	}
	if err := decode(w, r, &req); err != nil {
		writeErr(w, 400, "invalid request: "+err.Error())
		return
	}
	ids := rsi.Edit{LessonIDs: req.LessonIDs}.Normalize().LessonIDs
	if ids == nil {
		ids = []int64{}
	}
	if len(ids) > rsi.MaxLessonsPerEdit {
		writeErr(w, 400, fmt.Sprintf("at most %d lessons can be pinned", rsi.MaxLessonsPerEdit))
		return
	}
	lessons, err := s.m.PG().LessonsByIDs(r.Context(), ids)
	if err != nil {
		writeErr(w, 500, "lesson lookup failed")
		return
	}
	if len(lessons) != len(ids) {
		writeErr(w, 400, "every pinned id must name a lesson that is not retired")
		return
	}
	if _, rendered := db.RenderLessons(lessons, 1800); len(rendered) != len(ids) {
		writeErr(w, 400, "the pinned lessons do not all fit in the lesson block; pin fewer")
		return
	}
	js, _ := json.Marshal(ids)
	if err := s.m.PG().SetSetting(benchLessonsKey, string(js)); err != nil {
		writeErr(w, 500, "bench lesson write failed")
		return
	}
	s.benchLessonsState(w, r)
}
