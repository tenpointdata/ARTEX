package agent

import (
	"context"
	"errors"
	"log"

	"github.com/Autumn-27/artex/transcriptsync"
)

// SetTranscriptMirror shares this worker's per-intent transcripts through m
// (E7: any node resumes any intent). nil = local disk only (single process).
func (w *Worker) SetTranscriptMirror(m *transcriptsync.Mirror) { w.mirror = m }

// leaseTranscript takes the run's lease on its session before the resume reads
// the local file, so the file holds the shared history. The returned release
// flushes the last lines; call it after the session stops writing. Without a
// mirror both are no-ops.
func (w *Worker) leaseTranscript(ctx context.Context, sessionID string) (release func(), err error) {
	if w.mirror == nil || w.tx == nil {
		return func() {}, nil
	}
	lease, err := w.mirror.Begin(ctx, w.tx.MainPath(sessionID), sessionID)
	if err != nil {
		return nil, err
	}
	return func() {
		if err := lease.Close(); errors.Is(err, transcriptsync.ErrFenced) {
			log.Printf("[worker] transcript %s: taken over by another node; this run's tail was not kept", sessionID)
		} else if err != nil {
			log.Printf("[worker] transcript %s: final push: %v", sessionID, err)
		}
	}, nil
}
