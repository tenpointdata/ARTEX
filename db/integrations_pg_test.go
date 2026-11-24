package db

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// PG-backed: skipped when no database is configured.

func TestTicketTrackerConfigSealedAtRest(t *testing.T) {
	d := notifyTestDB(t)
	ctx := context.Background()
	tr := &TicketTracker{Name: "gh", Kind: "github", Enabled: true,
		Config: json.RawMessage(`{"repo":"acme/app","token":"ghp_supersecrettoken"}`)}
	id, err := d.SaveTicketTracker(ctx, tr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Exec(`DELETE FROM ticket_trackers WHERE id=$1`, id) })
	var raw string
	if err := d.QueryRow(`SELECT config::text FROM ticket_trackers WHERE id=$1`, id).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(raw, "ghp_supersecrettoken") {
		t.Fatalf("token stored in clear: %s", raw)
	}
	got, err := d.TicketTrackerByID(ctx, id)
	if err != nil || !strings.Contains(string(got.Config), "ghp_supersecrettoken") {
		t.Fatalf("open: %v %s", err, got.Config)
	}
	if err := d.DeleteTicketTracker(ctx, id); err != nil {
		t.Fatal(err)
	}
	if _, err := d.TicketTrackerByID(ctx, id); err != ErrTicketTrackerNotFound {
		t.Fatalf("want not found, got %v", err)
	}
}

func TestImportedFindingDedupNeverPromotes(t *testing.T) {
	d := notifyTestDB(t)
	ctx := context.Background()
	im := &ScanImport{Format: "nuclei-jsonl", ToolName: "nuclei", FileSHA256: "abc"}
	if err := d.CreateScanImport(ctx, im); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Exec(`DELETE FROM scan_imports WHERE id=$1`, im.ID) })
	f := ImportedFinding{ImportID: im.ID, VulnClass: "cve-2021-1", Name: "cve-2021-1", Severity: SeverityHigh,
		Summary: "nuclei: template cve-2021-1", Evidence: `"x"`, Worker: "import:nuclei", Tool: "nuclei", RuleID: "cve-2021-1",
		DedupKey: "imp-test-" + t.Name(), Record: "line 1", Note: "imported; not verified"}
	id, created, err := d.AddImportedFinding(ctx, f)
	if err != nil || !created {
		t.Fatalf("first insert: id=%d created=%v err=%v", id, created, err)
	}
	t.Cleanup(func() { d.Exec(`DELETE FROM findings WHERE id=$1`, id) })
	got, err := d.GetFinding(id)
	if err != nil {
		t.Fatal(err)
	}
	if got.VerificationStatus != VerificationUnverified || got.PoELevel != 0 || got.Status != FindingPending {
		t.Fatalf("imported finding must be unverified/pending/POE0: %+v", got)
	}
	// An operator later verifies and triages it; a re-import must not touch that.
	if err := d.SetFindingVerification(id, VerificationVerified, 3, ""); err != nil {
		t.Fatal(err)
	}
	f.Severity = SeverityCritical
	id2, created, err := d.AddImportedFinding(ctx, f)
	if err != nil || created || id2 != id {
		t.Fatalf("re-import: id=%d created=%v err=%v", id2, created, err)
	}
	got, _ = d.GetFinding(id)
	if got.Severity != SeverityHigh || got.VerificationStatus != VerificationVerified {
		t.Fatalf("re-import changed the finding: %+v", got)
	}
	src, err := d.FindingImportSourceByFinding(ctx, id)
	if err != nil || src == nil || src.Tool != "nuclei" || src.ImportID == nil || *src.ImportID != im.ID {
		t.Fatalf("provenance: %+v %v", src, err)
	}
	if _, _, err := d.AddImportedFinding(ctx, ImportedFinding{Severity: "info", DedupKey: "k"}); err != ErrImportedFindingInvalid {
		t.Fatalf("want invalid, got %v", err)
	}
}

func TestFindingTicketUpsert(t *testing.T) {
	d := notifyTestDB(t)
	ctx := context.Background()
	tid, err := d.SaveTicketTracker(ctx, &TicketTracker{Name: "dd", Kind: "defectdojo", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Exec(`DELETE FROM ticket_trackers WHERE id=$1`, tid) })
	fid, err := d.AddFinding(0, 0, "xss", "xss", SeverityLow, "s", "e", "w", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Exec(`DELETE FROM findings WHERE id=$1`, fid) })
	ft := &FindingTicket{FindingID: fid, TrackerID: tid, DedupKey: "artex-1", ExternalID: "7", SyncedStatus: FindingPending, RemoteOpen: true}
	if err := d.UpsertFindingTicket(ctx, ft); err != nil {
		t.Fatal(err)
	}
	if err := d.SetFindingTicketError(ctx, ft.ID, "boom"); err != nil {
		t.Fatal(err)
	}
	ft.SyncedStatus, ft.RemoteOpen, ft.RemoteResolution = FindingFixed, false, "fixed"
	if err := d.UpsertFindingTicket(ctx, ft); err != nil {
		t.Fatal(err)
	}
	got, err := d.FindingTicket(ctx, tid, fid)
	if err != nil || got == nil || got.RemoteOpen || got.SyncedStatus != FindingFixed || got.LastError != "" || got.ExternalID != "7" {
		t.Fatalf("got %+v err=%v", got, err)
	}
	all, _ := d.FindingTicketsByTracker(ctx, tid)
	if len(all) != 1 {
		t.Fatalf("want one link, got %d", len(all))
	}
}
