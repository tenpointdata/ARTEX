package traffic

import (
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"io"
	"log"
	"net"
	"os"

	"github.com/Autumn-27/artex/egress"
)

// SOCKS5 authentication policy (RFC 1929 username/password and RFC 1928 no-auth).
//
// A client is admitted in exactly one of three ways:
//
//  1. Scope token: the password is a live task scope token (egress.ScopeToken,
//     minted by Traffic.ScopedProxyURL here, or resolved by the remote scope
//     resolver for a distributed worker). The connection runs on that scope's
//     rules. A token-shaped password neither holds (an ended run, a wrong
//     worker token) is refused, never downgraded to the global rules.
//  2. Operator credential: the password equals the configured operator
//     password (ARTEX_SOCKS5_PASSWORD; the user defaults to "artex" and is
//     ARTEX_SOCKS5_USER). The connection runs on the global rules. The pace
//     and scope marker users (egress.PaceUser, egress.ScopeUser) are accepted
//     as the username too, so a client can ask for pacing; they are not
//     secrets, the password is.
//  3. Open loopback mode: no operator credential is configured AND the
//     listener is bound to a loopback address. This keeps the pre-audit
//     behaviour for a developer's local listener: a loopback peer may use
//     no-auth (0x00) or the egress.PaceUser marker as its credential, and runs
//     on the global rules. Any other username/password is refused. The bind
//     address is the boundary here; the executor uid's egress filter and the
//     target gate still apply to every dial.
//
// Anything else gets RFC 1929 status 0x01 (or the 0xFF method refusal for
// no-auth). In particular a listener bound beyond loopback with no operator
// credential admits scope-token clients only, and once an operator credential
// is set, no-auth is refused even from loopback (a co-resident process could
// otherwise reach the global rules without it). Secrets are compared in
// constant time; the scope-token lookup is a map read of a 128-bit random
// value, which is not a practical timing oracle.

const socks5UserPassFail = 0x01

// DefaultSOCKS5User is the operator username when only a password is configured.
const DefaultSOCKS5User = "artex"

// socks5OperatorCred fills cfg's operator credential from the environment.
func socks5OperatorCred(cfg SOCKS5Config) SOCKS5Config {
	user := os.Getenv(SOCKS5UserEnv)
	pass := os.Getenv(SOCKS5PasswordEnv)
	if pass == "" {
		if user != "" {
			log.Printf("[traffic/socks5] %s is set without %s; no operator credential configured", SOCKS5UserEnv, SOCKS5PasswordEnv)
		}
		return cfg
	}
	if user == "" {
		user = DefaultSOCKS5User
	}
	cfg.Username, cfg.Password = user, pass
	return cfg
}

// validateAuth rejects a credential RFC 1929 cannot carry (fields are one-byte
// length prefixed) or a username with no password.
func (c SOCKS5Config) validateAuth() error {
	if c.Password == "" {
		if c.Username != "" {
			return errors.New("socks5: a username needs a password")
		}
		return nil
	}
	if len(c.Username) > 255 || len(c.Password) > 255 || c.Username == "" {
		return errors.New("socks5: operator credential must be a 1-255 byte username and a 1-255 byte password")
	}
	return nil
}

func isLoopbackBound(addr net.Addr) bool {
	t, ok := addr.(*net.TCPAddr)
	return ok && t.IP != nil && t.IP.IsLoopback()
}

func logSOCKS5AuthMode(addr net.Addr, cfg SOCKS5Config, open bool) {
	switch {
	case cfg.Password != "":
		log.Printf("[traffic/socks5] %s requires the operator credential (or a task scope token)", addr)
	case open:
		log.Printf("[traffic/socks5] %s is loopback-only with no operator credential: no-auth from loopback peers; set %s to require one", addr, SOCKS5PasswordEnv)
	default:
		log.Printf("[traffic/socks5] WARNING %s is not loopback-bound and %s is unset: only clients carrying a task scope token are admitted", addr, SOCKS5PasswordEnv)
	}
}

// secretEqual compares in constant time, whatever the lengths.
func secretEqual(a, b string) bool {
	ha, hb := sha256.Sum256([]byte(a)), sha256.Sum256([]byte(b))
	return subtle.ConstantTimeCompare(ha[:], hb[:]) == 1
}

// authUserPass runs the RFC 1929 subnegotiation: parse, decide, answer.
func (s *socks5Server) authUserPass(conn net.Conn) (cred socks5Cred, ok bool) {
	user, pass, ok := readSOCKS5UserPass(conn)
	if !ok {
		return socks5Cred{}, false
	}
	cred, ok = s.checkCredential(user, pass)
	status := byte(0x00)
	if !ok {
		status = socks5UserPassFail
	}
	if _, err := conn.Write([]byte{socks5UserPassVer, status}); err != nil {
		return socks5Cred{}, false
	}
	return cred, ok
}

// checkCredential applies the policy above to a presented username/password.
func (s *socks5Server) checkCredential(user, pass string) (socks5Cred, bool) {
	paced := user == egress.PaceUser
	if tok, isTok := egress.ScopeToken(pass); isTok && s.t != nil {
		// Local registry first, then the remote resolver (scope_remote.go):
		// a distributed worker's token is minted by another node.
		if _, live := s.t.resolveScope(tok); live {
			return socks5Cred{paced: paced, token: tok}, true
		}
	}
	if s.cfg.Password != "" {
		userOK := secretEqual(user, s.cfg.Username) || paced || user == egress.ScopeUser
		passOK := secretEqual(pass, s.cfg.Password)
		return socks5Cred{paced: paced}, userOK && passOK
	}
	if s.allowNoAuth && paced && (pass == "" || pass == egress.PaceUser) {
		return socks5Cred{paced: true}, true
	}
	return socks5Cred{}, false
}

// readSOCKS5UserPass parses an RFC 1929 username/password request.
func readSOCKS5UserPass(conn net.Conn) (user, pass string, ok bool) {
	var head [2]byte
	if _, err := io.ReadFull(conn, head[:]); err != nil {
		return "", "", false
	}
	if head[0] != socks5UserPassVer {
		_, _ = conn.Write([]byte{socks5UserPassVer, socks5UserPassFail})
		return "", "", false
	}
	uname := make([]byte, int(head[1]))
	if _, err := io.ReadFull(conn, uname); err != nil {
		return "", "", false
	}
	var plen [1]byte
	if _, err := io.ReadFull(conn, plen[:]); err != nil {
		return "", "", false
	}
	pw := make([]byte, int(plen[0]))
	if plen[0] > 0 {
		if _, err := io.ReadFull(conn, pw); err != nil {
			return "", "", false
		}
	}
	return string(uname), string(pw), true
}
