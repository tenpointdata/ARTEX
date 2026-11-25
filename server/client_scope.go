package server

import (
	"context"
	"log"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/Autumn-27/artex/db"
)

// Client binding: a client_read user sees only the companies it is bound to
// (artex_user_companies, migration 0019). rbacPolicy decides which routes the
// role may call; clientBinders decides, per route, which rows of that route the
// user may see. Every route the policy opens to client_read must have a binder
// (TestClientBindersCoverClientRoutes): a route without one is refused, so a
// new client route fails closed until it is bound. Other roles are global.
//
// Ids outside the binding answer 404, as if they did not exist. A binder
// narrows the request through clientScope in its context; the handlers apply
// it with clientFindingFilter (findings) or read it directly (report, stats).

// clientScope is what one client_read request may see.
type clientScope struct {
	Companies []int64 // every company the user is bound to, ascending
	Company   int64   // the company a findings route is narrowed to (0 = none chosen)
	// TaskCompanies are the bound companies the requested task is scoped to,
	// for task routes; non-empty once the task is admitted.
	TaskCompanies []int64
}

type clientScopeKey struct{}

// clientScopeFrom returns the binding requireAuth resolved, if the caller is a
// client_read user.
func clientScopeFrom(ctx context.Context) (clientScope, bool) {
	sc, ok := ctx.Value(clientScopeKey{}).(clientScope)
	return sc, ok
}

// clientFindingFilter narrows f to the request's bound company, with the strict
// company rule. Unchanged for callers who are not client-bound.
func clientFindingFilter(r *http.Request, f db.FindingFilter) db.FindingFilter {
	if sc, ok := clientScopeFrom(r.Context()); ok {
		f.Company = strconv.FormatInt(sc.Company, 10) // 0 matches nothing
		f.StrictCompany = true
	}
	return f
}

// bindErr is a binder's refusal: an HTTP status and message.
type bindErr struct {
	code int
	msg  string
}

var (
	errClientTaskNotFound    = &bindErr{404, "task not found"}
	errClientFindingNotFound = &bindErr{404, "finding not found"}
	errClientCompanyNotFound = &bindErr{404, "company not found"}
)

// clientBinder admits a client_read request on one route, filling sc, or refuses it.
type clientBinder func(s *Server, r *http.Request, sc *clientScope) *bindErr

// clientBinders maps each ServeMux pattern client_read may call to its binder.
var clientBinders = map[string]clientBinder{
	"GET /api/report":                            bindTaskQuery,
	"GET /api/stats":                             bindTaskQuery,
	"GET /api/tasks/{id}/outcome":                bindTaskPath,
	"GET /api/tasks/{id}/coverage":               bindTaskPath,
	"GET /api/me/companies":                      bindBound,
	"GET /api/me/tasks":                          bindCompany,
	"GET /api/events/stream":                     bindCompany,
	"GET /api/exploration/findings":              bindFindingsList,
	"GET /api/exploration/findings/groups":       bindCompany,
	"GET /api/exploration/findings/asset-tree":   bindCompany,
	"GET /api/exploration/findings/stats":        bindCompany,
	"GET /api/exploration/findings/export":       bindFindingsExport,
	"GET /api/exploration/findings/{id}":         bindFindingPath,
	"GET /api/exploration/findings/{id}/lineage": bindFindingPath,
	"GET /api/exploration/findings/{id}/retests": bindFindingPath,
	// The finding's captured request/response evidence.
	"GET /api/exploration/findings/{id}/traffic":                   bindFindingPath,
	"GET /api/exploration/findings/{id}/traffic/{binding_id}/body": bindFindingPath,
}

// bindClient applies the binding to a client_read request. It writes the
// refusal and returns false, or returns the request carrying its clientScope.
func (s *Server) bindClient(w http.ResponseWriter, r *http.Request, h http.Handler, p principal) (*http.Request, bool) {
	pattern := ""
	if mux, ok := h.(*http.ServeMux); ok {
		_, pattern = mux.Handler(r)
	}
	deny := func(code int, msg string) (*http.Request, bool) {
		s.appendAuditBestEffort(p.Username, db.AuditClientScopeDenied, r.URL.Path, map[string]string{
			"method": r.Method, "path": r.URL.Path, "query": r.URL.RawQuery, "status": strconv.Itoa(code),
		})
		writeErr(w, code, msg)
		return nil, false
	}
	binder := clientBinders[pattern]
	if binder == nil {
		return deny(403, "forbidden: this route is not available to client accounts")
	}
	if s.m == nil || s.m.pg == nil {
		writeErr(w, 503, errDataSourceUnavailable)
		return nil, false
	}
	companies, err := s.m.pg.UserCompanies(p.UserID)
	if err != nil {
		log.Printf("[rbac] client bindings of user %d: %v", p.UserID, err)
		writeErr(w, 503, errDataSourceUnavailable)
		return nil, false
	}
	if len(companies) == 0 {
		return deny(403, "forbidden: this client account is not bound to any company; ask an admin to bind it")
	}
	sc := clientScope{Companies: companies}
	// The mux sets path values only when it serves the request; the binder
	// runs before that, so give it the values of the pattern it will serve.
	r = withPatternValues(r, pattern)
	if e := binder(s, r, &sc); e != nil {
		if e.code >= 500 {
			writeErr(w, e.code, e.msg)
			return nil, false
		}
		return deny(e.code, e.msg)
	}
	return r.WithContext(context.WithValue(r.Context(), clientScopeKey{}, sc)), true
}

var errClientStore = &bindErr{503, errDataSourceUnavailable}

// bindCompany narrows a findings route to ?company=, which must be bound; with
// one binding it may be left out.
func bindCompany(_ *Server, r *http.Request, sc *clientScope) *bindErr {
	raw := strings.TrimSpace(r.URL.Query().Get("company"))
	if raw == "" {
		if len(sc.Companies) != 1 {
			return &bindErr{400, "company is required: this client account is bound to several companies"}
		}
		sc.Company = sc.Companies[0]
		return nil
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || !slices.Contains(sc.Companies, id) {
		return errClientCompanyNotFound
	}
	sc.Company = id
	return nil
}

// bindBound admits the request as is: the handler lists only sc.Companies.
func bindBound(*Server, *http.Request, *clientScope) *bindErr { return nil }

// bindTask admits a task the user's companies own.
func bindTask(s *Server, raw string, sc *clientScope) *bindErr {
	id, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || id <= 0 {
		return errClientTaskNotFound
	}
	owned, err := s.m.pg.TaskCompaniesIn(id, sc.Companies)
	if err != nil {
		log.Printf("[rbac] client task %d binding: %v", id, err)
		return errClientStore
	}
	if len(owned) == 0 {
		return errClientTaskNotFound
	}
	sc.TaskCompanies = owned
	return nil
}

// bindTaskQuery admits ?task=<id>. It is required: the active task and the
// global view may belong to another client.
func bindTaskQuery(s *Server, r *http.Request, sc *clientScope) *bindErr {
	raw := r.URL.Query().Get("task")
	if strings.TrimSpace(raw) == "" {
		return &bindErr{400, "task is required for client accounts"}
	}
	return bindTask(s, raw, sc)
}

func bindTaskPath(s *Server, r *http.Request, sc *clientScope) *bindErr {
	return bindTask(s, r.PathValue("id"), sc)
}

// bindFindingsList: ?task= reads that task's graph; without it, the global
// table narrowed to the company.
func bindFindingsList(s *Server, r *http.Request, sc *clientScope) *bindErr {
	if r.URL.Query().Get("task") != "" {
		return bindTaskQuery(s, r, sc)
	}
	return bindCompany(s, r, sc)
}

// bindFindingsExport narrows the export to the company; selected ids must all
// belong to it.
func bindFindingsExport(s *Server, r *http.Request, sc *clientScope) *bindErr {
	if e := bindCompany(s, r, sc); e != nil {
		return e
	}
	q := r.URL.Query()
	if q.Get("scope") != "selected" {
		return nil
	}
	f := clientCompanyFilter(sc.Company)
	for _, part := range strings.Split(q.Get("ids"), ",") {
		if part = strings.TrimSpace(part); part == "" {
			continue
		}
		id, err := strconv.ParseInt(part, 10, 64)
		if err != nil {
			continue // the handler answers 400
		}
		if e := findingVisible(s, f, id); e != nil {
			return e
		}
	}
	return nil
}

// bindFindingPath admits a finding of any bound company, and ?context_task=
// only when that task is bound too.
func bindFindingPath(s *Server, r *http.Request, sc *clientScope) *bindErr {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		return errClientFindingNotFound
	}
	var last *bindErr
	for _, c := range sc.Companies {
		if last = findingVisible(s, clientCompanyFilter(c), id); last == nil {
			sc.Company = c
			break
		}
		if last != errClientFindingNotFound {
			return last
		}
	}
	if last != nil {
		return last
	}
	if ct := strings.TrimSpace(r.URL.Query().Get("context_task")); ct != "" {
		return bindTask(s, ct, sc)
	}
	return nil
}

func clientCompanyFilter(company int64) db.FindingFilter {
	return db.FindingFilter{Company: strconv.FormatInt(company, 10), StrictCompany: true}
}

func findingVisible(s *Server, f db.FindingFilter, id int64) *bindErr {
	ok, err := s.m.pg.FindingMatches(f, id)
	if err != nil {
		log.Printf("[rbac] client finding %d binding: %v", id, err)
		return errClientStore
	}
	if !ok {
		return errClientFindingNotFound
	}
	return nil
}

// clientAssetCounts counts assets by type over the given companies, for the
// report of a client-bound caller (the operator report counts every asset).
func (s *Server) clientAssetCounts(companies []int64, types []string) map[string]int {
	counts := map[string]int{}
	as := s.m.Assets()
	if as == nil {
		return counts
	}
	for _, ty := range types {
		for _, c := range companies {
			if n, err := as.CountByCompany(c, ty); err == nil && n > 0 {
				counts[ty] += n
			}
		}
	}
	return counts
}

// withPatternValues returns a copy of r whose PathValue answers the wildcards
// of pattern ("GET /a/{id}"), as the mux will set them when it serves r.
func withPatternValues(r *http.Request, pattern string) *http.Request {
	pat := pathSegments(routePath(pattern))
	segs := pathSegments(r.URL.Path)
	r = r.WithContext(r.Context())
	for i, p := range pat {
		if i >= len(segs) || !strings.HasPrefix(p, "{") || !strings.HasSuffix(p, "}") {
			continue
		}
		name := strings.TrimSuffix(strings.TrimSuffix(strings.TrimPrefix(p, "{"), "}"), "...")
		r.SetPathValue(name, segs[i])
	}
	return r
}
