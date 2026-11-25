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

// The stat cards narrow by lane and import like the list: under ?lane= and
// ?import_id= the card counts equal the filtered list's total.
func TestFindingStatsByLaneAndImport(t *testing.T) {
	d := notifyTestDB(t)
	ctx := context.Background()
	im := &ScanImport{Format: "sarif", ToolName: "semgrep", FileSHA256: "lane-stats"}
	if err := d.CreateScanImport(ctx, im); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Exec(`DELETE FROM scan_imports WHERE id=$1`, im.ID) })
	for i, lane := range []string{"sast", "sast", "sca", "runtime"} {
		sev := SeverityLow
		if i == 0 {
			sev = SeverityHigh
		}
		id, _, err := d.AddImportedFinding(ctx, ImportedFinding{ImportID: im.ID, VulnClass: "stats-" + lane, Name: "x",
			Severity: sev, Summary: "s", Worker: "import:semgrep", Tool: "semgrep", RuleID: "rule",
			DedupKey: "lane-stats-" + t.Name() + "-" + itoa(int64(i)), Lane: lane, Location: "src/x.go", Line: i + 1})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { d.Exec(`DELETE FROM findings WHERE id=$1`, id) })
	}
	worker, err := d.AddFinding(0, 0, "xss", "x", SeverityLow, "s", "e", "worker", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Exec(`DELETE FROM findings WHERE id=$1`, worker) })

	imp := itoa(im.ID)
	for _, c := range []struct {
		f                FindingFilter
		total, high, low int // -1 = only compare with the list
	}{
		{FindingFilter{ImportID: imp}, 4, 1, 3},
		{FindingFilter{ImportID: imp, Lane: "sast"}, 2, 1, 1},
		{FindingFilter{ImportID: imp, Lane: "sca"}, 1, 0, 1},
		{FindingFilter{ImportID: imp, Lane: "runtime"}, 1, 0, 1},
		{FindingFilter{Lane: "sast"}, -1, -1, -1},
		{FindingFilter{Lane: "runtime"}, -1, -1, -1},
		{FindingFilter{Lane: "bogus"}, 0, 0, 0},
	} {
		st, err := d.FindingStatsFor(c.f)
		if err != nil {
			t.Fatal(err)
		}
		_, total, err := d.ListFindingsPage(c.f, 1, 1)
		if err != nil {
			t.Fatal(err)
		}
		if st.Total != total {
			t.Fatalf("%+v: stats total %d, list total %d", c.f, st.Total, total)
		}
		if c.total >= 0 && (st.Total != c.total || st.High != c.high || st.Low != c.low) {
			t.Fatalf("%+v: stats = %+v, want total=%d high=%d low=%d", c.f, st, c.total, c.high, c.low)
		}
	}
	if st, err := d.FindingStatsFor(FindingFilter{ImportID: imp, Lane: "sca"}); err != nil ||
		len(st.VulnClasses) != 1 || st.VulnClasses[0] != "stats-sca" {
		t.Fatalf("sca vuln classes = %+v, %v", st, err)
	}
}
