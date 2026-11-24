package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Autumn-27/artex/db"
	"github.com/Autumn-27/artex/intercept"
)

func TestGovernanceEndpointsPG(t *testing.T) {
	dsn, _, err := db.DSN()
	if err != nil {
		db.SkipPGUnavailable(t, err)
	}
	d, err := db.Open(dsn)
	if err != nil {
		db.SkipPGUnavailable(t, err)
	}
	t.Cleanup(func() { _ = d.Close() })
	task, err := d.CreateTaskWithOptions("governance", "authorization artifact", db.TaskCreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.DeleteTask(task.ID) })
	s := &Server{m: &Manager{pg: d, interceptor: intercept.New(d)}, jwtKey: []byte("governance-http-test-signing-key")}
	h := s.Handler()
	token, err := signJWT(s.jwtKey)
	if err != nil {
		t.Fatal(err)
	}
	do := func(method, path, body string) (int, map[string]any) {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		var out map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return rec.Code, out
	}
	base := fmt.Sprintf("/api/tasks/%d/authorization", task.ID)
	now := time.Now().UTC()
	body := fmt.Sprintf(`{"reference":"SOW-9","authorized_by":"Client CISO","scope":[{"kind":"domain","value":"Example.com"}],
		"techniques":["scanning"],"not_before":%q,"not_after":%q}`, now.Add(-time.Hour).Format(time.RFC3339), now.Add(time.Hour).Format(time.RFC3339))

	if code, out := do("POST", base, `{"not_before":"2026-01-02T00:00:00Z","not_after":"2026-01-01T00:00:00Z"}`); code != 400 {
		t.Fatalf("reversed window: %d %v", code, out)
	}
	if code, out := do("POST", base, strings.Replace(body, "scanning", "teleport", 1)); code != 400 {
		t.Fatalf("unknown technique: %d %v", code, out)
	}
	code, out := do("POST", base, body)
	if code != 200 || out["version"].(float64) != 1 || out["created_by"] != "ARTEX" {
		t.Fatalf("create: %d %v", code, out)
	}
	code, out = do("GET", base, "")
	if code != 200 || out["in_force"] != true || len(out["technique_classes"].([]any)) == 0 {
		t.Fatalf("get: %d %v", code, out)
	}
	if code, out = do("POST", base+"/revoke", `{"note":"engagement ended"}`); code != 200 || out["revoked"] != true || out["version"].(float64) != 2 {
		t.Fatalf("revoke: %d %v", code, out)
	}
	if code, _ = do("POST", base+"/revoke", ""); code != 409 {
		t.Fatalf("second revoke: %d", code)
	}
	if code, out = do("GET", base, ""); code != 200 || out["in_force"] != false {
		t.Fatalf("after revoke: %d %v", code, out)
	}
	if code, _ = do("POST", "/api/tasks/999999999/authorization", body); code != 404 {
		t.Fatalf("missing task: %d", code)
	}

	code, out = do("GET", "/api/intercept/history/chain?kind=row.&limit=5", "")
	if code != 200 || len(out["entries"].([]any)) == 0 {
		t.Fatalf("chain list: %d %v", code, out)
	}
	if code, _ = do("GET", "/api/intercept/history/chain?after_seq=-1", ""); code != 400 {
		t.Fatalf("bad after_seq: %d", code)
	}
	code, out = do("GET", "/api/intercept/history/chain/verify", "")
	if code != 200 || out["ok"] != true {
		t.Fatalf("verify: %d %v", code, out)
	}

	// spawn_task inheritance: the caller is read from the call's scope.
	ctx := intercept.WithTargetScope(context.Background(), intercept.TargetScope{TaskID: strconv.FormatInt(task.ID, 10)})
	if _, err := s.callerAuthorization(ctx); err == nil {
		t.Fatal("a revoked artifact allowed a spawn")
	}
	if code, _ = do("POST", base, body); code != 200 {
		t.Fatalf("re-authorize: %d", code)
	}
	a, err := s.callerAuthorization(ctx)
	if err != nil || a == nil || a.Version != 3 {
		t.Fatalf("caller artifact = %+v, %v", a, err)
	}
	if a, err := s.callerAuthorization(context.Background()); a != nil || err != nil {
		t.Fatalf("no scope: %+v %v", a, err)
	}
	child, err := d.CreateTaskWithOptions("governance child", "inherits", db.TaskCreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.DeleteTask(child.ID) })
	if err := s.inheritAuthorization(strconv.FormatInt(child.ID, 10), a); err != nil {
		t.Fatal(err)
	}
	got, err := d.CurrentAuthorization(child.ID)
	if err != nil || got == nil || got.Reference != "SOW-9" || len(got.Scope) != 1 || got.CreatedBy != fmt.Sprintf("spawn_task:task-%d", task.ID) {
		t.Fatalf("child artifact = %+v, %v", got, err)
	}
}
