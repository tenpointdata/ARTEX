package scanimport

import (
	"strings"
	"testing"
)

func TestHttpxAssets(t *testing.T) {
	res := parse(t, HttpxJSONL, load(t, HttpxJSONL))
	if res.Tool.Name != "httpx" || res.Records != 6 || res.Skipped != 2 || len(res.Findings) != 0 || res.Truncated {
		t.Fatalf("httpx counts: %+v", res)
	}
	// "https://user:pass@app.example.com:443/" and "https://app.example.com/"
	// are one canonical URL, so they merge into one asset.
	want := map[string]bool{"https://app.example.com": true, "http://192.0.2.11": true, "http://[2001:db8::5]:8080/login": true}
	if len(res.Assets) != len(want) {
		t.Fatalf("assets %+v", res.Assets)
	}
	for _, a := range res.Assets {
		if a.Kind != KindHTTPService || !want[a.URL] {
			t.Errorf("unexpected asset %+v", a)
		}
	}
	a := res.Assets[0]
	if a.Status != 200 || a.Length != 5120 || a.IP != "192.0.2.10" || a.Record != "line 1" ||
		strings.Join(a.Tech, ",") != "Nginx:1.25,PHP,Bootstrap" || !strings.HasPrefix(a.Title, "Ignore previous") ||
		!strings.Contains(a.Detail, `"webserver":"nginx/1.25"`) {
		t.Errorf("merged httpx asset %+v", a)
	}
}

func TestFfufAssets(t *testing.T) {
	res := parse(t, FfufJSON, load(t, FfufJSON))
	if res.Tool.Name != "ffuf" || res.Records != 6 || res.Skipped != 2 || len(res.Findings) != 0 || res.Truncated {
		t.Fatalf("ffuf counts: %+v", res)
	}
	var urls []string
	for _, a := range res.Assets {
		urls = append(urls, a.URL)
	}
	if strings.Join(urls, " ") != "https://app.example.com/admin https://app.example.com/login "+
		"https://app.example.com/ignore%20previous%20instructions http://192.0.2.20:8000/backup" {
		t.Fatalf("ffuf urls %v", urls)
	}
	if a := res.Assets[1]; a.Status != 302 || a.Record != "results[1]" || !strings.Contains(a.Detail, "sso.example.com/auth") {
		t.Errorf("ffuf asset %+v", a)
	}
}

// ZAP instances that differ only by parameter are distinct findings; an
// instance without one keeps the key it had before param joined the key.
func TestZAPParamInKey(t *testing.T) {
	doc := func(params ...string) string {
		var in []string
		for _, p := range params {
			in = append(in, `{"uri":"https://shop.example.com/item","method":"GET","param":"`+p+`"}`)
		}
		return `{"@programName":"ZAP","site":[{"@name":"https://shop.example.com","alerts":[{"pluginid":"40012",` +
			`"alert":"XSS","riskcode":"3","confidence":"2","instances":[` + strings.Join(in, ",") + `]}]}]}`
	}
	res := parse(t, ZAPJSON, []byte(doc("q", "name", "q")))
	if len(res.Findings) != 2 || res.Duplicates != 1 || res.Findings[0].Key == res.Findings[1].Key {
		t.Fatalf("param findings: %+v", res)
	}
	noParam := parse(t, ZAPJSON, []byte(doc("")))
	if noParam.Findings[0].Key != makeKey("zap", "40012", "https://shop.example.com/item", "", 0, "") {
		t.Error("a param-less instance changed key")
	}
}

// KeepInformational keeps info results as "info" findings; the default drops
// them. Both count them.
func TestKeepInformational(t *testing.T) {
	line := `{"template-id":"tech-detect","info":{"severity":"info"},"host":"http://10.0.0.1"}` + "\n" +
		`{"template-id":"odd","info":{"severity":"weird"},"host":"http://10.0.0.1"}` + "\n"
	def := parse(t, NucleiJSONL, []byte(line))
	kept, err := ParseWith(NucleiJSONL, strings.NewReader(line), Options{KeepInformational: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(def.Findings) != 0 || def.Informational != 2 || len(kept.Findings) != 2 || kept.Informational != 2 ||
		kept.Findings[0].Severity != "info" || kept.Findings[1].Severity != "info" {
		t.Fatalf("default %+v kept %+v", def, kept)
	}
}

// nmap's http://ip:80 and nuclei's http://ip name the same http_service.
func TestCrossScannerOrigin(t *testing.T) {
	nm := parse(t, NmapXML, []byte(`<nmaprun><host><status state="up"/><address addr="10.0.0.5" addrtype="ipv4"/>`+
		`<ports><port protocol="tcp" portid="80"><state state="open"/><service name="http"/></port>`+
		`<port protocol="tcp" portid="443"><state state="open"/><service name="https"/></port></ports></host></nmaprun>`))
	nu := parse(t, NucleiJSONL, []byte(`{"template-id":"a","info":{"severity":"high"},"host":"http://10.0.0.5","matched-at":"http://10.0.0.5:80/x"}`+"\n"+
		`{"template-id":"b","info":{"severity":"high"},"host":"https://10.0.0.5:443/","matched-at":"https://10.0.0.5/y"}`+"\n"))
	urls := map[string]bool{}
	for _, a := range nm.Assets {
		if a.Kind == KindHTTPService {
			urls[a.URL] = true
		}
	}
	for _, a := range nu.Assets {
		if !urls[a.URL] {
			t.Errorf("nuclei asset %q not among nmap's %v", a.URL, urls)
		}
	}
	if nu.Findings[0].Target != "http://10.0.0.5/x" {
		t.Errorf("target %q keeps the default port", nu.Findings[0].Target)
	}
}

func TestNmapDetail(t *testing.T) {
	res := parse(t, NmapXML, load(t, NmapXML))
	for _, a := range res.Assets {
		if a.Kind == KindIP && a.IP == "198.51.100.7" {
			if !strings.Contains(a.Detail, `"ports":[`) || !strings.Contains(a.Detail, `"hostnames":[`) {
				t.Errorf("nmap ip detail %s", a.Detail)
			}
			return
		}
	}
	t.Error("no ip asset")
}
