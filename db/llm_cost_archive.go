package db

import (
	"encoding/json"
	"strconv"
	"time"
)

// archiveCostCell is one (profile, worker, model, UTC day) bucket of an archived
// task's LLM usage. Archives written since c-f4a69d carry these under
// aggregate_stats.cost_cells, so the cost view can group an archived task by
// every dimension and window it by day. It holds counts only, never content.
type archiveCostCell struct {
	ProfileName      string    `json:"profile_name"`
	Worker           string    `json:"worker"`
	Model            string    `json:"model"`
	Date             string    `json:"date"` // YYYY-MM-DD (UTC)
	Calls            int       `json:"calls"`
	Errors           int       `json:"errors"`
	InputTokens      int       `json:"input_tokens"`
	OutputTokens     int       `json:"output_tokens"`
	CacheReadTokens  int       `json:"cache_read_tokens"`
	CacheWriteTokens int       `json:"cache_write_tokens"`
	LatencyMs        int64     `json:"latency_ms"`
	FirstTs          time.Time `json:"first_ts"` // earliest call start (ts - latency_ms), as CostRow.FirstTs
	LastTs           time.Time `json:"last_ts"`
}

// archiveCostCellsQuery builds aggregate_stats.cost_cells at archive time. It
// matches the rows the other token aggregates match ($1 task id, $2 exploration).
const archiveCostCellsQuery = `SELECT COALESCE(jsonb_agg(to_jsonb(x)),'[]'::jsonb) FROM (
SELECT COALESCE(profile_name,'') profile_name, COALESCE(worker,'') worker, COALESCE(model,'') model,
       to_char(ts AT TIME ZONE 'UTC','YYYY-MM-DD') date, count(*) calls,
       count(*) FILTER (WHERE COALESCE(status,'ok') <> 'ok') errors,
       COALESCE(sum(input_tokens),0) input_tokens, COALESCE(sum(output_tokens),0) output_tokens,
       COALESCE(sum(cache_read),0) cache_read_tokens, COALESCE(sum(cache_write),0) cache_write_tokens,
       COALESCE(sum(latency_ms),0) latency_ms, min(ts - COALESCE(latency_ms,0) * interval '1 ms') first_ts, max(ts) last_ts
FROM llm_usage WHERE COALESCE(task_id,'')=$1 OR exploration_id=$2
GROUP BY 1,2,3,4 ORDER BY 4,1,2,3) x`

// CostArchiveCoverage says how much of the archived tasks' usage a cost view
// could include. Archives older than cost_cells keep only per-profile totals,
// with no worker, model or time; those count toward the view only for all time
// grouped by task or profile, and are counted as omitted otherwise.
type CostArchiveCoverage struct {
	Included int `json:"archived_included"` // archived tasks folded into the rows
	Omitted  int `json:"archived_omitted"`  // archived tasks with usage the view cannot place
}

// archivedCost is the cost-relevant part of one ready archive.
type archivedCost struct {
	TaskID        string
	CostCells     []archiveCostCell `json:"cost_cells"`
	TokenProfiles []ProfileUsage    `json:"token_profiles"`
}

func (d *DB) archivedCosts() ([]archivedCost, error) {
	rows, err := d.Query(`SELECT archive.task_id, jsonb_build_object(
  'cost_cells', archive.aggregate_stats->'cost_cells',
  'token_profiles', archive.aggregate_stats->'token_profiles')
FROM task_archives archive
JOIN tasks task ON task.id=archive.task_id
WHERE task.archived_at IS NOT NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []archivedCost
	for rows.Next() {
		var id int64
		var raw []byte
		if err := rows.Scan(&id, &raw); err != nil {
			return nil, err
		}
		var a archivedCost
		if err := json.Unmarshal(raw, &a); err != nil {
			return nil, err
		}
		a.TaskID = strconv.FormatInt(id, 10)
		out = append(out, a)
	}
	return out, rows.Err()
}

// foldArchivedCost adds archived tasks' usage into byKey. Call it after the live
// distinct-task count: an archived task's rows left llm_usage when it was
// archived, so it adds one task to each group it touches without overlap.
// days > 0 keeps a cell whose last call falls inside the window, so the window
// is exact to the UTC day for archived usage.
func (d *DB) foldArchivedCost(dim CostDimension, days int, byKey map[string]*CostRow, prices map[string]TokenPrices) (CostArchiveCoverage, error) {
	var cov CostArchiveCoverage
	archives, err := d.archivedCosts()
	if err != nil {
		return cov, err
	}
	since := time.Time{}
	if days > 0 {
		since = time.Now().Add(-time.Duration(days) * 24 * time.Hour)
	}
	for _, a := range archives {
		touched := map[string]bool{}
		if a.CostCells != nil {
			for _, c := range a.CostCells {
				if c.LastTs.Before(since) {
					continue
				}
				k := archiveCellKey(dim, a.TaskID, c)
				first, last := c.FirstTs, c.LastTs
				foldCost(costRowFor(byKey, k), CostRow{
					Calls: c.Calls, Errors: c.Errors, InputTokens: c.InputTokens, OutputTokens: c.OutputTokens,
					CacheReadTokens: c.CacheReadTokens, CacheWriteTokens: c.CacheWriteTokens,
					LatencyMs: c.LatencyMs, FirstTs: &first, LastTs: &last,
				}, prices[c.ProfileName])
				touched[k] = true
			}
		} else if legacyUsage(a.TokenProfiles) {
			if days > 0 || (dim != CostByTask && dim != CostByProfile) {
				cov.Omitted++
				continue
			}
			for _, p := range a.TokenProfiles {
				k := a.TaskID
				if dim == CostByProfile {
					k = p.ProfileName
				}
				foldCost(costRowFor(byKey, k), CostRow{
					Calls: p.Calls, InputTokens: p.InputTokens, OutputTokens: p.OutputTokens,
					CacheReadTokens: p.CacheReadTokens, CacheWriteTokens: p.CacheWriteTokens,
				}, prices[p.ProfileName])
				touched[k] = true
			}
		}
		for k := range touched {
			byKey[k].Tasks++
		}
		if len(touched) > 0 {
			cov.Included++
		}
	}
	return cov, nil
}

func archiveCellKey(dim CostDimension, taskID string, c archiveCostCell) string {
	switch dim {
	case CostByProfile:
		return c.ProfileName
	case CostByWorker:
		return c.Worker
	case CostByModel:
		return c.Model
	default:
		return taskID
	}
}

func legacyUsage(ps []ProfileUsage) bool {
	for _, p := range ps {
		if p.Calls > 0 || p.InputTokens > 0 || p.OutputTokens > 0 {
			return true
		}
	}
	return false
}

func costRowFor(byKey map[string]*CostRow, k string) *CostRow {
	row := byKey[k]
	if row == nil {
		row = &CostRow{Key: k, Label: k}
		byKey[k] = row
	}
	return row
}
