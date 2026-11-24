package server

import (
	"log"
	"strconv"
	"time"

	"github.com/Autumn-27/artex/db"
)

// TaskAuthorizationDTO tells the console, without a per-task request, that a
// task has an engagement authorization (so the target gate cannot be turned
// off: PATCH answers 409) and whether it ended (so resume answers 409 until a
// new version is stored). Absent when the task has none.
type TaskAuthorizationDTO struct {
	Version int  `json:"version"`
	Revoked bool `json:"revoked"`
	Ended   bool `json:"ended"`
}

func taskAuthorizationDTO(s *db.AuthorizationSummary) *TaskAuthorizationDTO {
	if s == nil {
		return nil
	}
	return &TaskAuthorizationDTO{Version: s.Version, Revoked: s.Revoked, Ended: s.Ended}
}

// applyTaskAuthorization fills one task's authorization summary through the
// gate's per-task cache. Best effort: a read error leaves it absent and the
// server's own checks still refuse what the UI could not foresee.
func (s *Server) applyTaskAuthorization(dto *TaskDTO) {
	if s.m == nil || s.m.pg == nil {
		return
	}
	id, err := strconv.ParseInt(dto.ID, 10, 64)
	if err != nil {
		return
	}
	a, err := s.m.pg.CachedAuthorization(id)
	if err != nil {
		log.Printf("[tasks] task %s: could not read the engagement authorization: %v", dto.ID, err)
		return
	}
	dto.Authorization = taskAuthorizationDTO(db.SummarizeAuthorization(a, time.Now()))
}

// taskAuthorizationSummaries is the list endpoint's single read of every
// task's summary; a failure logs and leaves all of them absent.
func (s *Server) taskAuthorizationSummaries() map[int64]db.AuthorizationSummary {
	if s.m == nil || s.m.pg == nil {
		return nil
	}
	out, err := s.m.pg.AuthorizationSummaries(time.Now())
	if err != nil {
		log.Printf("[tasks] list authorization summaries failed: %v", err)
	}
	return out
}

func applyTaskAuthorizationSummary(dto *TaskDTO, summaries map[int64]db.AuthorizationSummary) {
	id, err := strconv.ParseInt(dto.ID, 10, 64)
	if err != nil {
		return
	}
	if s, ok := summaries[id]; ok {
		dto.Authorization = taskAuthorizationDTO(&s)
	}
}
