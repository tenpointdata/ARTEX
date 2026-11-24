# Cost view and OpenTelemetry export

Both are built on the always-on `llm_usage` ledger: one row per LLM call (task, worker lane, model,
profile, latency, input/output/cache-read/cache-write tokens, status), written by `llmrec`. Neither
carries prompt or response content.

## Cost & efficiency page

**Function → Cost & efficiency** (`web/src/app/(main)/function/cost/`) and `GET /api/llm/cost?by=&days=`
(`server/llm_cost.go`, `db/llm_cost.go`).

- `by`: `task` (default) | `profile` (provider) | `worker` (role: planner, worker, mainagent, goals,
  judge, …) | `model`. Anything else is a 400; the dimension is a whitelist, never interpolated text.
- `days`: window; `0` = all time; default 30.
- Each row: calls, distinct tasks, errors (status ≠ ok), the four token buckets, summed call latency
  ("LLM time"), first/last call (wall time = last − first), `cost_eur` and `unpriced_calls`.
- Cost uses the per-profile prices (EUR per 1M tokens, LLM settings). Each (group, profile) partial is
  priced with its own profile, so a task that used two providers is priced correctly. `input_tokens`
  includes both cache buckets; only the fresh remainder is billed at the input price
  (`db.TokenPrices.Cost`, shared with the dashboard's per-profile card). `cost_eur` is null when no call
  in the group is priced.
- Efficiency in the UI: cache hit rate = cache_read / input; output/input ratio; EUR per 1M tokens;
  average latency per call.
- Archived tasks: their usage leaves `llm_usage` when archived, so the view folds in the archive
  aggregate (`db/llm_cost_archive.go`). Archives record `aggregate_stats.cost_cells`, one bucket per
  (profile, worker, model, UTC day) with calls, errors, tokens, latency and first/last call, so they
  count in every dimension; a day window keeps a bucket whose last call is inside it (exact to the UTC
  day). Archives made before cost_cells existed hold only per-profile totals: they count under All
  time by task or profile, and are reported as `archived_omitted` (the page says how many) otherwise.
  Their rows have `first_ts`/`last_ts` null. The response also carries `archived_included` and
  `tasks`, the distinct task total (the rows' `tasks` overlap across profiles, workers and models).

## OpenTelemetry export

Off unless an OTLP endpoint is configured. Standard variables (read once at start-up,
`otlp.ConfigFromEnv`):

| Variable | Meaning |
| --- | --- |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | Collector base URL, e.g. `http://otel-collector:4318`; `/v1/traces` and `/v1/metrics` are appended |
| `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` / `…_METRICS_ENDPOINT` | Full per-signal URLs (override the base) |
| `OTEL_EXPORTER_OTLP_HEADERS` | `k=v,k2=v2` (percent-decoded), e.g. `authorization=Bearer%20…` |
| `OTEL_EXPORTER_OTLP_PROTOCOL` | Must be `http/json` if set (the only protocol spoken; gRPC/protobuf are refused with a log line) |
| `OTEL_SERVICE_NAME`, `OTEL_RESOURCE_ATTRIBUTES` | Resource identity (default service `artex`; `service.version` is added) |
| `OTEL_METRIC_EXPORT_INTERVAL` | Export period in ms (default 15000) |
| `OTEL_EXPORTER_OTLP_TIMEOUT` | Per-request timeout in ms (default 10000) |
| `OTEL_TRACES_EXPORTER=none`, `OTEL_METRICS_EXPORTER=none`, `OTEL_SDK_DISABLED=true` | Turn signals off |

What is sent per LLM call (`cmd/artex/telemetry.go`):

- Metrics (cumulative), labels `gen_ai.operation.name`, `gen_ai.request.model`, `artex.llm.profile`,
  `artex.llm.worker` (+ `artex.llm.status` where noted):
  - `artex.llm.calls` `{call}` (+status)
  - `artex.llm.tokens` `{token}` with `artex.llm.token_kind` = `input_fresh` | `cache_read` |
    `cache_write` | `output` (the first three sum to input)
  - `gen_ai.client.token.usage` histogram with `gen_ai.token.type` = `input` | `output`
  - `gen_ai.client.operation.duration` histogram, seconds (+status)
- Traces: one client span `chat <model>` per call with the same labels, `artex.task_id` and the
  `gen_ai.usage.*` token counts; status error when the call failed. All calls of one task share a trace
  id derived from the task id (`otlp.TraceIDFor`), so a task is one trace in Jaeger/Tempo.
- Task ids are on spans only, never metric labels, to keep series cardinality bounded.
- Cost is not exported: prices live in ARTEX; compute it in the backend from `artex.llm.tokens`, or
  use the cost page.

The `otlp` package is a dependency-free OTLP/HTTP-JSON exporter (counters, explicit-bucket
histograms, spans) with no ARTEX knowledge; it is reusable by other code. Spans queue in memory
(bounded at 2048, overflow dropped and logged); a failed export drops that batch and logs it; metrics
are cumulative so the next export carries the totals. Verified against
`otel/opentelemetry-collector` 0.165.0 (debug exporter) with the opt-in test:

```sh
docker run --rm -p 4318:4318 -v $PWD/otel.yaml:/etc/otelcol/config.yaml otel/opentelemetry-collector
ARTEX_OTLP_COLLECTOR_TEST=1 OTEL_EXPORTER_OTLP_ENDPOINT=http://127.0.0.1:4318 \
  go test ./cmd/artex/ -run TestTelemetryAgainstCollector -v
```

The collector is a destination the operator chooses; the egress noise governor and scope gate apply to
agent traffic toward targets, not to this server-side export.
