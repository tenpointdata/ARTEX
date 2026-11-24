package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func findCookie(w *httptest.ResponseRecorder, name string) *http.Cookie {
	for _, c := range w.Result().Cookies() {
		if c.Name == name {
			return c
		}
	}
	return nil
}

func TestLoginSetsHardenedSessionCookie(t *testing.T) {
	_, post := loginServer(t, newFakeClock())
	w := post("192.0.2.1:1", oldPassword, nil)
	if w.Code != 200 {
		t.Fatalf("login status %d", w.Code)
	}
	c := findCookie(w, sessionCookiePlain)
	if c == nil {
		t.Fatal("no session cookie on plain HTTP login")
	}
	if !c.HttpOnly || c.SameSite != http.SameSiteStrictMode || c.Path != "/" || c.Secure || c.MaxAge <= 0 {
		t.Fatalf("plain cookie attributes wrong: %+v", c)
	}
}

func TestSessionCookieSecureUsesHostPrefix(t *testing.T) {
	s := &Server{}
	r := httptest.NewRequest("POST", "https://example.test/api/auth/login", nil)
	c := s.sessionCookie(r, "tok") // httptest sets r.TLS for https URLs
	if c.Name != sessionCookieSecure || !c.Secure || !c.HttpOnly || c.Path != "/" || c.Domain != "" {
		t.Fatalf("secure cookie must satisfy __Host- rules: %+v", c)
	}
}

func TestForwardedProtoOnlyFromTrustedProxy(t *testing.T) {
	s := &Server{}
	r := httptest.NewRequest("POST", "/api/auth/login", nil)
	r.RemoteAddr = "192.0.2.9:5"
	r.Header.Set("X-Forwarded-Proto", "https")
	if s.requestIsSecure(r) {
		t.Fatal("X-Forwarded-Proto from an untrusted peer must be ignored")
	}
}

func TestExtractTokenReadsBothCookieNames(t *testing.T) {
	for _, name := range []string{sessionCookiePlain, sessionCookieSecure} {
		r := httptest.NewRequest("GET", "/api/x", nil)
		r.AddCookie(&http.Cookie{Name: name, Value: "abc"})
		if got := extractToken(r); got != "abc" || !cookieAuthenticated(r) {
			t.Errorf("%s: token=%q", name, got)
		}
	}
}

func TestLogoutClearsBothCookies(t *testing.T) {
	s := &Server{}
	w := httptest.NewRecorder()
	s.authLogout(w, httptest.NewRequest("POST", "/api/auth/logout", nil))
	for _, name := range []string{sessionCookiePlain, sessionCookieSecure} {
		c := findCookie(w, name)
		if c == nil || c.MaxAge >= 0 || c.Value != "" || !c.HttpOnly {
			t.Errorf("%s not cleared: %+v", name, c)
		}
	}
}

func TestSessionEndpointReportsCookieSession(t *testing.T) {
	s := &Server{jwtKey: []byte("0123456789abcdef0123456789abcdef")}
	tok, err := signJWT(s.jwtKey)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("GET", "/api/auth/session", nil)
	r.AddCookie(&http.Cookie{Name: sessionCookiePlain, Value: tok})
	w := httptest.NewRecorder()
	s.authSession(w, r)
	// The legacy login reports itself as the admin (rbac.go).
	want := "{\"authenticated\":true,\"permissions\":[\"client_read\",\"read\",\"review\",\"operate\",\"admin\"],\"role\":\"admin\",\"username\":\"ARTEX\"}\n"
	if w.Code != 200 || w.Body.String() != want {
		t.Fatalf("got %d %q", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	s.authSession(w, httptest.NewRequest("GET", "/api/auth/session", nil))
	if w.Body.String() != "{\"authenticated\":false}\n" {
		t.Fatalf("anonymous got %q", w.Body.String())
	}
}
