package db

import (
	"context"
	"testing"
)

// The lane filter (E10): sast / sca match their code-scanner imports, runtime
// matches ARTEX's own findings and runtime-scanner imports; each listed row
// carries its import source, and each import row counts its findings per lane.
func TestFindingFilterByLane(t *testing.T) {
	d := notifyTestDB(t)
	ctx := context.Background()
	im := &ScanImport{Format: "sarif", ToolName: "semgrep", FileSHA256: "lane"}
	if err := d.CreateScanImport(ctx, im); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Exec(`DELETE FROM scan_imports WHERE id=$1`, im.ID) })
	add := func(lane, key string) int64 {
		id, _, err := d.AddImportedFinding(ctx, ImportedFinding{ImportID: im.ID, VulnClass: "x", Name: "x",
			Severity: SeverityLow, Summary: "s", Worker: "import:semgrep", Tool: "semgrep", RuleID: "rule." + key,
			DedupKey: "lane-filter-" + t.Name() + "-" + key, Lane: lane, Location: "src/" + key + ".go", Line: 7})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { d.Exec(`DELETE FROM findings WHERE id=$1`, id) })
		return id
	}
	sast, sca, rt := add("sast", "a"), add("sca", "b"), add("runtime", "c")
	worker, err := d.AddFinding(0, 0, "xss", "x", SeverityLow, "s", "e", "worker", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Exec(`DELETE FROM findings WHERE id=$1`, worker) })

	list := func(lane string) map[int64]*DBFinding {
		rows, err := d.ListFindingsForExport(FindingFilter{Lane: lane}, nil)
		if err != nil {
			t.Fatal(err)
		}
		out := map[int64]*DBFinding{}
		for _, r := range rows {
			out[r.ID] = r
		}
		return out
	}
	for lane, want := range map[string]int64{"sast": sast, "sca": sca} {
		got := list(lane)
		if got[want] == nil {
			t.Fatalf("lane %s misses finding %d", lane, want)
		}
		for _, other := range []int64{sast, sca, rt, worker} {
			if other != want && got[other] != nil {
				t.Fatalf("lane %s lists finding %d", lane, other)
			}
		}
	}
	got := list("runtime")
	if got[rt] == nil || got[worker] == nil || got[sast] != nil || got[sca] != nil {
		t.Fatalf("runtime lane: rt=%v worker=%v sast=%v sca=%v", got[rt] != nil, got[worker] != nil, got[sast] != nil, got[sca] != nil)
	}
	if got[worker].ImportSource != nil {
		t.Fatalf("worker finding has an import source: %+v", got[worker].ImportSource)
	}
	if s := got[rt].ImportSource; s == nil || s.Lane != "runtime" || s.Tool != "semgrep" {
		t.Fatalf("runtime import source = %+v", s)
	}
	if len(list("bogus")) != 0 {
		t.Fatal("an unknown lane matched findings")
	}

	page, _, err := d.ListFindingsPage(FindingFilter{Lane: "sast", ImportID: itoa(im.ID)}, 1, 20)
	if err != nil || len(page) != 1 || page[0].ImportSource == nil || page[0].ImportSource.Location != "src/a.go" || page[0].ImportSource.Line != 7 {
		t.Fatalf("page = %+v, %v", page, err)
	}
	one, err := d.GetFinding(sca)
	if err != nil || one.ImportSource == nil || one.ImportSource.Lane != "sca" {
		t.Fatalf("GetFinding(sca) = %+v, %v", one, err)
	}

	imports, _, err := d.ListScanImports(ctx, 500, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range imports {
		if row.ID == im.ID {
			if row.LaneCounts["sast"] != 1 || row.LaneCounts["sca"] != 1 || row.LaneCounts["runtime"] != 1 {
				t.Fatalf("lane counts = %v", row.LaneCounts)
			}
			return
		}
	}
	t.Fatalf("import %d not listed", im.ID)
}
