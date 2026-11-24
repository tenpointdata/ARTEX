package scanimport

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

type zapInstance struct {
	URI       flex `json:"uri"`
	Method    flex `json:"method"`
	Param     flex `json:"param"`
	Attack    flex `json:"attack"`
	Evidence  flex `json:"evidence"`
	OtherInfo flex `json:"otherinfo"`
}

type zapAlert struct {
	PluginID   flex          `json:"pluginid"`
	AlertRef   flex          `json:"alertRef"`
	Alert      flex          `json:"alert"`
	Name       flex          `json:"name"`
	RiskCode   flex          `json:"riskcode"`
	Confidence flex          `json:"confidence"`
	RiskDesc   flex          `json:"riskdesc"`
	Desc       flex          `json:"desc"`
	Solution   flex          `json:"solution"`
	Reference  flex          `json:"reference"`
	CWEID      flex          `json:"cweid"`
	WASCID     flex          `json:"wascid"`
	OtherInfo  flex          `json:"otherinfo"`
	Instances  []zapInstance `json:"instances"`
}

// zapSite is the site-level context an alert is read in.
type zapSite struct {
	name, host, port, ssl flex
	idx                   int
	added                 bool
}

// zapSeverity maps ZAP's riskcode (0..3) to a severity, "" for informational.
func zapSeverity(code string) string {
	switch strings.TrimSpace(code) {
	case "3":
		return "high"
	case "2":
		return "medium"
	case "1":
		return "low"
	}
	return ""
}

// zapFalsePositive reports ZAP's confidence 0 / "False Positive".
func zapFalsePositive(conf string) bool {
	c := strings.ToLower(strings.TrimSpace(conf))
	return c == "0" || c == "false positive" || c == "falsepositive"
}

// parseZAP streams ZAP's traditional JSON report: sites one at a time and,
// inside a site, alerts one at a time, so a file cut short keeps the alerts
// read before the cut.
func parseZAP(c *collector, r io.Reader) error {
	dec := json.NewDecoder(r)
	recognized := false
	siteIdx := 0
	err := walkObject(dec, func(key string) error {
		switch key {
		case "@programName", "@version":
			var v flex
			if _, err := decodeLoose(dec, &v); err != nil {
				return err
			}
			if key == "@programName" {
				recognized = true
				c.res.Tool.Name = toolName(v.s())
				if c.res.Tool.Name == "" {
					c.res.Tool.Name = "zap"
				}
			} else {
				c.res.Tool.Version = versionToken(v.s())
			}
			return nil
		case "site":
			recognized = true
			return walkArray(dec, func() error {
				s := &zapSite{idx: siteIdx}
				siteIdx++
				err := walkObject(dec, func(k string) error { return c.zapSiteKey(dec, s, k) })
				c.zapSiteAsset(s)
				return err
			})
		}
		return skipValue(dec)
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
		return fmt.Errorf("scanimport: zap-json: not a ZAP JSON report: %w", err)
	}
	return errors.New("scanimport: zap-json: no site or @programName key")
}

func (c *collector) zapSiteKey(dec *json.Decoder, s *zapSite, key string) error {
	var dst *flex
	switch key {
	case "@name":
		dst = &s.name
	case "@host":
		dst = &s.host
	case "@port":
		dst = &s.port
	case "@ssl":
		dst = &s.ssl
	case "alerts":
		c.zapSiteAsset(s)
		alertIdx := 0
		return walkArray(dec, func() error {
			var a zapAlert
			ok, err := decodeLoose(dec, &a)
			if err != nil {
				return err
			}
			rec := fmt.Sprintf("site[%d].alert[%d]", s.idx, alertIdx)
			alertIdx++
			c.res.Records++
			if !ok {
				c.res.Skipped++
				return nil
			}
			if c.full() {
				return errStop
			}
			c.zapAlert(s, a, rec)
			return nil
		})
	default:
		return skipValue(dec)
	}
	_, err := decodeLoose(dec, dst)
	return err
}

// zapSiteAsset records the site as an http_service asset once.
func (c *collector) zapSiteAsset(s *zapSite) {
	if s.added {
		return
	}
	s.added = true
	o := Origin(s.name.s())
	if o == "" && Host(s.host.s()) != "" {
		scheme := "http"
		if l := strings.ToLower(s.ssl.s()); l == "true" || l == "1" {
			scheme = "https"
		}
		o = Origin(scheme + "://" + joinHost(Host(s.host.s()), validPort(s.port.s())))
	}
	if o != "" {
		c.asset(Asset{Kind: KindHTTPService, URL: o, Record: fmt.Sprintf("site[%d]", s.idx)})
	}
}

func validPort(p string) string {
	if n, err := strconv.Atoi(strings.TrimSpace(p)); err == nil && n > 0 && n <= 65535 {
		return strconv.Itoa(n)
	}
	return ""
}

func (c *collector) zapAlert(s *zapSite, a zapAlert, rec string) {
	if zapFalsePositive(a.Confidence.s()) {
		c.res.Skipped++
		return
	}
	rule := Token(a.PluginID.s())
	if rule == "" {
		rule = Token(a.AlertRef.s())
	}
	if rule == "" {
		c.res.Skipped++
		return
	}
	sev := zapSeverity(a.RiskCode.s())
	title := a.Name.s()
	if title == "" {
		title = a.Alert.s()
	}
	var cwe []int
	if n, err := strconv.Atoi(strings.TrimSpace(a.CWEID.s())); err == nil && n > 0 {
		cwe = []int{n}
	}
	insts := a.Instances
	if len(insts) == 0 {
		insts = []zapInstance{{URI: s.name}}
	}
	for i, in := range insts {
		irec := rec
		if len(a.Instances) > 0 {
			irec = fmt.Sprintf("%s.instance[%d]", rec, i)
		}
		target, host := URL(in.URI.s())
		if target == "" {
			c.res.Skipped++
			continue
		}
		// The parameter is part of the identity: the same rule on one URL
		// through two parameters is two places to fix. It is hashed into the
		// key only; its text stays in the untrusted detail.
		c.findingVariant(Finding{Tool: "zap", RuleID: rule, Severity: sev, Target: target, Host: host,
			Title: title, CWE: cwe, Record: irec}, zapVariant(in.Param.s()), map[string]any{
			"alert":       a.Alert.s(),
			"riskdesc":    a.RiskDesc.s(),
			"confidence":  a.Confidence.s(),
			"description": a.Desc.s(),
			"solution":    a.Solution.s(),
			"reference":   a.Reference.s(),
			"cwe":         cwe,
			"wasc":        a.WASCID.s(),
			"method":      in.Method.s(),
			"param":       in.Param.s(),
			"attack":      in.Attack.s(),
			"evidence":    in.Evidence.s(),
			"other_info":  firstNonEmpty(in.OtherInfo.s(), a.OtherInfo.s()),
		})
	}
}

// zapVariant is the key variant of a ZAP instance: its parameter, "" when it
// has none (header and page-level alerts then collapse per URL as before).
func zapVariant(param string) string {
	if p := strings.TrimSpace(param); p != "" {
		return "param=" + p
	}
	return ""
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}
