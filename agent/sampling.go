package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"slices"
	"strconv"

	"github.com/Autumn-27/artex/db"
)

// Sampling pin (E8 A/B runs): a profile may pin a sampling seed and a default
// temperature so replicates of an evaluation suite vary less. The norma SDK has
// no seed field, so both are applied on the wire, by samplingTransport, which
// adds each key only when the request body does not already carry it. That
// keeps a code-owned temperature (for example the interception assessment's)
// intact: the pin fills, it never overrides.

// SupportsSeed reports whether a profile format's wire API takes "seed".
func SupportsSeed(format string) bool { return slices.Contains(db.SamplingSeedFormats, format) }

// ValidateSampling checks a profile's sampling pin against its format. A seed
// on a format that cannot send it is an error rather than a silent drop, so a
// stamped A/B run never claims a seed the provider never saw.
func ValidateSampling(format string, s db.Sampling) error {
	if s.Seed != nil && !SupportsSeed(format) {
		return fmt.Errorf("a sampling seed is only sent for the %v formats, not %q", db.SamplingSeedFormats, format)
	}
	if t := s.Temperature; t != nil && (math.IsNaN(*t) || *t < 0 || *t > db.MaxSamplingTemperature) {
		return fmt.Errorf("sampling temperature must be between 0 and %g", db.MaxSamplingTemperature)
	}
	return nil
}

// samplingFields renders the pinned fields that apply to wire format, as JSON
// object members keyed by name; empty when nothing is pinned.
func samplingFields(format string, s db.Sampling) map[string]string {
	out := map[string]string{}
	if s.Seed != nil && SupportsSeed(format) {
		out["seed"] = strconv.FormatInt(*s.Seed, 10)
	}
	if s.Temperature != nil {
		out["temperature"] = strconv.FormatFloat(*s.Temperature, 'f', -1, 64)
	}
	return out
}

// samplingTransport adds the pinned fields to each JSON request body that does
// not already set them. It sits outermost so the LLM recorder captures the body
// the provider actually receives.
type samplingTransport struct {
	base   http.RoundTripper
	fields map[string]string
}

func (t samplingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Body == nil || req.Method != http.MethodPost {
		return t.base.RoundTrip(req)
	}
	raw, err := io.ReadAll(req.Body)
	req.Body.Close()
	if err != nil {
		return nil, err
	}
	body := withSamplingFields(raw, t.fields)
	r2 := req.Clone(req.Context())
	r2.Body = io.NopCloser(bytes.NewReader(body))
	r2.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(body)), nil }
	r2.ContentLength = int64(len(body))
	return t.base.RoundTrip(r2)
}

// withSamplingFields splices the missing fields into a JSON object body, in a
// stable key order, leaving the original bytes otherwise untouched. A body that
// is not a JSON object is returned as is.
func withSamplingFields(raw []byte, fields map[string]string) []byte {
	var top map[string]json.RawMessage
	if json.Unmarshal(raw, &top) != nil || top == nil {
		return raw
	}
	var add bytes.Buffer
	for _, k := range []string{"seed", "temperature"} {
		v, ok := fields[k]
		if _, set := top[k]; !ok || set {
			continue
		}
		fmt.Fprintf(&add, "%q:%s", k, v)
		add.WriteByte(',')
	}
	if add.Len() == 0 {
		return raw
	}
	open := bytes.IndexByte(raw, '{')
	rest := bytes.TrimLeft(raw[open+1:], " \t\r\n")
	if len(rest) > 0 && rest[0] == '}' { // empty object: no trailing comma
		add.Truncate(add.Len() - 1)
	}
	out := make([]byte, 0, len(raw)+add.Len())
	out = append(out, raw[:open+1]...)
	out = append(out, add.Bytes()...)
	return append(out, raw[open+1:]...)
}
