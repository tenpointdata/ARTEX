package db

import (
	"context"
	"testing"
)

// PG-backed: skipped when no database is configured.

func TestImportedFindingReReportRecordsLink(t *testing.T) {
	d := notifyTestDB(t)
	ctx := context.Background()
	taskA, err := d.CreateTask("link-a", "g", nil, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	taskB, err := d.CreateTask("link-b", "g", nil, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Exec(`DELETE FROM tasks WHERE id IN ($1,$2)`, taskA.ID, taskB.ID) })
	mk := func(task int64, sha string) *ScanImport {
		im := &ScanImport{Format: "nuclei-jsonl", ToolName: "nuclei", FileSHA256: sha, TaskID: &task}
		if err := d.CreateScanImport(ctx, im); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { d.Exec(`DELETE FROM scan_imports WHERE id=$1`, im.ID) })
		return im
	}
	imA, imB := mk(taskA.ID, "a"), mk(taskB.ID, "b")
	f := ImportedFinding{ImportID: imA.ID, TaskID: &taskA.ID, VulnClass: "xss", Name: "xss", Severity: SeverityHigh,
		Summary: "s", Evidence: "ev-a", Worker: "import:nuclei", Tool: "nuclei", RuleID: "r1",
		DedupKey: "imp-link-" + t.Name(), Record: "line 1", Note: "imported"}
	id, created, err := d.AddImportedFinding(ctx, f)
	if err != nil || !created {
		t.Fatalf("first: id=%d created=%v err=%v", id, created, err)
	}
	t.Cleanup(func() { d.Exec(`DELETE FROM findings WHERE id=$1`, id) })
	before, err := d.GetFinding(id)
	if err != nil {
		t.Fatal(err)
	}
	f2 := f
	f2.ImportID, f2.TaskID, f2.Evidence, f2.Record = imB.ID, &taskB.ID, "ev-b", "line 9"
	id2, created, err := d.AddImportedFinding(ctx, f2)
	if err != nil || created || id2 != id {
		t.Fatalf("re-report: id=%d created=%v err=%v", id2, created, err)
	}
	// Same import again is idempotent.
	if _, _, err := d.AddImportedFinding(ctx, f2); err != nil {
		t.Fatal(err)
	}
	after, _ := d.GetFinding(id)
	if after.VerificationStatus != before.VerificationStatus || after.PoELevel != before.PoELevel ||
		after.Evidence != before.Evidence || *after.TaskID != *before.TaskID {
		t.Fatalf("finding changed: before %+v after %+v", before, after)
	}
	src, err := d.FindingImportSourceByFinding(ctx, id)
	if err != nil || src == nil || src.ImportID == nil || *src.ImportID != imA.ID {
		t.Fatalf("source: %+v %v", src, err)
	}
	if len(src.Imports) != 2 {
		t.Fatalf("want 2 links, got %+v", src.Imports)
	}
	a, b := src.Imports[0], src.Imports[1]
	if a.ImportID != imA.ID || a.Record != "line 1" || a.Format != "nuclei-jsonl" || a.TaskID == nil || *a.TaskID != taskA.ID ||
		b.ImportID != imB.ID || b.Record != "line 9" || b.TaskID == nil || *b.TaskID != taskB.ID {
		t.Fatalf("links: %+v", src.Imports)
	}
}
