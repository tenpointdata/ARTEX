package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/Autumn-27/artex/db"
)

var revokeTestKey = []byte("0123456789abcdef0123456789abcdef")

func TestLogoutRevokesPresentedToken(t *testing.T) {
	s := &Server{jwtKey: revokeTestKey}
	tok, _ := signJWT(s.jwtKey)
	other, _ := signJWT(s.jwtKey)
	if !s.sessionOK(tok) || !s.sessionOK(other) {
		t.Fatal("fresh tokens rejected")
	}
	w := httptest.NewRecorder()
	s.authLogout(w, requestWithToken("POST", "/api/auth/logout", tok))
	if w.Code != 200 {
		t.Fatalf("logout status %d", w.Code)
	}
	if s.sessionOK(tok) {
		t.Fatal("a logged-out token is still a session")
	}
	if !s.sessionOK(other) {
		t.Fatal("logout revoked a different session")
	}
}

// A request carrying a bearer token and a cookie ends both sessions.
func TestLogoutRevokesEveryPresentedToken(t *testing.T) {
	s := &Server{jwtKey: revokeTestKey}
	bearer, _ := signJWT(s.jwtKey)
	cookie, _ := signJWT(s.jwtKey)
	r := requestWithToken("POST", "/api/auth/logout", bearer)
	r.AddCookie(&http.Cookie{Name: sessionCookiePlain, Value: cookie})
	w := httptest.NewRecorder()
	s.authLogout(w, r)
	if w.Code != 200 {
		t.Fatalf("logout status %d", w.Code)
	}
	if s.sessionOK(bearer) || s.sessionOK(cookie) {
		t.Fatalf("after logout bearer=%v cookie=%v, want both revoked", s.sessionOK(bearer), s.sessionOK(cookie))
	}
}

func TestTokenWithoutJTIRejected(t *testing.T) {
	legacy, err := jwt.NewWithClaims(jwt.SigningMethodHS256, sessionClaims{RegisteredClaims: jwt.RegisteredClaims{
		Subject: "ARTEX", ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	}}).SignedString(revokeTestKey)
	if err != nil {
		t.Fatal(err)
	}
	if verifyJWT(legacy, revokeTestKey, 0) {
		t.Fatal("a token without a jti cannot be revoked and must be refused")
	}
}

func TestVersionZeroAcceptsOnlyVersionZero(t *testing.T) {
	later, _ := signJWTVersion(revokeTestKey, 3)
	if verifyJWT(later, revokeTestKey, 0) {
		t.Fatal("version 0 accepted a token signed at a later version")
	}
	a, _ := signJWTVersion(revokeTestKey, 3)
	b, _ := signJWTVersion(revokeTestKey, 3)
	if a == b {
		t.Fatal("two tokens share a jti")
	}
}

// pgAuthServer is a Server on PostgreSQL with settings keys of its own.
func pgAuthServer(t *testing.T) (*Server, func() *Server) {
	t.Helper()
	m, err := NewManager(t.TempDir(), "")
	if err != nil {
		db.SkipPGUnavailable(t, err)
	}
	t.Cleanup(func() { _ = m.Close() })
	stamp := time.Now().UnixNano()
	mk := func() *Server {
		return &Server{m: m, jwtKey: revokeTestKey, loginLimit: newLoginLimiter(time.Now), pwdLimit: newLoginLimiter(time.Now),
			authHashKey: fmt.Sprintf("test.password_hash.%d", stamp), authVersionKey: fmt.Sprintf("test.password_version.%d", stamp)}
	}
	s := mk()
	t.Cleanup(func() {
		_, _ = m.pg.Exec(`DELETE FROM settings WHERE key=$1 OR starts_with(key, $2)`, s.authHashKey, s.authVersionKey)
	})
	return s, mk
}

func authToken(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var out struct {
		Token string `json:"token"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &out) != nil || out.Token == "" {
		t.Fatalf("status=%d body=%s, want a token", w.Code, w.Body.String())
	}
	return out.Token
}

func setupPassword(t *testing.T, s *Server, pw string) string {
	t.Helper()
	s.setupToken = "setup-ok"
	w := httptest.NewRecorder()
	s.authInit(w, httptest.NewRequest("POST", "/api/auth/init", strings.NewReader(fmt.Sprintf(`{"password":%q,"setup_token":"setup-ok"}`, pw))))
	return authToken(t, w)
}

func TestLogoutRevocationSurvivesRestartPG(t *testing.T) {
	s, restart := pgAuthServer(t)
	tok := setupPassword(t, s, "correct-horse-1")
	w := httptest.NewRecorder()
	s.authLogin(w, httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(`{"username":"ARTEX","password":"correct-horse-1"}`)))
	kept := authToken(t, w)

	w = httptest.NewRecorder()
	s.authLogout(w, requestWithToken("POST", "/api/auth/logout", tok))
	if w.Code != 200 || s.sessionOK(tok) {
		t.Fatalf("logout status=%d, token still valid=%v", w.Code, s.sessionOK(tok))
	}
	s2 := restart()
	s2.loadAuthState()
	if s2.sessionOK(tok) {
		t.Fatal("the revocation was lost across a restart")
	}
	if !s2.sessionOK(kept) {
		t.Fatal("a session that never logged out was rejected after the restart")
	}

	// Expired revocations are pruned.
	if err := s.m.pg.RevokeToken(s.revokedPrefix(), "stale", time.Now().Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	ids, err := s.m.pg.RevokedTokens(s.revokedPrefix())
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := ids["stale"]; ok || len(ids) != 1 {
		t.Fatalf("revoked ids = %v, want only the logged-out token", ids)
	}
}

// First-run setup after an operator reset must never hand out a version an
// earlier token carries, whether only the hash row or both rows were removed.
func TestSetupAfterResetKeepsVersionMonotonicPG(t *testing.T) {
	s, _ := pgAuthServer(t)
	first := setupPassword(t, s, "correct-horse-1")
	v1 := s.pwdVersion.Load()
	if v1 < time.Now().Add(-time.Hour).UnixMilli() {
		t.Fatalf("first version %d is not clock-floored", v1)
	}
	if _, err := s.m.pg.Exec(`DELETE FROM settings WHERE key=$1`, s.authHashKey); err != nil {
		t.Fatal(err)
	}
	second := setupPassword(t, s, "correct-horse-2")
	v2 := s.pwdVersion.Load()
	if v2 <= v1 || s.sessionOK(first) || !s.sessionOK(second) {
		t.Fatalf("after a hash reset: v1=%d v2=%d first valid=%v", v1, v2, s.sessionOK(first))
	}
	if _, err := s.m.pg.Exec(`DELETE FROM settings WHERE key=$1 OR key=$2`, s.authHashKey, s.authVersionKey); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond) // the floor is the clock in ms; v2 may be the same ms
	setupPassword(t, s, "correct-horse-3")
	if v3 := s.pwdVersion.Load(); v3 < v2 || s.sessionOK(second) {
		t.Fatalf("after a full reset: v2=%d v3=%d second valid=%v", v2, v3, s.sessionOK(second))
	}
}
