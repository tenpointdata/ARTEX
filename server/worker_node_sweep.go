package server

import (
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Autumn-27/artex/agent"
)

// Worker-node file sweep (E7, docs/DISTRIBUTED.md "Local files on worker
// nodes"). Deleting or archiving a task removes its run directory, spill
// directory and transcripts only on the node that runs the delete or archive;
// the shared copies in PostgreSQL go with the task. A worker node keeps its own
// copies until this sweep finds the task retired (deleted, archived or gone)
// and removes them. It runs on the worker's dispatch goroutine, so no worker
// loop of a task can start while the sweep decides about it, and it never
// touches a task whose loops still run here.

// taskFileSweepInterval is how often a worker node sweeps after the first
// sweep at start-up.
const taskFileSweepInterval = 10 * time.Minute

// localTaskFiles lists the task ids (tasks/<T>, spill/tasks/<T>) and the
// exploration ids (transcripts/exp<E>-…) that dataDir holds files for. Names
// that are not of that form are not a task's and are ignored.
func localTaskFiles(dataDir string) (taskIDs, explorationIDs []int64, err error) {
	seen := map[int64]bool{}
	for _, dir := range []string{filepath.Join(dataDir, "tasks"), filepath.Dir(agent.TaskSpillDir(dataDir, "x"))} {
		names, err := dirNames(dir)
		if err != nil {
			return nil, nil, err
		}
		for _, name := range names {
			if id, err := strconv.ParseInt(name, 10, 64); err == nil && id > 0 && !seen[id] {
				seen[id] = true
				taskIDs = append(taskIDs, id)
			}
		}
	}
	names, err := dirNames(filepath.Join(dataDir, "transcripts"))
	if err != nil {
		return nil, nil, err
	}
	seenExp := map[int64]bool{}
	for _, name := range names {
		rest, ok := strings.CutPrefix(name, "exp")
		if !ok {
			continue
		}
		num, _, ok := strings.Cut(rest, "-")
		if id, err := strconv.ParseInt(num, 10, 64); ok && err == nil && id > 0 && !seenExp[id] {
			seenExp[id] = true
			explorationIDs = append(explorationIDs, id)
		}
	}
	return taskIDs, explorationIDs, nil
}

// dirNames lists a directory's entry names; a missing directory is empty.
func dirNames(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Name()
	}
	return names, nil
}

// sweepRetiredTaskFiles removes this node's local files of retired tasks and
// returns how many paths it removed. Any failure to read the database removes
// nothing (an unknown state is not a retired one). Call it only from the
// worker dispatch goroutine.
func (s *Server) sweepRetiredTaskFiles() (int, error) {
	dataDir := s.m.dir
	taskIDs, expIDs, err := localTaskFiles(dataDir)
	if err != nil || len(taskIDs)+len(expIDs) == 0 {
		return 0, err
	}
	retiredTasks, retiredExps, err := s.m.pg.RetiredTasks(taskIDs, expIDs)
	if err != nil {
		return 0, err
	}
	// Explorations of tasks whose loops still run here (a retired task is
	// stopped by the next dispatch pass; it is swept on a later one). A running
	// task this node holds no handle for has an unknown exploration: then no
	// transcript is removed in this pass.
	busyExp, expKnown := map[int64]bool{}, true
	s.engine.started.Range(func(k, _ any) bool {
		id, _ := k.(string)
		if t, ok := s.m.Task(id); ok {
			busyExp[t.ExpID] = true
		} else {
			expKnown = false
		}
		return true
	})
	var targets []string
	for _, n := range retiredTasks {
		id := strconv.FormatInt(n, 10)
		if s.engine.Started(id) {
			continue
		}
		s.m.forgetTaskHandle(id)
		paths, err := taskFileTargets(dataDir, id, 0)
		if err != nil {
			return 0, err
		}
		targets = append(targets, paths...)
	}
	for _, x := range retiredExps {
		if busyExp[x] || !expKnown {
			continue
		}
		paths, err := transcriptTargets(dataDir, x)
		if err != nil {
			return 0, err
		}
		targets = append(targets, paths...)
	}
	stage, err := stageTaskPaths(dataDir, "sweep-", targets)
	if err != nil {
		return 0, err
	}
	return len(targets), stage.commit()
}

// sweepTaskFilesIfDue runs the sweep at start-up and then every
// taskFileSweepInterval, logging what it did.
func (s *Server) sweepTaskFilesIfDue(last *time.Time) {
	if !last.IsZero() && time.Since(*last) < taskFileSweepInterval {
		return
	}
	*last = time.Now()
	n, err := s.sweepRetiredTaskFiles()
	switch {
	case err != nil:
		log.Printf("[node %s] sweep retired task files: %v", s.node.NodeID, err)
	case n > 0:
		log.Printf("[node %s] removed %d local path(s) of deleted or archived tasks", s.node.NodeID, n)
	}
}
