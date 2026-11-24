package otlp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// Exporter batches telemetry in memory and posts it to an OTLP/HTTP collector.
// Recording methods never block on the network and are safe for concurrent use.
type Exporter struct {
	cfg    Config
	client *http.Client
	meter  *meter
	res    resourceJSON

	mu    sync.Mutex
	spans []spanJSON

	dropped atomic.Int64
	stop    chan struct{}
	done    chan struct{}
	once    sync.Once
}

type resourceJSON struct {
	Attributes []keyValue `json:"attributes"`
}

type scopeJSON struct {
	Name string `json:"name"`
}

// New makes an exporter; call Start to begin periodic export.
func New(cfg Config) *Exporter {
	if cfg.Interval <= 0 {
		cfg.Interval = 15 * time.Second
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 10 * time.Second
	}
	if cfg.MaxSpans <= 0 {
		cfg.MaxSpans = 2048
	}
	attrs := append([]Attr{String("service.name", cfg.ServiceName)}, cfg.Resource...)
	return &Exporter{
		cfg:    cfg,
		client: &http.Client{Timeout: cfg.Timeout},
		meter:  newMeter(time.Now()),
		res:    resourceJSON{Attributes: encodeAttrs(attrs)},
		stop:   make(chan struct{}),
		done:   make(chan struct{}),
	}
}

// Add increments a monotonic cumulative counter.
func (e *Exporter) Add(name, unit, desc string, v float64, attrs ...Attr) {
	if e == nil || e.cfg.MetricsURL == "" {
		return
	}
	e.meter.add(name, unit, desc, v, attrs)
}

// Record adds one observation to an explicit-bucket histogram. The bounds of
// the first Record for a name are kept.
func (e *Exporter) Record(name, unit, desc string, v float64, bounds []float64, attrs ...Attr) {
	if e == nil || e.cfg.MetricsURL == "" {
		return
	}
	e.meter.record(name, unit, desc, v, bounds, attrs)
}

// Span queues a finished span; when the queue is full it is dropped and counted.
func (e *Exporter) Span(s Span) {
	if e == nil || e.cfg.TracesURL == "" {
		return
	}
	enc := encodeSpan(s)
	e.mu.Lock()
	if len(e.spans) >= e.cfg.MaxSpans {
		e.mu.Unlock()
		e.dropped.Add(1)
		return
	}
	e.spans = append(e.spans, enc)
	e.mu.Unlock()
}

// Start runs periodic export until Shutdown.
func (e *Exporter) Start() {
	go func() {
		defer close(e.done)
		t := time.NewTicker(e.cfg.Interval)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				e.flushLogged(context.Background())
			case <-e.stop:
				return
			}
		}
	}()
}

// Shutdown stops the loop and makes a final export within ctx.
func (e *Exporter) Shutdown(ctx context.Context) error {
	var err error
	e.once.Do(func() {
		close(e.stop)
		select {
		case <-e.done:
		case <-ctx.Done():
		}
		err = e.Flush(ctx)
	})
	return err
}

func (e *Exporter) flushLogged(ctx context.Context) {
	if err := e.Flush(ctx); err != nil {
		log.Printf("[otlp] export: %v", err)
	}
	if n := e.dropped.Swap(0); n > 0 {
		log.Printf("[otlp] dropped %d spans (queue full)", n)
	}
}

// Flush sends queued spans and the current metric state now.
func (e *Exporter) Flush(ctx context.Context) error {
	var errs []error
	if e.cfg.TracesURL != "" {
		e.mu.Lock()
		batch := e.spans
		e.spans = nil
		e.mu.Unlock()
		if len(batch) > 0 {
			body := map[string]any{"resourceSpans": []map[string]any{{
				"resource":   e.res,
				"scopeSpans": []map[string]any{{"scope": scopeJSON{Name: e.cfg.ServiceName}, "spans": batch}},
			}}}
			if err := e.post(ctx, e.cfg.TracesURL, body); err != nil {
				errs = append(errs, fmt.Errorf("traces: %w", err))
			}
		}
	}
	if e.cfg.MetricsURL != "" {
		if ms := e.meter.snapshot(time.Now()); len(ms) > 0 {
			body := map[string]any{"resourceMetrics": []map[string]any{{
				"resource":     e.res,
				"scopeMetrics": []map[string]any{{"scope": scopeJSON{Name: e.cfg.ServiceName}, "metrics": ms}},
			}}}
			if err := e.post(ctx, e.cfg.MetricsURL, body); err != nil {
				errs = append(errs, fmt.Errorf("metrics: %w", err))
			}
		}
	}
	if len(errs) == 1 {
		return errs[0]
	}
	if len(errs) > 1 {
		return fmt.Errorf("%v; %v", errs[0], errs[1])
	}
	return nil
}

func (e *Exporter) post(ctx context.Context, url string, body any) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range e.cfg.Headers {
		req.Header.Set(k, v)
	}
	resp, err := e.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("%s: HTTP %d", url, resp.StatusCode)
	}
	return nil
}
