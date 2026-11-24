package db

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

// llm_usage.ts is when a call completed, so a group's wall time must start at
// the first call's start (ts - latency_ms), not its completion. Two sequential
// calls, the first one long: wall time = last end - first start, in both the
// live query and the archive cost_cells aggregate.
func TestCostWallTimeIncludesFirstCallLatency(t *testing.T) {
	d, err := Open(testDSN(t))
	if err != nil {
		SkipPGUnavailable(t, err)
	}
	t.Cleanup(func() { d.Close() })
	if err := d.EnsureLLMUsageTable(); err != nil {
		t.Fatal(err)
	}
	task := fmt.Sprintf("t-cost-wall-%d", time.Now().UnixNano())
	t.Cleanup(func() { d.Exec(`DELETE FROM llm_usage WHERE task_id=$1`, task) })

	// Seen live: started 21:47:31, completed 21:48:21 (50 s); the last call
	// completed 21:49:29. True span 118 s; min(ts) alone gives 68 s.
	base := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	firstStart := base
	firstEnd := base.Add(50 * time.Second)
	lastEnd := base.Add(118 * time.Second)
	for _, c := range []struct {
		latency int
		end     time.Time
	}{{50_000, firstEnd}, {3_000, lastEnd}} {
		u := LLMUsage{TaskID: task, Worker: "planner", Model: "m", LatencyMs: c.latency, Status: "ok"}
		if err := d.InsertLLMUsage(&u); err != nil {
			t.Fatal(err)
		}
		if _, err := d.Exec(`UPDATE llm_usage SET ts=$1 WHERE task_id=$2 AND latency_ms=$3`,
			c.end, task, c.latency); err != nil {
			t.Fatal(err)
		}
	}

	row := findCostRow(t, costReport(t, d, CostByTask, 1).Rows, task)
	if row.FirstTs == nil || row.LastTs == nil {
		t.Fatalf("times missing %+v", row)
	}
	if !row.FirstTs.Equal(firstStart) || !row.LastTs.Equal(lastEnd) {
		t.Fatalf("live first/last = %v/%v, want %v/%v", row.FirstTs, row.LastTs, firstStart, lastEnd)
	}
	if wall := row.LastTs.Sub(*row.FirstTs); wall != 118*time.Second {
		t.Fatalf("live wall time %v, want 118s", wall)
	}

	var raw []byte
	if err := d.QueryRow(archiveCostCellsQuery, task, int64(-1)).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var cells []archiveCostCell
	if err := json.Unmarshal(raw, &cells); err != nil {
		t.Fatal(err)
	}
	if len(cells) != 1 {
		t.Fatalf("cost cells %s", raw)
	}
	if c := cells[0]; !c.FirstTs.Equal(firstStart) || !c.LastTs.Equal(lastEnd) {
		t.Fatalf("archive first/last = %v/%v, want %v/%v", c.FirstTs, c.LastTs, firstStart, lastEnd)
	}
}
