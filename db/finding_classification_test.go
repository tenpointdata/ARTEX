package db

import (
	"context"
	"errors"
	"strconv"
	"testing"

	"github.com/Autumn-27/artex/vulnmeta"
)

// A classification patch round-trips through every read path (GetFinding, the
// list/export scans and the task report's quality lookup), a later patch keeps
// the parts it does not touch, an invalid patch changes nothing, and the stored
// score is the one computed from the vector.
func TestFindingClassificationRoundTrip(t *testing.T) {
	d, err := Open(testDSN(t))
	if err != nil {
		SkipPGUnavailable(t, err)
	}
	defer d.Close()
	ctx := context.Background()
	tk, err := d.CreateTask("e1 classification", "goal", nil, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.DeleteTask(tk.ID) })
	taskID := tk.ID
	nodeID, err := d.Exploration(tk.ExplorationID).AddNode(KindFinding, map[string]any{"summary": "x"}, 5, "confirmed", "worker", nil)
	if err != nil {
		t.Fatal(err)
	}
	id, err := d.AddFinding(taskID, nodeID, "sqli", "SQL injection", "high", "s", "e", "w", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = d.DeleteFinding(id) })

	f, _ := d.GetFinding(id)
	if !f.Classification.IsZero() {
		t.Fatalf("new finding is classified: %+v", f.Classification)
	}

	vec := "CVSS:4.0/AV:N/AC:L/AT:N/PR:N/UI:N/VC:H/VI:H/VA:H/SC:N/SI:N/SA:N"
	rem := "Use parameterized queries."
	tags := []vulnmeta.Tag{{Framework: "cwe", ID: "89"}, {Framework: "owasp-wstg", ID: "WSTG-INPV-05"}, {Framework: "mitre-attack", ID: "T1190"}}
	got, found, err := d.SetFindingClassification(ctx, id, vulnmeta.Patch{CVSSVector: &vec, Remediation: &rem, Tags: &tags,
		EPSS: &vulnmeta.EPSS{Probability: 0.42, Percentile: 0.9, Date: "2026-11-20", CVE: "CVE-2024-0001"}})
	if err != nil || !found {
		t.Fatalf("set: found=%v err=%v", found, err)
	}
	if got.CVSS.Score != 9.3 {
		t.Fatalf("score %v, want 9.3", got.CVSS.Score)
	}

	check := func(where string, c vulnmeta.Classification) {
		t.Helper()
		if c.CVSS == nil || c.CVSS.Vector != vec || c.CVSS.Score != 9.3 || c.CVSS.Rating != "critical" {
			t.Errorf("%s: cvss %+v", where, c.CVSS)
		}
		if c.EPSS == nil || c.EPSS.Probability != 0.42 || c.EPSS.Date != "2026-11-20" || c.EPSS.CVE != "CVE-2024-0001" {
			t.Errorf("%s: epss %+v", where, c.EPSS)
		}
		if c.Remediation != rem || len(c.Tags) != 3 || c.Tags[0] != (vulnmeta.Tag{Framework: "cwe", ID: "CWE-89"}) {
			t.Errorf("%s: remediation %q tags %v", where, c.Remediation, c.Tags)
		}
	}
	f, _ = d.GetFinding(id)
	check("GetFinding", f.Classification)
	list, err := d.ListFindingsForExport(FindingFilter{}, []int64{id})
	if err != nil || len(list) != 1 {
		t.Fatalf("export list: %v %d", err, len(list))
	}
	check("ListFindingsForExport", list[0].Classification)
	page, _, err := d.ListFindingsPage(FindingFilter{TaskID: strconv.FormatInt(taskID, 10)}, 1, 10)
	if err != nil || len(page) != 1 {
		t.Fatalf("page: %v %d", err, len(page))
	}
	check("ListFindingsPage", page[0].Classification)
	q, err := d.FindingQualityByNodeID(taskID)
	if err != nil {
		t.Fatal(err)
	}
	check("FindingQualityByNodeID", q[nodeID].Classification)

	// A partial patch keeps the rest; an invalid one changes nothing.
	clearEPSS := vulnmeta.Patch{EPSS: &vulnmeta.EPSS{}}
	if got, _, err = d.SetFindingClassification(ctx, id, clearEPSS); err != nil || got.EPSS != nil || got.CVSS == nil {
		t.Fatalf("clear epss: %+v %v", got, err)
	}
	bad := "CVSS:4.0/AV:Q"
	if _, found, err = d.SetFindingClassification(ctx, id, vulnmeta.Patch{CVSSVector: &bad}); !found || !errors.Is(err, vulnmeta.ErrInvalid) {
		t.Fatalf("bad vector: found=%v err=%v", found, err)
	}
	f, _ = d.GetFinding(id)
	if f.Classification.CVSS == nil || f.Classification.CVSS.Vector != vec {
		t.Fatalf("invalid patch changed the row: %+v", f.Classification)
	}
	if _, found, err = d.SetFindingClassification(ctx, -1, clearEPSS); found || err != nil {
		t.Fatalf("missing finding: found=%v err=%v", found, err)
	}
}
