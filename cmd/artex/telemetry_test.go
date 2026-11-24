package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Autumn-27/artex/llmrec"
	"github.com/Autumn-27/artex/otlp"
	"github.com/Autumn-27/norma/llm"
)

// One metered call becomes GenAI metrics and a task-scoped client span, with
// no content beyond labels and counts.
func TestExportLLMCallReachesCollector(t *testing.T) {
	var mu sync.Mutex
	got := map[string]string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		got[r.URL.Path] += string(b)
		mu.Unlock()
	}))
	defer srv.Close()
	exp := otlp.New(otlp.Config{TracesURL: srv.URL + "/v1/traces", MetricsURL: srv.URL + "/v1/metrics", ServiceName: "artex", Interval: time.Hour})
	exportLLMCall(exp, llmrec.CallEvent{
		TaskID: "42", Worker: "planner", Model: "claude-x", Profile: "prod", Status: "ok",
		Start: time.Now().Add(-2 * time.Second), Latency: 2 * time.Second,
		Usage: llm.Usage{InputTokens: 1000, OutputTokens: 50, CacheReadTokens: 600, CacheWriteTokens: 100},
	})
	if err := exp.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	m, tr := got["/v1/metrics"], got["/v1/traces"]
	for _, want := range []string{"artex.llm.calls", "gen_ai.client.token.usage", "gen_ai.client.operation.duration",
		`"stringValue":"input_fresh"`, `"asDouble":300`, `"asDouble":600`, `"stringValue":"planner"`} {
		if !strings.Contains(m, want) {
			t.Errorf("metrics missing %s", want)
		}
	}
	if strings.Contains(m, "artex.task_id") {
		t.Error("task id leaked into metric labels")
	}
	for _, want := range []string{`"name":"chat claude-x"`, otlp.TraceIDFor("artex-task:42"), `"intValue":"1000"`, `"kind":3`} {
		if !strings.Contains(tr, want) {
			t.Errorf("traces missing %s", want)
		}
	}
}
