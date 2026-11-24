package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/Autumn-27/artex/db"
)

// rbacTestServer is a Server over the test database with the users API and a
// stand-in for every other route behind requireAuth.
func rbacTestServer(t *testing.T) (*Server, http.Handler) {
	t.Helper()
	m, err := NewManager(t.TempDir(), "")
	if err != nil {
		db.SkipPGUnavailable(t, err)
	}
	t.Cleanup(func() { _ = m.Close() })
	s := &Server{m: m, jwtKey: []byte("0123456789abcdef0123456789abcdef"),
		loginLimit: newLoginLimiter(time.Now), pwdLimit: newLoginLimiter(time.Now)}
	mux := http.NewServeMux()
	s.registerRBACRoutes(mux)
	ok := func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"approver": s.approver(r)})
	}
	for _, p := range []string{"GET /api/tasks", "POST /api/tasks", "GET /api/report",
		"POST /api/intercept/pending/{id}/decide", "PUT /api/settings", "GET /api/tasks/{id}/credentials"} {
		mux.HandleFunc(p, ok)
	}
	return s, s.requireAuth(mux)
}

func uniqueName(prefix string) string {
	return fmt.Sprintf("%s%d", prefix, time.Now().UnixNano())
}

// seedUser creates a user directly in the store and removes it at cleanup.
func seedUser(t *testing.T, s *Server, role, password string) db.User {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	u, err := s.m.pg.CreateUser("test-setup", uniqueName("u"), string(hash), role)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = s.m.pg.Exec(`DELETE FROM artex_users WHERE id = $1`, u.ID) })
	return u
}

func loginAs(t *testing.T, s *Server, username, password string) (int, string) {
	t.Helper()
	body := fmt.Sprintf(`{"username":%q,"password":%q}`, username, password)
	r := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(body))
	r.RemoteAddr = "198.51.100.7:1234"
	w := httptest.NewRecorder()
	s.authLogin(w, r)
	var out struct {
		Token string `json:"token"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out.Token
}

func call(h http.Handler, tok, method, path, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if tok != "" {
		r.Header.Set("Authorization", "Bearer "+tok)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

// auditSince returns entries of kind for subject appended after seq.
func auditSince(t *testing.T, s *Server, seq int64, kind, subject string) []db.AuditEntry {
	t.Helper()
	es, err := s.m.pg.ListAudit(db.AuditFilter{AfterSeq: seq, Kind: kind, Subject: subject, Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	return es
}

func auditHead(t *testing.T, s *Server) int64 {
	t.Helper()
	var seq int64
	if err := s.m.pg.QueryRow(`SELECT seq FROM audit_chain_head WHERE id = 1`).Scan(&seq); err != nil {
		t.Fatal(err)
	}
	return seq
}

func TestUserLoginAndRevocation(t *testing.T) {
	s, h := rbacTestServer(t)
	head := auditHead(t, s)
	u := seedUser(t, s, db.RoleReviewer, "reviewer-pass-1")

	if code, _ := loginAs(t, s, u.Username, "wrong-password-1"); code != 401 {
		t.Fatalf("wrong password: %d", code)
	}
	if code, _ := loginAs(t, s, "nobody-"+u.Username, "whatever-123"); code != 401 {
		t.Fatalf("unknown user: %d", code)
	}
	code, tok := loginAs(t, s, strings.ToUpper(u.Username), "reviewer-pass-1")
	if code != 200 || tok == "" {
		t.Fatalf("login: %d", code)
	}
	failed := auditSince(t, s, head, db.AuditLoginFailed, u.Username)
	if len(failed) != 1 || failed[0].Actor != auditAnonymous || strings.Contains(string(failed[0].Payload), "wrong-password") ||
		!strings.Contains(string(failed[0].Payload), "198.51.100.7") {
		t.Fatalf("failed-login audit: %+v", failed)
	}
	if ok := auditSince(t, s, head, db.AuditLoginOK, u.Username); len(ok) != 1 || ok[0].Actor != u.Username {
		t.Fatalf("login audit: %+v", ok)
	}

	// The session reports the role; the approver is the username.
	sr := httptest.NewRequest("GET", "/api/auth/session", nil)
	sr.Header.Set("Authorization", "Bearer "+tok)
	sw := httptest.NewRecorder()
	s.authSession(sw, sr)
	if !strings.Contains(sw.Body.String(), `"role":"reviewer"`) || !strings.Contains(sw.Body.String(), `"username":"`+u.Username+`"`) {
		t.Fatalf("session: %s", sw.Body.String())
	}
	if w := call(h, tok, "POST", "/api/intercept/pending/3/decide", "{}"); w.Code != 200 || !strings.Contains(w.Body.String(), u.Username) {
		t.Fatalf("reviewer decide: %d %s", w.Code, w.Body.String())
	}

	// A reviewer cannot operate, and the refusal is audited.
	if w := call(h, tok, "POST", "/api/tasks", "{}"); w.Code != 403 {
		t.Fatalf("reviewer POST /api/tasks: %d", w.Code)
	}
	denied := auditSince(t, s, head, db.AuditRoleDenied, "/api/tasks")
	if len(denied) != 1 || denied[0].Actor != u.Username || !strings.Contains(string(denied[0].Payload), `"required":"operate"`) {
		t.Fatalf("denial audit: %+v", denied)
	}
	if w := call(h, tok, "GET", "/api/tasks/7/credentials", ""); w.Code != 403 {
		t.Fatalf("reviewer read credentials: %d", w.Code)
	}

	// Any change to the user bumps its version and ends the session.
	role := db.RoleOperator
	if _, err := s.m.pg.UpdateUser("test-setup", u.ID, db.UserPatch{Role: &role}); err != nil {
		t.Fatal(err)
	}
	if w := call(h, tok, "GET", "/api/tasks", ""); w.Code != 401 {
		t.Fatalf("token after version bump: %d", w.Code)
	}
	_, tok = loginAs(t, s, u.Username, "reviewer-pass-1")
	if w := call(h, tok, "POST", "/api/tasks", "{}"); w.Code != 200 {
		t.Fatalf("operator POST /api/tasks: %d", w.Code)
	}
	if w := call(h, tok, "PUT", "/api/settings", "{}"); w.Code != 403 {
		t.Fatalf("operator PUT /api/settings: %d", w.Code)
	}

	// Disabled: the session ends and login fails.
	off := true
	if _, err := s.m.pg.UpdateUser("test-setup", u.ID, db.UserPatch{Disabled: &off}); err != nil {
		t.Fatal(err)
	}
	if s.sessionOK(tok) {
		t.Fatal("disabled user's token accepted")
	}
	if code, _ := loginAs(t, s, u.Username, "reviewer-pass-1"); code != 401 {
		t.Fatalf("disabled login: %d", code)
	}

	// Deleted: fails closed.
	on := false
	if _, err := s.m.pg.UpdateUser("test-setup", u.ID, db.UserPatch{Disabled: &on}); err != nil {
		t.Fatal(err)
	}
	_, tok = loginAs(t, s, u.Username, "reviewer-pass-1")
	if err := s.m.pg.DeleteUser("test-setup", u.ID); err != nil {
		t.Fatal(err)
	}
	if s.sessionOK(tok) {
		t.Fatal("deleted user's token accepted")
	}
}

func TestClientReadIsDeniedByDefault(t *testing.T) {
	s, h := rbacTestServer(t)
	u := seedUser(t, s, db.RoleClientRead, "client-pass-12")
	_, tok := loginAs(t, s, u.Username, "client-pass-12")
	if w := call(h, tok, "GET", "/api/report", ""); w.Code != 200 {
		t.Fatalf("client report: %d", w.Code)
	}
	for _, p := range []string{"/api/tasks", "/api/users"} {
		if w := call(h, tok, "GET", p, ""); w.Code != 403 {
			t.Fatalf("client GET %s: %d", p, w.Code)
		}
	}
}

func TestUserTokenFailsClosedOnDBError(t *testing.T) {
	s, h := rbacTestServer(t)
	u := seedUser(t, s, db.RoleAdmin, "admin-pass-123")
	_, tok := loginAs(t, s, u.Username, "admin-pass-123")
	if w := call(h, tok, "GET", "/api/tasks", ""); w.Code != 200 {
		t.Fatalf("before: %d", w.Code)
	}
	if err := s.m.pg.Close(); err != nil {
		t.Fatal(err)
	}
	if w := call(h, tok, "GET", "/api/tasks", ""); w.Code != 401 {
		t.Fatalf("with the store down: %d", w.Code)
	}
	if code, _ := loginAs(t, s, u.Username, "admin-pass-123"); code != 503 {
		t.Fatalf("login with the store down: %d", code)
	}
}

func TestUsersAPI(t *testing.T) {
	s, h := rbacTestServer(t)
	legacy, err := signJWT(s.jwtKey)
	if err != nil {
		t.Fatal(err)
	}
	name := uniqueName("api")
	w := call(h, legacy, "POST", "/api/users", fmt.Sprintf(`{"username":%q,"password":"short","role":"admin"}`, name))
	if w.Code != 400 {
		t.Fatalf("short password: %d", w.Code)
	}
	if w := call(h, legacy, "POST", "/api/users", `{"username":"artex","password":"long-enough-pass","role":"admin"}`); w.Code != 400 {
		t.Fatalf("reserved name: %d", w.Code)
	}
	if w := call(h, legacy, "POST", "/api/users", fmt.Sprintf(`{"username":%q,"password":"long-enough-pass","role":"root"}`, name)); w.Code != 400 {
		t.Fatalf("bad role: %d", w.Code)
	}
	w = call(h, legacy, "POST", "/api/users", fmt.Sprintf(`{"username":%q,"password":"long-enough-pass","role":"admin"}`, name))
	if w.Code != 201 || strings.Contains(w.Body.String(), "hash") {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	var created db.User
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = s.m.pg.Exec(`DELETE FROM artex_users WHERE id = $1`, created.ID) })
	if created.CreatedBy != "ARTEX" {
		t.Fatalf("created_by %q", created.CreatedBy)
	}
	if w := call(h, legacy, "POST", "/api/users", fmt.Sprintf(`{"username":%q,"password":"long-enough-pass","role":"admin"}`, name)); w.Code != 409 {
		t.Fatalf("duplicate: %d", w.Code)
	}
	if w := call(h, legacy, "GET", "/api/users", ""); w.Code != 200 || strings.Contains(w.Body.String(), "$2a$") {
		t.Fatalf("list: %d %s", w.Code, w.Body.String())
	}

	// The new admin cannot lock themselves out, but can manage others.
	_, tok := loginAs(t, s, name, "long-enough-pass")
	id := fmt.Sprint(created.ID)
	for _, body := range []string{`{"role":"reviewer"}`, `{"disabled":true}`} {
		if w := call(h, tok, "PATCH", "/api/users/"+id, body); w.Code != 403 {
			t.Fatalf("self %s: %d", body, w.Code)
		}
	}
	if w := call(h, tok, "DELETE", "/api/users/"+id, ""); w.Code != 403 {
		t.Fatalf("self delete: %d", w.Code)
	}
	other := seedUser(t, s, db.RoleOperator, "operator-pass-1")
	w = call(h, tok, "PATCH", fmt.Sprintf("/api/users/%d", other.ID), `{"role":"reviewer","password":"new-operator-pass"}`)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"version":2`) {
		t.Fatalf("patch other: %d %s", w.Code, w.Body.String())
	}
	if code, _ := loginAs(t, s, other.Username, "new-operator-pass"); code != 200 {
		t.Fatalf("login with the new password: %d", code)
	}
	// A non-admin cannot reach the API at all.
	_, otok := loginAs(t, s, other.Username, "new-operator-pass")
	if w := call(h, otok, "GET", "/api/users", ""); w.Code != 403 {
		t.Fatalf("reviewer list users: %d", w.Code)
	}
	if w := call(h, tok, "DELETE", fmt.Sprintf("/api/users/%d", other.ID), ""); w.Code != 200 {
		t.Fatalf("delete other: %d", w.Code)
	}
	if w := call(h, tok, "DELETE", fmt.Sprintf("/api/users/%d", other.ID), ""); w.Code != 404 {
		t.Fatalf("delete again: %d", w.Code)
	}
}
