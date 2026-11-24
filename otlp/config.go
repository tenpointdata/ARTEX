// Package otlp is a small, dependency-free OpenTelemetry exporter speaking
// OTLP/HTTP with JSON encoding (the stable protobuf-JSON mapping), for
// cumulative counters, explicit-bucket histograms and spans. It is a general
// mechanism with no knowledge of its caller; ARTEX wires LLM-call telemetry
// into it in cmd/artex.
//
// Limits: JSON encoding only (no protobuf, no gRPC); cumulative temporality
// only; spans are dropped, not blocked on, when the queue is full.
package otlp

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config selects where and how telemetry is sent.
type Config struct {
	TracesURL   string            // full URL of the traces endpoint ("" = no traces)
	MetricsURL  string            // full URL of the metrics endpoint ("" = no metrics)
	Headers     map[string]string // extra request headers, e.g. an auth token
	ServiceName string
	Resource    []Attr        // extra resource attributes
	Interval    time.Duration // metric export and span flush period
	Timeout     time.Duration // per-request timeout
	MaxSpans    int           // span queue bound
}

// ConfigFromEnv reads the standard OpenTelemetry environment variables:
// OTEL_EXPORTER_OTLP_ENDPOINT (base; /v1/traces and /v1/metrics are appended),
// OTEL_EXPORTER_OTLP_{TRACES,METRICS}_ENDPOINT (full URLs, override the base),
// OTEL_EXPORTER_OTLP_HEADERS, OTEL_SERVICE_NAME, OTEL_RESOURCE_ATTRIBUTES,
// OTEL_METRIC_EXPORT_INTERVAL (ms), OTEL_EXPORTER_OTLP_TIMEOUT (ms),
// OTEL_{TRACES,METRICS}_EXPORTER=none and OTEL_SDK_DISABLED. ok is false when
// no endpoint is configured or the SDK is disabled. A protocol other than
// http/json is an error, since that is the only one spoken here.
func ConfigFromEnv(defaultService string) (cfg Config, ok bool, err error) {
	return configFrom(os.Getenv, defaultService)
}

func configFrom(env func(string) string, defaultService string) (Config, bool, error) {
	get := func(k string) string { return strings.TrimSpace(env(k)) }
	if strings.EqualFold(get("OTEL_SDK_DISABLED"), "true") {
		return Config{}, false, nil
	}
	for _, k := range []string{"OTEL_EXPORTER_OTLP_PROTOCOL", "OTEL_EXPORTER_OTLP_TRACES_PROTOCOL", "OTEL_EXPORTER_OTLP_METRICS_PROTOCOL"} {
		if p := get(k); p != "" && p != "http/json" {
			return Config{}, false, fmt.Errorf("%s=%s: only http/json is supported", k, p)
		}
	}
	cfg := Config{
		ServiceName: defaultService,
		Interval:    msEnv(get("OTEL_METRIC_EXPORT_INTERVAL"), 15*time.Second),
		Timeout:     msEnv(get("OTEL_EXPORTER_OTLP_TIMEOUT"), 10*time.Second),
		MaxSpans:    2048,
	}
	base := strings.TrimRight(get("OTEL_EXPORTER_OTLP_ENDPOINT"), "/")
	cfg.TracesURL = signalURL(get("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT"), base, "/v1/traces")
	cfg.MetricsURL = signalURL(get("OTEL_EXPORTER_OTLP_METRICS_ENDPOINT"), base, "/v1/metrics")
	if strings.EqualFold(get("OTEL_TRACES_EXPORTER"), "none") {
		cfg.TracesURL = ""
	}
	if strings.EqualFold(get("OTEL_METRICS_EXPORTER"), "none") {
		cfg.MetricsURL = ""
	}
	for _, u := range []string{cfg.TracesURL, cfg.MetricsURL} {
		if u == "" {
			continue
		}
		if pu, err := url.Parse(u); err != nil || (pu.Scheme != "http" && pu.Scheme != "https") || pu.Host == "" {
			return Config{}, false, fmt.Errorf("OTLP endpoint %q is not an http(s) URL", u)
		}
	}
	if cfg.TracesURL == "" && cfg.MetricsURL == "" {
		return Config{}, false, nil
	}
	cfg.Headers = parsePairs(get("OTEL_EXPORTER_OTLP_HEADERS"))
	for k, v := range parsePairs(get("OTEL_RESOURCE_ATTRIBUTES")) {
		if k == "service.name" {
			cfg.ServiceName = v
			continue
		}
		cfg.Resource = append(cfg.Resource, String(k, v))
	}
	if s := get("OTEL_SERVICE_NAME"); s != "" {
		cfg.ServiceName = s
	}
	return cfg, true, nil
}

func signalURL(full, base, path string) string {
	if full != "" {
		return full
	}
	if base == "" {
		return ""
	}
	return base + path
}

func msEnv(s string, def time.Duration) time.Duration {
	if n, err := strconv.Atoi(s); err == nil && n > 0 {
		return time.Duration(n) * time.Millisecond
	}
	return def
}

// parsePairs parses the W3C-baggage-like "k1=v1,k2=v2" form used by the OTel
// header and resource variables; values are percent-decoded.
func parsePairs(s string) map[string]string {
	out := map[string]string{}
	for _, part := range strings.Split(s, ",") {
		k, v, ok := strings.Cut(part, "=")
		k = strings.TrimSpace(k)
		if !ok || k == "" {
			continue
		}
		if dv, err := url.QueryUnescape(strings.TrimSpace(v)); err == nil {
			v = dv
		}
		out[k] = strings.TrimSpace(v)
	}
	return out
}
