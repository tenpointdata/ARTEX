package scanimport

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
)

type burpIssue struct {
	Serial     string `xml:"serialNumber"`
	Type       string `xml:"type"`
	Name       string `xml:"name"`
	Path       string `xml:"path"`
	Location   string `xml:"location"`
	Severity   string `xml:"severity"`
	Confidence string `xml:"confidence"`
	Background string `xml:"issueBackground"`
	Detail     string `xml:"issueDetail"`
	RemBackgr  string `xml:"remediationBackground"`
	RemDetail  string `xml:"remediationDetail"`
	Classifs   string `xml:"vulnerabilityClassifications"`
	Host       struct {
		IP   string `xml:"ip,attr"`
		Text string `xml:",chardata"`
	} `xml:"host"`
	// Only the presence of requestresponse is read: its request and response
	// bodies can carry credentials and are never decoded.
	RR []struct {
		XMLName xml.Name
	} `xml:"requestresponse"`
}

// parseBurp streams a Burp Suite issues XML export one <issue> at a time. A
// file cut short keeps the issues read before the cut.
func parseBurp(c *collector, r io.Reader) error {
	dec := newXMLDecoder(r)
	root := false
	idx := 0
	for {
		tok, err := dec.Token()
		if err != nil {
			if !root {
				return fmt.Errorf("scanimport: burp-xml: not a Burp XML file: %w", err)
			}
			if !errors.Is(err, io.EOF) {
				c.res.Truncated = true
			}
			return nil
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		if !root {
			if se.Name.Local != "issues" {
				return errors.New("scanimport: burp-xml: no issues element")
			}
			root = true
			c.res.Tool = ToolInfo{Name: "burp"}
			for _, a := range se.Attr {
				if a.Name.Local == "burpVersion" {
					c.res.Tool.Version = versionToken(a.Value)
				}
			}
			continue
		}
		if se.Name.Local != "issue" {
			continue
		}
		var is burpIssue
		if err := dec.DecodeElement(&is, &se); err != nil {
			c.res.Truncated = true
			return nil
		}
		c.res.Records++
		if c.full() {
			return nil
		}
		c.burpIssue(is, fmt.Sprintf("issue[%d]", idx))
		idx++
	}
}

func (c *collector) burpIssue(is burpIssue, rec string) {
	hostText := strings.TrimSpace(is.Host.Text)
	origin := Origin(hostText)
	rule := Token(strings.TrimSpace(is.Type))
	target, host := URL(hostText + strings.TrimSpace(is.Path))
	if target == "" && origin != "" {
		target, host = URL(origin)
	}
	if rule == "" || target == "" {
		c.res.Skipped++
		return
	}
	c.asset(Asset{Kind: KindHTTPService, URL: origin, Record: rec})
	if ip := Host(is.Host.IP); ip != "" && net.ParseIP(ip) != nil {
		c.asset(Asset{Kind: KindIP, IP: ip, Record: rec})
	}
	sev := normSeverity(is.Severity)
	cwe := parseCWE(is.Classifs)
	c.finding(Finding{Tool: "burp", RuleID: rule, Severity: sev, Target: target, Host: host,
		Title: is.Name, CWE: cwe, Record: rec}, map[string]any{
		"serial_number":          strings.TrimSpace(is.Serial),
		"confidence":             strings.TrimSpace(is.Confidence),
		"location":               is.Location,
		"issue_background":       is.Background,
		"issue_detail":           is.Detail,
		"remediation_background": is.RemBackgr,
		"remediation_detail":     is.RemDetail,
		"classifications":        is.Classifs,
		"cwe":                    cwe,
		"has_request_response":   len(is.RR) > 0,
	})
}
