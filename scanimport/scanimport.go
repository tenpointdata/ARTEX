// Package scanimport turns scanner output files (nmap, nuclei, ZAP, Burp,
// SARIF) into typed assets and unverified candidate findings, with a
// provenance pointer into the file for every item.
//
// The parsers are deterministic and never interpret target-shaped text: only
// typed fields are validated (hosts, URLs, ports, rule ids, severities); every
// free-text field (titles, descriptions, evidence) is returned as untrusted
// data for the caller to quote. Credentials embedded in URLs or free text are
// stripped. The package imports nothing from the rest of ARTEX.
package scanimport

import (
	"fmt"
	"io"
)

// Format names a supported scanner output format.
type Format string

// Supported formats.
const (
	NmapXML     Format = "nmap-xml"
	NucleiJSONL Format = "nuclei-jsonl"
	ZAPJSON     Format = "zap-json"
	BurpXML     Format = "burp-xml"
	SARIF       Format = "sarif"
)

// Limits.
const (
	// MaxItems caps the assets plus findings one file may yield. Records past
	// it are dropped and the result is marked Truncated.
	MaxItems = 50000
	// MaxLineBytes caps one JSONL line; a longer line is counted in Oversized.
	MaxLineBytes = 64 * 1024
	// MaxDetailBytes caps a Finding's Detail JSON.
	MaxDetailBytes = 16 * 1024
	// MaxTitleBytes caps a Finding's Title.
	MaxTitleBytes = 512
)

// Formats lists the supported formats in a stable order.
func Formats() []Format {
	return []Format{NmapXML, NucleiJSONL, ZAPJSON, BurpXML, SARIF}
}

// AssetKind is the kind of an imported asset.
type AssetKind string

// Asset kinds.
const (
	KindIP          AssetKind = "ip"
	KindSubdomain   AssetKind = "subdomain"
	KindHTTPService AssetKind = "http_service"
	KindService     AssetKind = "service"
)

// Port is an open port on an ip asset.
type Port struct {
	Port    int    `json:"port"`
	Proto   string `json:"proto"`
	Service string `json:"service"`
}

// Asset is something the file says exists. Every field is validated.
type Asset struct {
	Kind      AssetKind `json:"kind"`
	IP        string    `json:"ip,omitempty"`     // validated via Host
	Domain    string    `json:"domain,omitempty"` // validated via Host
	URL       string    `json:"url,omitempty"`    // http_service: scheme://host[:port], no userinfo or path
	Port      int       `json:"port,omitempty"`   // service
	Proto     string    `json:"proto,omitempty"`  // service: tcp|udp|sctp
	Service   string    `json:"service,omitempty"`
	OpenPorts []Port    `json:"open_ports,omitempty"` // ip
	Record    string    `json:"record"`               // provenance, e.g. "host[3]", "line 17"
}

// Finding is an unverified candidate finding reported by the scanner. Title
// and Detail are untrusted free text: the caller must quote them, never
// interpret them.
type Finding struct {
	Tool     string `json:"tool"`
	RuleID   string `json:"rule_id"`
	Severity string `json:"severity"` // critical|high|medium|low
	Target   string `json:"target"`   // sanitized URL or host[:port]; SARIF "path[:line]"
	Host     string `json:"host,omitempty"`
	Location string `json:"location,omitempty"` // SARIF repo-relative path
	Line     int    `json:"line,omitempty"`
	Title    string `json:"title"`            // UNTRUSTED
	Detail   string `json:"detail,omitempty"` // UNTRUSTED compact JSON, <= MaxDetailBytes
	CWE      []int  `json:"cwe,omitempty"`
	Key      string `json:"key"` // stable dedup key
	Record   string `json:"record"`
}

// ToolInfo identifies the scanner that produced the file, when it says.
type ToolInfo struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

// Result is what one file yields.
//
// Records counts the records examined (nmap hosts, JSONL lines, ZAP alerts,
// Burp issues, SARIF results). Skipped counts records or instances dropped
// because a typed field failed validation (or ZAP false positives).
// Informational counts would-be findings of informational severity, which are
// not findings. Oversized counts JSONL lines over MaxLineBytes. Duplicates
// counts findings dropped because the same Key was already produced in this
// file. Truncated is set when the file ended early, was malformed after a
// valid start, or hit MaxItems.
type Result struct {
	Format        Format    `json:"format"`
	Tool          ToolInfo  `json:"tool"`
	Assets        []Asset   `json:"assets"`
	Findings      []Finding `json:"findings"`
	Records       int       `json:"records"`
	Skipped       int       `json:"skipped"`
	Informational int       `json:"informational"`
	Oversized     int       `json:"oversized"`
	Duplicates    int       `json:"duplicates"`
	Truncated     bool      `json:"truncated"`
}

// Parse reads one scanner file. It returns an error only when the input is not
// that format at all; a partial or cut-short file yields what was read with
// Truncated set.
func Parse(f Format, r io.Reader) (*Result, error) {
	c := newCollector(f)
	var err error
	switch f {
	case NmapXML:
		err = parseNmap(c, r)
	case NucleiJSONL:
		err = parseNuclei(c, r)
	case ZAPJSON:
		err = parseZAP(c, r)
	case BurpXML:
		err = parseBurp(c, r)
	case SARIF:
		err = parseSARIF(c, r)
	default:
		return nil, fmt.Errorf("scanimport: unknown format %q", f)
	}
	if err != nil {
		return nil, err
	}
	return c.res, nil
}
