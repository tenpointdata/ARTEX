package otlp

import (
	"math"
	"sort"
	"strconv"
	"sync"
	"time"
)

type instrumentKind int

const (
	kindSum instrumentKind = iota
	kindHistogram
)

// DefaultBounds suit durations in seconds (the OTel GenAI duration buckets).
var DefaultBounds = []float64{0.01, 0.02, 0.04, 0.08, 0.16, 0.32, 0.64, 1.28, 2.56, 5.12, 10.24, 20.48, 40.96, 81.92}

// TokenBounds suit token counts (the OTel GenAI token-usage buckets).
var TokenBounds = []float64{1, 4, 16, 64, 256, 1024, 4096, 16384, 65536, 262144, 1048576, 4194304, 16777216, 67108864}

type point struct {
	attrs   []Attr
	sum     float64
	count   uint64
	buckets []uint64
	min     float64
	max     float64
}

type instrument struct {
	name, unit, desc string
	kind             instrumentKind
	bounds           []float64
	points           map[string]*point
}

// meter holds cumulative state for every instrument and attribute set.
type meter struct {
	mu    sync.Mutex
	start time.Time
	insts map[string]*instrument
	order []string
}

func newMeter(start time.Time) *meter {
	return &meter{start: start, insts: map[string]*instrument{}}
}

func (m *meter) instrument(name, unit, desc string, kind instrumentKind, bounds []float64) *instrument {
	in := m.insts[name]
	if in == nil {
		in = &instrument{name: name, unit: unit, desc: desc, kind: kind, bounds: bounds, points: map[string]*point{}}
		m.insts[name] = in
		m.order = append(m.order, name)
	}
	return in
}

func (in *instrument) point(attrs []Attr) *point {
	key, sorted := attrKey(attrs)
	p := in.points[key]
	if p == nil {
		p = &point{attrs: sorted, min: math.Inf(1), max: math.Inf(-1)}
		if in.kind == kindHistogram {
			p.buckets = make([]uint64, len(in.bounds)+1)
		}
		in.points[key] = p
	}
	return p
}

func (m *meter) add(name, unit, desc string, v float64, attrs []Attr) {
	if v < 0 || math.IsNaN(v) {
		return // monotonic: negative increments are invalid
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	in := m.instrument(name, unit, desc, kindSum, nil)
	if in.kind != kindSum {
		return
	}
	in.point(attrs).sum += v
}

func (m *meter) record(name, unit, desc string, v float64, bounds []float64, attrs []Attr) {
	if math.IsNaN(v) {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	in := m.instrument(name, unit, desc, kindHistogram, bounds)
	if in.kind != kindHistogram {
		return
	}
	p := in.point(attrs)
	p.sum += v
	p.count++
	p.buckets[sort.SearchFloat64s(in.bounds, v)]++
	p.min = math.Min(p.min, v)
	p.max = math.Max(p.max, v)
}

// OTLP/JSON payload shapes for metrics.

type numberPoint struct {
	Attributes  []keyValue `json:"attributes"`
	StartTimeNs string     `json:"startTimeUnixNano"`
	TimeNs      string     `json:"timeUnixNano"`
	AsDouble    float64    `json:"asDouble"`
}

type histPoint struct {
	Attributes     []keyValue `json:"attributes"`
	StartTimeNs    string     `json:"startTimeUnixNano"`
	TimeNs         string     `json:"timeUnixNano"`
	Count          string     `json:"count"`
	Sum            float64    `json:"sum"`
	BucketCounts   []string   `json:"bucketCounts"`
	ExplicitBounds []float64  `json:"explicitBounds"`
	Min            float64    `json:"min"`
	Max            float64    `json:"max"`
}

type sumData struct {
	Temporality int           `json:"aggregationTemporality"` // 2 = cumulative
	Monotonic   bool          `json:"isMonotonic"`
	DataPoints  []numberPoint `json:"dataPoints"`
}

type histData struct {
	Temporality int         `json:"aggregationTemporality"`
	DataPoints  []histPoint `json:"dataPoints"`
}

type metricJSON struct {
	Name        string    `json:"name"`
	Unit        string    `json:"unit,omitempty"`
	Description string    `json:"description,omitempty"`
	Sum         *sumData  `json:"sum,omitempty"`
	Histogram   *histData `json:"histogram,omitempty"`
}

func nanos(t time.Time) string { return strconv.FormatInt(t.UnixNano(), 10) }

// snapshot renders the cumulative state; nil when nothing was recorded.
func (m *meter) snapshot(now time.Time) []metricJSON {
	m.mu.Lock()
	defer m.mu.Unlock()
	start, ts := nanos(m.start), nanos(now)
	var out []metricJSON
	for _, name := range m.order {
		in := m.insts[name]
		keys := make([]string, 0, len(in.points))
		for k := range in.points {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		mj := metricJSON{Name: in.name, Unit: in.unit, Description: in.desc}
		if in.kind == kindSum {
			mj.Sum = &sumData{Temporality: 2, Monotonic: true}
			for _, k := range keys {
				p := in.points[k]
				mj.Sum.DataPoints = append(mj.Sum.DataPoints, numberPoint{
					Attributes: encodeAttrs(p.attrs), StartTimeNs: start, TimeNs: ts, AsDouble: p.sum})
			}
		} else {
			mj.Histogram = &histData{Temporality: 2}
			for _, k := range keys {
				p := in.points[k]
				bc := make([]string, len(p.buckets))
				for i, c := range p.buckets {
					bc[i] = strconv.FormatUint(c, 10)
				}
				mj.Histogram.DataPoints = append(mj.Histogram.DataPoints, histPoint{
					Attributes: encodeAttrs(p.attrs), StartTimeNs: start, TimeNs: ts,
					Count: strconv.FormatUint(p.count, 10), Sum: p.sum, BucketCounts: bc,
					ExplicitBounds: in.bounds, Min: p.min, Max: p.max})
			}
		}
		out = append(out, mj)
	}
	return out
}
