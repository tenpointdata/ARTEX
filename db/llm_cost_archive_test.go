package db

import (
	"encoding/json"
	"fmt"
	"strconv"
	"testing"
	"time"
)

// An archived task's usage leaves llm_usage, so the cost view reads it from the
// archive aggregate: a cost_cells archive appears in every dimension and in a
// day window; a legacy archive (token_profiles only) appears for all time by
// task and profile, and is counted as omitted elsewhere.
func TestCostBreakdownIncludesArchivedTasks(t *testing.T) {
	d, err := Open(testDSN(t))
	if err != nil {
		SkipPGUnavailable(t, err)
	}
	t.Cleanup(func() { d.Close() })
	if err := d.EnsureLLMUsageTable(); err != nil {
		t.Fatal(err)
	}
	priced, _ := costProfiles(t, d)
	suffix := time.Now().UnixNano()
	planner, model := fmt.Sprintf("t-arch-planner-%d", suffix), fmt.Sprintf("t-arch-model-%d", suffix)

	cells := archivedCostTask(t, d, "cost archive cells")
	for _, r := range []LLMUsage{
		// 1.00 (see TestCostBreakdownGroupsAndPrices)
		{Worker: planner, InputTokens: 600_000, CacheRead: 200_000, OutputTokens: 40_000, LatencyMs: 1500, Status: "ok"},
		// 1.19
		{Worker: "worker", InputTokens: 400_000, CacheWrite: 300_000, OutputTokens: 60_000, LatencyMs: 500, Status: "error"},
	} {
		r.TaskID, r.ExplorationID, r.ProfileName, r.Model = strconv.FormatInt(cells.ID, 10), cells.ExplorationID, priced, model
		if err := d.InsertLLMUsage(&r); err != nil {
			t.Fatal(err)
		}
	}
	tx, err := d.Begin()
	if err != nil {
		t.Fatal(err)
	}
	stats, err := taskArchiveAggregates(tx, cells.ID, cells.ExplorationID)
	_ = tx.Rollback()
	if err != nil {
		t.Fatal(err)
	}
	archiveForCost(t, d, cells, stats)

	// 1M fresh input at 2.00 = 2.00; no worker, model or times in a legacy archive.
	legacy := archivedCostTask(t, d, "cost archive legacy")
	archiveForCost(t, d, legacy, map[string]any{"token_profiles": []ProfileUsage{
		{ProfileName: priced, Calls: 1, Tasks: 1, InputTokens: 1_000_000},
	}})
	cellsKey, legacyKey := strconv.FormatInt(cells.ID, 10), strconv.FormatInt(legacy.ID, 10)

	byTask := costReport(t, d, CostByTask, 1)
	row := findCostRow(t, byTask.Rows, cellsKey)
	if row.Calls != 2 || row.Errors != 1 || row.Tasks != 1 || row.LatencyMs != 2000 ||
		row.CacheWriteTokens != 300_000 || row.Label != "cost archive cells" || row.FirstTs == nil || row.LastTs == nil {
		t.Fatalf("archived task row %+v", row)
	}
	assertCost(t, row.CostEur, 2.19)
	if hasCostRow(byTask.Rows, legacyKey) || byTask.Omitted < 1 || byTask.Included < 1 {
		t.Fatalf("legacy archive in a day window: omitted=%d included=%d", byTask.Omitted, byTask.Included)
	}

	allTime := costReport(t, d, CostByTask, 0)
	leg := findCostRow(t, allTime.Rows, legacyKey)
	assertCost(t, leg.CostEur, 2.00)
	if leg.FirstTs != nil || leg.Tasks != 1 {
		t.Fatalf("legacy row %+v", leg)
	}

	taskRows := 0
	for _, r := range allTime.Rows {
		if r.Key != "" {
			taskRows++
		}
	}
	if allTime.Tasks != taskRows {
		t.Fatalf("distinct tasks %d, task rows %d", allTime.Tasks, taskRows)
	}
	prof := findCostRow(t, costReport(t, d, CostByProfile, 0).Rows, priced)
	assertCost(t, prof.CostEur, 4.19)
	if prof.Calls != 3 || prof.Tasks != 2 {
		t.Fatalf("profile row %+v", prof)
	}

	byWorker := costReport(t, d, CostByWorker, 0)
	// The cells task spans two workers, so the rows' task counts sum past the
	// distinct total, which counts it once.
	summed := 0
	for _, r := range byWorker.Rows {
		summed += r.Tasks
	}
	if summed <= byWorker.Tasks {
		t.Fatalf("worker rows sum %d tasks, distinct total %d", summed, byWorker.Tasks)
	}
	w := findCostRow(t, byWorker.Rows, planner)
	assertCost(t, w.CostEur, 1.00)
	if w.Tasks != 1 || byWorker.Omitted < 1 {
		t.Fatalf("worker row %+v omitted=%d", w, byWorker.Omitted)
	}
	m := findCostRow(t, costReport(t, d, CostByModel, 7).Rows, model)
	assertCost(t, m.CostEur, 2.19)
}

func costReport(t *testing.T, d *DB, dim CostDimension, days int) *CostReport {
	t.Helper()
	r, err := d.CostBreakdown(dim, days)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func hasCostRow(rows []CostRow, key string) bool {
	for _, r := range rows {
		if r.Key == key {
			return true
		}
	}
	return false
}

func archivedCostTask(t *testing.T, d *DB, name string) *Task {
	t.Helper()
	task, err := d.CreateTaskWithOptions(name, "cost view archive test", TaskCreateOptions{Name: name})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = d.Exec(`DELETE FROM llm_usage WHERE task_id=$1 OR exploration_id=$2`, strconv.FormatInt(task.ID, 10), task.ExplorationID)
		_, _ = d.Exec(`DELETE FROM task_archives WHERE task_id=$1`, task.ID)
		_ = d.DeleteTask(task.ID)
	})
	return task
}

// archiveForCost stores a ready archive with these aggregates and finishes it
// the way archiving does for usage: the live llm_usage rows are removed.
func archiveForCost(t *testing.T, d *DB, task *Task, stats map[string]any) {
	t.Helper()
	raw, err := json.Marshal(stats)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Exec(`INSERT INTO task_archives(task_id,state,aggregate_stats,archived_at) VALUES($1,'ready',$2,now())`, task.ID, raw); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Exec(`UPDATE tasks SET archived_at=now() WHERE id=$1`, task.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Exec(`DELETE FROM llm_usage WHERE task_id=$1 OR exploration_id=$2`, strconv.FormatInt(task.ID, 10), task.ExplorationID); err != nil {
		t.Fatal(err)
	}
}
