package server

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Autumn-27/artex/db"
)

func doChangePassword(s *Server, tok, oldPw, newPw string) *httptest.ResponseRecorder {
	body := fmt.Sprintf(`{"old_password":%q,"new_password":%q}`, oldPw, newPw)
	r := httptest.NewRequest("POST", "/api/auth/change-password", strings.NewReader(body))
	r.Header.Set("Authorization", "Bearer "+tok)
	r.RemoteAddr = "198.51.100.9:4321"
	w := httptest.NewRecorder()
	s.authChangePassword(w, r)
	return w
}

func TestUserChangesOwnPassword(t *testing.T) {
	s, h := rbacTestServer(t)
	head := auditHead(t, s)
	u := seedUser(t, s, db.RoleOperator, "old-password-1234")
	_, tok := loginAs(t, s, u.Username, "old-password-1234")
	if w := call(h, tok, "GET", "/api/tasks", ""); w.Code != 200 {
		t.Fatalf("before: %d", w.Code)
	}

	// Wrong current password: 401, nothing changes, the session survives.
	if w := doChangePassword(s, tok, "not-the-password-1", "new-password-5678"); w.Code != 401 || !strings.Contains(w.Body.String(), "Current password is wrong") {
		t.Fatalf("wrong old password: %d %s", w.Code, w.Body.String())
	}
	if w := call(h, tok, "GET", "/api/tasks", ""); w.Code != 200 {
		t.Fatalf("session after a refused change: %d", w.Code)
	}
	if got, _ := s.m.pg.GetUser(u.ID); got.Version != u.Version {
		t.Fatalf("version moved on a refused change: %d -> %d", u.Version, got.Version)
	}
	// A weak new password is refused before anything is read.
	if w := doChangePassword(s, tok, "old-password-1234", "short"); w.Code != 400 {
		t.Fatalf("weak new password: %d %s", w.Code, w.Body.String())
	}

	w := doChangePassword(s, tok, "old-password-1234", "new-password-5678")
	if w.Code != 200 {
		t.Fatalf("change: %d %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "token") {
		t.Fatalf("the response must carry no replacement token: %s", w.Body.String())
	}
	if w := call(h, tok, "GET", "/api/tasks", ""); w.Code != 401 {
		t.Fatalf("old token after the change: %d", w.Code)
	}
	if code, _ := loginAs(t, s, u.Username, "old-password-1234"); code != 401 {
		t.Fatalf("login with the old password: %d", code)
	}
	code, tok2 := loginAs(t, s, u.Username, "new-password-5678")
	if code != 200 {
		t.Fatalf("login with the new password: %d", code)
	}
	if w := call(h, tok2, "GET", "/api/tasks", ""); w.Code != 200 {
		t.Fatalf("new session: %d", w.Code)
	}

	// The audit chain names the user, and never records the hash.
	es := auditSince(t, s, head, "row.artex_users", fmt.Sprint(u.ID))
	var found bool
	for _, e := range es {
		if strings.Contains(string(e.Payload), `"op": "UPDATE"`) || strings.Contains(string(e.Payload), `"op":"UPDATE"`) {
			found = true
			if e.Actor != u.Username {
				t.Fatalf("audit actor %q want %q", e.Actor, u.Username)
			}
			if strings.Contains(string(e.Payload), "password_hash") {
				t.Fatalf("audit payload holds the hash: %s", e.Payload)
			}
		}
	}
	if !found {
		t.Fatalf("no UPDATE audit entry for the user: %+v", es)
	}
}

func TestUserChangePasswordRefusals(t *testing.T) {
	s, _ := rbacTestServer(t)
	u := seedUser(t, s, db.RoleOperator, "old-password-1234")
	_, tok := loginAs(t, s, u.Username, "old-password-1234")
	who := principal{Username: u.Username, Role: u.Role, UserID: u.ID}

	// A disabled user is refused, and its password is not changed. (The
	// handler's own session check also refuses a disabled user's token; this
	// calls the step below it.)
	off := true
	if _, err := s.m.pg.UpdateUser("test-setup", u.ID, db.UserPatch{Disabled: &off}); err != nil {
		t.Fatal(err)
	}
	if w := doChangePassword(s, tok, "old-password-1234", "new-password-5678"); w.Code != 401 {
		t.Fatalf("disabled user's token: %d", w.Code)
	}
	r := httptest.NewRequest("POST", "/api/auth/change-password", nil)
	w := httptest.NewRecorder()
	s.changeUserPassword(w, r, who, u.Version+1, "old-password-1234", "new-password-5678")
	if w.Code != 403 {
		t.Fatalf("disabled user: %d %s", w.Code, w.Body.String())
	}
	if code, _ := loginAs(t, s, u.Username, "old-password-1234"); code != 401 {
		t.Fatalf("disabled login: %d", code)
	}
	on := false
	if _, err := s.m.pg.UpdateUser("test-setup", u.ID, db.UserPatch{Disabled: &on}); err != nil {
		t.Fatal(err)
	}
	if code, _ := loginAs(t, s, u.Username, "old-password-1234"); code != 200 {
		t.Fatalf("the password changed although the change was refused: %d", code)
	}
}

func TestUserChangePasswordVersionRace(t *testing.T) {
	s, _ := rbacTestServer(t)
	u := seedUser(t, s, db.RoleOperator, "old-password-1234")
	who := principal{Username: u.Username, Role: u.Role, UserID: u.ID}

	// An admin changes the user after the caller's token was issued (version
	// u.Version) but before the caller's write: the admin's change wins.
	role := db.RoleReviewer
	if _, err := s.m.pg.UpdateUser("test-setup", u.ID, db.UserPatch{Role: &role}); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "/api/auth/change-password", nil)
	w := httptest.NewRecorder()
	s.changeUserPassword(w, r, who, u.Version, "old-password-1234", "new-password-5678")
	if w.Code != 409 {
		t.Fatalf("stale version: %d %s", w.Code, w.Body.String())
	}
	if code, _ := loginAs(t, s, u.Username, "old-password-1234"); code != 200 {
		t.Fatalf("the password changed in a lost race: %d", code)
	}
	got, _ := s.m.pg.GetUser(u.ID)
	if got.Version != u.Version+1 || got.Role != role {
		t.Fatalf("user after the lost race: %+v", got)
	}

	// The db method refuses a stale version and a disabled user the same way.
	if _, err := s.m.pg.ChangeOwnPassword(u.Username, u.ID, u.Version, "x"); err != db.ErrUserChanged {
		t.Fatalf("stale db change: %v", err)
	}
}

func TestConsoleUserChangePasswordNotForbidden(t *testing.T) {
	// The bootstrap path is covered by auth_epoch_test.go and
	// change_password_limit_test.go; this pins that a console user's token is no
	// longer answered with the old 403.
	s, _ := rbacTestServer(t)
	u := seedUser(t, s, db.RoleReviewer, "old-password-1234")
	_, tok := loginAs(t, s, u.Username, "old-password-1234")
	if w := doChangePassword(s, tok, "old-password-1234", "new-password-5678"); w.Code != 200 {
		t.Fatalf("console user change: %d %s", w.Code, w.Body.String())
	}
}

// A re-check inside an open stream serves the user from the cache, the first
// check does not, and every in-process revocation reaches the next re-check.
func TestSessionStillOKUsesCacheAndRevocationInvalidates(t *testing.T) {
	s, _ := rbacTestServer(t)
	u := seedUser(t, s, db.RoleReviewer, "reviewer-pass-1")
	_, tok := loginAs(t, s, u.Username, "reviewer-pass-1")

	if !s.sessionOK(tok) || !s.sessionStillOK(tok) {
		t.Fatal("a fresh token must pass both checks")
	}
	// A change made behind the application's back (no invalidation) is not seen
	// by a re-check until the TTL, while the uncached first check sees it.
	if _, err := s.m.pg.Exec(`UPDATE artex_users SET disabled = true WHERE id = $1`, u.ID); err != nil {
		t.Fatal(err)
	}
	if s.sessionOK(tok) {
		t.Fatal("the first check must read the database")
	}
	if !s.sessionStillOK(tok) {
		t.Fatal("a re-check within the TTL must not query the database")
	}
	time.Sleep(2200 * time.Millisecond)
	if s.sessionStillOK(tok) {
		t.Fatal("a re-check after the TTL must see the out-of-process disable")
	}
	enable := false
	if _, err := s.m.pg.UpdateUser("test-setup", u.ID, db.UserPatch{Disabled: &enable}); err != nil {
		t.Fatal(err)
	}

	// An admin disabling the user through the API takes effect on the next re-check.
	_, tok = loginAs(t, s, u.Username, "reviewer-pass-1")
	if !s.sessionStillOK(tok) { // fills the cache
		t.Fatal("re-check before the disable")
	}
	off := true
	if _, err := s.m.pg.UpdateUser("test-setup", u.ID, db.UserPatch{Disabled: &off}); err != nil {
		t.Fatal(err)
	}
	if s.sessionStillOK(tok) {
		t.Fatal("a re-check right after UpdateUser(disabled) accepted the token")
	}
	on := false
	if _, err := s.m.pg.UpdateUser("test-setup", u.ID, db.UserPatch{Disabled: &on}); err != nil {
		t.Fatal(err)
	}

	// The user's own password change, and a delete, do too.
	_, tok = loginAs(t, s, u.Username, "reviewer-pass-1")
	if !s.sessionStillOK(tok) {
		t.Fatal("re-check before the change")
	}
	if w := doChangePassword(s, tok, "reviewer-pass-1", "reviewer-pass-2"); w.Code != 200 {
		t.Fatalf("change: %d %s", w.Code, w.Body.String())
	}
	if s.sessionStillOK(tok) {
		t.Fatal("a re-check right after ChangeOwnPassword accepted the token")
	}
	_, tok = loginAs(t, s, u.Username, "reviewer-pass-2")
	if !s.sessionStillOK(tok) {
		t.Fatal("re-check before the delete")
	}
	if err := s.m.pg.DeleteUser("test-setup", u.ID); err != nil {
		t.Fatal(err)
	}
	if s.sessionStillOK(tok) {
		t.Fatal("a re-check right after DeleteUser accepted the token")
	}
}
