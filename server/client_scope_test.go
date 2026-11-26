package server

import (
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/Autumn-27/artex/db"
)

// TestClientBindersCoverClientRoutes: every route rbacPolicy opens to
// client_read has a binder, and every binder names such a route. A new client
// route without a binder would be refused (bindClient fails closed); this test
// says so at build time instead of in production.
func TestClientBindersCoverClientRoutes(t *testing.T) {
	patterns := registeredPatterns(t)
	mux := patternMux(patterns)
	s := &Server{}
	var open []string
	for _, p := range patterns {
		method, path := splitPattern(p)
		r := httptest.NewRequest(method, concrete(path), nil)
		if _, got := mux.Handler(r); got != p {
			continue
		}
		w := httptest.NewRecorder()
		if s.authorizeRole(w, r, mux, principal{Username: "c", Role: db.RoleClientRead}) {
			open = append(open, p)
			if clientBinders[p] == nil {
				t.Errorf("%s is open to client_read but has no client binder", p)
			}
		}
	}
	for p := range clientBinders {
		if !slices.Contains(open, p) {
			t.Errorf("client binder %s names no route open to client_read", p)
		}
	}
}
