package server

import (
	"log"
	"net/http"
	"slices"
	"strconv"
	"strings"
)

// The client console's own routes (GET /api/me/...): what the signed-in user
// may pick from. A client_read caller gets its bound companies and their
// tasks (client_scope.go narrows the request); other roles are global.

// ClientTaskDTO is a task as a client sees it in its task list: what it is
// and how far it got. Goal text, models, tokens and task-wide finding counts
// (which span every company the task is scoped to) stay operator-only.
type ClientTaskDTO struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Status        string `json:"status"`
	CreatedAt     string `json:"created_at"`
	CompletedAt   string `json:"completed_at"`
	LastActivity  int64  `json:"last_activity_unix"`
	CoverageReady bool   `json:"coverage_enabled"`
}

// myCompanies: GET /api/me/companies. A client's bound companies; every
// company for other roles.
func (s *Server) myCompanies(w http.ResponseWriter, r *http.Request) {
	if s.m == nil || s.m.pg == nil {
		writeErr(w, 503, errDataSourceUnavailable)
		return
	}
	var ids []int64 // nil = every company
	if sc, ok := clientScopeFrom(r.Context()); ok {
		ids = sc.Companies
	}
	rows, err := s.m.pg.CompanySummaries(ids)
	if err != nil {
		log.Printf("[client] my companies: %v", err)
		writeErr(w, 500, "company query failed")
		return
	}
	writeJSON(w, 200, rows)
}

// myTasks: GET /api/me/tasks?company=<id>. The tasks an operator scoped to
// the company, newest first. The company is required for operators; a client
// bound to one company may leave it out (bindCompany).
func (s *Server) myTasks(w http.ResponseWriter, r *http.Request) {
	if s.m == nil || s.m.pg == nil {
		writeErr(w, 503, errDataSourceUnavailable)
		return
	}
	company := int64(0)
	if sc, ok := clientScopeFrom(r.Context()); ok {
		company = sc.Company
	} else {
		id, err := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("company")), 10, 64)
		if err != nil || id <= 0 {
			writeErr(w, 400, "company is required")
			return
		}
		company = id
	}
	ids, err := s.m.pg.CompanyTaskIDs(company)
	if err != nil {
		log.Printf("[client] tasks of company %d: %v", company, err)
		writeErr(w, 500, "task query failed")
		return
	}
	out := []ClientTaskDTO{}
	for _, t := range s.m.List() {
		id, err := strconv.ParseInt(t.ID, 10, 64)
		if err != nil || !slices.Contains(ids, id) {
			continue
		}
		d := taskDTO(t, s.resolvedTaskStatus(t))
		last := s.engine.LastActivity(t.ID)
		out = append(out, ClientTaskDTO{
			ID: d.ID, Name: d.Name, Status: d.Status, CreatedAt: d.CreatedAt,
			CompletedAt: d.CompletedAt, LastActivity: last, CoverageReady: d.CoverageEnabled,
		})
	}
	slices.SortFunc(out, func(a, b ClientTaskDTO) int { return strings.Compare(b.CreatedAt, a.CreatedAt) })
	writeJSON(w, 200, out)
}
