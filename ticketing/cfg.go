package ticketing

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/Autumn-27/artex/notify"
)

// Config readers. Tracker configs come from a JSONB column, so after
// encoding/json they are map[string]any with float64 numbers and []any arrays;
// these helpers tolerate that (and form-typed strings) like notify's.

func cfgString(cfg map[string]any, key string) string {
	s, _ := cfg[key].(string)
	return strings.TrimSpace(s)
}

func cfgInt(cfg map[string]any, key string) int {
	switch v := cfg[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil {
			return 0
		}
		return n
	default:
		return 0
	}
}

// cfgStrings reads a string array (a lone string counts as one element).
func cfgStrings(cfg map[string]any, key string) []string {
	raw, ok := cfg[key].([]any)
	if !ok {
		if s := cfgString(cfg, key); s != "" {
			return []string{s}
		}
		return nil
	}
	var out []string
	for _, v := range raw {
		if s, ok := v.(string); ok {
			if s = strings.TrimSpace(s); s != "" {
				out = append(out, s)
			}
		}
	}
	return out
}

// cfgInts reads an integer array, accepting numbers and numeric strings.
func cfgInts(cfg map[string]any, key string) []int {
	raw, ok := cfg[key].([]any)
	if !ok {
		if n := cfgInt(cfg, key); n != 0 {
			return []int{n}
		}
		return nil
	}
	var out []int
	for _, v := range raw {
		if n := cfgInt(map[string]any{"v": v}, "v"); n != 0 {
			out = append(out, n)
		}
	}
	return out
}

// baseURL returns the validated, slash-trimmed base URL from cfg["base_url"]
// (def when absent and def != ""). It must be http(s) with a host and no
// embedded credentials. The error names the field only, never the value.
func baseURL(cfg map[string]any, def string) (string, error) {
	raw := cfgString(cfg, "base_url")
	if raw == "" {
		raw = def
	}
	if raw == "" {
		return "", fmt.Errorf("base_url is required")
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", fmt.Errorf("base_url must be an http(s) URL with a host")
	}
	if u.User != nil {
		return "", fmt.Errorf("base_url must not embed credentials; use the credential fields")
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("base_url must not contain a query or fragment")
	}
	return strings.TrimRight(u.String(), "/"), nil
}

// call sends a request through notify.Do. On a non-2xx status it returns the
// response together with the (already redacted) error.
func call(ctx context.Context, req notify.Request) (*notify.Response, error) {
	return notify.Do(ctx, req)
}

// callJSON is call plus decoding of the JSON response body into T. An empty body
// yields the zero value.
func callJSON[T any](ctx context.Context, req notify.Request) (T, error) {
	resp, err := call(ctx, req)
	if err != nil {
		var zero T
		return zero, err
	}
	return decodeBody[T](resp)
}

// decodeBody decodes a successful response body into T (empty body: zero value).
func decodeBody[T any](resp *notify.Response) (T, error) {
	var out T
	if len(resp.Body) == 0 {
		return out, nil
	}
	if err := json.Unmarshal(resp.Body, &out); err != nil {
		// The body is not echoed: it may contain data we did not intend to log.
		return out, fmt.Errorf("unexpected response from tracker (HTTP %d): not valid JSON", resp.Status)
	}
	return out, nil
}

// hasString reports whether list contains s.
func hasString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// uniqueStrings returns list without blanks or duplicates, order preserved.
func uniqueStrings(list ...string) []string {
	var out []string
	for _, s := range list {
		if s = strings.TrimSpace(s); s != "" && !hasString(out, s) {
			out = append(out, s)
		}
	}
	return out
}
