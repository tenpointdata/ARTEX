package egress

import (
	"context"
	"net/url"
)

// Bash commands reach targets through the recording proxy, outside the clients
// this package governs. With traffic capture on, the proxy URL in their
// environment carries a marker userinfo (MarkProxyURL). The proxy paces each
// request of a marked client connection with Wait, and leaves unmarked
// connections alone: WebFetch, the http custom tools and the enrich probe use
// the plain proxy URL and are already paced by their governor, so no request
// is paced twice. Best effort, not a security boundary: a command that ignores
// the proxy, or drops the marker, is not paced.

// PaceUser is the user (and, unscoped, the password) of the marker userinfo.
const PaceUser = "artex-pace"

// MarkProxyURL returns raw with the marker userinfo. A scoped URL
// (ScopedProxyURL) keeps its scope token and takes PaceUser as its user. raw is
// returned unchanged when it does not parse or carries another userinfo.
func MarkProxyURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return raw
	}
	switch {
	case u.User == nil:
		u.User = url.UserPassword(PaceUser, PaceUser)
	case u.User.Username() == ScopeUser:
		pass, _ := u.User.Password()
		u.User = url.UserPassword(PaceUser, pass)
	default:
		return raw
	}
	return u.String()
}

// IsPaceMarker reports whether a Proxy-Authorization value is Basic
// credentials for PaceUser.
func IsPaceMarker(proxyAuthorization string) bool {
	user, _, ok := ProxyCredential(proxyAuthorization)
	return ok && user == PaceUser
}

// Wait paces one request to hostKey under the current profile. See
// Governor.Wait.
func Wait(ctx context.Context, hostKey string) error { return Current().Wait(ctx, hostKey) }

// Wait takes a token of the host's bucket plus the jitter, as a governed request
// does, and counts the request in the host's noise counter. It takes no
// in-flight slot, and nothing is cached, coalesced or retried. Off, it returns
// at once and counts nothing. An error means ctx ended first; the request is
// counted either way, since the caller sends it regardless.
func (g *Governor) Wait(ctx context.Context, hostKey string) error {
	if g.off() {
		return nil
	}
	h := g.host(hostKey)
	err := sleep(ctx, g.reserve(h)+g.jitter())
	g.mu.Lock()
	h.sent++
	g.mu.Unlock()
	return err
}
