package otlp

import (
	"sort"
	"strconv"
	"strings"
)

// Attr is one typed attribute: a string or an integer value.
type Attr struct {
	Key   string
	str   string
	num   int64
	isInt bool
}

// String makes a string attribute.
func String(k, v string) Attr { return Attr{Key: k, str: v} }

// Int makes an integer attribute.
func Int(k string, v int64) Attr { return Attr{Key: k, num: v, isInt: true} }

type anyValue struct {
	StringValue *string `json:"stringValue,omitempty"`
	IntValue    *string `json:"intValue,omitempty"` // int64 is a JSON string in OTLP/JSON
}

type keyValue struct {
	Key   string   `json:"key"`
	Value anyValue `json:"value"`
}

func encodeAttrs(attrs []Attr) []keyValue {
	out := make([]keyValue, 0, len(attrs))
	for _, a := range attrs {
		kv := keyValue{Key: a.Key}
		if a.isInt {
			s := strconv.FormatInt(a.num, 10)
			kv.Value.IntValue = &s
		} else {
			s := a.str
			kv.Value.StringValue = &s
		}
		out = append(out, kv)
	}
	return out
}

// attrKey is a stable identity for an attribute set, so the same labels in a
// different order land in the same time series.
func attrKey(attrs []Attr) (string, []Attr) {
	sorted := append([]Attr(nil), attrs...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Key < sorted[j].Key })
	var b strings.Builder
	for _, a := range sorted {
		b.WriteString(a.Key)
		b.WriteByte(0)
		if a.isInt {
			b.WriteString("i" + strconv.FormatInt(a.num, 10))
		} else {
			b.WriteString("s" + a.str)
		}
		b.WriteByte(0)
	}
	return b.String(), sorted
}
