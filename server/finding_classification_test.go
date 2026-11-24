package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The finding API accepts and returns the typed classification (E1), rejects a
// malformed one without writing anything, and the exports carry it; the
// branded HTML export uses the stored branding with per-export overrides.
func TestFindingClassificationHTTP(t *testing.T) {
	skipWithoutDSN(t)
	m, err := NewManager(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	tk, err := m.pg.CreateTask("e1 http", "goal", nil, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = m.pg.DeleteTask(tk.ID) }()
	fid, err := m.pg.AddFinding(tk.ID, 0, "sqli", "SQL injection", "high", "s", "e", "w", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = m.pg.DeleteFinding(fid) }()
	defer func() { _, _ = m.pg.Exec(`DELETE FROM settings WHERE key=$1`, settingReportBranding) }()

	ctx, cancel := context.WithCancel(context.Background())
	s := New(ctx, m, t.TempDir(), t.TempDir(), t.TempDir())
	defer stopTestServer(t, s, cancel)
	token, err := signJWT(s.jwtKey)
	if err != nil {
		t.Fatal(err)
	}
	do := func(method, path, body string) (int, string) {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		return rec.Code, rec.Body.String()
	}
	path := fmt.Sprintf("/api/exploration/findings/%d", fid)
	vec := "CVSS:4.0/AV:N/AC:L/AT:N/PR:N/UI:N/VC:H/VI:H/VA:H/SC:N/SI:N/SA:N"

	// A bad tag rejects the whole request: the status in the same body is not applied.
	if code, body := do(http.MethodPatch, path, `{"status":"confirmed","framework_tags":[{"framework":"nist","id":"AC-2"}]}`); code != 400 {
		t.Fatalf("bad tag: %d %s", code, body)
	}
	if f, _ := m.pg.GetFinding(fid); f.Status != "pending" {
		t.Fatalf("a rejected patch wrote status %q", f.Status)
	}

	code, body := do(http.MethodPatch, path, `{"cvss_vector":"`+vec+`","remediation":"Parameterize queries.",
		"epss":{"probability":0.3},"framework_tags":[{"framework":"cwe","id":"89"},{"framework":"owasp-wstg","id":"WSTG-INPV-05"}]}`)
	if code != 200 {
		t.Fatalf("patch: %d %s", code, body)
	}
	var dto struct {
		CVSS *struct {
			Vector string  `json:"vector"`
			Score  float64 `json:"score"`
			Rating string  `json:"rating"`
		} `json:"cvss"`
		EPSS *struct {
			Probability float64 `json:"probability"`
		} `json:"epss"`
		Remediation string `json:"remediation"`
		Tags        []struct {
			Framework string `json:"framework"`
			ID        string `json:"id"`
		} `json:"framework_tags"`
	}
	check := func(where, body string) {
		t.Helper()
		if err := json.Unmarshal([]byte(body), &dto); err != nil {
			t.Fatalf("%s: %v %s", where, err, body)
		}
		if dto.CVSS == nil || dto.CVSS.Score != 9.3 || dto.CVSS.Rating != "critical" || dto.CVSS.Vector != vec ||
			dto.EPSS == nil || dto.EPSS.Probability != 0.3 || dto.Remediation != "Parameterize queries." ||
			len(dto.Tags) != 2 || dto.Tags[0].ID != "CWE-89" {
			t.Fatalf("%s: classification %s", where, body)
		}
	}
	check("PATCH response", body)
	code, body = do(http.MethodGet, path, "")
	if code != 200 {
		t.Fatalf("get: %d %s", code, body)
	}
	check("GET", body)

	if code, body := do(http.MethodPatch, path, `{"cvss_vector":"CVSS:4.0/AV:N"}`); code != 400 {
		t.Fatalf("bad vector: %d %s", code, body)
	}

	if code, body := do(http.MethodPut, "/api/settings/report-branding", `{"organization":"Ten Point Data","accent_color":"#123abc","marking":"CONFIDENTIAL"}`); code != 200 {
		t.Fatalf("put branding: %d %s", code, body)
	}
	if code, body := do(http.MethodPut, "/api/settings/report-branding", `{"accent_color":"red"}`); code != 400 {
		t.Fatalf("bad branding: %d %s", code, body)
	}
	if code, body := do(http.MethodGet, "/api/settings/report-branding", ""); code != 200 || !strings.Contains(body, `"accent_color":"#123abc"`) {
		t.Fatalf("get branding: %d %s", code, body)
	}
	export := fmt.Sprintf("/api/exploration/findings/export?scope=selected&ids=%d&format=", fid)
	code, body = do(http.MethodGet, export+"html&client=Acme", "")
	if code != 200 {
		t.Fatalf("html export: %d %s", code, body)
	}
	for _, want := range []string{"--accent: #123abc", "Prepared for <strong>Acme</strong>", "by <strong>Ten Point Data</strong>", "CONFIDENTIAL",
		"Executive Summary", "9.3 Critical", "CWE-89", "WSTG-INPV-05", "Parameterize queries."} {
		if !strings.Contains(body, want) {
			t.Errorf("html export missing %q", want)
		}
	}
	if code, body = do(http.MethodGet, export+"md-single", ""); code != 200 || !strings.Contains(body, "- **CVSS v4.0**: 9.3 Critical") || !strings.Contains(body, "## Executive Summary") {
		t.Fatalf("md export: %d %s", code, body)
	}
	if code, body = do(http.MethodGet, export+"json", ""); code != 200 || !strings.Contains(body, `"framework_tags"`) {
		t.Fatalf("json export: %d %s", code, body)
	}
}
