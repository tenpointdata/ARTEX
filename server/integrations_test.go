package server

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/Autumn-27/artex/db"
	"github.com/Autumn-27/artex/notify"
	"github.com/Autumn-27/artex/scanimport"
	"github.com/Autumn-27/artex/ticketing"
)

// fakeGitHub is a stateful stand-in for the GitHub issues API: search, create,
// get and patch. Humans "close" an issue by calling close().
type fakeGitHub struct {
	mu      sync.Mutex
	issues  map[int]map[string]any
	creates int
	auth    []string
}

func newFakeGitHub(t *testing.T) (*fakeGitHub, *httptest.Server) {
	g := &fakeGitHub{issues: map[int]map[string]any{}}
	srv := httptest.NewServer(http.HandlerFunc(g.serve))
	t.Cleanup(srv.Close)
	return g, srv
}

func (g *fakeGitHub) serve(w http.ResponseWriter, r *http.Request) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.auth = append(g.auth, r.Header.Get("Authorization"))
	body := map[string]any{}
	if raw, _ := io.ReadAll(r.Body); len(raw) > 0 {
		_ = json.Unmarshal(raw, &body)
	}
	w.Header().Set("Content-Type", "application/json")
	const prefix = "/repos/acme/app/issues"
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/search/issues":
		items := []map[string]any{}
		for _, is := range g.issues {
			if b, _ := is["body"].(string); strings.Contains(b, "artex:key=") && strings.Contains(r.URL.Query().Get("q"), keyIn(b)) {
				items = append(items, is)
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"total_count": len(items), "items": items})
	case r.Method == http.MethodPost && r.URL.Path == prefix:
		g.creates++
		n := len(g.issues) + 1
		is := map[string]any{"number": n, "html_url": fmt.Sprintf("https://github.example/acme/app/issues/%d", n),
			"title": body["title"], "body": body["body"], "state": "open", "state_reason": nil, "labels": labelObjs(body["labels"])}
		g.issues[n] = is
		w.WriteHeader(201)
		_ = json.NewEncoder(w).Encode(is)
	case strings.HasPrefix(r.URL.Path, prefix+"/"):
		n, _ := strconv.Atoi(strings.TrimPrefix(r.URL.Path, prefix+"/"))
		is, ok := g.issues[n]
		if !ok {
			w.WriteHeader(404)
			return
		}
		if r.Method == http.MethodPatch {
			for k, v := range body {
				if k == "labels" {
					v = labelObjs(v)
				}
				is[k] = v
			}
		}
		_ = json.NewEncoder(w).Encode(is)
	default:
		w.WriteHeader(404)
	}
}

func keyIn(body string) string {
	rest := body[strings.Index(body, "artex:key="):]
	return strings.Fields(rest)[0]
}

func labelObjs(v any) []map[string]any {
	out := []map[string]any{}
	list, _ := v.([]any)
	for _, l := range list {
		out = append(out, map[string]any{"name": l})
	}
	return out
}

// close simulates a human closing the issue in GitHub.
func (g *fakeGitHub) close(n int, reason string, labels ...string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.issues[n]["state"], g.issues[n]["state_reason"] = "closed", reason
	ls := g.issues[n]["labels"].([]map[string]any)
	for _, l := range labels {
		ls = append(ls, map[string]any{"name": l})
	}
	g.issues[n]["labels"] = ls
}

func (g *fakeGitHub) state(n int) (string, string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	r, _ := g.issues[n]["state_reason"].(string)
	return g.issues[n]["state"].(string), r
}

// TestFindingRoundTripsStatusWithGitHubIssue is the E2 done criterion: a finding
// pushed to a GitHub issue carries its status out, and a status change made in
// GitHub comes back to the finding.
func TestFindingRoundTripsStatusWithGitHubIssue(t *testing.T) {
	t.Setenv(notify.AllowLocalTargetsEnv, "1")
	s, rec, request := trafficEvidenceServer(t)
	pg := s.m.pg
	gh, srv := newFakeGitHub(t)

	cfg := fmt.Sprintf(`{"kind":"github","name":"gh","config":{"base_url":%q,"repo":"acme/app","token":"ghp_roundtrip_secret_value"}}`, srv.URL)
	w := request("POST", "/api/integrations/trackers", cfg)
	if w.Code != 201 {
		t.Fatalf("create tracker: %d %s", w.Code, w.Body)
	}
	var tr ticketTrackerDTO
	_ = json.Unmarshal(w.Body.Bytes(), &tr)
	t.Cleanup(func() { _ = pg.DeleteTicketTracker(context.Background(), tr.ID) })
	if strings.Contains(w.Body.String(), "ghp_roundtrip_secret_value") {
		t.Fatalf("token echoed back: %s", w.Body)
	}

	fid := rec.FindingID
	push := func() {
		t.Helper()
		w := request("POST", fmt.Sprintf("/api/integrations/trackers/%d/push", tr.ID), fmt.Sprintf(`{"finding_ids":[%d]}`, fid))
		if w.Code != 200 || strings.Contains(w.Body.String(), `"error"`) {
			t.Fatalf("push: %d %s", w.Code, w.Body)
		}
	}
	sync := func() []ticketSyncResult {
		t.Helper()
		w := request("POST", fmt.Sprintf("/api/integrations/trackers/%d/sync", tr.ID), ``)
		var out struct {
			Results []ticketSyncResult `json:"results"`
		}
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &out) != nil {
			t.Fatalf("sync: %d %s", w.Code, w.Body)
		}
		return out.Results
	}
	status := func() string {
		f, err := pg.GetFinding(fid)
		if err != nil {
			t.Fatal(err)
		}
		return f.Status
	}

	push()
	push() // idempotent: the second push updates the same issue
	if gh.creates != 1 {
		t.Fatalf("want one issue created, got %d", gh.creates)
	}
	if st, _ := gh.state(1); st != "open" {
		t.Fatalf("issue should be open, is %s", st)
	}
	for _, a := range gh.auth {
		if a != "Bearer ghp_roundtrip_secret_value" {
			t.Fatalf("auth header %q", a)
		}
	}

	// Out: the operator marks the finding fixed in ARTEX; sync closes the issue.
	if _, _, _, err := pg.SetFindingStatusWithNotify(context.Background(), fid, db.FindingFixed); err != nil {
		t.Fatal(err)
	}
	if r := sync(); len(r) != 1 || r[0].Action != "pushed" {
		t.Fatalf("sync out: %+v", r)
	}
	if st, reason := gh.state(1); st != "closed" || reason != "completed" {
		t.Fatalf("issue: %s/%s", st, reason)
	}
	if r := sync(); r[0].Action != "unchanged" {
		t.Fatalf("second sync should be a no-op: %+v", r)
	}

	// In: someone reopens it in GitHub; sync reopens the finding as confirmed.
	gh.mu.Lock()
	gh.issues[1]["state"], gh.issues[1]["state_reason"] = "open", "reopened"
	gh.mu.Unlock()
	if r := sync(); r[0].Action != "pulled" || status() != db.FindingConfirmed {
		t.Fatalf("reopen pull: %+v status=%s", r, status())
	}

	// In: closed as a false positive in GitHub; the finding follows.
	gh.close(1, "not_planned", "artex:false-positive")
	if r := sync(); r[0].Action != "pulled" || status() != db.FindingFalsePositive {
		t.Fatalf("false-positive pull: %+v status=%s", r, status())
	}

	w = request("GET", fmt.Sprintf("/api/exploration/findings/%d/integrations", fid), "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"external_id":"1"`) {
		t.Fatalf("finding integrations: %d %s", w.Code, w.Body)
	}

	// A push right after a change in GitHub pulls it instead of overwriting it.
	if _, _, _, err := pg.SetFindingStatusWithNotify(context.Background(), fid, db.FindingConfirmed); err != nil {
		t.Fatal(err)
	}
	if r := sync(); r[0].Action != "pushed" {
		t.Fatalf("reopen push: %+v", r)
	}
	gh.close(1, "not_planned", "artex:risk-accepted")
	push()
	if st, _ := gh.state(1); st != "closed" || status() != db.FindingRiskAccepted {
		t.Fatalf("push overwrote a tracker change: issue=%s finding=%s", st, status())
	}

	// A destination change drops the links: the old issue numbers mean nothing in the new repo.
	w = request("PATCH", fmt.Sprintf("/api/integrations/trackers/%d", tr.ID),
		`{"config":{"repo":"acme/other","token":"ghp_roundtrip_secret_value"}}`)
	if w.Code != 200 {
		t.Fatalf("repo change: %d %s", w.Code, w.Body)
	}
	if links, _ := pg.FindingTicketsByTracker(context.Background(), tr.ID); len(links) != 0 {
		t.Fatalf("links kept across a destination change: %d", len(links))
	}
}

func TestTrackerDestinationChangeNeedsCredentials(t *testing.T) {
	s, _, request := trafficEvidenceServer(t)
	w := request("POST", "/api/integrations/trackers", `{"kind":"github","config":{"repo":"acme/app","token":"ghp_dest_secret"}}`)
	if w.Code != 201 {
		t.Fatalf("create: %d %s", w.Code, w.Body)
	}
	var tr ticketTrackerDTO
	_ = json.Unmarshal(w.Body.Bytes(), &tr)
	t.Cleanup(func() { _ = s.m.pg.DeleteTicketTracker(context.Background(), tr.ID) })
	w = request("PATCH", fmt.Sprintf("/api/integrations/trackers/%d", tr.ID), `{"config":{"base_url":"https://evil.example"}}`)
	if w.Code != 400 {
		t.Fatalf("destination change without the token must be refused: %d %s", w.Code, w.Body)
	}
}

func TestStatusTicketMappingRoundTrips(t *testing.T) {
	for _, st := range []string{db.FindingFixed, db.FindingFalsePositive, db.FindingRiskAccepted, db.FindingDuplicate, db.FindingIgnored, db.FindingConfirmed, db.FindingPending} {
		if got := ticketToStatus(statusToTicket(st), db.FindingConfirmed); st != db.FindingPending && got != st {
			t.Errorf("%s → %+v → %s", st, statusToTicket(st), got)
		}
	}
	if got := ticketToStatus(ticketing.State{Open: false, Resolution: ticketing.ResFixed}, db.FindingResolved); got != db.FindingResolved {
		t.Errorf("resolved should stay resolved, got %s", got)
	}
	ev := redactEvidence("GET /?access_token=qqq1&a=b HTTP/1.1\nCookie: sid=abc\nAuthorization: Bearer xyz\nHost: x\n" +
		`curl -H 'Authorization: Bearer eyJcurl' -d '{"password":"pw1","user":"u"}'`)
	for _, leak := range []string{"abc", "xyz", "qqq1", "eyJcurl", "pw1"} {
		if strings.Contains(ev, leak) {
			t.Errorf("redaction kept %q: %q", leak, ev)
		}
	}
	if !strings.Contains(ev, "Host: x") || !strings.Contains(ev, `"user":"u"`) || !strings.Contains(ev, "a=b") {
		t.Errorf("redaction removed too much: %q", ev)
	}
	impl, _ := ticketing.Get(ticketing.KindJira)
	sy := &ticketSyncer{impl: impl, cfg: map[string]any{"email": "a@b.c", "token": "jiratok123"}}
	basic := base64.StdEncoding.EncodeToString([]byte("a@b.c:jiratok123"))
	if msg := sy.scrub(fmt.Errorf("HTTP 400: echo Authorization: Basic %s token jiratok123", basic)); strings.Contains(msg, "jiratok123") || strings.Contains(msg, basic) {
		t.Errorf("scrub kept the credential: %q", msg)
	}
}

// TestScanImportCreatesAssetsAndUnverifiedFindings is the E2 ingest criterion
// for Nuclei and Nmap: assets and findings with provenance, never verified,
// and a re-import adds nothing.
func TestScanImportCreatesAssetsAndUnverifiedFindings(t *testing.T) {
	s, _, request := trafficEvidenceServer(t)
	pg := s.m.pg
	ctx := context.Background()
	for _, tc := range []struct {
		format   scanimport.Format
		file     string
		findings bool
	}{
		{scanimport.NmapXML, "nmap.xml", false},
		{scanimport.NucleiJSONL, "nuclei.jsonl", true},
		{scanimport.ZAPJSON, "zap.json", true},
		{scanimport.BurpXML, "burp.xml", true},
		{scanimport.SARIF, "semgrep.sarif", true},
	} {
		t.Run(string(tc.format), func(t *testing.T) {
			data, err := os.ReadFile("../scanimport/testdata/" + tc.file)
			if err != nil {
				t.Skip("fixture missing: " + tc.file)
			}
			out, err := runScanImport(ctx, pg, scanImportRequest{Format: tc.format, FileName: "/tmp/" + tc.file, Data: data})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				for _, id := range out.NewFindings {
					_, _ = pg.DeleteFinding(id)
				}
				_, _ = pg.Exec(`DELETE FROM scan_imports WHERE id=$1`, out.Import.ID)
			})
			if out.Import.FileName != tc.file {
				t.Errorf("file name %q", out.Import.FileName)
			}
			if tc.format != scanimport.SARIF && out.Import.Assets == 0 {
				t.Fatalf("no assets: %+v %v", out.Import, out.AssetErrors)
			}
			var linked int
			_ = pg.QueryRow(`SELECT count(*) FROM scan_import_assets WHERE import_id=$1`, out.Import.ID).Scan(&linked)
			if tc.format != scanimport.SARIF && linked == 0 {
				t.Fatal("asset provenance not recorded")
			}
			if tc.findings && len(out.NewFindings) == 0 {
				t.Fatalf("no findings: %+v %v", out.Import, out.FindingError)
			}
			for _, id := range out.NewFindings {
				f, err := pg.GetFinding(id)
				if err != nil {
					t.Fatal(err)
				}
				if f.VerificationStatus != db.VerificationUnverified || f.PoELevel != 0 || !strings.HasPrefix(f.Worker, "import:") {
					t.Fatalf("imported finding must be unverified: %+v", f)
				}
				if strings.Contains(f.Summary, "Ignore previous") || strings.Contains(f.Summary, "pass@") || strings.Contains(f.Evidence, "pass@") {
					t.Fatalf("untrusted text leaked into summary or credentials kept: %q", f.Summary)
				}
				src, err := pg.FindingImportSourceByFinding(ctx, id)
				if err != nil || src == nil || src.ImportID == nil || *src.ImportID != out.Import.ID || src.Record == "" {
					t.Fatalf("provenance: %+v %v", src, err)
				}
			}
			again, err := runScanImport(ctx, pg, scanImportRequest{Format: tc.format, Data: data})
			if err != nil {
				t.Fatal(err)
			}
			_, _ = pg.Exec(`DELETE FROM scan_imports WHERE id=$1`, again.Import.ID)
			if len(again.NewFindings) != 0 || again.Import.FindingsExisting != out.Import.FindingsNew {
				t.Fatalf("re-import must add nothing: %+v", again.Import)
			}
			// The findings list filtered by import shows exactly that import's new findings;
			// the re-import, which only matched them by dedup key, shows none.
			for importID, want := range map[int64]int{out.Import.ID: len(out.NewFindings), again.Import.ID: 0} {
				w := request("GET", fmt.Sprintf("/api/exploration/findings?page=1&limit=200&import_id=%d", importID), "")
				var page struct {
					Items []FindingDTO `json:"items"`
					Total int          `json:"total"`
				}
				if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &page) != nil || page.Total != want || len(page.Items) != want {
					t.Fatalf("import_id=%d: code %d total %d items %d, want %d", importID, w.Code, page.Total, len(page.Items), want)
				}
			}
		})
	}
	if w := request("POST", "/api/integrations/imports?format=bogus", "x"); w.Code != 400 {
		t.Fatalf("bogus format: %d", w.Code)
	}
	// The imports list pages: limit/offset select a window, total counts them all.
	var first, second struct {
		Imports []db.ScanImport `json:"imports"`
		Total   int             `json:"total"`
	}
	for i := 0; i < 3; i++ {
		im := &db.ScanImport{Format: "nmap-xml", ToolName: "nmap", FileSHA256: "page"}
		if err := pg.CreateScanImport(ctx, im); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _, _ = pg.Exec(`DELETE FROM scan_imports WHERE id=$1`, im.ID) })
	}
	w := request("GET", "/api/integrations/imports?limit=2", "")
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &first) != nil || len(first.Imports) != 2 || first.Total < 3 {
		t.Fatalf("imports page 1: %d %s", w.Code, w.Body.String())
	}
	w = request("GET", "/api/integrations/imports?limit=2&offset=1", "")
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &second) != nil || len(second.Imports) != 2 ||
		second.Total != first.Total || second.Imports[0].ID != first.Imports[1].ID {
		t.Fatalf("imports page with offset: %d %s", w.Code, w.Body.String())
	}
}
