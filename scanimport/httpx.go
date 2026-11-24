package scanimport

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"strings"
)

type httpxLine struct {
	URL           flex            `json:"url"`
	StatusCode    *int            `json:"status_code"` // required: it tells httpx from other JSONL
	ContentLength int64           `json:"content_length"`
	Title         flex            `json:"title"`
	Tech          flexList        `json:"tech"`
	Webserver     flex            `json:"webserver"`
	Host          flex            `json:"host"`
	IP            flex            `json:"ip"`
	A             flexList        `json:"a"`
	TLS           json.RawMessage `json:"tls"`
}

// parseHttpx reads httpx -json (JSONL) output. Each probed URL becomes an
// http_service asset carrying the status and length seen; the page title and
// technologies are untrusted text. httpx yields no findings. A file in which
// no line carries a valid http(s) url is an error.
func parseHttpx(c *collector, r io.Reader) error {
	c.res.Tool = ToolInfo{Name: "httpx"}
	if !readJSONL(c, r, c.httpxLine) {
		return errors.New("scanimport: httpx-jsonl: no line is an httpx result")
	}
	return nil
}

func (c *collector) httpxLine(line []byte, rec string) (recognized, syntaxErr bool) {
	var r httpxLine
	if bad, syn := decodeLine(line, &r); bad {
		c.res.Skipped += boolInt(!syn)
		return false, syn
	}
	u, _ := URL(r.URL.s())
	if u == "" || r.StatusCode == nil {
		c.res.Skipped++
		return false, false
	}
	a := Asset{Kind: KindHTTPService, URL: u, Status: httpStatus(*r.StatusCode), Length: max(r.ContentLength, 0),
		IP: firstIP(append([]string{r.IP.s(), r.Host.s()}, r.A...)...), Title: cleanText(r.Title.s(), MaxTitleBytes),
		Tech: cleanList(r.Tech), Record: rec}
	tls := ""
	if len(r.TLS) > 0 && r.TLS[0] == '{' {
		tls = string(r.TLS)
	}
	a.Detail = buildDetail(map[string]any{"webserver": r.Webserver.s(), "host": StripUserinfo(r.Host.s()), "ip": r.IP.s(), "tls": tls})
	c.asset(a)
	return true, false
}

// decodeLine unmarshals one JSONL record into v. bad is true when it failed;
// syntax is then true when the line is not JSON at all (the caller decides
// between skipped and truncated), false when it is JSON of the wrong shape.
func decodeLine(line []byte, v any) (bad, syntax bool) {
	err := json.Unmarshal(line, v)
	if err == nil {
		return false, false
	}
	var se *json.SyntaxError
	return true, errors.As(err, &se) || errors.Is(err, io.ErrUnexpectedEOF)
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// httpStatus returns n when it is a plausible HTTP status, 0 otherwise.
func httpStatus(n int) int {
	if n >= 100 && n <= 999 {
		return n
	}
	return 0
}

// firstIP returns the first candidate that parses as an IP, canonical.
func firstIP(cands ...string) string {
	for _, s := range cands {
		if ip := net.ParseIP(strings.TrimSpace(s)); ip != nil {
			return ip.String()
		}
	}
	return ""
}

// cleanList cleans each untrusted entry and drops the empty ones, capped at
// 100 entries of 256 bytes.
func cleanList(l []string) []string {
	var out []string
	for _, s := range l {
		if len(out) >= 100 {
			break
		}
		if s = cleanText(s, 256); s != "" {
			out = append(out, s)
		}
	}
	return out
}
