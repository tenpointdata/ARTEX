package ticketing

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/Autumn-27/artex/notify"
)

// DefectDojo adapter (REST API v2, findings).
//
// Config: base_url, api_key (secret, sent as `Authorization: Token <key>`),
// test_id (the DefectDojo test findings are attached to), found_by (optional
// list of test-type ids; DefectDojo servers commonly require it on create).
//
// Identity: unique_id_from_tool carries Key; Find filters on it within the test.
// State and resolution are expressed by DefectDojo's boolean flags (active,
// is_mitigated, false_p, risk_accepted, duplicate), see ddFlags.
//
// "verified" is only ever sent true when Ticket.Verified is true. On update it is
// omitted otherwise, so a human's verification is never cleared by a re-push.

type defectDojoTracker struct{}

func (defectDojoTracker) Kind() string              { return KindDefectDojo }
func (defectDojoTracker) SecretKeys() []string      { return []string{"api_key"} }
func (defectDojoTracker) DestinationKeys() []string { return []string{"base_url", "test_id"} }

func (defectDojoTracker) Validate(cfg map[string]any) error {
	if _, err := baseURL(cfg, ""); err != nil {
		return err
	}
	if cfgString(cfg, "api_key") == "" {
		return fmt.Errorf("api_key is required")
	}
	if cfgInt(cfg, "test_id") <= 0 {
		return fmt.Errorf("test_id is required (a positive DefectDojo test id)")
	}
	return nil
}

type ddTarget struct {
	base    string
	testID  int
	foundBy []int
	headers map[string]string
}

func ddOf(cfg map[string]any) (ddTarget, error) {
	if err := (defectDojoTracker{}).Validate(cfg); err != nil {
		return ddTarget{}, notify.Permanent(err)
	}
	base, _ := baseURL(cfg, "")
	return ddTarget{
		base: base, testID: cfgInt(cfg, "test_id"), foundBy: cfgInts(cfg, "found_by"),
		headers: map[string]string{"Authorization": "Token " + cfgString(cfg, "api_key"), "Accept": "application/json"},
	}, nil
}

type ddFinding struct {
	ID           int  `json:"id"`
	Active       bool `json:"active"`
	IsMitigated  bool `json:"is_mitigated"`
	FalseP       bool `json:"false_p"`
	RiskAccepted bool `json:"risk_accepted"`
	Duplicate    bool `json:"duplicate"`
}

func (d ddTarget) ref(id int) Ref {
	return Ref{ID: strconv.Itoa(id), URL: d.base + "/finding/" + strconv.Itoa(id)}
}

func (d ddTarget) findingURL(id string) string {
	return d.base + "/api/v2/findings/" + url.PathEscape(id) + "/"
}

// ddSeverity maps ARTEX severity to DefectDojo's title-case name and S0..S4 code.
func ddSeverity(s string) (name, numerical string) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case SevCritical:
		return "Critical", "S0"
	case SevHigh:
		return "High", "S1"
	case SevMedium:
		return "Medium", "S2"
	case SevLow:
		return "Low", "S3"
	default:
		return "Info", "S4"
	}
}

// ddFlags maps the desired open/closed state and resolution to DefectDojo flags.
func ddFlags(t Ticket) map[string]any {
	f := map[string]any{"active": true, "is_mitigated": false, "false_p": false, "risk_accepted": false, "duplicate": false}
	if t.Open {
		return f
	}
	f["active"] = false
	switch closedResolution(t) {
	case ResFixed:
		f["is_mitigated"] = true
	case ResFalsePositive:
		f["false_p"] = true
	case ResRiskAccepted:
		f["risk_accepted"] = true
	case ResDuplicate:
		f["duplicate"] = true
	}
	return f // wont_fix: only active=false
}

func ddFields(t Ticket) map[string]any {
	sev, num := ddSeverity(t.Severity)
	f := ddFlags(t)
	f["title"] = t.Title
	f["description"] = t.Body
	f["severity"] = sev
	f["numerical_severity"] = num
	return f
}

func (defectDojoTracker) Find(ctx context.Context, cfg map[string]any, key string) (*Ref, error) {
	d, err := ddOf(cfg)
	if err != nil {
		return nil, err
	}
	q := url.Values{"test": {strconv.Itoa(d.testID)}, "unique_id_from_tool": {key}, "limit": {"1"}}
	res, err := callJSON[struct {
		Results []ddFinding `json:"results"`
	}](ctx, notify.Request{Method: http.MethodGet, URL: d.base + "/api/v2/findings/?" + q.Encode(), Headers: d.headers})
	if err != nil {
		return nil, fmt.Errorf("defectdojo: search findings: %w", err)
	}
	if len(res.Results) == 0 {
		return nil, nil
	}
	r := d.ref(res.Results[0].ID)
	return &r, nil
}

func (defectDojoTracker) Create(ctx context.Context, cfg map[string]any, t Ticket) (Ref, error) {
	d, err := ddOf(cfg)
	if err != nil {
		return Ref{}, err
	}
	body := ddFields(t)
	body["test"] = d.testID
	body["verified"] = t.Verified
	body["unique_id_from_tool"] = t.Key
	if len(d.foundBy) > 0 {
		body["found_by"] = d.foundBy
	}
	f, err := callJSON[ddFinding](ctx, notify.Request{
		Method: http.MethodPost, URL: d.base + "/api/v2/findings/", Headers: d.headers, Body: body,
	})
	if err != nil {
		return Ref{}, fmt.Errorf("defectdojo: create finding: %w", err)
	}
	if f.ID == 0 {
		return Ref{}, fmt.Errorf("defectdojo: create finding: response carried no id")
	}
	return d.ref(f.ID), nil
}

func (defectDojoTracker) Update(ctx context.Context, cfg map[string]any, ref Ref, t Ticket) error {
	d, err := ddOf(cfg)
	if err != nil {
		return err
	}
	body := ddFields(t)
	if t.Verified {
		body["verified"] = true
	}
	if _, err := call(ctx, notify.Request{
		Method: http.MethodPatch, URL: d.findingURL(ref.ID), Headers: d.headers, Body: body,
	}); err != nil {
		return fmt.Errorf("defectdojo: update finding: %w", err)
	}
	return nil
}

func (defectDojoTracker) State(ctx context.Context, cfg map[string]any, ref Ref) (State, error) {
	d, err := ddOf(cfg)
	if err != nil {
		return State{}, err
	}
	f, err := callJSON[ddFinding](ctx, notify.Request{Method: http.MethodGet, URL: d.findingURL(ref.ID), Headers: d.headers})
	if err != nil {
		return State{}, fmt.Errorf("defectdojo: read finding: %w", err)
	}
	switch {
	case f.FalseP:
		return State{Resolution: ResFalsePositive}, nil
	case f.RiskAccepted:
		return State{Resolution: ResRiskAccepted}, nil
	case f.Duplicate:
		return State{Resolution: ResDuplicate}, nil
	case f.IsMitigated:
		return State{Resolution: ResFixed}, nil
	case !f.Active:
		return State{Resolution: ResWontFix}, nil
	}
	return State{Open: true}, nil
}
