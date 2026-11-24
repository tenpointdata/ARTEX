package server

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/Autumn-27/artex/db"
)

// registeredPatterns returns every literal "METHOD /api/..." route pattern in
// the package's non-test sources, plus the ones built from variables.
func registeredPatterns(t *testing.T) []string {
	t.Helper()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile(`"((?:GET|POST|PUT|PATCH|DELETE) /api/[^"]*)"`)
	seen := map[string]bool{}
	var out []string
	add := func(p string) {
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range re.FindAllStringSubmatch(string(src), -1) {
			add(m[1])
		}
	}
	// finding_traffic.go builds its patterns from a base path.
	for _, p := range []string{
		"GET /api/exploration/findings/{id}/traffic", "POST /api/exploration/findings/{id}/traffic",
		"PATCH /api/exploration/findings/{id}/traffic/{binding_id}", "GET /api/exploration/findings/{id}/traffic/{binding_id}/body",
	} {
		add(p)
	}
	if len(out) < 200 {
		t.Fatalf("found only %d route patterns; the scan is broken", len(out))
	}
	return out
}

// patternMux registers every pattern on a fresh mux, as Handler() does.
func patternMux(patterns []string) *http.ServeMux {
	mux := http.NewServeMux()
	for _, p := range patterns {
		mux.HandleFunc(p, func(http.ResponseWriter, *http.Request) {})
	}
	return mux
}

// concrete turns a pattern's path into a request path.
func concrete(path string) string {
	return regexp.MustCompile(`\{[^}]+\}`).ReplaceAllString(path, "7")
}

func splitPattern(p string) (method, path string) {
	method, path, _ = strings.Cut(p, " ")
	return
}

// clientReadAllowed is the allowlist for the client_read role.
func clientReadAllowed(method, path string) bool {
	if method != http.MethodGet {
		return false
	}
	switch path {
	case "/api/report", "/api/stats", "/api/tasks/{id}/outcome", "/api/tasks/{id}/coverage":
		return true
	}
	switch path {
	case "/api/exploration/findings/retests/active": // live retests across every client
		return false
	case "/api/exploration/findings/{id}/integrations": // tracker tickets and raw scanner records
		return false
	}
	return strings.HasPrefix(path, "/api/report/") || path == "/api/exploration/findings" ||
		strings.HasPrefix(path, "/api/exploration/findings/")
}

func TestRBACEveryRegisteredRouteResolves(t *testing.T) {
	patterns := registeredPatterns(t)
	mux := patternMux(patterns)
	s := &Server{}
	for _, p := range patterns {
		method, path := splitPattern(p)
		r := httptest.NewRequest(method, concrete(path), nil)
		_, got := mux.Handler(r)
		if got != p {
			// A more specific sibling pattern serves this concrete path; it is checked on its own.
			continue
		}
		need := requiredPermission(method, routePath(got))
		known := false
		for _, q := range allPermissions {
			known = known || q == need
		}
		if !known {
			t.Errorf("%s resolves to unknown permission %q", p, need)
		}
		if !roleHas(db.RoleAdmin, need) {
			t.Errorf("%s: admin lacks %q", p, need)
		}
		w := httptest.NewRecorder()
		allowed := s.authorizeRole(w, r, mux, principal{Username: "c", Role: db.RoleClientRead})
		if allowed != clientReadAllowed(method, path) {
			t.Errorf("%s: client_read allowed=%v, want %v", p, allowed, !allowed)
		}
		if !allowed && w.Code != 403 {
			t.Errorf("%s: refusal status %d, want 403", p, w.Code)
		}
	}
}

func TestRBACPolicyByRole(t *testing.T) {
	cases := []struct {
		method, path string
		need         rbacPerm
	}{
		{"GET", "/api/users", permAdmin},
		{"POST", "/api/users", permAdmin},
		{"PATCH", "/api/users/3", permAdmin},
		{"DELETE", "/api/users/3", permAdmin},
		{"POST", "/api/tasks/t1/authorization", permAdmin},
		{"POST", "/api/tasks/t1/authorization/revoke", permAdmin},
		{"GET", "/api/tasks/t1/authorization", permRead},
		{"POST", "/api/intercept/rules", permAdmin},
		{"PUT", "/api/intercept/rules/4", permAdmin},
		{"POST", "/api/intercept/rules/4/toggle", permAdmin},
		{"DELETE", "/api/asset-intercept/rules/4", permAdmin},
		{"PUT", "/api/target-gate/exemptions", permAdmin},
		{"PUT", "/api/intercept/judge", permAdmin},
		{"PUT", "/api/intercept/tool-config", permAdmin},
		{"PUT", "/api/settings", permAdmin},
		{"POST", "/api/tasks/t1/intercept-rules", permAdmin},
		{"PUT", "/api/tasks/t1/intercept-rules/2", permAdmin},
		{"POST", "/api/intercept/pending/9/always-allow", permAdmin},
		{"GET", "/api/intercept/rules", permRead},
		{"GET", "/api/target-gate/exemptions", permRead},
		{"POST", "/api/intercept/pending/9/decide", permReview},
		{"POST", "/api/intercept/pending/batch-decide", permReview},
		{"PATCH", "/api/exploration/findings/5", permReview},
		{"POST", "/api/exploration/findings/bulk-status", permReview},
		{"POST", "/api/exploration/findings/bulk-report-state", permReview},
		{"GET", "/api/tasks/t1/credentials", permOperate},
		{"GET", "/api/workspace/download", permOperate},
		{"GET", "/api/workspace/read", permOperate},
		{"GET", "/api/llm/records", permOperate},
		{"GET", "/api/llm/records/12", permOperate},
		{"GET", "/api/logs/stream", permOperate},
		{"HEAD", "/api/logs", permOperate},
		{"GET", "/api/traffic/exchange", permOperate},
		{"GET", "/api/traffic/blob", permOperate},
		{"GET", "/api/traffic/export/har", permOperate},
		{"GET", "/api/traffic/stream", permOperate},
		{"GET", "/api/conversations/3/messages/7", permOperate},
		{"GET", "/api/traffic/hosts", permRead},
		{"GET", "/api/intercept/history/chain", permRead},
		{"GET", "/api/intercept/history/chain/verify", permRead},
		{"GET", "/api/report", permClientRead},
		{"GET", "/api/exploration/findings", permClientRead},
		{"GET", "/api/exploration/findings/5/lineage", permClientRead},
		{"GET", "/api/exploration/findings/5/integrations", permRead},
		{"POST", "/api/integrations/trackers", permAdmin},
		{"PATCH", "/api/integrations/trackers/2", permAdmin},
		{"DELETE", "/api/integrations/trackers/2", permAdmin},
		{"POST", "/api/integrations/trackers/2/push", permOperate},
		{"POST", "/api/integrations/trackers/2/sync", permOperate},
		{"POST", "/api/integrations/imports", permOperate},
		{"GET", "/api/integrations/imports", permRead},
		{"GET", "/api/stats", permClientRead},
		{"GET", "/api/tasks/t1/outcome", permClientRead},
		{"GET", "/api/tasks/t1/coverage", permClientRead},
		{"GET", "/api/tasks/t1/coverage-graph", permRead},
		{"GET", "/api/tasks", permRead},
		{"POST", "/api/tasks", permOperate},
		{"POST", "/api/tasks/t1/control", permOperate},
		{"DELETE", "/api/exploration/findings/5", permOperate},
		// Dot segments and doubled slashes are cleaned before matching.
		{"GET", "/api/x/../tasks/t1/credentials", permOperate},
		{"POST", "/api//users", permAdmin},
	}
	roles := map[string]map[rbacPerm]bool{
		db.RoleAdmin:      {permClientRead: true, permRead: true, permReview: true, permOperate: true, permAdmin: true},
		db.RoleOperator:   {permClientRead: true, permRead: true, permReview: true, permOperate: true},
		db.RoleReviewer:   {permClientRead: true, permRead: true, permReview: true},
		db.RoleClientRead: {permClientRead: true},
		"bogus":           {},
	}
	for _, c := range cases {
		need := requiredPermission(c.method, c.path)
		if need != c.need {
			t.Errorf("%s %s: need %q, want %q", c.method, c.path, need, c.need)
			continue
		}
		for role, holds := range roles {
			if got := roleHas(role, need); got != holds[need] {
				t.Errorf("%s %s as %s: allowed=%v want %v", c.method, c.path, role, got, holds[need])
			}
		}
	}
}

// The mux unescapes %2F inside one segment; the policy must see the route the
// mux will serve, not a differently split path.
func TestRBACMatchesTheServedRoute(t *testing.T) {
	mux := patternMux([]string{"GET /api/tasks/{id}/credentials", "GET /api/tasks/{id}"})
	s := &Server{}
	r := httptest.NewRequest("GET", "/api/tasks/a%2Fb/credentials", nil)
	w := httptest.NewRecorder()
	if s.authorizeRole(w, r, mux, principal{Username: "rev", Role: db.RoleReviewer}) {
		t.Fatal("reviewer read credentials through an escaped slash in the task id")
	}
	if w.Code != 403 || !strings.Contains(w.Body.String(), "operate") || !strings.Contains(w.Body.String(), "reviewer") {
		t.Fatalf("refusal: %d %s", w.Code, w.Body.String())
	}
}

func TestLegacyTokenIsAdminAndKeepsItsClaims(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	s := &Server{jwtKey: key}
	tok, err := signJWT(key)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := base64.RawURLEncoding.DecodeString(strings.Split(tok, ".")[1])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(payload), `"uid"`) || strings.Contains(string(payload), `"uv"`) {
		t.Fatalf("legacy token gained user claims: %s", payload)
	}
	p, ok := s.sessionPrincipal(tok)
	if !ok || p.Username != "ARTEX" || p.Role != db.RoleAdmin || p.UserID != 0 {
		t.Fatalf("legacy principal = %+v, %v", p, ok)
	}
	// A legacy token still follows the password version.
	s.applyPasswordVersion(3)
	if s.sessionOK(tok) {
		t.Fatal("version-0 legacy token accepted at password version 3")
	}
	cur, _ := signJWTVersion(key, 3)
	if !s.sessionOK(cur) {
		t.Fatal("current legacy token rejected")
	}
	r := httptest.NewRequest("POST", "/api/intercept/pending/1/decide", nil)
	r.Header.Set("Authorization", "Bearer "+cur)
	if got := s.approver(r); got != "ARTEX" {
		t.Fatalf("approver = %q", got)
	}
}

func TestUserTokenFailsClosedWithoutUserStore(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	s := &Server{jwtKey: key}
	tok, err := signUserJWT(key, db.User{ID: 5, Username: "alice", Version: 1, Role: db.RoleAdmin})
	if err != nil {
		t.Fatal(err)
	}
	if s.sessionOK(tok) {
		t.Fatal("user token accepted with no user store to check it against")
	}
	// The user token must not pass as a legacy token at password version 0.
	if _, ok := s.legacyPrincipal(&sessionClaims{UID: 5}); ok {
		t.Fatal("subject-less claims accepted as legacy")
	}
	r := httptest.NewRequest("GET", "/api/tasks", nil)
	r.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	s.requireAuth(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("handler ran for an unverifiable user token")
	})).ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatalf("status %d, want 401", w.Code)
	}
}

func TestValidateUsername(t *testing.T) {
	for _, ok := range []string{"alice", "a1", "ops.team_2-x"} {
		if msg := validateUsername(ok); msg != "" {
			t.Errorf("%q rejected: %s", ok, msg)
		}
	}
	for _, bad := range []string{"", "a", "artex", "Alice", "-x", "has space", strings.Repeat("a", 65)} {
		if validateUsername(bad) == "" {
			t.Errorf("%q accepted", bad)
		}
	}
}
