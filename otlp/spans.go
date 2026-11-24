package otlp

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"time"
)

// SpanKind values from the OTLP enum.
const (
	SpanKindInternal = 1
	SpanKindClient   = 3
)

// Span is one finished operation.
type Span struct {
	Name    string
	Kind    int // SpanKindClient, SpanKindInternal, ...; 0 = internal
	Start   time.Time
	End     time.Time
	TraceID string // 32 hex chars; "" = a random trace
	Attrs   []Attr
	Error   string // non-empty marks the span failed with this message
}

// TraceIDFor derives a stable trace id from a name, so every span about one
// long-lived thing (a task, a session) joins the same trace across restarts.
func TraceIDFor(name string) string {
	sum := sha256.Sum256([]byte(name))
	return hex.EncodeToString(sum[:16])
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

type statusJSON struct {
	Code    int    `json:"code"` // 1 ok, 2 error
	Message string `json:"message,omitempty"`
}

type spanJSON struct {
	TraceID     string     `json:"traceId"`
	SpanID      string     `json:"spanId"`
	Name        string     `json:"name"`
	Kind        int        `json:"kind"`
	StartTimeNs string     `json:"startTimeUnixNano"`
	EndTimeNs   string     `json:"endTimeUnixNano"`
	Attributes  []keyValue `json:"attributes"`
	Status      statusJSON `json:"status"`
}

func encodeSpan(s Span) spanJSON {
	tid := s.TraceID
	if len(tid) != 32 {
		tid = randomHex(16)
	}
	kind := s.Kind
	if kind == 0 {
		kind = SpanKindInternal
	}
	st := statusJSON{Code: 1}
	if s.Error != "" {
		st = statusJSON{Code: 2, Message: s.Error}
	}
	return spanJSON{
		TraceID: tid, SpanID: randomHex(8), Name: s.Name, Kind: kind,
		StartTimeNs: nanos(s.Start), EndTimeNs: nanos(s.End),
		Attributes: encodeAttrs(s.Attrs), Status: st,
	}
}
