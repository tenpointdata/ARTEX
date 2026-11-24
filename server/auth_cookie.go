package server

import (
	"errors"
	"log"
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

// POST /api/auth/logout ends the session. The browser is told to drop the
// session cookie (both names, whichever it holds), and when the request carries
// a valid session the server revokes it: the session epoch (the counter behind
// the token's pwd claim, also bumped by a password change) moves on, so every
// token issued before now is rejected, copies included. The console has one
// account, so this signs out every browser and bearer client ("log out
// everywhere"). A cookie session can revoke only from the console's own origin,
// so a foreign page cannot sign the operator out. A request without a valid
// session only clears cookies.
func (s *Server) authLogout(w http.ResponseWriter, r *http.Request) {
	for _, name := range []string{sessionCookieSecure, sessionCookiePlain} {
		http.SetCookie(w, &http.Cookie{
			Name: name, Value: "", Path: "/", MaxAge: -1,
			HttpOnly: true, Secure: name == sessionCookieSecure, SameSite: http.SameSiteStrictMode,
		})
	}
	tok := extractToken(r)
	if tok == "" || !s.sessionOK(tok) || (cookieAuthenticated(r) && !sameOrigin(r)) {
		writeJSON(w, 200, map[string]any{"ok": true, "revoked": false})
		return
	}
	if err := s.revokeSessions(); err != nil {
		log.Printf("[auth] logout could not revoke the session: %v", err)
		writeErr(w, 503, "Signed out in this browser, but the session could not be revoked: "+errDataSourceUnavailable)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "revoked": true})
}

// revokeSessions advances the session epoch so every token issued so far is
// rejected. The epoch lives in PostgreSQL so it survives a restart; without a
// database there is nothing to record it in.
func (s *Server) revokeSessions() error {
	if s.m == nil || s.m.pg == nil {
		return errNoSessionStore
	}
	n, err := s.m.pg.IncrementCounter(s.passwordVersionKey())
	if err != nil {
		return err
	}
	s.applyPasswordVersion(n)
	return nil
}

var errNoSessionStore = errors.New("no database to record the revocation")

// GET /api/auth/session says whether the request carries a valid session. It
// answers 200 either way so the UI can ask without triggering its 401 redirect.
func (s *Server) authSession(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"authenticated": s.sessionOK(extractToken(r))})
}
