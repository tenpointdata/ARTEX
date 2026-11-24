package db

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"
)

// archiveAuthFixture is a paused task with two artifact versions: v1 in force
// and v2 a revocation carrying v1's terms.
type archiveAuthFixture struct {
	d    *DB
	task *Task
	v1   *EngagementAuthorization
	v2   *EngagementAuthorization
}

func newArchiveAuthFixture(t *testing.T) *archiveAuthFixture {
	t.Helper()
	d, err := Open(testDSN(t))
	if err != nil {
		SkipPGUnavailable(t, err)
	}
	t.Cleanup(func() { d.Close() })
	task, err := d.CreateTaskWithOptions("archive authorization", "carry the artifact", TaskCreateOptions{Name: "auth archive"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = d.Exec(`DELETE FROM task_archives WHERE task_id=$1`, task.ID)
		_ = d.DeleteTask(task.ID)
	})
	if err := d.SetPaused(task.ID, true); err != nil {
		t.Fatal(err)
	}
	start := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	in := AuthorizationInput{
		Reference: "SOW-17", AuthorizedBy: "ciso@client.example", Note: "initial",
		Scope:      []AuthScopeEntry{{Kind: AuthScopeDomain, Value: "client.example"}, {Kind: AuthScopeCIDR, Value: "10.1.0.0/16"}},
		Techniques: []string{"recon", "scan"},
		NotBefore:  start, NotAfter: start.Add(48 * time.Hour),
	}
	v1, err := d.AddAuthorization(task.ID, in, false, "alice")
	if err != nil {
		t.Fatal(err)
	}
	in.Note = "revoked"
	v2, err := d.AddAuthorization(task.ID, in, true, "bob")
	if err != nil {
		t.Fatal(err)
	}
	return &archiveAuthFixture{d: d, task: task, v1: v1, v2: v2}
}

// archive runs the queue, claim, snapshot and complete steps.
func (f *archiveAuthFixture) archive(t *testing.T) (int64, *TaskArchiveSnapshot) {
	t.Helper()
	archive, err := f.d.QueueTaskArchive(f.task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if claimed, err := f.d.ClaimTaskArchiveJob(t.Context()); err != nil || claimed == nil || claimed.ID != archive.ID {
		t.Fatalf("claim = %+v, %v", claimed, err)
	}
	snapshot, err := f.d.SnapshotTaskArchive(f.task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.d.CompleteTaskArchive(archive.ID, snapshot, "/tmp/auth.tar.zst", "abc", 10, 5); err != nil {
		t.Fatal(err)
	}
	return archive.ID, snapshot
}

// restore runs the queue, claim, restore and complete steps.
func (f *archiveAuthFixture) restore(t *testing.T, archiveID int64, snapshot *TaskArchiveSnapshot) []string {
	t.Helper()
	if _, err := f.d.QueueTaskArchiveRestore(archiveID); err != nil {
		t.Fatal(err)
	}
	if claimed, err := f.d.ClaimTaskArchiveJob(t.Context()); err != nil || claimed == nil || claimed.ID != archiveID {
		t.Fatalf("restore claim = %+v, %v", claimed, err)
	}
	warnings, err := f.d.RestoreTaskArchive(archiveID, snapshot, 600)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.d.CompleteTaskArchiveRestore(archiveID); err != nil {
		t.Fatal(err)
	}
	return warnings
}

func (f *archiveAuthFixture) versions(t *testing.T) []*EngagementAuthorization {
	t.Helper()
	got, err := f.d.ListAuthorizations(f.task.ID)
	if err != nil {
		t.Fatal(err)
	}
	slices.Reverse(got) // oldest first
	return got
}

func (f *archiveAuthFixture) auditSeq(t *testing.T) int64 {
	t.Helper()
	var seq int64
	if err := f.d.QueryRow(`SELECT COALESCE(max(seq),0) FROM audit_chain`).Scan(&seq); err != nil {
		t.Fatal(err)
	}
	return seq
}

// assertSameTerms compares every field an artifact carries except the row id.
func assertSameTerms(t *testing.T, want, got *EngagementAuthorization) {
	t.Helper()
	w, g := *want, *got
	w.ID, g.ID = 0, 0
	wj, _ := json.Marshal(w)
	gj, _ := json.Marshal(g)
	if string(wj) != string(gj) {
		t.Fatalf("version %d differs:\nwant %s\n got %s", want.Version, wj, gj)
	}
}

func hasWarning(warnings []string, part string) bool {
	return slices.ContainsFunc(warnings, func(w string) bool { return strings.Contains(w, part) })
}

func TestTaskArchiveSnapshotCarriesAuthorizationAndKeepsRows(t *testing.T) {
	f := newArchiveAuthFixture(t)
	_, snapshot := f.archive(t)
	if snapshot.DataCounts["engagement_authorizations"] != 2 {
		t.Fatalf("snapshot counts = %v", snapshot.DataCounts)
	}
	var rows []struct {
		Version int  `json:"version"`
		Revoked bool `json:"revoked"`
		TaskID  int64
	}
	if err := json.Unmarshal(snapshot.Tables["engagement_authorizations"], &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].Version != 1 || rows[0].Revoked || rows[1].Version != 2 || !rows[1].Revoked {
		t.Fatalf("snapshot rows = %+v", rows)
	}
	// Archive completion leaves the artifact with the stub.
	got := f.versions(t)
	if len(got) != 2 {
		t.Fatalf("rows after archive = %d, want 2", len(got))
	}
	assertSameTerms(t, f.v1, got[0])
	assertSameTerms(t, f.v2, got[1])
}

func TestTaskArchiveRestoreRecreatesMissingAuthorization(t *testing.T) {
	f := newArchiveAuthFixture(t)
	// An audited neighbour in the same restore transaction: it keeps its own actor.
	var interceptID int64
	if err := f.d.QueryRow(`INSERT INTO intercept_pending(task_id,tool_name,status,decided_by,decision_source,decided_at)
VALUES($1,'http','allowed','human','unknown',now()) RETURNING id`, fmt.Sprint(f.task.ID)).Scan(&interceptID); err != nil {
		t.Fatal(err)
	}
	archiveID, snapshot := f.archive(t)
	if _, err := f.d.Exec(`DELETE FROM engagement_authorizations WHERE task_id=$1`, f.task.ID); err != nil {
		t.Fatal(err)
	}
	seq := f.auditSeq(t)
	warnings := f.restore(t, archiveID, snapshot)
	t.Cleanup(func() { _, _ = f.d.Exec(`DELETE FROM intercept_pending WHERE id=$1`, interceptID) })
	var interceptActor string
	if err := f.d.QueryRow(`SELECT actor FROM audit_chain WHERE seq>$1 AND kind='row.intercept_pending' AND subject=$2
AND payload::jsonb->>'op'='INSERT'`, seq, fmt.Sprint(interceptID)).Scan(&interceptActor); err != nil || interceptActor != "human" {
		t.Fatalf("restored intercept row actor = %q, %v; want human", interceptActor, err)
	}
	if !hasWarning(warnings, "re-created 2 engagement authorization versions") {
		t.Fatalf("warnings = %v", warnings)
	}
	got := f.versions(t)
	if len(got) != 2 {
		t.Fatalf("restored %d versions, want 2", len(got))
	}
	assertSameTerms(t, f.v1, got[0])
	assertSameTerms(t, f.v2, got[1])
	cur, err := f.d.CurrentAuthorization(f.task.ID)
	if err != nil || cur == nil || cur.Version != 2 || !cur.Revoked {
		t.Fatalf("current = %+v, %v", cur, err)
	}
	var inserts, byRestore int
	if err := f.d.QueryRow(`SELECT count(*), count(*) FILTER (WHERE actor='archive-restore') FROM audit_chain
WHERE seq>$1 AND kind='row.engagement_authorizations' AND payload::jsonb->>'op'='INSERT'
  AND subject IN (SELECT id::text FROM engagement_authorizations WHERE task_id=$2)`, seq, f.task.ID).Scan(&inserts, &byRestore); err != nil {
		t.Fatal(err)
	}
	if inserts != 2 || byRestore != 2 {
		var all string
		_ = f.d.QueryRow(`SELECT COALESCE(string_agg(actor||':'||kind||':'||subject, ', '),'') FROM audit_chain WHERE seq>$1`, seq).Scan(&all)
		t.Log(all)
		t.Fatalf("audit INSERT entries = %d, by archive-restore = %d, want 2 and 2", inserts, byRestore)
	}
	// The actor is scoped to the restore transaction.
	if _, err := f.d.AddAuthorization(f.task.ID, cur.Input(), false, "carol"); err != nil {
		t.Fatal(err)
	}
	var actor string
	if err := f.d.QueryRow(`SELECT actor FROM audit_chain WHERE kind='row.engagement_authorizations' ORDER BY seq DESC LIMIT 1`).Scan(&actor); err != nil || actor != "carol" {
		t.Fatalf("actor after restore = %q, %v", actor, err)
	}
}

func TestTaskArchiveRestoreLeavesPresentAuthorizationAlone(t *testing.T) {
	f := newArchiveAuthFixture(t)
	archiveID, snapshot := f.archive(t)
	seq := f.auditSeq(t)
	warnings := f.restore(t, archiveID, snapshot)
	if hasWarning(warnings, "engagement authorization") {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	got := f.versions(t)
	if len(got) != 2 || got[0].ID != f.v1.ID || got[1].ID != f.v2.ID {
		t.Fatalf("rows changed: %+v", got)
	}
	var entries int
	if err := f.d.QueryRow(`SELECT count(*) FROM audit_chain WHERE seq>$1 AND kind='row.engagement_authorizations'
AND subject IN ($2,$3)`, seq, fmt.Sprint(f.v1.ID), fmt.Sprint(f.v2.ID)).Scan(&entries); err != nil || entries != 0 {
		t.Fatalf("audit entries for untouched rows = %d, %v", entries, err)
	}
}

func TestTaskArchiveRestoreKeepsDifferingAuthorization(t *testing.T) {
	f := newArchiveAuthFixture(t)
	archiveID, snapshot := f.archive(t)
	// Out of band: v2 is replaced by a different v2 and v1 is gone.
	if _, err := f.d.Exec(`DELETE FROM engagement_authorizations WHERE task_id=$1`, f.task.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.d.Exec(`INSERT INTO engagement_authorizations(task_id,version,scope,not_before,not_after,created_by,note)
VALUES($1,2,'[]',now()-interval '1 hour',now()+interval '1 hour','dave','different')`, f.task.ID); err != nil {
		t.Fatal(err)
	}
	warnings := f.restore(t, archiveID, snapshot)
	if !hasWarning(warnings, "re-created 1 engagement authorization versions") ||
		!hasWarning(warnings, "version 2 differs from the archive") {
		t.Fatalf("warnings = %v", warnings)
	}
	got := f.versions(t)
	if len(got) != 2 || got[0].Version != 1 || got[1].Version != 2 || got[1].Note != "different" || got[1].Revoked {
		t.Fatalf("rows = %+v", got)
	}
}

func TestTaskArchiveRestoreWithoutAuthorizationKey(t *testing.T) {
	f := newArchiveAuthFixture(t)
	archiveID, snapshot := f.archive(t)
	delete(snapshot.Tables, "engagement_authorizations")
	delete(snapshot.DataCounts, "engagement_authorizations")
	warnings := f.restore(t, archiveID, snapshot)
	if hasWarning(warnings, "engagement authorization") {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if got := f.versions(t); len(got) != 2 {
		t.Fatalf("rows = %d, want 2", len(got))
	}

	// A task with no artifact at all archives and restores too.
	d := f.d
	task, err := d.CreateTaskWithOptions("archive no authorization", "none", TaskCreateOptions{Name: "plain"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = d.Exec(`DELETE FROM task_archives WHERE task_id=$1`, task.ID)
		_ = d.DeleteTask(task.ID)
	})
	if err := d.SetPaused(task.ID, true); err != nil {
		t.Fatal(err)
	}
	plain := &archiveAuthFixture{d: d, task: task}
	id, snap := plain.archive(t)
	if snap.DataCounts["engagement_authorizations"] != 0 {
		t.Fatalf("counts = %v", snap.DataCounts)
	}
	if warnings := plain.restore(t, id, snap); hasWarning(warnings, "engagement authorization") {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if got := plain.versions(t); len(got) != 0 {
		t.Fatalf("rows = %d, want 0", len(got))
	}
}
