package server

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/Autumn-27/artex/transcriptsync"
)

// Worker transcripts shared across nodes (E7 follow-up c-29ff19,
// docs/DISTRIBUTED.md "Transcripts"). The local JSONL file under
// <dataDir>/transcripts stays the SDK's write path; on a distributed node each
// worker run leases its session from PostgreSQL (migration 0025), so a node
// that takes over a claim, reruns a reopened intent, or serves the human
// message path resumes from the last node's history. A single-process
// deployment (RoleAll) keeps local files only.

// transcriptMirror is this node's transcript mirror, or nil when it does not
// share transcripts.
func (m *Manager) transcriptMirror() *transcriptsync.Mirror {
	if m == nil || m.pg == nil || !m.node.Distributed() {
		return nil
	}
	return &transcriptsync.Mirror{
		Backend: m.pg.SessionTranscripts(),
		OnError: func(session string, err error) {
			log.Printf("[transcript] %s: push: %v", session, err)
		},
	}
}

func (m *Manager) transcriptDir() string { return filepath.Join(m.dir, "transcripts") }

// gatherTranscripts writes an exploration's shared transcripts to this node's
// disk so the task archive, which packs local files, carries the ones written
// on worker nodes too. A local file the store extends is replaced; one that
// already holds the stored lines is kept.
func (m *Manager) gatherTranscripts(ctx context.Context, explorationID int64) error {
	if m.transcriptMirror() == nil {
		return nil
	}
	st := m.pg.SessionTranscripts()
	sessions, err := st.ExplorationSessions(ctx, explorationID)
	if err != nil {
		return fmt.Errorf("list shared transcripts: %w", err)
	}
	for _, session := range sessions {
		lines, err := st.Lines(ctx, session)
		if err != nil {
			return fmt.Errorf("read shared transcript %s: %w", session, err)
		}
		if err := transcriptsync.Materialize(filepath.Join(m.transcriptDir(), session+".jsonl"), lines); err != nil {
			return fmt.Errorf("write transcript %s: %w", session, err)
		}
	}
	return nil
}

// seedTranscripts stores the worker transcripts a restored archive put on this
// node's disk, so the restored task's intents resume on any node. It runs
// before the database restore makes the task claimable, so no node can start
// an intent of it from an empty store first; the rows are linked to the
// exploration once its row exists (LinkExplorationTranscripts).
func (m *Manager) seedTranscripts(ctx context.Context, explorationID int64) error {
	mirror := m.transcriptMirror()
	if mirror == nil {
		return nil
	}
	entries, err := os.ReadDir(m.transcriptDir())
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	prefix := fmt.Sprintf("exp%d-worker-", explorationID)
	var errs []error
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, ".jsonl") {
			continue
		}
		session := strings.TrimSuffix(name, ".jsonl")
		lease, err := mirror.Begin(ctx, filepath.Join(m.transcriptDir(), name), session)
		if err == nil {
			err = lease.Close()
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", session, err))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("seed restored transcripts: %v", errs)
	}
	return nil
}

// linkTranscripts ties seeded transcripts to the restored exploration row.
func (m *Manager) linkTranscripts(explorationID int64) error {
	if m.transcriptMirror() == nil {
		return nil
	}
	return m.pg.SessionTranscripts().LinkExplorationTranscripts(context.Background(), explorationID)
}

// unseedTranscripts drops what seedTranscripts stored when the restore fails.
func (m *Manager) unseedTranscripts(explorationID int64) error {
	if m.transcriptMirror() == nil {
		return nil
	}
	return m.pg.SessionTranscripts().DeleteUnlinkedTranscripts(context.Background(), explorationID)
}
