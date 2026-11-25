import { costMetrics, fmtDuration, fmtEur, fmtInt, fmtPct, sortCostRows, totalRow } from "./cost-metrics.ts";
import assert from "node:assert/strict";
import test from "node:test";

const row = (over) => ({
  key: "k",
  label: "",
  calls: 4,
  tasks: 1,
  errors: 0,
  input_tokens: 1000,
  output_tokens: 250,
  cache_read_tokens: 400,
  cache_write_tokens: 100,
  latency_ms: 2000,
  first_ts: "2026-11-20T09:00:00Z",
  last_ts: "2026-11-20T09:01:30Z",
  cost_eur: 2.5,
  unpriced_calls: 0,
  ...over,
});

test("costMetrics derives ratios, wall time and EUR per 1M tokens", () => {
  const m = costMetrics(row());
  assert.equal(m.tokens, 1250);
  assert.equal(m.cacheHit, 0.4);
  assert.equal(m.outIn, 0.25);
  assert.equal(m.eurPerMTok, 2000);
  assert.equal(m.wallMs, 90_000);
  assert.equal(m.avgLatencyMs, 500);
});

test("costMetrics returns null instead of NaN for empty denominators", () => {
  const m = costMetrics(
    row({ input_tokens: 0, output_tokens: 0, calls: 0, cost_eur: null, first_ts: null, last_ts: null }),
  );
  assert.deepEqual([m.cacheHit, m.outIn, m.eurPerMTok, m.wallMs, m.avgLatencyMs], [null, null, null, null, null]);
  assert.equal(costMetrics(row({ cost_eur: 1, input_tokens: 0, output_tokens: 0 })).eurPerMTok, null);
});

test("totalRow sums groups, takes the outer timestamps and the given distinct task count", () => {
  const total = totalRow(
    [
      row({ key: "a", tasks: 2, first_ts: "2026-11-20T10:00:00Z", last_ts: "2026-11-20T11:00:00Z" }),
      row({ key: "b", tasks: 2, cost_eur: null, unpriced_calls: 4, first_ts: null, last_ts: null }),
      row({ key: "c", tasks: 1, cost_eur: 1.25, first_ts: "2026-11-19T08:00:00Z", last_ts: "2026-11-20T09:00:00Z" }),
    ],
    3,
  );
  assert.equal(total.tasks, 3, "tasks is the distinct count, not the 5 the rows sum to");
  assert.equal(total.calls, 12);
  assert.equal(total.input_tokens, 3000);
  assert.equal(total.latency_ms, 6000);
  assert.equal(total.unpriced_calls, 4);
  assert.equal(total.cost_eur, 3.75);
  assert.equal(total.first_ts, "2026-11-19T08:00:00Z");
  assert.equal(total.last_ts, "2026-11-20T11:00:00Z");
});

test("totalRow keeps cost null when nothing is priced and timestamps null when none are known", () => {
  const total = totalRow([row({ cost_eur: null, first_ts: null, last_ts: null })], 1);
  assert.equal(total.cost_eur, null);
  assert.equal(total.first_ts, null);
  assert.equal(costMetrics(total).wallMs, null);
  assert.equal(totalRow([], 0).cost_eur, null);
});

test("sortCostRows sorts both ways with nulls last and ties on tokens", () => {
  const rows = [
    row({ key: "cheap", cost_eur: 1 }),
    row({ key: "unpriced", cost_eur: null }),
    row({ key: "dear", cost_eur: 9 }),
    row({ key: "unpriced-big", cost_eur: null, input_tokens: 9000 }),
  ];
  const keys = (rs) => rs.map((r) => r.key);
  assert.deepEqual(keys(sortCostRows(rows, "cost", "desc")), ["dear", "cheap", "unpriced-big", "unpriced"]);
  assert.deepEqual(keys(sortCostRows(rows, "cost", "asc")), ["cheap", "dear", "unpriced-big", "unpriced"]);
  assert.deepEqual(keys(rows), ["cheap", "unpriced", "dear", "unpriced-big"], "input is not mutated");
});

test("sortCostRows sorts names by label, falling back to key, case-insensitively", () => {
  const rows = [row({ key: "2", label: "beta" }), row({ key: "Alpha", label: "" }), row({ key: "1", label: "Gamma" })];
  assert.deepEqual(
    sortCostRows(rows, "name", "asc").map((r) => r.key),
    ["Alpha", "2", "1"],
  );
});

test("formatters", () => {
  assert.equal(fmtInt(999), "999");
  assert.equal(fmtInt(12_345), "12.3k");
  assert.equal(fmtInt(2_500_000), "2.50M");
  assert.equal(fmtPct(null), "—");
  assert.equal(fmtPct(0.1234), "12.3%");
  assert.equal(fmtEur(null), "—");
  assert.equal(fmtEur(1.5), "€1.50");
  assert.equal(fmtDuration(null), "—");
  assert.equal(fmtDuration(380), "380ms");
  assert.equal(fmtDuration(4200), "4.2s");
  assert.equal(fmtDuration(185_000), "3m 05s");
  assert.equal(fmtDuration(3_720_000), "1h 02m");
});
