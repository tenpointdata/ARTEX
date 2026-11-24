package server

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
)

// The create-task request carries the operator's methodology choice into the
// task (canonical ids, echoed in the DTO), refuses an unknown id, and the
// coverage endpoint then scores only the chosen checklist.
func TestCreateTaskMethodologies(t *testing.T) {
	g := newGateServer(t)

	req := httptest.NewRequest("GET", "/api/methodologies", nil)
	req.Header.Set("Authorization", "Bearer "+g.token)
	rec := httptest.NewRecorder()
	g.s.Handler().ServeHTTP(rec, req)
	var list []methodologySummaryDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil || len(list) < 2 {
		t.Fatalf("methodology list = %s, %v", rec.Body.String(), err)
	}

	code, body := g.do(t, "POST", "/api/tasks", `{"description":"test 10.0.0.5:8080","goal":"`+gateTestGoal+`","methodologies":["owasp-nope"]}`)
	if code != 400 || !strings.Contains(fmt.Sprint(body["error"]), "unknown methodology") {
		t.Fatalf("unknown methodology: %d %v", code, body)
	}

	code, body = g.do(t, "POST", "/api/tasks", `{"description":"test 10.0.0.5:8080","goal":"`+gateTestGoal+`","methodologies":["OWASP API Top10"]}`)
	id, _ := body["id"].(string)
	if code != 201 || id == "" {
		t.Fatalf("create: %d %v", code, body)
	}
	g.cleanupTask(t, id)
	if ms, _ := body["methodologies"].([]any); len(ms) != 1 || ms[0] != "owasp-api-top10" {
		t.Fatalf("DTO methodologies = %v", body["methodologies"])
	}
	code, cov := g.do(t, "GET", "/api/tasks/"+id+"/coverage", "")
	scores, _ := cov["methodologies"].([]any)
	if code != 200 || len(scores) != 1 {
		t.Fatalf("coverage: %d %v", code, cov["methodologies"])
	}
	if first, _ := scores[0].(map[string]any); first["id"] != "owasp-api-top10" {
		t.Fatalf("coverage scores %v, want owasp-api-top10 only", scores)
	}
}

// PATCH /api/tasks/{id} rewrites the methodology choice of an existing task:
// ["none"] turns the checklists off despite a matching goal, [] goes back to
// automatic, and an invalid choice is a 400 that leaves the task unchanged.
func TestPatchTaskMethodologies(t *testing.T) {
	g := newGateServer(t)
	code, body := g.do(t, "POST", "/api/tasks", `{"description":"test https://shop.example.test","goal":"`+gateTestGoal+`"}`)
	id, _ := body["id"].(string)
	if code != 201 || id == "" {
		t.Fatalf("create: %d %v", code, body)
	}
	g.cleanupTask(t, id)
	scores := func() []any {
		t.Helper()
		code, cov := g.do(t, "GET", "/api/tasks/"+id+"/coverage", "")
		if code != 200 {
			t.Fatalf("coverage: %d %v", code, cov)
		}
		s, _ := cov["methodologies"].([]any)
		return s
	}
	if len(scores()) != 1 {
		t.Fatal("automatic: the web goal should select WSTG")
	}

	code, body = g.do(t, "PATCH", "/api/tasks/"+id, `{"methodologies":["none"]}`)
	if ms, _ := body["methodologies"].([]any); code != 200 || len(ms) != 1 || ms[0] != "none" {
		t.Fatalf("patch none: %d %v", code, body)
	}
	if s := scores(); len(s) != 0 {
		t.Fatalf("none: coverage still scores %v", s)
	}
	code, body = g.do(t, "PATCH", "/api/tasks/"+id, `{"methodologies":["none","owasp-wstg"]}`)
	if code != 400 {
		t.Fatalf("none beside a methodology: %d %v", code, body)
	}
	code, body = g.do(t, "PATCH", "/api/tasks/"+id, `{"methodologies":[]}`)
	if ms, _ := body["methodologies"].([]any); code != 200 || len(ms) != 0 {
		t.Fatalf("patch automatic: %d %v", code, body)
	}
	if len(scores()) != 1 {
		t.Fatal("back to automatic: WSTG again")
	}
}
