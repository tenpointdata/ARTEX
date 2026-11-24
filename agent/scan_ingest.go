package agent

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/Autumn-27/artex/db"
	"github.com/Autumn-27/artex/scanimport"
)

// Deterministic ingestion of scanner output files for the in-run agent. The
// parsing is package scanimport's, the same parsers the operator import uses;
// this file adapts a scanimport.Result into insert_assets items and
// tool_output facts.
//
// The untrusted-text rules: summaries and notable items are built only from
// validated, typed fields (hosts, canonical URLs, ports, rule tokens,
// severities, counts). Every free-text field the target shapes (banner,
// product, title, technology, template name, extracted value) is kept only
// inside one Go-quoted fact detail, or in the typed asset columns insert_assets
// already treats as data. Nothing here creates a finding: a scanner result is
// unverified until the planner checks it.

// scanNotableMax caps the notable items returned per type.
const scanNotableMax = 20

// scanMaxItems caps the assets and facts one file may yield. The items past it
// are counted in OverCap, not ingested, and the result is marked truncated.
const scanMaxItems = 50000

// scanIngest is what one scanner file yields: counters, the insert_assets
// items, the facts, and a capped list of notable items per type.
type scanIngest struct {
	Events, Ports, Skipped, Oversized, OverCap int
	Truncated                                  bool
	hosts, urls                                map[string]bool
	Assets                                     []assetInputItem
	Facts                                      []factItem
	Notable                                    map[string][]string
}

func newScanIngest() *scanIngest {
	return &scanIngest{hosts: map[string]bool{}, urls: map[string]bool{}, Notable: map[string][]string{}}
}

// parseScanOutput parses r as format with scanimport, keeping informational
// results (the agent wants them as observations), and adapts the result.
func parseScanOutput(format scanimport.Format, r io.Reader) (*scanIngest, error) {
	res, err := scanimport.ParseWith(format, r, scanimport.Options{KeepInformational: true})
	if err != nil {
		return nil, err
	}
	return adaptScanResult(res), nil
}

// adaptScanResult turns a parsed file into asset items and facts.
func adaptScanResult(res *scanimport.Result) *scanIngest {
	s := newScanIngest()
	s.Events = max(res.Records-res.Skipped-res.Oversized, 0)
	s.Skipped, s.Oversized, s.Truncated = res.Skipped, res.Oversized, res.Truncated
	tool := scanimport.Token(res.Tool.Name)
	if tool == "" {
		tool = string(res.Format)
	}
	// A hostname the operator scanned is bound to its IP on the ip item, as
	// nmap reports it, instead of becoming a separate subdomain item.
	ips, bound := map[string]bool{}, map[string][]string{}
	for _, a := range res.Assets {
		if a.Kind == scanimport.KindIP {
			ips[a.IP] = true
		}
	}
	for _, a := range res.Assets {
		if a.Kind == scanimport.KindSubdomain && ips[a.IP] {
			bound[a.IP] = append(bound[a.IP], a.Domain)
		}
	}
	for _, a := range res.Assets {
		if !s.full() {
			s.addAsset(tool, a, ips, bound)
		}
	}
	for _, f := range res.Findings {
		if !s.full() {
			s.addFinding(f)
		}
	}
	return s
}

func (s *scanIngest) note(kind, item string) {
	if len(s.Notable[kind]) < scanNotableMax {
		s.Notable[kind] = append(s.Notable[kind], item)
	}
}

// full reports whether the item cap is reached; the record is then counted in
// OverCap and dropped. It is checked once per record, so the last record
// admitted may take the total slightly past the cap.
func (s *scanIngest) full() bool {
	if len(s.Assets)+len(s.Facts) < scanMaxItems {
		return false
	}
	s.OverCap++
	s.Truncated = true
	return true
}

// fact records a tool_output fact: the summary is built by the caller from
// typed fields only, the detail is the compact JSON of detail as one Go-quoted
// inert string.
func (s *scanIngest) fact(summary string, detail any) {
	b, _ := json.Marshal(detail)
	s.Facts = append(s.Facts, factItem{Summary: summary, Detail: strconv.Quote(string(b)), Source: factSourceToolOutput})
}

// untrustedDetail is the detail of a fact: the scanner's untrusted fields,
// with scanimport's already-compact Detail JSON nested as is.
func untrustedDetail(fields map[string]any, raw string) map[string]any {
	if raw != "" && json.Valid([]byte(raw)) {
		fields["detail"] = json.RawMessage(raw)
	}
	return fields
}

func (s *scanIngest) addAsset(tool string, a scanimport.Asset, ips map[string]bool, bound map[string][]string) {
	switch a.Kind {
	case scanimport.KindIP:
		s.hosts[a.IP] = true
		var open []db.PortService
		var list []string
		for _, p := range a.OpenPorts {
			pp := fmt.Sprintf("%d/%s", p.Port, p.Proto)
			list = append(list, pp)
			open = append(open, db.PortService{Port: p.Port, Service: p.Service})
			s.note("ports", a.IP+" "+pp+" "+p.Service)
		}
		s.Ports += len(list)
		s.Assets = append(s.Assets, assetInputItem{Type: "ip", IP: a.IP, BoundDomains: bound[a.IP], OpenPorts: open})
		if len(list) == 0 && a.Detail == "" {
			return // a target named by a result, which has its own fact
		}
		s.note("hosts", fmt.Sprintf("%s (%d open ports)", a.IP, len(list)))
		shown := list[:min(len(list), 50)]
		summary := fmt.Sprintf("%s: host %s has %d open ports: %s", tool, a.IP, len(list), strings.Join(shown, ", "))
		if len(list) > len(shown) {
			summary += fmt.Sprintf(" and %d more", len(list)-len(shown))
		}
		s.fact(summary, untrustedDetail(map[string]any{"ip": a.IP}, a.Detail))
	case scanimport.KindSubdomain:
		if !ips[a.IP] { // a bound name counts with its IP, as before
			s.hosts[a.Domain] = true
			s.Assets = append(s.Assets, assetInputItem{Type: "subdomain", Domain: a.Domain})
		}
	case scanimport.KindService:
		s.Assets = append(s.Assets, assetInputItem{Type: "service", IP: a.IP, Domain: a.Domain, Port: a.Port, ServiceName: a.Service})
	case scanimport.KindHTTPService:
		s.addHTTPService(tool, a)
	}
}

func (s *scanIngest) addHTTPService(tool string, a scanimport.Asset) {
	if _, host := scanimport.URL(a.URL); host != "" {
		s.hosts[host] = true
	}
	s.urls[a.URL] = true
	item := assetInputItem{Type: "service", URL: a.URL, IP: a.IP, PageTitle: a.Title, Technologies: a.Tech}
	if a.Status > 0 {
		st := a.Status
		item.StatusCode = &st
	}
	if a.Length > 0 {
		n := a.Length
		item.ContentLength = &n
	}
	s.Assets = append(s.Assets, item)
	if a.Status == 0 {
		s.note("urls", a.URL)
		return // a service seen on a port scan; the host's fact covers it
	}
	s.note("urls", fmt.Sprintf("%s %d", a.URL, a.Status))
	s.fact(fmt.Sprintf("%s: %s responded with status %d, %d bytes", tool, a.URL, a.Status, a.Length),
		untrustedDetail(map[string]any{"title": a.Title, "tech": a.Tech}, a.Detail))
}

// addFinding records a scanner result as a fact, not a finding.
func (s *scanIngest) addFinding(f scanimport.Finding) {
	where := f.Host
	if where != "" {
		s.hosts[where] = true
	} else if f.Location != "" {
		where = strconv.Quote(f.Location)
		if f.Line > 0 {
			where += " line " + strconv.Itoa(f.Line)
		}
	} else {
		where = "an unnamed target"
	}
	if u, _ := scanimport.URL(f.Target); u != "" {
		s.urls[u] = true
	}
	noun := "rule"
	if f.Tool == "nuclei" {
		noun = "template"
	}
	s.note("results", f.RuleID+" ("+f.Severity+") on "+where)
	s.fact(fmt.Sprintf("%s: %s %s (severity %s) matched on %s; unverified scanner result", f.Tool, noun, f.RuleID, f.Severity, where),
		untrustedDetail(map[string]any{"title": f.Title, "target": f.Target, "cwe": f.CWE, "record": f.Record}, f.Detail))
}
