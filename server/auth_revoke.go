package server

import (
	"log"
	"sync"
	"time"
)

// Logout revokes the presented token server-side: its jti goes into an
// in-memory set every session check consults, and is persisted in the settings
// table (db.RevokeToken) so a restart keeps it revoked. Entries expire with the
// token (jwtTTL at most), so the set stays bounded by the logins of one week.

// revokedKeySuffix follows the password version key, so the test key overrides
// (authVersionKey) isolate revocations too.
const revokedKeySuffix = ":revoked_jti:"

func (s *Server) revokedPrefix() string { return s.passwordVersionKey() + revokedKeySuffix }

// revocations is the in-memory set of revoked jtis with their token expiry.
type revocations struct {
	mu  sync.Mutex
	ids map[string]time.Time
}

func (v *revocations) has(id string) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	_, ok := v.ids[id]
	return ok
}

// add revokes id until exp and drops expired entries.
func (v *revocations) add(id string, exp time.Time) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.ids == nil {
		v.ids = map[string]time.Time{}
	}
	now := time.Now()
	for k, e := range v.ids {
		if now.After(e) {
			delete(v.ids, k)
		}
	}
	v.ids[id] = exp
}

// merge adds the stored set; entries only in memory (a failed persist) stay.
func (v *revocations) merge(stored map[string]time.Time) {
	for id, exp := range stored {
		v.add(id, exp)
	}
}

// loadAuthState reads the revocation list, then the password version. The
// list is merged before the version read can mark the state ready, so a
// recovery from pwdUnreadable never has a moment where a logged-out token
// passes. A failed list read leaves pwdState unreadable, which rejects every
// session until login reads both again: a lost list must not revive tokens.
func (s *Server) loadAuthState() {
	if s.m == nil || s.m.pg == nil {
		s.loadPasswordVersion()
		return
	}
	var stored map[string]time.Time
	var err error
	for attempt := 1; attempt <= 3; attempt++ {
		if stored, err = s.m.pg.RevokedTokens(s.revokedPrefix()); err == nil {
			break
		}
		log.Printf("[auth] revoked-token list attempt %d/3: %v", attempt, err)
		if attempt < 3 {
			time.Sleep(time.Duration(attempt) * 100 * time.Millisecond)
		}
	}
	if err != nil {
		log.Printf("[auth] revoked-token list unreadable; rejecting sessions until login can read it: %v", err)
		s.pwdState.Store(pwdUnreadable)
		return
	}
	s.revoked.merge(stored)
	s.loadPasswordVersion()
}

// session returns the claims of token when it is a live session (sessionOK).
func (s *Server) session(token string) (*sessionClaims, bool) {
	if s.pwdState.Load() == pwdUnreadable {
		return nil, false
	}
	c, ok := sessionToken(token, s.jwtKey, s.pwdVersion.Load())
	if !ok || s.revoked.has(c.ID) {
		return nil, false
	}
	return c, true
}

// revokeSession revokes token when it is validly signed at the current
// password version (revoked already or not, so a retry after a failed persist
// writes it). It returns an error only when the revocation could not be
// persisted; the token is revoked in this process either way.
func (s *Server) revokeSession(token string) error {
	c, ok := sessionToken(token, s.jwtKey, s.pwdVersion.Load())
	if !ok {
		return nil
	}
	exp := time.Now().Add(jwtTTL)
	if c.ExpiresAt != nil {
		exp = c.ExpiresAt.Time
	}
	s.revoked.add(c.ID, exp)
	if s.m == nil || s.m.pg == nil {
		return nil
	}
	return s.m.pg.RevokeToken(s.revokedPrefix(), c.ID, exp)
}
