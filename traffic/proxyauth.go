package traffic

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/Autumn-27/artex/egress"
)

// ProxyAuthEnv holds the optional credentials ("user:password") the capture
// proxy demands from its clients. Unset, the proxy takes no credentials and
// stays loopback-only (see ResolveListenAddr).
const ProxyAuthEnv = "ARTEX_PROXY_AUTH"

// ProxyCreds are the Basic credentials the HTTP capture proxy (and, when it
// runs, the SOCKS5 listener via RFC 1929) requires.
type ProxyCreds struct {
	User, Pass string
}

// ParseProxyAuth parses "user:password". The split is at the first colon, so
// the password may contain colons; the user may not. Both parts must be
// non-empty. The error never echoes the value.
func ParseProxyAuth(raw string) (ProxyCreds, error) {
	user, pass, ok := strings.Cut(raw, ":")
	if !ok || user == "" || pass == "" {
		return ProxyCreds{}, fmt.Errorf("%s must be user:password with a non-empty user and password", ProxyAuthEnv)
	}
	return ProxyCreds{User: user, Pass: pass}, nil
}

// ProxyAuthFromEnv reads ARTEX_PROXY_AUTH. ok is false when it is unset or
// blank; a set but malformed value is an error (never silently ignored, since
// that would leave a proxy the operator meant to protect open).
func ProxyAuthFromEnv() (c ProxyCreds, ok bool, err error) {
	raw := os.Getenv(ProxyAuthEnv)
	if strings.TrimSpace(raw) == "" {
		return ProxyCreds{}, false, nil
	}
	c, err = ParseProxyAuth(raw)
	if err != nil {
		return ProxyCreds{}, false, err
	}
	return c, true, nil
}

// userinfo returns the credentials as URL userinfo.
func (c ProxyCreds) userinfo() *url.Userinfo { return url.UserPassword(c.User, c.Pass) }

// matches compares the offered credentials to c in constant time. Each part is
// hashed first so neither length nor content of the secret leaks through
// timing. The pace marker user (egress.PaceUser) is accepted with the right
// password: with auth on, the marker rides in the username and the password
// stays the secret (see egress.MarkProxyURL).
func (c ProxyCreds) matches(user, pass string) bool {
	eq := func(a, b string) int {
		ha, hb := sha256.Sum256([]byte(a)), sha256.Sum256([]byte(b))
		return subtle.ConstantTimeCompare(ha[:], hb[:])
	}
	userOK := eq(user, c.User) | eq(user, egress.PaceUser)
	return userOK&eq(pass, c.Pass) == 1
}

// errProxyAuth is the failure the proxy logs; it carries no credential.
var errProxyAuth = errors.New("proxy authentication required")

// authorize is the go-mitmproxy auth hook: it passes a request carrying
// matching Proxy-Authorization Basic credentials and fails every other one
// (the library then answers 407 with a Proxy-Authenticate challenge). It runs
// for every request that reaches the listener, CONNECT included.
func (c ProxyCreds) authorize(_ http.ResponseWriter, req *http.Request) (bool, error) {
	// http.Request.BasicAuth reads Authorization; point it at Proxy-Authorization.
	user, pass, ok := (&http.Request{Header: http.Header{"Authorization": {req.Header.Get("Proxy-Authorization")}}}).BasicAuth()
	if !ok || !c.matches(user, pass) {
		return false, errProxyAuth
	}
	return true, nil
}

// basicHeader renders creds as a Proxy-Authorization value (tests, clients).
func (c ProxyCreds) basicHeader() string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(c.User+":"+c.Pass))
}
