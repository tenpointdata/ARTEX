package server

import (
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/Autumn-27/artex/agent"
)

// A task whose engagement authorization artifact ended (expired at not_after
// or revoked) is paused. The target gate already refuses every call against a
// target at that point; pausing stops the task from spending LLM budget on
// refused calls and shows its state. It cannot be resumed until an admin
// stores a new version that is not ended. A window that has not started yet
// does not pause: the gate refuses until it starts.

// pauseEndedAuthorizations pauses every running or queued task whose
// artifact ended. Called on each scheduler tick and after a revocation.
func (s *Server) pauseEndedAuthorizations() {
	if s.m == nil || s.m.pg == nil {
		return
	}
	ids, err := s.m.pg.TasksWithEndedAuthorization(time.Now())
	if err != nil {
		log.Printf("[authorization] could not list tasks whose authorization ended: %v", err)
		return
	}
	for _, id := range ids {
		s.pauseForEndedAuthorization(id)
	}
}

func (s *Server) pauseForEndedAuthorization(id int64) {
	t, ok := s.m.Task(strconv.FormatInt(id, 10))
	if !ok {
		return // not loaded here (deleted or archived meanwhile)
	}
	if t.lifecycleSnapshot().Paused {
		return
	}
	if _, err := s.applyTaskControlWithCause(t, "pause", agent.AbortAuthorizationEnded); err != nil {
		log.Printf("[authorization] task #%d: authorization ended but the pause failed: %v", id, err)
		return
	}
	log.Printf("[authorization] task #%d paused: its engagement authorization is revoked or expired", id)
}

// refuseResumeWithEndedAuthorization is the resume check: a task whose
// artifact ended stays paused until a new version is stored.
func (s *Server) refuseResumeWithEndedAuthorization(t *Task) error {
	if s.m == nil || s.m.pg == nil {
		return nil
	}
	id, err := strconv.ParseInt(t.ID, 10, 64)
	if err != nil {
		return nil
	}
	a, err := s.m.pg.CurrentAuthorization(id)
	if err != nil {
		return fmt.Errorf("read the engagement authorization: %w", err)
	}
	if a != nil && a.Ended(time.Now()) {
		return fmt.Errorf("the engagement authorization of this task (v%d) is revoked or expired; an admin must store a new version before the task can resume", a.Version)
	}
	return nil
}
