package server

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Autumn-27/artex/db"
)

// spendRoutes aggregate the global llm_usage ledger across every task and
// company, so a client_read user (bound to some companies) must not reach them.
var spendRoutes = []string{
	"GET /api/llm/cost",
	"GET /api/tokens/usage",
	"GET /api/tokens/daily",
	"GET /api/tokens/conversations",
}

// TestSpendRoutesRefuseClientRead: the cost and token-usage views answer 403
// to client_read and serve every internal role (c-9ca57a).
func TestSpendRoutesRefuseClientRead(t *testing.T) {
	mux := patternMux(registeredPatterns(t))
	s := &Server{}
	for _, p := range spendRoutes {
		method, path := splitPattern(p)
		r := httptest.NewRequest(method, path+"?days=30", nil)
		if _, got := mux.Handler(r); got != p {
			t.Fatalf("%s is not registered (mux matched %q)", p, got)
		}
		w := httptest.NewRecorder()
		if s.authorizeRole(w, r, mux, principal{Username: "client", Role: db.RoleClientRead}) {
			t.Errorf("%s: client_read was allowed", p)
		} else if w.Code != 403 || !strings.Contains(w.Body.String(), `read\" permission`) {
			t.Errorf("%s: client_read got %d %q, want 403 naming the read permission", p, w.Code, w.Body.String())
		}
		for _, role := range []string{db.RoleReviewer, db.RoleOperator, db.RoleAdmin} {
			if !s.authorizeRole(httptest.NewRecorder(), r, mux, principal{Username: role, Role: role}) {
				t.Errorf("%s: %s was refused", p, role)
			}
		}
		// No binder may narrow these to a client: the rows are not company-scoped.
		if clientBinders[p] != nil {
			t.Errorf("%s has a client binder; spend rows are global and cannot be narrowed", p)
		}
	}
}
