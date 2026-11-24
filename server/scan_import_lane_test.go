package server

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/Autumn-27/artex/db"
)

// TestCodeScanJoinsTaskGraphAndReport is the E10 done criterion: a SAST run
// (semgrep SARIF) and an SCA run (trivy/osv SARIF) imported into a task
// produce findings with provenance that sit in the task's exploration graph,
// anchored to the runtime asset the code serves, and appear in the same task
// report as the task's runtime finding.
func TestCodeScanJoinsTaskGraphAndReport(t *testing.T) {
	s, runtime, request := trafficEvidenceServer(t)
	pg := s.m.pg
	var taskID int64
	if err := pg.QueryRow(`SELECT task_id FROM findings WHERE id=$1`, runtime.FindingID).Scan(&taskID); err != nil {
		t.Fatal(err)
	}
	asset, err := pg.Assets().UpsertHTTPService(db.UpsertHTTPServiceReq{URL: "https://shop.e10.test", TaskID: taskID})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = pg.Exec(`DELETE FROM assets WHERE id=$1`, asset) })

	type outcome struct {
		Import      db.ScanImport `json:"import"`
		NewFindings []int64       `json:"new_findings"`
	}
	imp := func(file string) outcome {
		t.Helper()
		data, err := os.ReadFile("../scanimport/testdata/" + file)
		if err != nil {
			t.Fatal(err)
		}
		w := request("POST", fmt.Sprintf("/api/integrations/imports?format=sarif&task_id=%d&asset_ids=%d&file_name=%s", taskID, asset, file), string(data))
		if w.Code != 200 {
			t.Fatalf("import %s: %d %s", file, w.Code, w.Body.String())
		}
		var out outcome
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _, _ = pg.Exec(`DELETE FROM scan_imports WHERE id=$1`, out.Import.ID) })
		return out
	}
	sast, sca := imp("semgrep.sarif"), imp("trivy-sca.sarif")
	for _, out := range []outcome{sast, sca} {
		if out.Import.FactNodeID == nil || len(out.NewFindings) == 0 {
			t.Fatalf("import did not join the graph: %+v", out)
		}
		for _, id := range out.NewFindings {
			f, err := pg.GetFinding(id)
			if err != nil {
				t.Fatal(err)
			}
			if f.NodeID == nil || f.TaskID == nil || *f.TaskID != taskID || len(f.AssetIDs) != 1 || f.AssetIDs[0] != asset {
				t.Fatalf("finding %d not in the task graph on the anchor asset: %+v", id, f)
			}
			if f.VerificationStatus != db.VerificationUnverified || f.PoELevel != 0 {
				t.Fatalf("imported finding must stay unverified: %+v", f)
			}
			var yields, anchored int
			_ = pg.QueryRow(`SELECT count(*) FROM exploration_edges WHERE src_id=$1 AND dst_id=$2 AND rel='yields'`, *out.Import.FactNodeID, *f.NodeID).Scan(&yields)
			_ = pg.QueryRow(`SELECT count(*) FROM exploration_anchors WHERE node_id=$1 AND asset_id=$2`, *f.NodeID, asset).Scan(&anchored)
			if yields != 1 || anchored != 1 {
				t.Fatalf("finding node %d: yields=%d anchored=%d", *f.NodeID, yields, anchored)
			}
		}
	}
	srcs, err := pg.ImportSourcesByFindingIDs(t.Context(), append(append([]int64{}, sast.NewFindings...), sca.NewFindings...))
	if err != nil {
		t.Fatal(err)
	}
	lanes := map[string]int{}
	for _, src := range srcs {
		lanes[src.Lane]++
		if src.Location == "" || src.Record == "" {
			t.Fatalf("code provenance missing: %+v", src)
		}
	}
	if lanes["sast"] != 4 || lanes["sca"] != 2 {
		t.Fatalf("lanes %v, want sast 4 (3 semgrep + 1 trivy misconfig), sca 2", lanes)
	}

	w := request("GET", fmt.Sprintf("/api/report?task=%d", taskID), "")
	if w.Code != 200 {
		t.Fatalf("report: %d %s", w.Code, w.Body.String())
	}
	md := w.Body.String()
	for _, want := range []string{
		"Evidence API", // the runtime finding
		"- Confirmed findings: **7**",
		"runtime testing 1, code scan (SAST) 4, dependency scan (SCA) 2",
		"at `app/views/orders.py:42` (import #",
		"dependency scan (SCA), imported from trivy rule `CVE-2022-24999` at `web/package-lock.json:1`",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("report lacks %q:\n%s", want, md)
		}
	}
	if strings.Contains(md, "Ignore previous instructions") && !strings.Contains(md, `title: "Ignore previous instructions`) {
		t.Error("untrusted scanner text must appear only quoted in evidence")
	}

	// Anchors need a task, and must exist.
	if w := request("POST", fmt.Sprintf("/api/integrations/imports?format=sarif&asset_ids=%d", asset), "{}"); w.Code != 400 {
		t.Fatalf("anchors without a task: %d", w.Code)
	}
	data, _ := os.ReadFile("../scanimport/testdata/semgrep.sarif")
	if w := request("POST", fmt.Sprintf("/api/integrations/imports?format=sarif&task_id=%d&asset_ids=987654321", taskID), string(data)); w.Code != 400 {
		t.Fatalf("unknown anchor: %d %s", w.Code, w.Body.String())
	}
}
