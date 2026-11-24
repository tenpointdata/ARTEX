package db

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"strconv"

	"github.com/Autumn-27/artex/transcriptsync"
)

// SessionTranscripts is the PostgreSQL transcriptsync.Backend (migration 0025):
// session transcripts shared by every node, so an intent claimed on one node
// resumes on another (E7, docs/DISTRIBUTED.md).
type SessionTranscripts struct{ d *DB }

// SessionTranscripts returns the database's transcript backend.
func (d *DB) SessionTranscripts() *SessionTranscripts { return &SessionTranscripts{d: d} }

var _ transcriptsync.Backend = (*SessionTranscripts)(nil)

// sessionExploration ties exp<id>-… sessions to their exploration so they go
// with the task on delete and archive (ON DELETE CASCADE).
var sessionExploration = regexp.MustCompile(`^exp(\d+)-`)

func explorationOfSession(session string) any {
	m := sessionExploration.FindStringSubmatch(session)
	if m == nil {
		return nil
	}
	n, err := strconv.ParseInt(m[1], 10, 64)
	if err != nil {
		return nil
	}
	return n
}

// Acquire makes holder the session's writer and returns its stored lines.
func (s *SessionTranscripts) Acquire(ctx context.Context, session, holder string) ([][]byte, error) {
	tx, err := s.d.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	exp := explorationOfSession(session)
	if exp != nil { // a session of a deleted exploration is not stored
		var ok bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM explorations WHERE id=$1)`, exp).Scan(&ok); err != nil {
			return nil, err
		}
		if !ok {
			exp = nil
		}
	}
	var n int
	if err := tx.QueryRowContext(ctx, `INSERT INTO session_transcripts(session_id, exploration_id, holder) VALUES ($1,$2,$3)
ON CONFLICT (session_id) DO UPDATE SET holder=EXCLUDED.holder, updated_at=now()
RETURNING lines`, session, exp, holder).Scan(&n); err != nil {
		return nil, err
	}
	lines, err := sessionLines(ctx, tx, session, n)
	if err != nil {
		return nil, err
	}
	return lines, tx.Commit()
}

// Append stores lines from position from when holder still holds the session.
func (s *SessionTranscripts) Append(ctx context.Context, session, holder string, from int, lines [][]byte) error {
	if len(lines) == 0 {
		return nil
	}
	tx, err := s.d.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `UPDATE session_transcripts SET lines=lines+$4, updated_at=now()
WHERE session_id=$1 AND holder=$2 AND lines=$3`, session, holder, from, len(lines))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return transcriptsync.ErrFenced
	}
	seqs := make([]int64, len(lines))
	texts := make([]string, len(lines))
	for i, ln := range lines {
		seqs[i], texts[i] = int64(from+i), string(ln)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO session_transcript_lines(session_id, seq, line)
SELECT $1, s, l FROM unnest($2::int[], $3::text[]) AS u(s, l)
ON CONFLICT (session_id, seq) DO UPDATE SET line=EXCLUDED.line`, session, seqs, texts); err != nil {
		return err
	}
	return tx.Commit()
}

// Lines reads a session's stored lines without acquiring it.
func (s *SessionTranscripts) Lines(ctx context.Context, session string) ([][]byte, error) {
	var n int
	err := s.d.QueryRowContext(ctx, `SELECT lines FROM session_transcripts WHERE session_id=$1`, session).Scan(&n)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return sessionLines(ctx, s.d, session, n)
}

// ExplorationSessions lists the stored sessions of an exploration.
func (s *SessionTranscripts) ExplorationSessions(ctx context.Context, explorationID int64) ([]string, error) {
	rows, err := s.d.QueryContext(ctx, `SELECT session_id FROM session_transcripts WHERE exploration_id=$1 ORDER BY session_id`, explorationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

type queryer interface {
	QueryContext(ctx context.Context, q string, args ...any) (*sql.Rows, error)
}

// sessionLines reads the first n lines (rows past n belong to no committed
// Append and are ignored).
func sessionLines(ctx context.Context, q queryer, session string, n int) ([][]byte, error) {
	rows, err := q.QueryContext(ctx, `SELECT line FROM session_transcript_lines WHERE session_id=$1 AND seq<$2 ORDER BY seq`, session, n)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([][]byte, 0, n)
	for rows.Next() {
		var l string
		if err := rows.Scan(&l); err != nil {
			return nil, err
		}
		out = append(out, []byte(l))
	}
	return out, rows.Err()
}
