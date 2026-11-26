package egress

import (
	"encoding/base64"
	"net/url"
	"strings"
)

// A proxy credential is the userinfo a client puts in its proxy URL and sends
// as Proxy-Authorization: Basic <user>:<password>. The recording proxy takes
// no real credentials; the userinfo only carries two platform markers:
//
//   - the user names the pacing: PaceUser paces the connection (pace.go),
//     ScopeUser does not;
//   - a password with ScopeTokenPrefix names a scope (a task) whose own rules
//     the proxy applies to the connection's upstream dials. A password without
//     the prefix names no scope: only the global rules apply.
//
// A scope token is not an authorization: it can only add a scope's rules to
// the global ones, never lift them, so a client that drops it gets the global
// rules, and one that names an unknown scope is refused (fail closed).

// ScopeUser is the user of a scoped proxy URL whose connections are not paced.
const ScopeUser = "artex-scope"

// ScopeTokenPrefix starts every scope token.
const ScopeTokenPrefix = "scope-"

// ProxyCredential decodes a Proxy-Authorization value. ok is false when it is
// absent or not Basic credentials.
func ProxyCredential(proxyAuthorization string) (user, password string, ok bool) {
	scheme, enc, found := strings.Cut(strings.TrimSpace(proxyAuthorization), " ")
	if !found || !strings.EqualFold(scheme, "Basic") {
		return "", "", false
	}
	dec, err := base64.StdEncoding.DecodeString(strings.TrimSpace(enc))
	if err != nil {
		return "", "", false
	}
	user, password, _ = strings.Cut(string(dec), ":")
	return user, password, true
}

// ScopeToken returns the scope token a credential password carries, if any.
func ScopeToken(password string) (string, bool) {
	if strings.HasPrefix(password, ScopeTokenPrefix) && len(password) > len(ScopeTokenPrefix) {
		return password, true
	}
	return "", false
}

// ScopedProxyURL returns raw with the userinfo ScopeUser:token. raw is returned
// unchanged when it does not parse, already carries a userinfo, or token is empty.
func ScopedProxyURL(raw, token string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil || token == "" {
		return raw
	}
	u.User = url.UserPassword(ScopeUser, token)
	return u.String()
}

// WithoutProxyCredential returns raw without its userinfo, for display (a
// prompt, a log line). raw is returned unchanged when it does not parse.
func WithoutProxyCredential(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.User == nil {
		return raw
	}
	u.User = nil
	return u.String()
}
