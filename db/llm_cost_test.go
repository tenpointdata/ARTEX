package db

import (
	"fmt"
	"testing"
	"time"
)

// CostBreakdown prices each (group, profile) partial with its own profile,
// counts unpriced calls and errors, and labels tasks by name.
func TestCostBreakdownGroupsAndPrices(t *testing.T) {
	d, err := Open(testDSN(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	priced, unpriced := costProfiles(t, d)
	task := fmt.Sprintf("t-cost-task-%d", time.Now().UnixNano())
	t.Cleanup(func() { d.Exec(`DELETE FROM llm_usage WHERE task_id=$1`, task) })
	for _, r := range []LLMUsage{
		// priced: fresh 400k×2.00 + 200k×0.20 + 40k×4.00 = 1.00
		{ProfileName: priced, Worker: "planner", InputTokens: 600_000, CacheRead: 200_000, OutputTokens: 40_000, LatencyMs: 1500, Status: "ok"},
		// priced: fresh 100k×2.00 + 300k×2.50 + 60k×4.00 = 1.19
		{ProfileName: priced, Worker: "worker", InputTokens: 400_000, CacheWrite: 300_000, OutputTokens: 60_000, LatencyMs: 500, Status: "error"},
		{ProfileName: unpriced, Worker: "worker", InputTokens: 1000, OutputTokens: 10, LatencyMs: 250, Status: "ok"},
	} {
		r.TaskID, r.Model = task, "m"
		if err := d.InsertLLMUsage(&r); err != nil {
			t.Fatal(err)
		}
	}

	byTask, err := d.CostBreakdown(CostByTask, 1)
	if err != nil {
		t.Fatal(err)
	}
	row := findCostRow(t, byTask, task)
	if row.Calls != 3 || row.Tasks != 1 || row.Errors != 1 || row.UnpricedCalls != 1 ||
		row.InputTokens != 1_001_000 || row.CacheReadTokens != 200_000 || row.LatencyMs != 2250 {
		t.Fatalf("task row %+v", row)
	}
	assertCost(t, row.CostEur, 2.19)
	if row.LastTs.Before(row.FirstTs) || row.Label != task {
		t.Fatalf("times/label %+v", row)
	}

	byWorker, err := d.CostBreakdown(CostByWorker, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range byWorker {
		if r.Key == "planner" && (r.CostEur == nil || *r.CostEur < 1.0-1e-9) {
			t.Fatalf("planner row %+v", r)
		}
	}

	if _, err := ParseCostDimension("task_id; DROP TABLE llm_usage"); err == nil {
		t.Fatal("unknown dimension accepted")
	}
	if dim, err := ParseCostDimension(""); err != nil || dim != CostByTask {
		t.Fatalf("default dimension %q %v", dim, err)
	}
}

func findCostRow(t *testing.T, rows []CostRow, key string) CostRow {
	t.Helper()
	for _, r := range rows {
		if r.Key == key {
			return r
		}
	}
	t.Fatalf("no row %q in %d rows", key, len(rows))
	return CostRow{}
}
