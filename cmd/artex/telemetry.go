package main

import (
	"context"
	"log"
	"time"

	"github.com/Autumn-27/artex/llmrec"
	"github.com/Autumn-27/artex/otlp"
)

// startTelemetry exports every metered LLM call to an OpenTelemetry collector
// when the standard OTEL_EXPORTER_OTLP_* variables name one. It returns the
// function that makes the final export on shutdown (a no-op when off). A bad
// configuration is logged and leaves export off; it never stops the server.
func startTelemetry(version string) func() {
	cfg, ok, err := otlp.ConfigFromEnv("artex")
	if err != nil {
		log.Printf("[otlp] disabled: %v", err)
		return func() {}
	}
	if !ok {
		return func() {}
	}
	cfg.Resource = append(cfg.Resource, otlp.String("service.version", version))
	exp := otlp.New(cfg)
	exp.Start()
	llmrec.SetObserver(func(ev llmrec.CallEvent) { exportLLMCall(exp, ev) })
	log.Printf("[otlp] exporting LLM telemetry (traces=%q metrics=%q, every %s)", cfg.TracesURL, cfg.MetricsURL, cfg.Interval)
	return func() {
		llmrec.SetObserver(nil)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := exp.Shutdown(ctx); err != nil {
			log.Printf("[otlp] final export: %v", err)
		}
	}
}

// exportLLMCall maps one metered call onto the OpenTelemetry GenAI semantic
// conventions (gen_ai.*) plus ARTEX counters that keep the cache buckets apart.
// Labels only: no prompt or response content leaves the process. Task ids are
// on spans, not metric series, to keep metric cardinality bounded; every call
// of one task shares a trace id derived from the task id.
func exportLLMCall(exp *otlp.Exporter, ev llmrec.CallEvent) {
	worker := ev.Worker
	if worker == "" {
		worker = "other"
	}
	labels := []otlp.Attr{
		otlp.String("gen_ai.operation.name", "chat"),
		otlp.String("gen_ai.request.model", ev.Model),
		otlp.String("artex.llm.profile", ev.Profile),
		otlp.String("artex.llm.worker", worker),
	}
	withStatus := append(append([]otlp.Attr(nil), labels...), otlp.String("artex.llm.status", ev.Status))
	exp.Add("artex.llm.calls", "{call}", "Metered LLM calls", 1, withStatus...)
	u := ev.Usage
	fresh := u.InputTokens - u.CacheReadTokens - u.CacheWriteTokens
	if fresh < 0 {
		fresh = 0
	}
	for _, b := range []struct {
		kind string
		n    int
	}{{"input_fresh", fresh}, {"cache_read", u.CacheReadTokens}, {"cache_write", u.CacheWriteTokens}, {"output", u.OutputTokens}} {
		if b.n > 0 {
			exp.Add("artex.llm.tokens", "{token}", "LLM tokens by kind (input_fresh + cache_read + cache_write = input)",
				float64(b.n), append(append([]otlp.Attr(nil), labels...), otlp.String("artex.llm.token_kind", b.kind))...)
		}
	}
	for _, b := range []struct {
		kind string
		n    int
	}{{"input", u.InputTokens}, {"output", u.OutputTokens}} {
		exp.Record("gen_ai.client.token.usage", "{token}", "Tokens used per LLM call", float64(b.n), otlp.TokenBounds,
			append(append([]otlp.Attr(nil), labels...), otlp.String("gen_ai.token.type", b.kind))...)
	}
	exp.Record("gen_ai.client.operation.duration", "s", "LLM call duration", ev.Latency.Seconds(), otlp.DefaultBounds, withStatus...)

	span := otlp.Span{
		Name: "chat " + ev.Model, Kind: otlp.SpanKindClient,
		Start: ev.Start, End: ev.Start.Add(ev.Latency),
		Attrs: append(append([]otlp.Attr(nil), labels...),
			otlp.String("artex.task_id", ev.TaskID),
			otlp.Int("gen_ai.usage.input_tokens", int64(u.InputTokens)),
			otlp.Int("gen_ai.usage.output_tokens", int64(u.OutputTokens)),
			otlp.Int("gen_ai.usage.cache_read_input_tokens", int64(u.CacheReadTokens)),
			otlp.Int("gen_ai.usage.cache_creation_input_tokens", int64(u.CacheWriteTokens)),
		),
	}
	if ev.TaskID != "" {
		span.TraceID = otlp.TraceIDFor("artex-task:" + ev.TaskID)
	}
	if ev.Status != "ok" {
		span.Error = ev.Status
	}
	exp.Span(span)
}
