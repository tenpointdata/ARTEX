package server

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/Autumn-27/artex/db"
)

// Principals and role enforcement. A session token is either the legacy
// bootstrap token (subject "ARTEX", no uid claim), which is the admin and
// behaves exactly as before, or a console-user token carrying uid and uv (the
// user's version). A user token is valid only while that user exists, is not
// disabled and still has version uv; it is looked up on every check and any
// lookup error rejects it.

// legacyUsername is the subject of the bootstrap login's tokens.
const legacyUsername = "ARTEX"

// auditAnonymous is the actor of events with no authenticated caller (failed
// logins): the attempted username is caller-supplied, so it goes only in the payload.
const auditAnonymous = "anonymous"

// principal is who a request acts as.
type principal struct {
	Username string // "ARTEX" for the legacy login
	Role     string
	UserID   int64 // 0 for the legacy login
}

type principalKey struct{}

func withPrincipal(ctx context.Context, p principal) context.Context {
	return context.WithValue(ctx, principalKey{}, p)
}

// principalFrom returns the principal requireAuth resolved for the request.
func principalFrom(ctx context.Context) (principal, bool) {
	p, ok := ctx.Value(principalKey{}).(principal)
	return p, ok
}

// parseSession verifies the token's signature and expiry and returns its claims.
func parseSession(tokenStr string, key []byte) (*sessionClaims, bool) {
	claims := &sessionClaims{}
	t, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return key, nil
	})
	if err != nil || !t.Valid {
		return nil, false
	}
	return claims, true
}

// sessionPrincipal validates token and returns who it acts as. It reads the
// users table directly: the first check of every request.
func (s *Server) sessionPrincipal(token string) (principal, bool) {
	return s.principalFor(token, false)
}

// principalFor is sessionPrincipal with the user lookup optionally served from
// the short-lived user cache (cached), for re-checking a session that already
// passed a direct check, such as each event of an open SSE stream.
func (s *Server) principalFor(token string, cached bool) (principal, bool) {
	if token == "" {
		return principal{}, false
	}
	claims, ok := parseSession(token, s.jwtKey)
	if !ok {
		return principal{}, false
	}
	if claims.UID == 0 {
		return s.legacyPrincipal(claims)
	}
	return s.userPrincipal(claims, cached)
}

// legacyPrincipal applies the bootstrap login's password-version check.
func (s *Server) legacyPrincipal(c *sessionClaims) (principal, bool) {
	if c.Subject != legacyUsername || s.pwdState.Load() == pwdUnreadable {
		return principal{}, false
	}
	if v := s.pwdVersion.Load(); v > 0 && c.Pwd != v {
		return principal{}, false
	}
	return principal{Username: legacyUsername, Role: db.RoleAdmin}, true
}

// userPrincipal checks a user token against the users table. Fails closed.
func (s *Server) userPrincipal(c *sessionClaims, cached bool) (principal, bool) {
	if s.m == nil || s.m.pg == nil {
		return principal{}, false
	}
	get := s.m.pg.GetUser
	if cached {
		get = s.m.pg.CachedUser
	}
	u, err := get(c.UID)
	if err != nil {
		if !errors.Is(err, db.ErrUserNotFound) {
			log.Printf("[auth] user %d lookup failed; rejecting the session: %v", c.UID, err)
		}
		return principal{}, false
	}
	if u.Disabled || u.Version != c.UV || u.Username != c.Subject {
		return principal{}, false
	}
	return principal{Username: u.Username, Role: u.Role, UserID: u.ID}, true
}

// signUserJWT issues a session token for a console user at its current version.
func signUserJWT(key []byte, u db.User) (string, error) {
	return jwt.NewWithClaims(jwt.SigningMethodHS256, sessionClaims{
		UID: u.ID,
		UV:  u.Version,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   u.Username,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(jwtTTL)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}).SignedString(key)
}

// authorizeRole enforces rbacPolicy for an authenticated request. h is the
// handler requireAuth wraps: when it is the API mux, the policy is matched
// against the route pattern the mux will serve. It answers 403 and returns
// false on refusal.
func (s *Server) authorizeRole(w http.ResponseWriter, r *http.Request, h http.Handler, p principal) bool {
	route := r.URL.Path
	if mux, ok := h.(*http.ServeMux); ok {
		if _, pattern := mux.Handler(r); pattern != "" {
			route = routePath(pattern)
		}
	}
	need := requiredPermission(r.Method, route)
	if roleHas(p.Role, need) {
		return true
	}
	s.appendAuditBestEffort(p.Username, db.AuditRoleDenied, r.URL.Path, map[string]string{
		"method": r.Method, "path": r.URL.Path, "role": p.Role, "required": string(need),
	})
	writeErr(w, 403, fmt.Sprintf("forbidden: role %q does not have the %q permission required for %s %s", p.Role, need, r.Method, r.URL.Path))
	return false
}

// appendAuditBestEffort appends an application event to the audit chain and
// logs a failure; the caller's decision does not depend on it.
func (s *Server) appendAuditBestEffort(actor, kind, subject string, payload any) {
	if s.m == nil || s.m.pg == nil {
		return
	}
	if _, err := s.m.pg.AppendAudit(actor, kind, subject, payload); err != nil {
		log.Printf("[audit] append %s for %q failed: %v", kind, actor, err)
	}
}

// permissionsOf lists role's permissions as strings, for the UI.
func permissionsOf(role string) []string {
	out := []string{}
	for _, p := range rolePermissions[role] {
		out = append(out, string(p))
	}
	return out
}

// dummyHash is compared when the username is unknown, so a miss costs as much
// as a wrong password.
var dummyHash = sync.OnceValue(func() []byte {
	h, err := bcrypt.GenerateFromPassword([]byte("artex-timing-equalizer"), bcrypt.DefaultCost)
	if err != nil {
		log.Printf("[auth] dummy hash: %v", err)
	}
	return h
})

// auditUsername bounds an attempted username before it goes into the chain.
func auditUsername(u string) string {
	u = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, strings.ToLower(strings.TrimSpace(u)))
	if len(u) > 64 {
		u = u[:64]
	}
	return u
}

// userLogin is login for a console user (any username but ARTEX). The caller
// has already counted the attempt against the client IP.
func (s *Server) userLogin(w http.ResponseWriter, r *http.Request, ip, username, password string) {
	name := auditUsername(username)
	fail := func() {
		s.appendAuditBestEffort(auditAnonymous, db.AuditLoginFailed, name, map[string]string{"username": name, "ip": ip})
		writeErr(w, 401, "Incorrect username or password")
	}
	if s.m == nil || s.m.pg == nil {
		fail()
		return
	}
	a, err := s.m.pg.GetUserAuth(name)
	if errors.Is(err, db.ErrUserNotFound) {
		_ = bcrypt.CompareHashAndPassword(dummyHash(), []byte(password))
		fail()
		return
	}
	if err != nil {
		log.Printf("[auth] user lookup failed: %v", err)
		writeErr(w, 503, errDataSourceUnavailable)
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(a.PasswordHash), []byte(password)) != nil || a.Disabled {
		fail()
		return
	}
	tok, err := signUserJWT(s.jwtKey, a.User)
	if err != nil {
		writeErr(w, 500, "token generation failed")
		return
	}
	s.loginLimit.reset(ip)
	s.appendAuditBestEffort(a.Username, db.AuditLoginOK, a.Username, map[string]string{"username": a.Username, "ip": ip})
	s.setSession(w, r, tok)
	writeJSON(w, 200, map[string]any{"token": tok})
}
