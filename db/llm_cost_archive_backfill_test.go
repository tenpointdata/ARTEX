package db

import (
	"fmt"
	"strconv"
	"testing"
	"time"
)

// A legacy archive (token_profiles only) is omitted from every view but All by
// task/profile; after its cold llm_usage rows are backfilled into cost_cells it
// is placed in every dimension and window, and archived_omitted drops to what
// other legacy archives in the database account for (0 on a clean database).
func TestBackfillArchiveCostCellsClearsOmitted(t *testing.T) {
	d, err := Open(testDSN(t))
	if err != nil {
		SkipPGUnavailable(t, err)
	}
	t.Cleanup(func() { d.Close() })
	if err := d.EnsureLLMUsageTable(); err != nil {
		t.Fatal(err)
	}
	priced, _ := costProfiles(t, d)
	model := fmt.Sprintf("t-backfill-model-%d", time.Now().UnixNano())
	task := archivedCostTask(t, d, "cost backfill legacy")
	taskKey := strconv.FormatInt(task.ID, 10)
	for _, r := range []LLMUsage{
		{Worker: "planner", InputTokens: 600_000, CacheRead: 200_000, OutputTokens: 40_000, LatencyMs: 1500, Status: "ok"},
		{Worker: "worker", InputTokens: 400_000, CacheWrite: 300_000, OutputTokens: 60_000, LatencyMs: 500, Status: "error"},
	} {
		r.TaskID, r.ExplorationID, r.ProfileName, r.Model = taskKey, task.ExplorationID, priced, model
		if err := d.InsertLLMUsage(&r); err != nil {
			t.Fatal(err)
		}
	}
	// The cold rows exactly as a package manifest holds them.
	usageRows, _, err := queryArchiveRows(d, `SELECT * FROM llm_usage WHERE COALESCE(task_id,'')=$1 OR exploration_id=$2 ORDER BY id`, taskKey, task.ExplorationID)
	if err != nil {
		t.Fatal(err)
	}
	archiveForCost(t, d, task, map[string]any{"token_profiles": []ProfileUsage{
		{ProfileName: priced, Calls: 2, Tasks: 1, InputTokens: 1_000_000, OutputTokens: 100_000},
	}})
	var archiveID int64
	if err := d.QueryRow(`SELECT id FROM task_archives WHERE task_id=$1`, task.ID).Scan(&archiveID); err != nil {
		t.Fatal(err)
	}
	missing := func() (others int, ours bool) {
		cs, err := d.ArchivesMissingCostCells()
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range cs {
			if c.ArchiveID == archiveID {
				ours = true
			} else {
				others++
			}
		}
		return others, ours
	}
	others, ours := missing()
	if !ours {
		t.Fatal("legacy archive not listed for backfill")
	}
	if r := costReport(t, d, CostByWorker, 7); r.Omitted != others+1 {
		t.Fatalf("before backfill omitted=%d, want %d", r.Omitted, others+1)
	}

	// No rows in the package: nothing written, the archive stays legacy.
	if wrote, err := d.BackfillArchiveCostCells(archiveID, nil); err != nil || wrote {
		t.Fatalf("empty rows wrote=%v err=%v", wrote, err)
	}
	if wrote, err := d.BackfillArchiveCostCells(archiveID, usageRows); err != nil || !wrote {
		t.Fatalf("backfill wrote=%v err=%v", wrote, err)
	}
	if wrote, err := d.BackfillArchiveCostCells(archiveID, usageRows); err != nil || wrote {
		t.Fatalf("second backfill wrote=%v err=%v", wrote, err)
	}
	others, ours = missing()
	if ours {
		t.Fatal("backfilled archive still listed")
	}

	for _, dim := range []CostDimension{CostByTask, CostByProfile, CostByWorker, CostByModel} {
		for _, days := range []int{0, 1, 7, 30, 90} {
			r := costReport(t, d, dim, days)
			if r.Omitted != others {
				t.Fatalf("%s/%dd omitted=%d, want %d", dim, days, r.Omitted, others)
			}
		}
	}
	row := findCostRow(t, costReport(t, d, CostByTask, 1).Rows, taskKey)
	if row.Calls != 2 || row.Errors != 1 || row.LatencyMs != 2000 || row.CacheWriteTokens != 300_000 || row.FirstTs == nil {
		t.Fatalf("backfilled task row %+v", row)
	}
	assertCost(t, row.CostEur, 2.19)
	assertCost(t, findCostRow(t, costReport(t, d, CostByWorker, 1).Rows, "planner").CostEur, 1.00)
	m := findCostRow(t, costReport(t, d, CostByModel, 30).Rows, model)
	assertCost(t, m.CostEur, 2.19)
	if m.Tasks != 1 {
		t.Fatalf("model row %+v", m)
	}
}
