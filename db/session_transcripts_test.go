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
