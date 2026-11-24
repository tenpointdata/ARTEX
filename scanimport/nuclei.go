package scanimport

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
)

type nucleiLine struct {
	TemplateID flex `json:"template-id"`
	Info       struct {
		Name           flex     `json:"name"`
		Severity       flex     `json:"severity"`
		Description    flex     `json:"description"`
		Tags           flexList `json:"tags"`
		Classification struct {
			CWE flexList `json:"cwe-id"`
		} `json:"classification"`
	} `json:"info"`
	Type        flex     `json:"type"`
	Host        flex     `json:"host"`
	IP          flex     `json:"ip"`
	MatchedAt   flex     `json:"matched-at"`
	MatcherName flex     `json:"matcher-name"`
	Extracted   flexList `json:"extracted-results"`
}

// parseNuclei reads nuclei -jsonl output line by line. A line over
// MaxLineBytes is counted in Oversized; a last line cut short marks the result
// truncated. A file in which no line is a nuclei result is an error.
func parseNuclei(c *collector, r io.Reader) error {
	c.res.Tool = ToolInfo{Name: "nuclei"}
	br := bufio.NewReaderSize(r, MaxLineBytes)
	recognized := false
	for lineNo := 1; ; lineNo++ {
		line, err := br.ReadSlice('\n')
		complete := err == nil
		if errors.Is(err, bufio.ErrBufferFull) {
			for errors.Is(err, bufio.ErrBufferFull) { // drop the rest of the line
				_, err = br.ReadSlice('\n')
			}
			c.res.Records++
			c.res.Oversized++
			line = nil
		} else if len(bytes.TrimSpace(line)) > 0 {
			c.res.Records++
			if c.full() {
				return nil
			}
			ok, syntaxErr := c.nucleiLine(line, fmt.Sprintf("line %d", lineNo))
			recognized = recognized || ok
			switch {
			case syntaxErr && !complete && errors.Is(err, io.EOF):
				c.res.Truncated = true // a final line cut short
			case syntaxErr:
				c.res.Skipped++
			}
		}
		if err != nil {
			if !errors.Is(err, io.EOF) {
				c.res.Truncated = true
			}
			break
		}
	}
	if c.res.Records > 0 && !recognized {
		return errors.New("scanimport: nuclei-jsonl: no line is a nuclei result")
	}
	return nil
}

// nucleiLine handles one JSONL record. recognized is true when the line is a
// JSON object carrying a template-id; syntaxErr is true when the line is not
// valid JSON (the caller decides between skipped and truncated).
func (c *collector) nucleiLine(line []byte, rec string) (recognized, syntaxErr bool) {
	var r nucleiLine
	if err := json.Unmarshal(line, &r); err != nil {
		var se *json.SyntaxError
		if errors.As(err, &se) || errors.Is(err, io.ErrUnexpectedEOF) {
			return false, true
		}
		c.res.Skipped++
		return false, false
	}
	recognized = r.TemplateID != ""
	tid := Token(r.TemplateID.s())
	target, host := targetOf(r.MatchedAt.s())
	hostTarget, hostHost := targetOf(r.Host.s())
	if host == "" {
		target, host = hostTarget, hostHost
	}
	if hostHost == "" {
		hostHost = host
	}
	if tid == "" || host == "" {
		c.res.Skipped++
		return recognized, false
	}
	// The scanned target becomes an asset whatever the severity.
	if o := Origin(r.Host.s()); o != "" {
		c.asset(Asset{Kind: KindHTTPService, URL: o, Record: rec})
	} else if net.ParseIP(hostHost) != nil {
		c.asset(Asset{Kind: KindIP, IP: hostHost, Record: rec})
	} else {
		c.asset(Asset{Kind: KindSubdomain, Domain: hostHost, Record: rec})
	}
	sev := normSeverity(r.Info.Severity.s())
	if sev == "" {
		c.res.Informational++
		return true, false
	}
	cwe := nucleiCWE(r.Info.Classification.CWE)
	c.finding(Finding{
		Tool: "nuclei", RuleID: tid, Severity: sev, Target: target, Host: host,
		Title: r.Info.Name.s(), CWE: cwe, Record: rec,
	}, map[string]any{
		"matched_at":        StripUserinfo(r.MatchedAt.s()),
		"host":              StripUserinfo(r.Host.s()),
		"matcher_name":      r.MatcherName.s(),
		"extracted_results": []string(r.Extracted),
		"description":       r.Info.Description.s(),
		"type":              r.Type.s(),
		"tags":              []string(r.Info.Tags),
		"cwe":               cwe,
		"ip":                r.IP.s(),
	})
	return true, false
}

// nucleiCWE parses classification.cwe-id entries ("CWE-79" or a bare number).
func nucleiCWE(l flexList) []int {
	texts := make([]string, 0, len(l))
	for _, e := range l {
		if _, err := strconv.Atoi(e); err == nil {
			e = "CWE-" + e
		}
		texts = append(texts, e)
	}
	return parseCWE(texts...)
}
