package scanimport

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path"
	"strconv"
	"strings"
)

type sarifProps struct {
	Tags   flexList `json:"tags"`
	SecSev flex     `json:"security-severity"`
}

type sarifRule struct {
	ID    flex `json:"id"`
	Name  flex `json:"name"`
	Short struct {
		Text flex `json:"text"`
	} `json:"shortDescription"`
	Props   sarifProps `json:"properties"`
	Default struct {
		Level flex `json:"level"`
	} `json:"defaultConfiguration"`
}

type sarifTool struct {
	Driver struct {
		Name     flex        `json:"name"`
		Semantic flex        `json:"semanticVersion"`
		Version  flex        `json:"version"`
		Rules    []sarifRule `json:"rules"`
	} `json:"driver"`
}

type sarifResult struct {
	RuleID    flex `json:"ruleId"`
	RuleIndex *int `json:"ruleIndex"`
	Rule      struct {
		ID flex `json:"id"`
	} `json:"rule"`
	Level   flex `json:"level"`
	Message struct {
		Text flex `json:"text"`
	} `json:"message"`
	Locations []struct {
		Physical struct {
			Artifact struct {
				URI flex `json:"uri"`
			} `json:"artifactLocation"`
			Region struct {
				StartLine int `json:"startLine"`
			} `json:"region"`
		} `json:"physicalLocation"`
	} `json:"locations"`
	Props sarifProps `json:"properties"`
}

// sarifRun is the per-run context results are read against.
type sarifRun struct {
	idx   int
	tool  string
	rules map[string]sarifRule
	list  []sarifRule
	seen  int // results processed
}

// parseSARIF streams a SARIF 2.1.0 log (e.g. semgrep --sarif): runs one at a
// time and, when the run lists its tool before its results (the usual order),
// results one at a time. SARIF carries code findings, so there are no assets.
func parseSARIF(c *collector, r io.Reader) error {
	dec := json.NewDecoder(r)
	recognized := false
	runIdx := 0
	err := walkObject(dec, func(key string) error {
		if key != "runs" {
			return skipValue(dec)
		}
		recognized = true
		return walkArray(dec, func() error {
			run := &sarifRun{idx: runIdx, rules: map[string]sarifRule{}}
			runIdx++
			return c.sarifRun(dec, run)
		})
	})
	switch {
	case errors.Is(err, errStop):
		return nil
	case err == nil && recognized:
		return nil
	case err != nil && recognized && !errors.Is(err, errNotObject) && !errors.Is(err, errNotArray):
		c.res.Truncated = true
		return nil
	case err != nil:
		return fmt.Errorf("scanimport: sarif: not a SARIF log: %w", err)
	}
	return errors.New("scanimport: sarif: no runs key")
}

func (c *collector) sarifRun(dec *json.Decoder, run *sarifRun) error {
	var pending []json.RawMessage // results seen before the tool
	toolSeen := false
	flush := func() {
		for _, raw := range pending {
			c.sarifResult(raw, run)
		}
		pending = nil
	}
	err := walkObject(dec, func(key string) error {
		switch key {
		case "tool":
			var t sarifTool
			if _, err := decodeLoose(dec, &t); err != nil {
				return err
			}
			c.sarifTool(run, t)
			toolSeen = true
			flush()
			return nil
		case "results":
			return walkArray(dec, func() error {
				var raw json.RawMessage
				if err := dec.Decode(&raw); err != nil {
					return err
				}
				if !toolSeen {
					pending = append(pending, raw)
					return nil
				}
				if c.full() {
					return errStop
				}
				c.sarifResult(raw, run)
				return nil
			})
		}
		return skipValue(dec)
	})
	if !toolSeen { // no tool object: results are read against an empty rule table
		run.tool = "sarif"
	}
	flush()
	return err
}

func (c *collector) sarifTool(run *sarifRun, t sarifTool) {
	run.tool = toolName(t.Driver.Name.s())
	if run.tool == "" {
		run.tool = "sarif"
	}
	if run.idx == 0 {
		c.res.Tool = ToolInfo{Name: run.tool, Version: versionToken(firstNonEmpty(t.Driver.Semantic.s(), t.Driver.Version.s()))}
	}
	run.list = t.Driver.Rules
	for _, ru := range t.Driver.Rules {
		if id := ru.ID.s(); id != "" {
			run.rules[id] = ru
		}
	}
}

// sarifSeverity applies the rule's security-severity (CVSS-like) when present,
// else the result level. "" means informational.
func sarifSeverity(secSev, level string) string {
	if secSev != "" {
		if f, err := strconv.ParseFloat(strings.TrimSpace(secSev), 64); err == nil {
			switch {
			case f >= 9:
				return "critical"
			case f >= 7:
				return "high"
			case f >= 4:
				return "medium"
			case f > 0:
				return "low"
			}
			return ""
		}
	}
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "error":
		return "high"
	case "warning", "":
		return "medium" // SARIF's default level is warning
	case "note":
		return "low"
	}
	return ""
}

// sarifPath reduces an artifact URI to a repo-relative path: file:// and any
// absolute prefix are stripped, and a path that climbs with ".." is cut to its
// basename. "" when nothing usable remains.
func sarifPath(uri string) string {
	p := strings.TrimSpace(uri)
	if len(p) > 4096 {
		return ""
	}
	p = strings.TrimPrefix(strings.TrimPrefix(p, "file:"), "//")
	if d, err := url.PathUnescape(p); err == nil {
		p = d
	}
	p = strings.ReplaceAll(p, "\\", "/")
	if len(p) >= 2 && p[1] == ':' { // drive letter
		p = p[2:]
	}
	p = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, p)
	climbs := false
	for _, seg := range strings.Split(p, "/") {
		climbs = climbs || seg == ".."
	}
	p = strings.TrimLeft(path.Clean("/"+p), "/")
	if climbs {
		p = path.Base(p)
	}
	if p == "." || p == "/" || p == "" {
		return ""
	}
	return clip(p, 512)
}

func (c *collector) sarifResult(raw json.RawMessage, run *sarifRun) {
	rec := fmt.Sprintf("run[%d].result[%d]", run.idx, run.seen)
	run.seen++
	c.res.Records++
	var r sarifResult
	if json.Unmarshal(raw, &r) != nil {
		c.res.Skipped++
		return
	}
	id := r.RuleID.s()
	if id == "" {
		id = r.Rule.ID.s()
	}
	rule, known := run.rules[id]
	if !known && r.RuleIndex != nil && *r.RuleIndex >= 0 && *r.RuleIndex < len(run.list) {
		rule = run.list[*r.RuleIndex]
		if id == "" {
			id = rule.ID.s()
		}
	}
	ruleID := Token(id)
	if ruleID == "" || len(r.Locations) == 0 {
		c.res.Skipped++
		return
	}
	loc := sarifPath(r.Locations[0].Physical.Artifact.URI.s())
	line := r.Locations[0].Physical.Region.StartLine
	if loc == "" {
		c.res.Skipped++
		return
	}
	if line < 0 {
		line = 0
	}
	level := firstNonEmpty(r.Level.s(), rule.Default.Level.s())
	sev := sarifSeverity(firstNonEmpty(rule.Props.SecSev.s(), r.Props.SecSev.s()), level)
	if sev == "" {
		c.res.Informational++
		return
	}
	target := loc
	if line > 0 {
		target += ":" + strconv.Itoa(line)
	}
	tags := append(append([]string{}, rule.Props.Tags...), r.Props.Tags...)
	cwe := parseCWE(tags...)
	title := firstNonEmpty(rule.Short.Text.s(), rule.Name.s(), r.Message.Text.s())
	c.finding(Finding{Tool: run.tool, RuleID: ruleID, Severity: sev, Target: target, Location: loc,
		Line: line, Title: title, CWE: cwe, Record: rec}, map[string]any{
		"message":           r.Message.Text.s(),
		"rule_name":         rule.Name.s(),
		"level":             level,
		"security_severity": firstNonEmpty(rule.Props.SecSev.s(), r.Props.SecSev.s()),
		"tags":              tags,
		"cwe":               cwe,
	})
}
