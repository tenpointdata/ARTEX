package db

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/Autumn-27/artex/transcriptsync"
)

// The fencing contract against PostgreSQL: Append succeeds only for the
// current holder at the stored position, a later Acquire fences the earlier
// holder, and the rows go with their exploration.
func TestSessionTranscriptsFencing(t *testing.T) {
	d, err := Open(testDSN(t))
	if err != nil {
		SkipPGUnavailable(t, err)
	}
	defer d.Close()
	ctx := context.Background()
	var expID int64
	if err := d.QueryRow(`INSERT INTO explorations(goal) VALUES ('transcript test') RETURNING id`).Scan(&expID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = d.Exec(`DELETE FROM explorations WHERE id=$1`, expID) })
	session := fmt.Sprintf("exp%d-worker-i%d", expID, time.Now().UnixNano())
	st := d.SessionTranscripts()

	if got, err := st.Acquire(ctx, session, "A"); err != nil || len(got) != 0 {
		t.Fatalf("first acquire = %q, %v", got, err)
	}
	if err := st.Append(ctx, session, "A", 0, [][]byte{[]byte(`{"n":1}`), []byte(`{"n":2}`)}); err != nil {
		t.Fatal(err)
	}
	if err := st.Append(ctx, session, "A", 0, [][]byte{[]byte(`{"n":9}`)}); !errors.Is(err, transcriptsync.ErrFenced) {
		t.Fatalf("append at a stale position = %v, want ErrFenced", err)
	}
	got, err := st.Acquire(ctx, session, "B")
	if err != nil || len(got) != 2 || string(got[1]) != `{"n":2}` {
		t.Fatalf("B acquire = %q, %v", got, err)
	}
	if err := st.Append(ctx, session, "A", 2, [][]byte{[]byte(`{"n":3}`)}); !errors.Is(err, transcriptsync.ErrFenced) {
		t.Fatalf("fenced holder append = %v, want ErrFenced", err)
	}
	if err := st.Append(ctx, session, "B", 2, [][]byte{[]byte(`{"n":3}`)}); err != nil {
		t.Fatal(err)
	}
	if lines, err := st.Lines(ctx, session); err != nil || len(lines) != 3 {
		t.Fatalf("lines = %q, %v", lines, err)
	}
	if ids, err := st.ExplorationSessions(ctx, expID); err != nil || len(ids) != 1 || ids[0] != session {
		t.Fatalf("exploration sessions = %v, %v", ids, err)
	}
	if _, err := d.Exec(`DELETE FROM explorations WHERE id=$1`, expID); err != nil {
		t.Fatal(err)
	}
	if lines, err := st.Lines(ctx, session); err != nil || len(lines) != 0 {
		t.Fatalf("lines after exploration delete = %q, %v", lines, err)
	}
}

// A retried Append whose first attempt committed succeeds; a session seeded
// before its exploration row exists links to it later, or is dropped when the
// restore fails.
func TestSessionTranscriptsRetryAndLink(t *testing.T) {
	d, err := Open(testDSN(t))
	if err != nil {
		SkipPGUnavailable(t, err)
	}
	defer d.Close()
	ctx := context.Background()
	st := d.SessionTranscripts()
	var expID int64
	if err := d.QueryRow(`SELECT COALESCE(max(id),0)+1000000 FROM explorations`).Scan(&expID); err != nil {
		t.Fatal(err)
	}
	session := fmt.Sprintf("exp%d-worker-i1", expID)
	t.Cleanup(func() {
		_, _ = d.Exec(`DELETE FROM session_transcripts WHERE session_id=$1`, session)
		_, _ = d.Exec(`DELETE FROM explorations WHERE id=$1`, expID)
	})
	if _, err := st.Acquire(ctx, session, "A"); err != nil {
		t.Fatal(err)
	}
	batch := [][]byte{[]byte(`{"r":1}`)}
	if err := st.Append(ctx, session, "A", 0, batch); err != nil {
		t.Fatal(err)
	}
	if err := st.Append(ctx, session, "A", 0, batch); err != nil {
		t.Fatalf("retry of a committed append = %v, want nil", err)
	}
	if err := st.Append(ctx, session, "A", 0, [][]byte{[]byte(`{"r":2}`)}); !errors.Is(err, transcriptsync.ErrFenced) {
		t.Fatalf("different lines at a stored position = %v, want ErrFenced", err)
	}

	// Unlinked (no exploration row yet): a failed restore drops it.
	if err := st.DeleteUnlinkedTranscripts(ctx, expID); err != nil {
		t.Fatal(err)
	}
	if lines, _ := st.Lines(ctx, session); len(lines) != 0 {
		t.Fatalf("unseeded session still has %d lines", len(lines))
	}
	// Seed again, then the exploration appears and the session links to it.
	if _, err := st.Acquire(ctx, session, "S"); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Exec(`INSERT INTO explorations(id, goal) VALUES ($1, 'restored')`, expID); err != nil {
		t.Fatal(err)
	}
	if err := st.LinkExplorationTranscripts(ctx, expID); err != nil {
		t.Fatal(err)
	}
	if ids, err := st.ExplorationSessions(ctx, expID); err != nil || len(ids) != 1 {
		t.Fatalf("linked sessions = %v, %v", ids, err)
	}
}
