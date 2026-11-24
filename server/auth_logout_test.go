package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Autumn-27/artex/db"
)

// newEpochServer is a Server on PostgreSQL with private password/epoch keys,
// initialised with password correct-horse-1.
func newEpochServer(t *testing.T) *Server {
	t.Helper()
	m, err := NewManager(t.TempDir(), "")
	if err != nil {
		db.SkipPGUnavailable(t, err)
	}
	t.Cleanup(func() { _ = m.Close() })
	stamp := time.Now().UnixNano()
	s := &Server{
		m:              m,
		jwtKey:         []byte("0123456789abcdef0123456789abcdef"),
		loginLimit:     newLoginLimiter(time.Now),
		pwdLimit:       newLoginLimiter(time.Now),
		authHashKey:    fmt.Sprintf("test.password_hash.%d", stamp),
		authVersionKey: fmt.Sprintf("test.password_version.%d", stamp),
		setupToken:     "setup-ok",
	}
	t.Cleanup(func() {
		_, _ = m.pg.Exec(`DELETE FROM settings WHERE key=$1 OR key=$2`, s.authHashKey, s.authVersionKey)
	})
	w := httptest.NewRecorder()
	s.authInit(w, httptest.NewRequest("POST", "/api/auth/init", strings.NewReader(`{"password":"correct-horse-1","setup_token":"setup-ok"}`)))
	if w.Code != 200 {
		t.Fatalf("init status=%d body=%s", w.Code, w.Body.String())
	}
	return s
}

func loginToken(t *testing.T, s *Server) string {
	t.Helper()
	w := httptest.NewRecorder()
	s.authLogin(w, httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(`{"username":"ARTEX","password":"correct-horse-1"}`)))
	if w.Code != 200 {
		t.Fatalf("login status=%d body=%s", w.Code, w.Body.String())
	}
	var out struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil || out.Token == "" {
		t.Fatalf("login token: %v body=%s", err, w.Body.String())
	}
	return out.Token
}

// protectedStatus is the status requireAuth gives a cookie replay of tok.
func protectedStatus(s *Server, tok string) int {
	r := httptest.NewRequest("GET", "/api/tasks", nil)
	r.AddCookie(&http.Cookie{Name: sessionCookiePlain, Value: tok})
	w := httptest.NewRecorder()
	s.requireAuth(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	})).ServeHTTP(w, r)
	return w.Code
}

func logoutWithCookie(s *Server, tok, origin string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("POST", "http://artex.localhost/api/auth/logout", nil)
	r.AddCookie(&http.Cookie{Name: sessionCookiePlain, Value: tok})
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	w := httptest.NewRecorder()
	s.authLogout(w, r)
	return w
}

func TestLogoutRevokesCopiedToken(t *testing.T) {
	s := newEpochServer(t)
	tok := loginToken(t, s)
	other := loginToken(t, s) // a second browser or a bearer client
	if protectedStatus(s, tok) != 200 {
		t.Fatal("fresh session rejected")
	}

	w := logoutWithCookie(s, tok, "http://artex.localhost")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"revoked":true`) {
		t.Fatalf("logout status=%d body=%s", w.Code, w.Body.String())
	}
	if c := findCookie(w, sessionCookiePlain); c == nil || c.MaxAge >= 0 {
		t.Fatalf("cookie not cleared: %+v", c)
	}
	if got := protectedStatus(s, tok); got != 401 {
		t.Fatalf("replayed token after logout: status %d, want 401", got)
	}
	if got := protectedStatus(s, other); got != 401 {
		t.Fatalf("other session after logout: status %d, want 401 (log out everywhere)", got)
	}

	// The revocation is persisted: a restarted process rejects the token too.
	restarted := &Server{m: s.m, jwtKey: s.jwtKey, authHashKey: s.authHashKey, authVersionKey: s.authVersionKey}
	restarted.loadPasswordVersion()
	if restarted.sessionOK(tok) {
		t.Fatal("revoked token accepted after a restart")
	}

	fresh := loginToken(t, s)
	if protectedStatus(s, fresh) != 200 {
		t.Fatal("login after logout issued a rejected token")
	}
	// Logging out with an already revoked token revokes nothing more.
	if w := logoutWithCookie(s, tok, "http://artex.localhost"); !strings.Contains(w.Body.String(), `"revoked":false`) {
		t.Fatalf("stale-token logout revoked: %s", w.Body.String())
	}
	if protectedStatus(s, fresh) != 200 {
		t.Fatal("a stale-token logout revoked the current session")
	}
}

func TestLogoutWithBearerRevokes(t *testing.T) {
	s := newEpochServer(t)
	tok := loginToken(t, s)
	w := httptest.NewRecorder()
	s.authLogout(w, requestWithToken("POST", "/api/auth/logout", tok))
	if w.Code != 200 || s.sessionOK(tok) {
		t.Fatalf("bearer logout status=%d body=%s; token still valid=%v", w.Code, w.Body.String(), s.sessionOK(tok))
	}
}

func TestCrossOriginLogoutDoesNotRevoke(t *testing.T) {
	s := newEpochServer(t)
	tok := loginToken(t, s)
	w := logoutWithCookie(s, tok, "http://evil.example")
	if !strings.Contains(w.Body.String(), `"revoked":false`) {
		t.Fatalf("cross-origin logout body=%s", w.Body.String())
	}
	if protectedStatus(s, tok) != 200 {
		t.Fatal("a foreign page revoked the operator's session")
	}
	w = httptest.NewRecorder()
	s.authLogout(w, httptest.NewRequest("POST", "/api/auth/logout", nil))
	if protectedStatus(s, tok) != 200 {
		t.Fatal("an anonymous logout revoked the session")
	}
}

func TestLogoutReportsFailedRevocation(t *testing.T) {
	s := newEpochServer(t)
	tok := loginToken(t, s)
	if _, err := s.m.pg.Exec(`UPDATE settings SET value='not-a-number' WHERE key=$1`, s.authVersionKey); err != nil {
		t.Fatal(err)
	}
	w := logoutWithCookie(s, tok, "http://artex.localhost")
	if w.Code != 503 {
		t.Fatalf("failed revocation status=%d want 503 body=%s", w.Code, w.Body.String())
	}
	if c := findCookie(w, sessionCookiePlain); c == nil || c.MaxAge >= 0 {
		t.Fatal("cookie must be cleared even when revocation fails")
	}
}

// Setting the password again after the hash was removed (a database reset of
// that row, say) must not restart the epoch and revive older tokens.
func TestResetupDoesNotReviveRevokedTokens(t *testing.T) {
	s := newEpochServer(t)
	tok := loginToken(t, s)
	logoutWithCookie(s, tok, "http://artex.localhost")
	if _, err := s.m.pg.Exec(`DELETE FROM settings WHERE key=$1`, s.authHashKey); err != nil {
		t.Fatal(err)
	}
	s.setupToken = "setup-again"
	w := httptest.NewRecorder()
	s.authInit(w, httptest.NewRequest("POST", "/api/auth/init", strings.NewReader(`{"password":"correct-horse-9","setup_token":"setup-again"}`)))
	if w.Code != 200 {
		t.Fatalf("re-setup status=%d body=%s", w.Code, w.Body.String())
	}
	if s.sessionOK(tok) {
		t.Fatal("re-setup revived a token revoked at logout")
	}
}
