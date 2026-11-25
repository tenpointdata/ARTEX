package otlp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func envOf(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestConfigFromEnv(t *testing.T) {
	cfg, ok, err := configFrom(envOf(map[string]string{
		"OTEL_EXPORTER_OTLP_ENDPOINT":         "http://collector:4318/",
		"OTEL_EXPORTER_OTLP_METRICS_ENDPOINT": "https://m.example/v1/m",
		"OTEL_EXPORTER_OTLP_HEADERS":          "authorization=Bearer%20abc, x-team = red",
		"OTEL_RESOURCE_ATTRIBUTES":            "service.name=from-res,deployment.environment=lab",
		"OTEL_METRIC_EXPORT_INTERVAL":         "500",
	}), "artex")
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if cfg.TracesURL != "http://collector:4318/v1/traces" || cfg.MetricsURL != "https://m.example/v1/m" {
		t.Fatalf("urls: %q %q", cfg.TracesURL, cfg.MetricsURL)
	}
	if cfg.Headers["authorization"] != "Bearer abc" || cfg.Headers["x-team"] != "red" {
		t.Fatalf("headers: %v", cfg.Headers)
	}
	if cfg.ServiceName != "from-res" || len(cfg.Resource) != 1 || cfg.Interval != 500*time.Millisecond {
		t.Fatalf("cfg: %+v", cfg)
	}

	for name, env := range map[string]map[string]string{
		"unset":    {},
		"disabled": {"OTEL_EXPORTER_OTLP_ENDPOINT": "http://c:4318", "OTEL_SDK_DISABLED": "true"},
		"none":     {"OTEL_EXPORTER_OTLP_ENDPOINT": "http://c:4318", "OTEL_TRACES_EXPORTER": "none", "OTEL_METRICS_EXPORTER": "none"},
	} {
		if _, ok, err := configFrom(envOf(env), "artex"); ok || err != nil {
			t.Errorf("%s: ok=%v err=%v, want off", name, ok, err)
		}
	}
	for name, env := range map[string]map[string]string{
		"grpc":   {"OTEL_EXPORTER_OTLP_ENDPOINT": "http://c:4317", "OTEL_EXPORTER_OTLP_PROTOCOL": "grpc"},
		"scheme": {"OTEL_EXPORTER_OTLP_ENDPOINT": "file:///tmp/x"},
	} {
		if _, _, err := configFrom(envOf(env), "artex"); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
}

type capture struct {
	mu     sync.Mutex
	bodies map[string][]map[string]json.RawMessage
	auth   string
}

func (c *capture) handler(w http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil || r.Header.Get("Content-Type") != "application/json" {
		w.WriteHeader(400)
		return
	}
	c.mu.Lock()
	c.bodies[r.URL.Path] = append(c.bodies[r.URL.Path], m)
	c.auth = r.Header.Get("authorization")
	c.mu.Unlock()
}

func TestExportRoundTrip(t *testing.T) {
	c := &capture{bodies: map[string][]map[string]json.RawMessage{}}
	srv := httptest.NewServer(http.HandlerFunc(c.handler))
	defer srv.Close()
	e := New(Config{TracesURL: srv.URL + "/v1/traces", MetricsURL: srv.URL + "/v1/metrics",
		Headers: map[string]string{"authorization": "k"}, ServiceName: "artex", Interval: time.Hour})

	e.Add("artex.llm.tokens", "{token}", "", 100, String("type", "input"), String("model", "m"))
	e.Add("artex.llm.tokens", "{token}", "", 50, String("model", "m"), String("type", "input")) // same series, other order
	e.Add("artex.llm.tokens", "{token}", "", -5, String("type", "input"))                       // ignored
	e.Record("dur", "s", "", 0.5, DefaultBounds, String("model", "m"))
	e.Record("dur", "s", "", 3, DefaultBounds, String("model", "m"))
	now := time.Now()
	e.Span(Span{Name: "chat m", Kind: SpanKindClient, Start: now.Add(-time.Second), End: now,
		TraceID: TraceIDFor("task:1"), Attrs: []Attr{Int("gen_ai.usage.input_tokens", 7)}, Error: "boom"})

	if err := e.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.auth != "k" || len(c.bodies["/v1/traces"]) != 1 || len(c.bodies["/v1/metrics"]) != 1 {
		t.Fatalf("auth=%q bodies=%v", c.auth, c.bodies)
	}
	traces := string(c.bodies["/v1/traces"][0]["resourceSpans"])
	for _, want := range []string{`"traceId":"` + TraceIDFor("task:1") + `"`, `"kind":3`, `"code":2`,
		`"intValue":"7"`, `"stringValue":"artex"`} {
		if !strings.Contains(traces, want) {
			t.Errorf("traces missing %s: %s", want, traces)
		}
	}
	metrics := string(c.bodies["/v1/metrics"][0]["resourceMetrics"])
	for _, want := range []string{`"asDouble":150`, `"isMonotonic":true`, `"aggregationTemporality":2`,
		`"count":"2"`, `"sum":3.5`, `"min":0.5`, `"max":3`} {
		if !strings.Contains(metrics, want) {
			t.Errorf("metrics missing %s: %s", want, metrics)
		}
	}
}

func TestSpanQueueBoundAndErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(503) }))
	defer srv.Close()
	e := New(Config{TracesURL: srv.URL, MaxSpans: 2, Interval: time.Hour})
	for i := 0; i < 5; i++ {
		e.Span(Span{Name: "s", Start: time.Now(), End: time.Now()})
	}
	if e.dropped.Load() != 3 {
		t.Fatalf("dropped=%d, want 3", e.dropped.Load())
	}
	if err := e.Flush(context.Background()); err == nil || !strings.Contains(err.Error(), "503") {
		t.Fatalf("want 503 error, got %v", err)
	}
	var nilExp *Exporter
	nilExp.Add("x", "", "", 1)
	nilExp.Span(Span{})
}

func TestStartShutdownFlushes(t *testing.T) {
	c := &capture{bodies: map[string][]map[string]json.RawMessage{}}
	srv := httptest.NewServer(http.HandlerFunc(c.handler))
	defer srv.Close()
	e := New(Config{MetricsURL: srv.URL + "/v1/metrics", Interval: time.Hour})
	e.Start()
	e.Add("c", "", "", 1)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := e.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.bodies["/v1/metrics"]) != 1 {
		t.Fatalf("final flush missing: %v", c.bodies)
	}
}
