package scanimport

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var fixtures = map[Format]string{
	NmapXML:     "nmap.xml",
	NucleiJSONL: "nuclei.jsonl",
	ZAPJSON:     "zap.json",
	BurpXML:     "burp.xml",
	SARIF:       "semgrep.sarif",
	HttpxJSONL:  "httpx.jsonl",
	FfufJSON:    "ffuf.json",
}

func load(t *testing.T, f Format) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", fixtures[f]))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func parse(t *testing.T, f Format, b []byte) *Result {
	t.Helper()
	res, err := Parse(f, bytes.NewReader(b))
	if err != nil {
		t.Fatalf("%s: %v", f, err)
	}
	return res
}

func TestCounts(t *testing.T) {
	tests := []struct {
		f                                                           Format
		tool                                                        string
		assets, findings, records, skipped, informational, oversize int
		severities                                                  []string
	}{
		{NmapXML, "nmap", 9, 0, 4, 1, 0, 0, nil},
		{NucleiJSONL, "nuclei", 4, 3, 5, 1, 1, 0, []string{"high", "critical", "medium"}},
		{ZAPJSON, "zap", 2, 4, 6, 2, 1, 0, []string{"high", "high", "medium", "low"}},
		{BurpXML, "burp", 4, 3, 5, 1, 1, 0, []string{"high", "medium", "low"}},
		{SARIF, "semgrep-oss", 0, 3, 5, 1, 1, 0, []string{"critical", "medium", "low"}},
		{HttpxJSONL, "httpx", 3, 0, 6, 2, 0, 0, nil},
		{FfufJSON, "ffuf", 4, 0, 6, 2, 0, 0, nil},
	}
	for _, tc := range tests {
		t.Run(string(tc.f), func(t *testing.T) {
			res := parse(t, tc.f, load(t, tc.f))
			if res.Format != tc.f || res.Tool.Name != tc.tool {
				t.Errorf("format/tool = %s/%q", res.Format, res.Tool.Name)
			}
			got := []int{len(res.Assets), len(res.Findings), res.Records, res.Skipped, res.Informational, res.Oversized}
			want := []int{tc.assets, tc.findings, tc.records, tc.skipped, tc.informational, tc.oversize}
			if fmt.Sprint(got) != fmt.Sprint(want) {
				t.Errorf("assets/findings/records/skipped/info/oversized = %v, want %v\n%+v", got, want, res)
			}
			var sevs []string
			for _, f := range res.Findings {
				sevs = append(sevs, f.Severity)
				if f.Record == "" || f.Tool == "" || f.RuleID == "" || f.Target == "" || !strings.HasPrefix(f.Key, "imp-") || len(f.Key) != 28 {
					t.Errorf("incomplete finding %+v", f)
				}
				if f.Detail == "" || len(f.Detail) > MaxDetailBytes || !json.Valid([]byte(f.Detail)) {
					t.Errorf("bad detail %q", f.Detail)
				}
			}
			if fmt.Sprint(sevs) != fmt.Sprint(tc.severities) {
				t.Errorf("severities = %v, want %v", sevs, tc.severities)
			}
			for _, a := range res.Assets {
				if a.Record == "" {
					t.Errorf("asset without record: %+v", a)
				}
			}
			if res.Truncated {
				t.Error("complete fixture marked truncated")
			}
		})
	}
}

func TestNmapAssets(t *testing.T) {
	res := parse(t, NmapXML, load(t, NmapXML))
	if res.Tool.Version != "7.94" {
		t.Errorf("version = %q", res.Tool.Version)
	}
	find := func(kind AssetKind, id string) *Asset {
		for i, a := range res.Assets {
			if a.Kind == kind && (a.IP == id || a.Domain == id || a.URL == id) {
				return &res.Assets[i]
			}
		}
		return nil
	}
	if ip := find(KindIP, "198.51.100.7"); ip == nil || len(ip.OpenPorts) != 4 || ip.Record != "host[0]" {
		t.Errorf("ip asset = %+v", ip)
	}
	if d := find(KindSubdomain, "web.example.com"); d == nil || d.IP != "198.51.100.7" || d.Record != "host[0]" {
		t.Errorf("subdomain asset = %+v", d)
	}
	for _, u := range []string{"http://198.51.100.7", "https://198.51.100.7", "http://198.51.100.9:8080"} {
		if find(KindHTTPService, u) == nil {
			t.Errorf("missing http_service %s", u)
		}
	}
	if find(KindSubdomain, "static-7.isp.example.net") != nil {
		t.Error("PTR name bound")
	}
	for _, a := range res.Assets {
		if a.Kind == KindService && a.Port == 53 && (a.Proto != "udp" || a.Service != "domain" || a.Record != "host[0].port[53/udp]") {
			t.Errorf("udp service = %+v", a)
		}
		if a.Kind == KindService && a.Port == 9999 && a.Service != "unknown" {
			t.Errorf("hostile service name kept: %+v", a)
		}
	}
}

func TestFindingDetails(t *testing.T) {
	nuc := parse(t, NucleiJSONL, load(t, NucleiJSONL))
	f := nuc.Findings[0]
	if f.RuleID != "exposed-panel" || f.Record != "line 1" || f.Target != "http://app.example.com:8080/admin" ||
		f.Host != "app.example.com" || len(f.CWE) != 1 || f.CWE[0] != 200 ||
		!strings.Contains(f.Title, "Ignore previous instructions") || !strings.Contains(f.Detail, "login-form") {
		t.Errorf("nuclei finding = %+v", f)
	}
	if g := nuc.Findings[1]; g.Target != "192.0.2.20:443" || g.Record != "line 2" || !strings.Contains(g.Detail, "root:x:0:0") {
		t.Errorf("nuclei finding 2 = %+v", g)
	}
	if g := nuc.Findings[2]; g.Record != "line 6" || g.Target != "https://api.example.com/v1/debug?x=1" {
		t.Errorf("nuclei finding 3 = %+v", g)
	}
	kinds := map[AssetKind]int{}
	for _, a := range nuc.Assets {
		kinds[a.Kind]++
		if a.Kind == KindHTTPService && strings.ContainsAny(strings.TrimPrefix(strings.TrimPrefix(a.URL, "https://"), "http://"), "/@") {
			t.Errorf("http_service not an origin: %q", a.URL)
		}
	}
	if kinds[KindHTTPService] != 3 || kinds[KindIP] != 1 {
		t.Errorf("nuclei asset kinds = %v", kinds)
	}

	zap := parse(t, ZAPJSON, load(t, ZAPJSON))
	if f := zap.Findings[0]; f.RuleID != "40018" || f.Target != "https://shop.example.com/item?id=1" ||
		f.Record != "site[0].alert[0].instance[0]" || len(f.CWE) != 1 || f.CWE[0] != 89 {
		t.Errorf("zap finding = %+v", f)
	}
	if f := zap.Findings[3]; f.Target != "http://blog.example.com:8080" || f.Record != "site[1].alert[0]" {
		t.Errorf("zap no-instance finding = %+v", f)
	}
	if zap.Tool.Version != "2.14.0" {
		t.Errorf("zap version %q", zap.Tool.Version)
	}

	burp := parse(t, BurpXML, load(t, BurpXML))
	if f := burp.Findings[0]; f.RuleID != "1049088" || f.Target != "https://shop.example.com/item" || f.Record != "issue[0]" ||
		len(f.CWE) != 1 || f.CWE[0] != 89 || !strings.Contains(f.Detail, `"has_request_response":true`) {
		t.Errorf("burp finding = %+v", f)
	}
	if burp.Tool.Version != "2024.2.1" {
		t.Errorf("burp version %q", burp.Tool.Version)
	}
	if strings.Contains(fmt.Sprint(burp), "SECRETREQBODY") || strings.Contains(fmt.Sprint(burp), "SECRETRESPBODY") {
		t.Error("request/response body leaked")
	}

	sar := parse(t, SARIF, load(t, SARIF))
	locs := []string{"app/views/orders.py", "home/runner/work/repo/repo/web/server.js", "app.conf"}
	for i, f := range sar.Findings {
		if f.Location != locs[i] || f.Tool != "semgrep-oss" || f.Host != "" {
			t.Errorf("sarif finding %d = %+v", i, f)
		}
	}
	if f := sar.Findings[0]; f.Target != "app/views/orders.py:42" || f.Line != 42 || len(f.CWE) != 1 || f.CWE[0] != 89 ||
		f.RuleID != "python.django.security.injection.sql.sql-injection-using-raw" || f.Record != "run[0].result[0]" {
		t.Errorf("sarif finding 0 = %+v", f)
	}
	if strings.Contains(sar.Findings[0].Detail, "hunter2") {
		t.Error("sarif snippet leaked into detail")
	}
	if sar.Tool.Version != "1.60.0" {
		t.Errorf("sarif version %q", sar.Tool.Version)
	}
}

func TestKeysStableAndDistinct(t *testing.T) {
	for _, f := range Formats() {
		a, b := parse(t, f, load(t, f)), parse(t, f, load(t, f))
		seen := map[string]bool{}
		for i := range a.Findings {
			if a.Findings[i].Key != b.Findings[i].Key {
				t.Errorf("%s: key unstable for %s", f, a.Findings[i].Record)
			}
			if seen[a.Findings[i].Key] {
				t.Errorf("%s: duplicate key %s", f, a.Findings[i].Key)
			}
			seen[a.Findings[i].Key] = true
		}
	}
	if k := makeKey("a", "b", "HTTP://X.com/p", "", 0, ""); k != makeKey("a", "b", "http://x.com/p", "", 0, "") {
		t.Error("host case changes key")
	}
	if makeKey("a", "b", "t", "", 0, "") == makeKey("a", "b", "t", "", 1, "") || makeKey("a", "b", "t", "", 0, "") == makeKey("a", "c", "t", "", 0, "") {
		t.Error("key collisions")
	}
}

func TestNoCredentialsAnywhere(t *testing.T) {
	for _, f := range Formats() {
		b, err := json.Marshal(parse(t, f, load(t, f)))
		if err != nil {
			t.Fatal(err)
		}
		for _, bad := range []string{"pass@", "hunter2", "s3cr3tpass", "user:pass", "SECRETREQBODY", "SECRETRESPBODY"} {
			if bytes.Contains(b, []byte(bad)) {
				t.Errorf("%s: result contains %q: %s", f, bad, b)
			}
		}
	}
}

func TestTruncated(t *testing.T) {
	for _, f := range Formats() {
		t.Run(string(f), func(t *testing.T) {
			full := load(t, f)
			cut := len(full) / 2
			res := parse(t, f, full[:cut])
			if !res.Truncated && full[cut-1] != '\n' {
				t.Errorf("cut file not marked truncated: %+v", res)
			}
			ref := parse(t, f, full)
			if len(res.Findings) > len(ref.Findings) || len(res.Assets) > len(ref.Assets) {
				t.Errorf("cut file yields more than the whole file")
			}
			late := parse(t, f, full[:len(full)*3/4])
			if len(late.Findings)+len(late.Assets) == 0 {
				t.Errorf("3/4 cut file kept nothing: %+v", late)
			}
		})
	}
	// A JSONL file ending mid-line is truncated, not skipped.
	res := parse(t, NucleiJSONL, []byte(`{"template-id":"a-b","info":{"severity":"low"},"host":"1.2.3.4"}`+"\n"+`{"template-id":"c`))
	if !res.Truncated || res.Skipped != 0 || len(res.Findings) != 1 {
		t.Errorf("mid-line cut: %+v", res)
	}
}

func TestWrongFormat(t *testing.T) {
	for _, f := range Formats() {
		for _, other := range Formats() {
			if f == other {
				continue
			}
			if _, err := Parse(f, bytes.NewReader(load(t, other))); err == nil {
				t.Errorf("Parse(%s) accepted a %s file", f, other)
			}
		}
		for _, junk := range []string{"", "hello world", "<html></html>", "[]", "{}"} {
			if _, err := Parse(f, strings.NewReader(junk)); err == nil && junk != "" {
				t.Errorf("Parse(%s) accepted %q", f, junk)
			}
		}
	}
	if _, err := Parse("nessus", strings.NewReader("")); err == nil {
		t.Error("unknown format accepted")
	}
}

func TestDetailCap(t *testing.T) {
	var many []string
	for i := 0; i < 200; i++ {
		many = append(many, strings.Repeat("x", 3000))
	}
	line, _ := json.Marshal(map[string]any{
		"template-id": "big", "host": "http://big.example.com",
		"info":              map[string]any{"name": strings.Repeat("N", 5000), "severity": "high", "description": strings.Repeat("d", 100000)},
		"extracted-results": many,
	})
	ok := `{"template-id":"ok","info":{"severity":"low"},"host":"1.2.3.4"}` + "\n"
	res := parse(t, NucleiJSONL, append(line, []byte("\n"+ok)...))
	if res.Oversized != 1 || len(res.Findings) != 1 { // the big line itself is over MaxLineBytes
		t.Fatalf("expected oversized line: %+v", res)
	}
	line, _ = json.Marshal(map[string]any{
		"template-id": "big", "host": "http://big.example.com",
		"info":              map[string]any{"name": strings.Repeat("N", 5000), "severity": "high", "description": strings.Repeat("d", 6000)},
		"extracted-results": many[:15],
	})
	res = parse(t, NucleiJSONL, append(line, '\n'))
	if len(res.Findings) != 1 {
		t.Fatalf("%+v", res)
	}
	f := res.Findings[0]
	if len(f.Detail) > MaxDetailBytes || !json.Valid([]byte(f.Detail)) || len(f.Title) > MaxTitleBytes {
		t.Errorf("detail %d bytes, title %d bytes", len(f.Detail), len(f.Title))
	}
	// Even when nothing fits the tightest tier, the cap holds and JSON stays valid.
	m := map[string]any{}
	for i := 0; i < 100; i++ {
		m[fmt.Sprintf("k%03d", i)] = strings.Repeat("z", 4000)
	}
	d := buildDetail(m)
	if len(d) > MaxDetailBytes || !json.Valid([]byte(d)) || !strings.Contains(d, `"truncated":true`) {
		t.Errorf("buildDetail: %d bytes valid=%v", len(d), json.Valid([]byte(d)))
	}
}

func TestOversizedAndItemCap(t *testing.T) {
	var b bytes.Buffer
	b.WriteString(`{"template-id":"a","info":{"severity":"low"},"host":"1.2.3.4"}` + "\n")
	b.WriteString(`{"template-id":"b","info":{"description":"` + strings.Repeat("x", MaxLineBytes) + `","severity":"low"},"host":"1.2.3.4"}` + "\n")
	b.WriteString(`{"template-id":"c","info":{"severity":"low"},"host":"1.2.3.4"}` + "\n")
	res := parse(t, NucleiJSONL, b.Bytes())
	if res.Oversized != 1 || res.Records != 3 || len(res.Findings) != 2 {
		t.Errorf("oversized: %+v", res)
	}
	b.Reset()
	for i := 0; i < MaxItems+20; i++ {
		fmt.Fprintf(&b, `{"template-id":"t%d","info":{"severity":"low"},"host":"1.2.3.4"}`+"\n", i)
	}
	res = parse(t, NucleiJSONL, b.Bytes())
	if !res.Truncated || len(res.Findings)+len(res.Assets) != MaxItems {
		t.Errorf("cap: truncated=%v items=%d", res.Truncated, len(res.Findings)+len(res.Assets))
	}
}

func TestHelpers(t *testing.T) {
	for in, want := range map[string]string{
		"Example.COM.": "example.com", "10.0.0.1": "10.0.0.1", "::1": "::1",
		"bad host": "", "a/b": "", "": "", "http://x": "",
	} {
		if got := Host(in); got != want {
			t.Errorf("Host(%q) = %q, want %q", in, got, want)
		}
	}
	for in, want := range map[string]string{"ok-1.2": "ok-1.2", "a b": "", "": "", strings.Repeat("a", 129): ""} {
		if got := Token(in); got != want {
			t.Errorf("Token(%q) = %q", in, got)
		}
	}
	u, h := URL(" HTTP://User:Pw@Example.com:8080/a/b?c=d#frag ")
	if u != "http://example.com:8080/a/b?c=d" || h != "example.com" {
		t.Errorf("URL = %q %q", u, h)
	}
	if u, _ := URL("ftp://x.com"); u != "" {
		t.Error("ftp accepted")
	}
	if u, _ := URL("http://[::1]:80/x"); u != "http://[::1]/x" {
		t.Errorf("ipv6 url = %q", u)
	}
	// Default ports and a bare "/" path drop out, so nmap's http://ip:80 and
	// nuclei's http://ip name one asset; a non-default port stays.
	for in, want := range map[string]string{
		"http://10.0.0.1:80": "http://10.0.0.1", "http://10.0.0.1/": "http://10.0.0.1", "https://h.com:443/": "https://h.com",
		"https://h.com:80": "https://h.com:80", "http://h.com:443": "http://h.com:443", "http://h.com/?": "http://h.com/?",
		"http://h.com/?a=1": "http://h.com/?a=1", "HTTPS://H.com:443/p": "https://h.com/p",
	} {
		if got, _ := URL(in); got != want {
			t.Errorf("URL(%q) = %q, want %q", in, got, want)
		}
	}
	if Origin("http://10.0.0.1:80/x") != Origin("http://10.0.0.1") {
		t.Error("Origin keeps the default port")
	}
	if o := Origin("https://u:p@x.com:8443/a?b"); o != "https://x.com:8443" {
		t.Errorf("Origin = %q", o)
	}
	for in, want := range map[string]string{
		"http://u:p@h.com/x": "http://h.com/x", "u:p@h.com:80": "h.com:80", "h.com": "h.com",
		"/path/with@at": "/path/with@at", "http://h.com/a@b": "http://h.com/a@b", "//u:p@h/x": "//h/x",
	} {
		if got := StripUserinfo(in); got != want {
			t.Errorf("StripUserinfo(%q) = %q, want %q", in, got, want)
		}
	}
	if got := ScrubCreds("see ftp://a:b@c.d/x and https://u:p@s@h.com/y ok"); strings.Contains(got, "a:b") || strings.Contains(got, "p@") {
		t.Errorf("ScrubCreds = %q", got)
	}
	for in, want := range map[string]string{
		"src/a.py": "src/a.py", "/abs/path/a.py": "abs/path/a.py", "file:///x/y.go": "x/y.go", "../../etc/passwd": "passwd",
		"a/../../b.txt": "b.txt", "C:\\proj\\a.cs": "proj/a.cs", "": "", "..": "", "a%2Fb.py": "a/b.py",
	} {
		if got := sarifPath(in); got != want {
			t.Errorf("sarifPath(%q) = %q, want %q", in, got, want)
		}
	}
	if got := sarifSeverity("9.0", "none"); got != "critical" {
		t.Error(got)
	}
	if sarifSeverity("0", "error") != "" || sarifSeverity("", "none") != "" || sarifSeverity("", "note") != "low" || sarifSeverity("x", "error") != "high" {
		t.Error("sarifSeverity")
	}
	if got := toolName("Semgrep OSS (beta)"); got != "semgrep-oss-beta" {
		t.Errorf("toolName = %q", got)
	}
	if got := cleanText("a\x00b\nc \xff", 100); got != "a b c" {
		t.Errorf("cleanText = %q", got)
	}
}
