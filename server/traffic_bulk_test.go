package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/Autumn-27/artex/db"
	"github.com/Autumn-27/artex/traffic"
)

// TestTrafficBulkExportAndDeleteByID: selecting two of three exchanges on the
// same host exports exactly those two as HAR and deletes exactly those two,
// leaving the sibling; both actions land in the audit chain.
func TestTrafficBulkExportAndDeleteByID(t *testing.T) {
	s, request := ingestServer(t)
	entries := make([]traffic.HAREntry, 3)
	for i := range entries {
		entries[i] = traffic.HAREntry{
			StartedDateTime: fmt.Sprintf("2026-06-01T08:00:0%dZ", i),
			Request:         traffic.HARRequest{Method: "GET", URL: fmt.Sprintf("https://bulk.example/p%d", i), Headers: []traffic.HARHeader{}},
			Response:        traffic.HARResponse{Status: 200, Headers: []traffic.HARHeader{}, Content: traffic.HARContent{MimeType: "text/plain", Text: "ok"}},
		}
	}
	raw, err := json.Marshal(traffic.HAR{Log: &traffic.HARLog{Version: "1.2", Creator: traffic.HARCreator{Name: "t", Version: "1"}, Entries: entries}})
	if err != nil {
		t.Fatal(err)
	}
	if w := request(http.MethodPost, "/api/traffic/import/har", string(raw)); w.Code != http.StatusOK {
		t.Fatalf("import status=%d body=%s", w.Code, w.Body.String())
	}
	list := func() []TrafficExchangeDTO {
		w := request(http.MethodGet, "/api/traffic?host=bulk.example&sort=ts&order=asc", "")
		var resp struct {
			Exchanges []TrafficExchangeDTO `json:"exchanges"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatal(err)
		}
		return resp.Exchanges
	}
	rows := list()
	if len(rows) != 3 {
		t.Fatalf("listed %d exchanges, want 3", len(rows))
	}
	pick, _ := json.Marshal(map[string][]string{"ids": {rows[0].ID, rows[2].ID, rows[0].ID}})

	w := request(http.MethodPost, "/api/traffic/bulk-export", string(pick))
	if w.Code != http.StatusOK || w.Header().Get("X-ARTEX-HAR-Exported") != "2" {
		t.Fatalf("bulk-export status=%d headers=%v body=%s", w.Code, w.Header(), w.Body.String())
	}
	var har traffic.HAR
	if err := json.Unmarshal(w.Body.Bytes(), &har); err != nil {
		t.Fatal(err)
	}
	if len(har.Log.Entries) != 2 || har.Log.Entries[0].Request.URL != rows[0].URL || har.Log.Entries[1].Request.URL != rows[2].URL {
		t.Fatalf("exported entries = %+v", har.Log.Entries)
	}

	w = request(http.MethodPost, "/api/traffic/bulk-delete", string(pick))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"deleted":2`) {
		t.Fatalf("bulk-delete status=%d body=%s", w.Code, w.Body.String())
	}
	if left := list(); len(left) != 1 || left[0].ID != rows[1].ID {
		t.Fatalf("remaining = %+v, want only %s", left, rows[1].ID)
	}

	for _, body := range []string{`{"ids":[]}`, `{"ids":["  "]}`, `not json`} {
		if w := request(http.MethodPost, "/api/traffic/bulk-delete", body); w.Code != http.StatusBadRequest {
			t.Fatalf("bulk-delete %q status=%d, want 400", body, w.Code)
		}
	}

	for _, kind := range []string{db.AuditTrafficBulkExport, db.AuditTrafficBulkDelete} {
		got, err := s.m.pg.ListAudit(db.AuditFilter{Kind: kind})
		if err != nil {
			t.Fatal(err)
		}
		if len(got) == 0 {
			t.Fatalf("no %s audit entry", kind)
		}
	}
}
