package server

import (
	"net/http"
	"net/netip"
	"strings"
)

// The session cookie. Over TLS it carries the __Host- prefix, which makes the
// browser refuse it unless it is Secure, Path=/ and has no Domain. Plain HTTP
// cannot use that prefix, so it keeps the bare name.
//
// Cookies are not isolated by port: a browser sends them to every port of the
// same host, so a lab target on localhost:8080 receives the cookie of the
// console on localhost:8787. Serve the console from its own hostname
// (e.g. artex.localhost); see the README.
const (
	sessionCookiePlain  = "artex_token"
	sessionCookieSecure = "__Host-artex_token"
)

// requestIsSecure reports whether the browser reached us over TLS, directly or
// through a trusted proxy that says so.
func (s *Server) requestIsSecure(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	if !strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https") {
		return false
	}
	addr, err := netip.ParseAddr(peerIP(r))
	return err == nil && isTrusted(addr, s.trustedProxies)
}

// sessionCookie builds the session cookie for token.
func (s *Server) sessionCookie(r *http.Request, token string) *http.Cookie {
	secure := s.requestIsSecure(r)
	c := &http.Cookie{
		Name:     sessionCookiePlain,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int(jwtTTL.Seconds()),
	}
	if secure {
		c.Name = sessionCookieSecure
	}
	return c
}

// setSession writes the session cookie for token.
func (s *Server) setSession(w http.ResponseWriter, r *http.Request, token string) {
	http.SetCookie(w, s.sessionCookie(r, token))
}

// sessionCookieValue returns the session token in the request's cookies.
func sessionCookieValue(r *http.Request) string {
	for _, name := range []string{sessionCookieSecure, sessionCookiePlain} {
		if c, err := r.Cookie(name); err == nil && c.Value != "" {
			return c.Value
		}
	}
	return ""
}

// POST /api/auth/logout clears the session cookie (both names, whichever the
// browser holds). Bearer clients have nothing to clear.
func (s *Server) authLogout(w http.ResponseWriter, r *http.Request) {
	for _, name := range []string{sessionCookieSecure, sessionCookiePlain} {
		http.SetCookie(w, &http.Cookie{
			Name: name, Value: "", Path: "/", MaxAge: -1,
			HttpOnly: true, Secure: name == sessionCookieSecure, SameSite: http.SameSiteStrictMode,
		})
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// GET /api/auth/session says whether the request carries a valid session. It
// answers 200 either way so the UI can ask without triggering its 401 redirect.
// A valid session also reports its username, role and permissions, so the UI
// can hide what the role cannot do (the API enforces it regardless).
func (s *Server) authSession(w http.ResponseWriter, r *http.Request) {
	p, ok := s.sessionPrincipal(extractToken(r))
	if !ok {
		writeJSON(w, 200, map[string]any{"authenticated": false})
		return
	}
	writeJSON(w, 200, map[string]any{
		"authenticated": true, "username": p.Username, "role": p.Role, "permissions": permissionsOf(p.Role),
	})
}
