package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"strconv"
	"testing"
	"time"
)

// auditStallBound is how long an audited write may take while another
// transaction holds audited writes open. Without contention each call takes
// milliseconds; before migration 0018 they waited for the other commit.
const auditStallBound = 3 * time.Second

// withinBound runs fn and fails the test if it does not return in time.
// onStall runs before the failure so a blocked fn can be released.
func withinBound(t *testing.T, what string, onStall func(), fn func() error) {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- fn() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("%s: %v", what, err)
		}
	case <-time.After(auditStallBound):
		onStall()
		<-done
		t.Fatalf("%s: stalled behind an open audited transaction", what)
	}
}

func insertInterceptTx(t *testing.T, tx *sql.Tx, taskRef string) int64 {
	t.Helper()
	var id int64
	if err := tx.QueryRow(`INSERT INTO intercept_pending(task_id, agent_name, tool_name, tool_input, reason)
VALUES ($1, 'worker', 'Bash', '{"command":"id"}', 'test') RETURNING id`, taskRef).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

// An archive deletes a task's intercept rows early in a long transaction. Gate
// decisions, new approval rows and login entries on other tasks must not wait
// for it, and the deletions must still be chained when it commits.
func TestAuditChainArchiveDoesNotStallGatePG(t *testing.T) {
	d := openTargetDB(t)
	ctx := context.Background()
	archived := targetTask(t, d, TaskCreateOptions{})
	other := targetTask(t, d, TaskCreateOptions{})
	archivedRef := strconv.FormatInt(archived.ID, 10)
	gone, err := d.CreateInterceptPending(0, 0, archivedRef, "worker", "Bash", []byte(`{"command":"id"}`), "test")
	if err != nil {
		t.Fatal(err)
	}
	before, err := d.VerifyAudit(ctx)
	if err != nil || !before.OK {
		t.Fatalf("verify before = %+v, %v", before, err)
	}

	// The archive's early step (CompleteTaskArchive), held open.
	archive, err := d.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Rollback() //nolint:errcheck
	if _, err := archive.Exec(`DELETE FROM intercept_pending WHERE COALESCE(task_id,'')=$1`, archivedRef); err != nil {
		t.Fatal(err)
	}

	release := func() { _ = archive.Rollback() }
	otherRef := strconv.FormatInt(other.ID, 10)
	var id int64
	withinBound(t, "new approval row", release, func() error {
		var err error
		id, err = d.CreateInterceptPending(0, 0, otherRef, "worker", "Bash", []byte(`{"command":"id"}`), "test")
		return err
	})
	withinBound(t, "gate decision", release, func() error {
		_, err := d.ResolveInterceptBy(id, "allowed", "allow", "ok", "human", "reviewer-x", "127.0.0.1")
		return err
	})
	withinBound(t, "login entry", release, func() error {
		_, err := d.AppendAudit("tester", AuditLoginOK, "tester", nil)
		return err
	})

	if err := archive.Commit(); err != nil {
		t.Fatal(err)
	}
	rows, err := d.ListAudit(AuditFilter{AfterSeq: before.HeadSeq, Kind: "row.intercept_pending", Subject: strconv.FormatInt(gone, 10)})
	if err != nil || len(rows) != 1 {
		t.Fatalf("archive deletion entries = %+v, %v", rows, err)
	}
	var entry struct {
		Op  string         `json:"op"`
		Row map[string]any `json:"row"`
	}
	if err := json.Unmarshal(rows[0].Payload, &entry); err != nil || entry.Op != "DELETE" || entry.Row["task_id"] != archivedRef {
		t.Fatalf("deletion entry = %s", rows[0].Payload)
	}
	after, err := d.VerifyAudit(ctx)
	if err != nil || !after.OK || after.HeadSeq != before.HeadSeq+4 {
		t.Fatalf("verify after = %+v (before head %d), %v", after, before.HeadSeq, err)
	}
}

// A rolled-back audited write leaves no entry, and its queued entry does not
// take the head.
func TestAuditChainRollbackLeavesNoEntryPG(t *testing.T) {
	d := openTargetDB(t)
	task := targetTask(t, d, TaskCreateOptions{})
	before, err := d.VerifyAudit(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	tx, err := d.Begin()
	if err != nil {
		t.Fatal(err)
	}
	insertInterceptTx(t, tx, strconv.FormatInt(task.ID, 10))
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	after, err := d.VerifyAudit(context.Background())
	if err != nil || !after.OK || after.HeadSeq != before.HeadSeq {
		t.Fatalf("verify = %+v, before head %d, %v", after, before.HeadSeq, err)
	}
}

// Two transactions that each write an intercept row and need one other lock
// must not deadlock: tx1 writes, tx2 takes the lock and writes, tx1 waits on
// the lock, tx2 commits. Before 0018 tx2's write waited on the head tx1 held.
func TestAuditChainNoLockOrderDeadlockPG(t *testing.T) {
	d := openTargetDB(t)
	task := targetTask(t, d, TaskCreateOptions{})
	taskRef := strconv.FormatInt(task.ID, 10)
	const lockKey = 0x4152_5445_5830 // arbitrary advisory key for this test
	tx1, err := d.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx1.Rollback() //nolint:errcheck
	tx2, err := d.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx2.Rollback() //nolint:errcheck
	release := func() { _ = tx1.Rollback(); _ = tx2.Rollback() }

	insertInterceptTx(t, tx1, taskRef)
	if _, err := tx2.Exec(`SELECT pg_advisory_xact_lock($1)`, lockKey); err != nil {
		t.Fatal(err)
	}
	withinBound(t, "second writer's intercept row", release, func() error {
		_, err := tx2.Exec(`INSERT INTO intercept_pending(task_id, agent_name, tool_name, tool_input, reason)
VALUES ($1, 'worker', 'Bash', '{"command":"id"}', 'test')`, taskRef)
		return err
	})
	waiting := make(chan error, 1)
	go func() {
		_, err := tx1.Exec(`SELECT pg_advisory_xact_lock($1)`, lockKey)
		waiting <- err
	}()
	withinBound(t, "second writer's commit", release, tx2.Commit)
	select {
	case err := <-waiting:
		if err != nil {
			t.Fatalf("first writer's lock: %v", err)
		}
	case <-time.After(auditStallBound):
		release()
		t.Fatal("first writer still waiting after the second committed")
	}
	if err := tx1.Commit(); err != nil {
		t.Fatal(err)
	}
	v, err := d.VerifyAudit(context.Background())
	if err != nil || !v.OK {
		t.Fatalf("verify = %+v, %v", v, err)
	}
}
