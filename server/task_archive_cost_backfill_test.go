package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/Autumn-27/artex/db"
)

// An archive made before cost_cells (simulated by removing them from a real
// archive) is reported as archived_omitted; the startup backfill reads the cold
// llm_usage rows from its package, after which GET /api/llm/cost omits nothing
// in any dimension or window.
func TestArchiveCostBackfillFromPackage(t *testing.T) {
	m, err := NewManager(t.TempDir(), "")
	if err != nil {
		db.SkipPGUnavailable(t, err)
	}
	defer m.Close()
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	dataDir := t.TempDir()
	s := New(cancelled, m, dataDir, dataDir, dataDir)
	s.archiveWG.Wait()
	s.ctx = context.Background()

	task, err := m.CreateTask("cost backfill package", "fixture", nil, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	taskID, _ := strconv.ParseInt(task.ID, 10, 64)
	defer func() {
		_, _ = m.pg.Exec(`DELETE FROM task_archives WHERE task_id=$1`, taskID)
		_ = m.pg.DeleteTask(taskID)
	}()
	for _, r := range []db.LLMUsage{
		{Worker: "planner", Model: "m-backfill", ProfileName: "p-backfill", InputTokens: 1000, OutputTokens: 100, LatencyMs: 40, Status: "ok"},
		{Worker: "worker", Model: "m-backfill", ProfileName: "p-backfill", InputTokens: 2000, OutputTokens: 300, LatencyMs: 60, Status: "error"},
	} {
		r.TaskID, r.ExplorationID = task.ID, task.ExpID
		if err := m.pg.InsertLLMUsage(&r); err != nil {
			t.Fatal(err)
		}
	}
	if err := m.SetTaskPaused(task.ID, true); err != nil {
		t.Fatal(err)
	}
	job, err := m.pg.QueueTaskArchive(taskID)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.runOneTaskArchiveJob(); err != nil {
		t.Fatal(err)
	}
	var want string
	if err := m.pg.QueryRow(`SELECT (aggregate_stats->'cost_cells')::text FROM task_archives WHERE id=$1 AND state='ready'`, job.ID).Scan(&want); err != nil {
		t.Fatal(err)
	}
	if _, err := m.pg.Exec(`UPDATE task_archives SET aggregate_stats=aggregate_stats - 'cost_cells' WHERE id=$1`, job.ID); err != nil {
		t.Fatal(err)
	}

	token, err := signJWT(s.jwtKey)
	if err != nil {
		t.Fatal(err)
	}
	omitted := func(by string, days int) int {
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/llm/cost?by=%s&days=%d", by, days), nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		var body struct {
			Omitted *int `json:"archived_omitted"`
		}
		if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &body) != nil || body.Omitted == nil {
			t.Fatalf("GET cost by=%s days=%d: %d %s", by, days, rec.Code, rec.Body.String())
		}
		return *body.Omitted
	}
	if omitted("worker", 30) < 1 {
		t.Fatal("legacy archive not reported as omitted")
	}

	done, err := s.backfillArchiveCostCells()
	if err != nil || done < 1 {
		t.Fatalf("backfill done=%d err=%v", done, err)
	}
	var got string
	if err := m.pg.QueryRow(`SELECT (aggregate_stats->'cost_cells')::text FROM task_archives WHERE id=$1`, job.ID).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("backfilled cells differ from archive-time cells:\n got %s\nwant %s", got, want)
	}
	left, err := m.pg.ArchivesMissingCostCells()
	if err != nil {
		t.Fatal(err)
	}
	for _, by := range []string{"task", "profile", "worker", "model"} {
		for _, days := range []int{0, 1, 7, 30, 90} {
			if n := omitted(by, days); n != len(left) {
				t.Fatalf("by=%s days=%d omitted=%d, want %d (unreadable legacy archives left)", by, days, n, len(left))
			}
		}
	}
}

func TestReadTaskArchiveUsageRowsChecksManifest(t *testing.T) {
	dir := t.TempDir()
	payload := t.TempDir()
	path := dir + "/task.tar.zst"
	snapshot := &db.TaskArchiveSnapshot{FormatVersion: db.TaskArchiveFormatVersion, TaskID: 7,
		Tables: map[string]json.RawMessage{"llm_usage": json.RawMessage(`[{"id":1}]`), "llm_records": json.RawMessage(`[]`)}}
	_, _, sum, err := writeTaskArchivePackage(path, payload, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := readTaskArchiveUsageRows(path, sum, 7)
	if err != nil || string(rows) != `[{"id":1}]` {
		t.Fatalf("rows=%s err=%v", rows, err)
	}
	if _, err := readTaskArchiveUsageRows(path, sum, 8); err != db.ErrTaskArchiveFormatMismatch {
		t.Fatalf("wrong task: %v", err)
	}
	if _, err := readTaskArchiveUsageRows(path, "00", 7); err == nil {
		t.Fatal("bad checksum accepted")
	}
}
