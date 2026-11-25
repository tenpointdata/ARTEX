package main

import (
	"os"
	"testing"
	"time"

	"github.com/Autumn-27/artex/llmrec"
	"github.com/Autumn-27/norma/llm"
)

// TestTelemetryAgainstCollector drives the real start-up path (OTEL_* env →
// exporter → llmrec observer → final export) against a live collector. Opt-in:
// set ARTEX_OTLP_COLLECTOR_TEST=1 and OTEL_EXPORTER_OTLP_ENDPOINT, then check
// the collector's debug exporter output for artex.llm.calls and "chat m-e2e".
func TestTelemetryAgainstCollector(t *testing.T) {
	if os.Getenv("ARTEX_OTLP_COLLECTOR_TEST") != "1" || os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") == "" {
		t.Skip("set ARTEX_OTLP_COLLECTOR_TEST=1 and OTEL_EXPORTER_OTLP_ENDPOINT to run")
	}
	stop := startTelemetry("e2e")
	llmrec.NotifyForTest(llmrec.CallEvent{
		TaskID: "e2e-1", Worker: "worker", Model: "m-e2e", Profile: "p", Status: "ok",
		Start: time.Now().Add(-time.Second), Latency: time.Second,
		Usage: llm.Usage{InputTokens: 900, OutputTokens: 90, CacheReadTokens: 300},
	})
	stop() // final export; errors are logged
}
