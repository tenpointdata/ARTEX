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
)

// clientFixture is two clients (companies A and B), each with a task scoped to
// it by an operator, an asset and a finding on that asset, plus a finding of
// task B with no live asset that an agent tried to attribute to A.
type clientFixture struct {
	h                      *Server
	companyA, companyB     int64
	taskA, taskB           int64
	findA, findB, leak     int64
	nameA, nameB, leakName string
}

func newClientFixture(t *testing.T) clientFixture {
	t.Helper()
	skipWithoutDSN(t)
	m, err := NewManager(t.TempDir(), "")
	if err != nil {
		db.SkipPGUnavailable(t, err)
	}
	t.Cleanup(func() { m.Close() })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s := New(ctx, m, t.TempDir(), t.TempDir(), t.TempDir())
	s.archiveWG.Wait()
	<-s.side.done

	stamp := time.Now().UnixNano()
	fx := clientFixture{h: s}
	company := func(name string) int64 {
		id, _, err := m.pg.Companies().UpsertCompany(fmt.Sprintf("client scope %s %d", name, stamp), "")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _, _ = m.pg.Exec(`DELETE FROM companies WHERE id = $1`, id) })
		return id
	}
	fx.companyA, fx.companyB = company("a"), company("b")
	task := func(name string, c int64) int64 {
		tk, err := m.CreateTaskWithOptions(name, "goal", db.TaskCreateOptions{CompanyIDs: []int64{c}})
		if err != nil {
			t.Fatal(err)
		}
		id, _ := strconv.ParseInt(tk.ID, 10, 64)
		t.Cleanup(func() { _, _ = m.pg.DeleteFindingsByTask(id); _, _ = m.DeleteTask(tk.ID, DeleteTaskOptions{}) })
		return id
	}
	fx.taskA, fx.taskB = task("client scope task a", fx.companyA), task("client scope task b", fx.companyB)
	asset := func(c int64, label string) int64 {
		root := fmt.Sprintf("client-scope-%s-%d.example", label, stamp)
		var id int64
		if err := m.pg.QueryRow(`INSERT INTO assets(type, domain, root_domain, company_id)
			VALUES ('root_domain', $1, $1, $2) RETURNING id`, root, c).Scan(&id); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _, _ = m.pg.Exec(`DELETE FROM assets WHERE id = $1`, id) })
		return id
	}
	finding := func(task int64, name string, assets []int64) int64 {
		id, err := m.pg.AddFinding(task, 0, "xss", name, "high", "s", "e", "w", assets)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	fx.nameA, fx.nameB, fx.leakName = fmt.Sprintf("finding-a-%d", stamp), fmt.Sprintf("finding-b-%d", stamp), fmt.Sprintf("finding-leak-%d", stamp)
	fx.findA = finding(fx.taskA, fx.nameA, []int64{asset(fx.companyA, "a")})
	fx.findB = finding(fx.taskB, fx.nameB, []int64{asset(fx.companyB, "b")})
	fx.leak = finding(fx.taskB, fx.leakName, nil)
	// An agent-added company row must not attribute task B's work to client A.
	if _, err := m.pg.Exec(`INSERT INTO task_scope(task_id, kind, company_id, source) VALUES ($1, 'company', $2, 'agent')`,
		fx.taskB, fx.companyA); err != nil {
		t.Fatal(err)
	}
	return fx
}

// TestClientReadBoundToCompanyPG: a client_read user bound to company A gets
// only A's findings, report, outcome and coverage, and 404 for B's.
func TestClientReadBoundToCompanyPG(t *testing.T) {
	fx := newClientFixture(t)
	s := fx.h
	h := s.Handler()
	admin, err := signJWT(s.jwtKey)
	if err != nil {
		t.Fatal(err)
	}
	user := seedUser(t, s, db.RoleClientRead, "client-pass-123456")
	_, tok := loginAs(t, s, user.Username, "client-pass-123456")
	if tok == "" {
		t.Fatal("client login failed")
	}

	// Unbound: nothing.
	if w := call(h, tok, "GET", "/api/exploration/findings?page=1&limit=50", ""); w.Code != 403 {
		t.Fatalf("unbound client: %d %s", w.Code, w.Body)
	}
	// An admin binds the user to A; the change is chained.
	seq := latestAuditSeq(t, s)
	path := fmt.Sprintf("/api/users/%d/companies", user.ID)
	if w := call(h, admin, "PUT", path, fmt.Sprintf(`{"company_ids":[%d]}`, fx.companyA)); w.Code != 200 {
		t.Fatalf("bind: %d %s", w.Code, w.Body)
	}
	if es := auditSince(t, s, seq, "row.artex_user_companies", ""); len(es) != 1 || es[0].Actor != legacyUsername {
		t.Fatalf("binding audit entries = %+v", es)
	}
	if w := call(h, admin, "PUT", path, `{"company_ids":[999000111]}`); w.Code != 400 {
		t.Fatalf("unknown company bind: %d %s", w.Code, w.Body)
	}
	// The client cannot rebind itself.
	if w := call(h, tok, "PUT", path, fmt.Sprintf(`{"company_ids":[%d]}`, fx.companyB)); w.Code != 403 {
		t.Fatalf("client self-bind: %d", w.Code)
	}

	onlyA := func(what, url string) {
		t.Helper()
		w := call(h, tok, "GET", url, "")
		body := w.Body.String()
		if w.Code != 200 || !strings.Contains(body, fx.nameA) || strings.Contains(body, fx.nameB) || strings.Contains(body, fx.leakName) {
			t.Errorf("%s: %d, want only A's finding: %.400s", what, w.Code, body)
		}
	}
	onlyA("list page", "/api/exploration/findings?page=1&limit=200")
	onlyA("list array", "/api/exploration/findings")
	onlyA("list with ?company=A", fmt.Sprintf("/api/exploration/findings?page=1&limit=200&company=%d", fx.companyA))
	onlyA("list filtered to task B", fmt.Sprintf("/api/exploration/findings?page=1&limit=200&task_id=%d", fx.taskA))
	onlyA("export filtered", "/api/exploration/findings/export?format=json")
	onlyA("export all", "/api/exploration/findings/export?format=json&scope=all")
	onlyA("finding A", fmt.Sprintf("/api/exploration/findings/%d", fx.findA))

	want := func(code int, url string) {
		t.Helper()
		if w := call(h, tok, "GET", url, ""); w.Code != code {
			t.Errorf("GET %s: %d, want %d: %.300s", url, w.Code, code, w.Body)
		}
	}
	for _, url := range []string{
		fmt.Sprintf("/api/exploration/findings?page=1&limit=50&company=%d", fx.companyB),
		fmt.Sprintf("/api/exploration/findings/stats?company=%d", fx.companyB),
		fmt.Sprintf("/api/exploration/findings/%d", fx.findB),
		fmt.Sprintf("/api/exploration/findings/%d", fx.leak),
		fmt.Sprintf("/api/exploration/findings/%d/lineage", fx.findB),
		fmt.Sprintf("/api/exploration/findings/%d/retests", fx.findB),
		fmt.Sprintf("/api/exploration/findings/%d?context_task=%d", fx.findA, fx.taskB),
		fmt.Sprintf("/api/exploration/findings/export?format=json&scope=selected&ids=%d,%d", fx.findA, fx.findB),
		fmt.Sprintf("/api/exploration/findings?task=%d", fx.taskB),
		fmt.Sprintf("/api/report?task=%d", fx.taskB),
		fmt.Sprintf("/api/stats?task=%d", fx.taskB),
		fmt.Sprintf("/api/tasks/%d/outcome", fx.taskB),
		fmt.Sprintf("/api/tasks/%d/coverage", fx.taskB),
		"/api/report?task=active",
	} {
		want(404, url)
	}
	want(400, "/api/report")
	want(400, "/api/stats")
	want(403, "/api/exploration/findings/retests/active")
	want(403, "/api/tasks")
	want(200, fmt.Sprintf("/api/report?task=%d", fx.taskA))
	want(200, fmt.Sprintf("/api/tasks/%d/outcome", fx.taskA))
	want(200, fmt.Sprintf("/api/tasks/%d/coverage", fx.taskA))
	want(200, fmt.Sprintf("/api/exploration/findings/%d/lineage", fx.findA))
	want(200, fmt.Sprintf("/api/exploration/findings?task=%d", fx.taskA))

	var st struct{ Total int }
	if w := call(h, tok, "GET", "/api/exploration/findings/stats", ""); w.Code != 200 || decodeInto(w, &st) != nil || st.Total != 1 {
		t.Errorf("stats: %d total=%d %s", w.Code, st.Total, w.Body)
	}
	w := call(h, tok, "GET", fmt.Sprintf("/api/stats?task=%d", fx.taskA), "")
	if w.Code != 200 || strings.Contains(w.Body.String(), `"asset_counts"`) || strings.Contains(w.Body.String(), `"traffic"`) {
		t.Errorf("task stats leak global counts: %d %s", w.Code, w.Body)
	}
	if w := call(h, tok, "GET", "/api/exploration/findings/groups", ""); w.Code != 200 || strings.Contains(w.Body.String(), `"task_id":`+strconv.FormatInt(fx.taskB, 10)) {
		t.Errorf("groups: %d %s", w.Code, w.Body)
	}
	if es := auditSince(t, s, seq, db.AuditClientScopeDenied, ""); len(es) == 0 {
		t.Error("client scope refusals were not audited")
	}

	// The operator view is unchanged: the admin still sees both clients.
	aw := call(h, admin, "GET", "/api/exploration/findings?page=1&limit=200", "")
	if !strings.Contains(aw.Body.String(), fx.nameA) || !strings.Contains(aw.Body.String(), fx.nameB) {
		t.Errorf("admin list lost a client's findings: %.300s", aw.Body)
	}

	// Bound to both: a findings route must name the company.
	if w := call(h, admin, "PUT", path, fmt.Sprintf(`{"company_ids":[%d,%d]}`, fx.companyA, fx.companyB)); w.Code != 200 {
		t.Fatalf("rebind: %d %s", w.Code, w.Body)
	}
	want(400, "/api/exploration/findings?page=1&limit=50")
	want(200, fmt.Sprintf("/api/exploration/findings/%d", fx.findB))
	want(200, fmt.Sprintf("/api/report?task=%d", fx.taskB))
	// Withdrawn: nothing again.
	if w := call(h, admin, "PUT", path, `{"company_ids":[]}`); w.Code != 200 {
		t.Fatalf("unbind: %d %s", w.Code, w.Body)
	}
	want(403, fmt.Sprintf("/api/report?task=%d", fx.taskA))
}

func latestAuditSeq(t *testing.T, s *Server) int64 {
	t.Helper()
	var seq int64
	if err := s.m.pg.QueryRow(`SELECT COALESCE(max(seq), 0) FROM audit_chain`).Scan(&seq); err != nil {
		t.Fatal(err)
	}
	return seq
}

func decodeInto(w *httptest.ResponseRecorder, v any) error {
	return json.Unmarshal(w.Body.Bytes(), v)
}
