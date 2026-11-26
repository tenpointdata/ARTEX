package scanimport

import (
	"encoding/json"
	"sort"
	"strconv"
)

// collector accumulates one Result: it dedupes assets and findings and
// enforces MaxItems.
type collector struct {
	res    *Result
	assets map[string]int // asset identity -> index in res.Assets
	keys   map[string]bool
}

func newCollector(f Format) *collector {
	return &collector{
		res:    &Result{Format: f, Assets: []Asset{}, Findings: []Finding{}},
		assets: map[string]int{},
		keys:   map[string]bool{},
	}
}

// full reports whether MaxItems is reached, marking the result truncated.
// Parsers call it once per record and stop when it is true.
func (c *collector) full() bool {
	if len(c.res.Assets)+len(c.res.Findings) < MaxItems {
		return false
	}
	c.res.Truncated = true
	return true
}

func assetIdentity(a Asset) string {
	switch a.Kind {
	case KindIP:
		return "ip|" + a.IP
	case KindSubdomain:
		return "sub|" + a.Domain
	case KindHTTPService:
		return "http|" + a.URL
	}
	return "svc|" + a.IP + "|" + a.Proto + "|" + strconv.Itoa(a.Port)
}

// asset adds a, or merges it into the asset with the same kind and identity
// (open ports are unioned; empty fields are filled; the first Record stays).
func (c *collector) asset(a Asset) {
	id := assetIdentity(a)
	if i, ok := c.assets[id]; ok {
		ex := &c.res.Assets[i]
		if ex.IP == "" {
			ex.IP = a.IP
		}
		if ex.Service == "" {
			ex.Service = a.Service
		}
		for _, p := range a.OpenPorts {
			dup := false
			for _, q := range ex.OpenPorts {
				dup = dup || (q.Port == p.Port && q.Proto == p.Proto)
			}
			if !dup {
				ex.OpenPorts = append(ex.OpenPorts, p)
			}
		}
		return
	}
	if c.full() {
		return
	}
	c.assets[id] = len(c.res.Assets)
	c.res.Assets = append(c.res.Assets, a)
}

// finding adds f with its Detail built from detail. Title is cleaned, Key is
// derived, and a finding whose Key was already produced is counted in
// Duplicates and dropped. It reports whether f was added.
func (c *collector) finding(f Finding, detail map[string]any) bool {
	if c.full() {
		return false
	}
	f.Title = cleanText(f.Title, MaxTitleBytes)
	if f.Lane == "" {
		f.Lane = LaneOf(c.res.Format)
	}
	f.Key = makeKey(f.Tool, f.RuleID, f.Target, f.Location, f.Line)
	if c.keys[f.Key] {
		c.res.Duplicates++
		return false
	}
	c.keys[f.Key] = true
	f.Detail = buildDetail(detail)
	c.res.Findings = append(c.res.Findings, f)
	return true
}

// detailTiers are the progressively tighter (string bytes, list length)
// limits applied until the Detail JSON fits MaxDetailBytes.
var detailTiers = [][2]int{{4096, 100}, {1024, 20}, {256, 5}}

// buildDetail renders m as compact JSON: values are strings, string lists,
// ints or bools; strings are cleaned of credentials and clipped; empty values
// are dropped. If the JSON still exceeds MaxDetailBytes after the tightest
// tier, the largest keys are dropped and "truncated":true is set. Keys are
// sorted by encoding/json, so the output is deterministic.
func buildDetail(m map[string]any) string {
	var out []byte
	for _, t := range detailTiers {
		out, _ = json.Marshal(shrink(m, t[0], t[1]))
		if len(out) <= MaxDetailBytes {
			return string(out)
		}
	}
	s := shrink(m, detailTiers[len(detailTiers)-1][0], detailTiers[len(detailTiers)-1][1])
	s["truncated"] = true
	for len(out) > MaxDetailBytes && len(s) > 1 {
		type kv struct {
			k string
			n int
		}
		var sizes []kv
		for k, v := range s {
			if k != "truncated" {
				b, _ := json.Marshal(v)
				sizes = append(sizes, kv{k, len(b)})
			}
		}
		if len(sizes) == 0 {
			break
		}
		sort.Slice(sizes, func(i, j int) bool {
			if sizes[i].n != sizes[j].n {
				return sizes[i].n > sizes[j].n
			}
			return sizes[i].k < sizes[j].k
		})
		delete(s, sizes[0].k)
		out, _ = json.Marshal(s)
	}
	return string(out)
}

func shrink(m map[string]any, maxStr, maxList int) map[string]any {
	s := make(map[string]any, len(m))
	for k, v := range m {
		switch x := v.(type) {
		case string:
			if x = scrubText(clip(x, maxStr)); x != "" {
				s[k] = x
			}
		case []string:
			var l []string
			for _, e := range x {
				if len(l) >= maxList {
					break
				}
				if e = scrubText(clip(e, maxStr)); e != "" {
					l = append(l, e)
				}
			}
			if len(l) > 0 {
				s[k] = l
			}
		case []int:
			if len(x) > 0 {
				s[k] = x
			}
		case int:
			if x != 0 {
				s[k] = x
			}
		case bool:
			if x {
				s[k] = x
			}
		}
	}
	return s
}
