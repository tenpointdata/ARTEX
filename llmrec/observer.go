package llmrec

import (
	"sync/atomic"
	"time"

	"github.com/Autumn-27/norma/llm"
)

// CallEvent is one metered LLM call, the same facts and at the same point as
// its llm_usage row. It carries labels and counts only, never prompt or
// response content, so it is safe to hand to an external telemetry sink.
type CallEvent struct {
	TaskID  string
	Worker  string // agent lane: worker / planner / mainagent / goals / judge
	Model   string
	Profile string
	Status  string // ok | error
	Start   time.Time
	Latency time.Duration
	Usage   llm.Usage
}

var observer atomic.Pointer[func(CallEvent)]

// SetObserver installs a process-wide callback that sees every metered call
// (nil removes it). It runs synchronously on the calling goroutine, so it must
// not block: the OpenTelemetry exporter only updates counters and queues a span.
func SetObserver(fn func(CallEvent)) {
	if fn == nil {
		observer.Store(nil)
		return
	}
	observer.Store(&fn)
}

func notifyObserver(ev CallEvent) {
	if fn := observer.Load(); fn != nil {
		(*fn)(ev)
	}
}

// NotifyForTest hands ev to the installed observer, as a metered call would.
// For integration tests of observer wiring outside this package.
func NotifyForTest(ev CallEvent) { notifyObserver(ev) }
