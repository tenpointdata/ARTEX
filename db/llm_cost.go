package db

import (
	"fmt"
	"sort"
	"time"
)

// TokenPrices is one LLM profile's price list in EUR per 1M tokens.
type TokenPrices struct {
	Input      float64
	Output     float64
	CacheRead  float64
	CacheWrite float64
}

// Priced reports whether at least one price is set.
func (p TokenPrices) Priced() bool {
	return p.Input != 0 || p.Output != 0 || p.CacheRead != 0 || p.CacheWrite != 0
}

// Cost prices one token bucket. input already includes both cached subsets:
// Norma normalizes it to input + cache read + cache write (OpenAI prompt_tokens
// semantics). The fresh share is therefore input minus cache reads minus cache
// writes; charging either subset at full input price as well bills it twice.
// Clamped at zero for gateways that report the buckets disjointly.
func (p TokenPrices) Cost(input, output, cacheRead, cacheWrite int) float64 {
	fresh := input - cacheRead - cacheWrite
	if fresh < 0 {
		fresh = 0
	}
	return float64(fresh)/1e6*p.Input + float64(output)/1e6*p.Output +
		float64(cacheRead)/1e6*p.CacheRead + float64(cacheWrite)/1e6*p.CacheWrite
}

// ProfilePrices returns the price list of every LLM profile by name. llm_usage
// records the profile name, not its id; the first profile with a name wins, and
// duplicate names are a configuration error the operator sees as a weird
// number, not a crash.
func (d *DB) ProfilePrices() (map[string]TokenPrices, error) {
	rows, err := d.Query(`SELECT name, COALESCE(price_input_per_m,0), COALESCE(price_output_per_m,0), COALESCE(price_cache_read_per_m,0), COALESCE(price_cache_write_per_m,0) FROM llm_profiles ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]TokenPrices{}
	for rows.Next() {
		var n string
		var p TokenPrices
		if err := rows.Scan(&n, &p.Input, &p.Output, &p.CacheRead, &p.CacheWrite); err != nil {
			return nil, err
		}
		if _, seen := out[n]; !seen {
			out[n] = p
		}
	}
	return out, rows.Err()
}

// CostDimension is a column of llm_usage that the cost view groups by.
type CostDimension string

const (
	CostByTask    CostDimension = "task"
	CostByProfile CostDimension = "profile"
	CostByWorker  CostDimension = "worker"
	CostByModel   CostDimension = "model"
)

// costColumns maps each dimension to its SQL expression. It is the whitelist:
// a dimension not in it is refused, so no caller text reaches the query.
var costColumns = map[CostDimension]string{
	CostByTask:    "COALESCE(u.task_id,'')",
	CostByProfile: "COALESCE(u.profile_name,'')",
	CostByWorker:  "COALESCE(u.worker,'')",
	CostByModel:   "COALESCE(u.model,'')",
}

// ParseCostDimension validates a dimension name; "" means by task.
func ParseCostDimension(s string) (CostDimension, error) {
	if s == "" {
		return CostByTask, nil
	}
	d := CostDimension(s)
	if _, ok := costColumns[d]; !ok {
		return "", fmt.Errorf("unknown cost dimension %q (task, profile, worker, model)", s)
	}
	return d, nil
}

// CostRow is the spend and efficiency of one group of LLM calls.
type CostRow struct {
	Key              string `json:"key"`
	Label            string `json:"label"`
	Calls            int    `json:"calls"`
	Tasks            int    `json:"tasks"`
	Errors           int    `json:"errors"`
	InputTokens      int    `json:"input_tokens"` // includes cache read and write
	OutputTokens     int    `json:"output_tokens"`
	CacheReadTokens  int    `json:"cache_read_tokens"`
	CacheWriteTokens int    `json:"cache_write_tokens"`
	LatencyMs        int64  `json:"latency_ms"` // summed call latency: time waiting on the model
	// FirstTs is when the earliest call started (ts - latency_ms: llm_usage.ts is
	// the completion time); LastTs is when the latest call completed. Wall time is
	// LastTs - FirstTs. Both are nil when only legacy archive totals (no times) are
	// in the group.
	FirstTs       *time.Time `json:"first_ts"`
	LastTs        *time.Time `json:"last_ts"`
	CostEur       *float64   `json:"cost_eur"`       // nil when nothing in the group is priced
	UnpricedCalls int        `json:"unpriced_calls"` // calls from profiles without a price
}

// CostReport is the cost view: one row per group, and how much archived usage
// the rows include.
type CostReport struct {
	Rows []CostRow `json:"rows"`
	// Tasks is the distinct tasks across all rows (live in the window plus
	// archived ones included). Summing the rows' Tasks double-counts a task that
	// spans several profiles, workers or models.
	Tasks int `json:"tasks"`
	CostArchiveCoverage
}

// CostBreakdown groups the live llm_usage ledger plus archived tasks' usage by
// one dimension over the past days (days <= 0: all time) and prices each group
// from the profile price lists. Live rows are grouped by (dimension, profile)
// in SQL so each partial is priced with its own profile, then folded; distinct
// live tasks are counted per group in a second query. Archived usage comes from
// the archive aggregates (foldArchivedCost), whose coverage the report states.
func (d *DB) CostBreakdown(dim CostDimension, days int) (*CostReport, error) {
	col, ok := costColumns[dim]
	if !ok {
		return nil, fmt.Errorf("unknown cost dimension %q", dim)
	}
	prices, err := d.ProfilePrices()
	if err != nil {
		return nil, err
	}
	rows, err := d.Query(`
SELECT `+col+` AS k, COALESCE(u.profile_name,'') AS p,
       COUNT(*), COUNT(*) FILTER (WHERE COALESCE(u.status,'ok') <> 'ok'),
       COALESCE(SUM(u.input_tokens),0), COALESCE(SUM(u.output_tokens),0),
       COALESCE(SUM(u.cache_read),0), COALESCE(SUM(u.cache_write),0),
       COALESCE(SUM(u.latency_ms),0), MIN(u.ts - COALESCE(u.latency_ms,0) * interval '1 ms'), MAX(u.ts)
FROM llm_usage u
WHERE $1 <= 0 OR u.ts >= now() - ($1 * interval '1 day')
GROUP BY 1, 2`, days)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	byKey := map[string]*CostRow{}
	for rows.Next() {
		var k, prof string
		var p CostRow
		if err := rows.Scan(&k, &prof, &p.Calls, &p.Errors, &p.InputTokens, &p.OutputTokens,
			&p.CacheReadTokens, &p.CacheWriteTokens, &p.LatencyMs, &p.FirstTs, &p.LastTs); err != nil {
			return nil, err
		}
		foldCost(costRowFor(byKey, k), p, prices[prof])
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := d.countCostTasks(col, days, byKey); err != nil {
		return nil, err
	}
	all := map[string]*CostRow{"": {}}
	if err := d.countCostTasks("''", days, all); err != nil {
		return nil, err
	}
	cov, err := d.foldArchivedCost(dim, days, byKey, prices)
	if err != nil {
		return nil, err
	}
	out := make([]CostRow, 0, len(byKey))
	for _, r := range byKey {
		out = append(out, *r)
	}
	if dim == CostByTask {
		if err := d.labelTasks(out); err != nil {
			return nil, err
		}
	}
	sortCostRows(out)
	return &CostReport{Rows: out, Tasks: all[""].Tasks + cov.Included, CostArchiveCoverage: cov}, nil
}

// countCostTasks sets the number of distinct non-empty task ids per group.
func (d *DB) countCostTasks(col string, days int, byKey map[string]*CostRow) error {
	rows, err := d.Query(`
SELECT `+col+`, COUNT(DISTINCT NULLIF(u.task_id,''))
FROM llm_usage u
WHERE $1 <= 0 OR u.ts >= now() - ($1 * interval '1 day')
GROUP BY 1`, days)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var k string
		var n int
		if err := rows.Scan(&k, &n); err != nil {
			return err
		}
		if r := byKey[k]; r != nil {
			r.Tasks = n
		}
	}
	return rows.Err()
}

// foldCost adds one (group, profile) partial into its group row.
func foldCost(row *CostRow, p CostRow, pr TokenPrices) {
	row.Calls += p.Calls
	row.Errors += p.Errors
	row.InputTokens += p.InputTokens
	row.OutputTokens += p.OutputTokens
	row.CacheReadTokens += p.CacheReadTokens
	row.CacheWriteTokens += p.CacheWriteTokens
	row.LatencyMs += p.LatencyMs
	if p.FirstTs != nil && (row.FirstTs == nil || p.FirstTs.Before(*row.FirstTs)) {
		row.FirstTs = p.FirstTs
	}
	if p.LastTs != nil && (row.LastTs == nil || p.LastTs.After(*row.LastTs)) {
		row.LastTs = p.LastTs
	}
	if !pr.Priced() {
		row.UnpricedCalls += p.Calls
		return
	}
	c := pr.Cost(p.InputTokens, p.OutputTokens, p.CacheReadTokens, p.CacheWriteTokens)
	if row.CostEur != nil {
		c += *row.CostEur
	}
	row.CostEur = &c
}

// labelTasks replaces task ids with task names where the task still exists.
func (d *DB) labelTasks(out []CostRow) error {
	rows, err := d.Query(`SELECT id::text, name FROM tasks WHERE name <> ''`)
	if err != nil {
		return err
	}
	defer rows.Close()
	names := map[string]string{}
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			return err
		}
		names[id] = name
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for i := range out {
		if n, ok := names[out[i].Key]; ok {
			out[i].Label = n
		}
	}
	return nil
}

// sortCostRows orders by cost, then total tokens, then key.
func sortCostRows(out []CostRow) {
	cost := func(r CostRow) float64 {
		if r.CostEur == nil {
			return -1
		}
		return *r.CostEur
	}
	sort.Slice(out, func(i, j int) bool {
		if ci, cj := cost(out[i]), cost(out[j]); ci != cj {
			return ci > cj
		}
		ti := out[i].InputTokens + out[i].OutputTokens
		tj := out[j].InputTokens + out[j].OutputTokens
		if ti != tj {
			return ti > tj
		}
		return out[i].Key < out[j].Key
	})
}
