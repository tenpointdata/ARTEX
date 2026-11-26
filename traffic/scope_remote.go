package traffic

import (
	"errors"
	"net"
	"net/http"
	"sync"
	"time"
)

// Remote scope tokens. A worker node (the same binary on another host, sharing
// the control node's database) sends its agents' target traffic through the
// control node's recording proxy, so it is recorded here, passes the upstream
// gate and is paced like a local client's. The worker mints the scope token of
// a task itself and registers its hash in the shared database; the control
// node's proxy does not hold it in its registry. The proxy then asks the remote
// resolver (SetRemoteScopeResolver), which maps the token to the scope key and
// ScopeFunc of the control node's own loaded task: the task's TargetScope is
// applied exactly as for a token minted here.
//
// Fail closed, everywhere:
//   - a token neither registry resolves is refused at the gate, as an ended
//     local token is; a resolver error is a refusal;
//   - a positive resolution is cached for remoteScopeTTL, then re-checked on
//     the next dial; a re-check that fails drops the token and closes its
//     client connections;
//   - ForgetScope(key) drops the remote tokens resolved to key and closes their
//     client connections, as for a local token (scope.go), including after the
//     cache entry is stale: the entry, not its freshness, is what ForgetScope
//     finds;
//   - a resolution in flight when ForgetScope(key) runs is not cached and is
//     refused, so a dial racing the end of the scope cannot re-open it.
//
// Exposure: once the proxy listens where workers reach it, a client without a
// token from a non-loopback address would get the global rules only: an open
// proxy onto the cluster network. RequireScopeForRemoteClients (on whenever a
// remote resolver is set) refuses its dials, and those of a non-loopback client
// naming a scope that does not resolve, even when a global egress proxy is set
// (that path bypasses the gate). Loopback clients keep today's behaviour.
//
// Binding is by the client connection's remote address (scope.go), which for a
// worker is its pod or host address and an ephemeral port: unique per TCP
// connection, as it is on loopback.

// RemoteScopeResolver resolves a scope token this proxy did not mint. key is the
// scope key the token belongs to (the one ForgetScope is called with: for a task,
// its ID); fn derives the dial context. ok is false when the token names no
// active scope or the lookup failed.
type RemoteScopeResolver func(token string) (key string, fn ScopeFunc, ok bool)

// remoteScopeTTL bounds how long a positive remote resolution is trusted
// without asking the resolver again.
const remoteScopeTTL = 30 * time.Second

// maxRemoteScopes caps the remote tokens held at once. A resolution over the
// cap is not cached (it is used once and re-asked next time): the cache only
// saves lookups, and ForgetScope must find every entry it holds.
const maxRemoteScopes = 4096

type remoteEntry struct {
	key     string
	fn      ScopeFunc
	checked time.Time
}

// remoteScopes is the remote-token cache. The zero value has no resolver.
type remoteScopes struct {
	mu       sync.Mutex
	resolve  RemoteScopeResolver
	require  bool // RequireScopeForRemoteClients
	byToken  map[string]remoteEntry
	gen      uint64            // bumped by every forget
	forgot   map[string]uint64 // key -> gen of its last forget, kept while resolutions are in flight
	inflight int
	now      func() time.Time // time.Now; tests replace it
}

// ErrScopeRequired refuses a dial of a non-loopback proxy client that names no
// active scope while RequireScopeForRemoteClients is on.
var ErrScopeRequired = errors.New("the recording proxy requires a run scope token from a non-loopback client; refusing the connection")

// SetRemoteScopeResolver sets how the proxy resolves a scope token it did not
// mint (nil: it resolves none, the default). Setting a resolver also turns on
// RequireScopeForRemoteClients; turn that off afterwards only on purpose.
func (t *Traffic) SetRemoteScopeResolver(fn RemoteScopeResolver) {
	t.remote.mu.Lock()
	t.remote.resolve = fn
	if fn != nil {
		t.remote.require = true
	}
	stale := t.remote.dropAllLocked()
	t.remote.mu.Unlock()
	t.closeTokens(stale)
}

// RequireScopeForRemoteClients sets whether a proxy client connecting from a
// non-loopback address must name an active scope (a token this proxy minted or
// the remote resolver resolves). On, such a client without one is refused, so
// the proxy is no open relay for the network it listens on. Loopback clients
// (local agents, scope relays) are unaffected.
func (t *Traffic) RequireScopeForRemoteClients(on bool) {
	t.remote.mu.Lock()
	t.remote.require = on
	t.remote.mu.Unlock()
}

func (r *remoteScopes) requireScope() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.require
}

func (r *remoteScopes) clock() time.Time {
	if r.now != nil {
		return r.now()
	}
	return time.Now()
}

// resolveScope resolves tok: the local registry first, then the remote one.
func (t *Traffic) resolveScope(tok string) (ScopeFunc, bool) {
	if fn, ok := t.scopes.lookup(tok); ok {
		return fn, true
	}
	fn, ok, drop := t.remote.lookup(tok)
	if drop {
		t.closeTokens([]string{tok})
	}
	return fn, ok
}

// lookup resolves tok remotely. drop is true when a cached token failed its
// re-check, so its client connections must be closed.
func (r *remoteScopes) lookup(tok string) (fn ScopeFunc, ok, drop bool) {
	r.mu.Lock()
	resolve := r.resolve
	if resolve == nil || tok == "" {
		r.mu.Unlock()
		return nil, false, false
	}
	now := r.clock()
	if e, hit := r.byToken[tok]; hit && now.Sub(e.checked) < remoteScopeTTL {
		r.mu.Unlock()
		return e.fn, true, false
	}
	g0 := r.gen
	r.inflight++
	r.mu.Unlock()

	key, fn, ok := resolve(tok)

	r.mu.Lock()
	defer r.mu.Unlock()
	r.inflight--
	if ok && (fn == nil || r.forgot[key] > g0) {
		ok = false // no scope, or its scope ended while it was resolved
	}
	if r.inflight == 0 {
		r.forgot = nil
	}
	if !ok {
		_, drop = r.byToken[tok]
		delete(r.byToken, tok)
		return nil, false, drop
	}
	if r.byToken == nil {
		r.byToken = map[string]remoteEntry{}
	}
	if _, held := r.byToken[tok]; held || len(r.byToken) < maxRemoteScopes {
		r.byToken[tok] = remoteEntry{key: key, fn: fn, checked: r.clock()}
	}
	return fn, true, false
}

// forget drops the tokens resolved to key and returns them.
func (r *remoteScopes) forget(key string) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.gen++
	if r.inflight > 0 {
		if r.forgot == nil {
			r.forgot = map[string]uint64{}
		}
		r.forgot[key] = r.gen
	}
	var toks []string
	for tok, e := range r.byToken {
		if e.key == key {
			toks = append(toks, tok)
			delete(r.byToken, tok)
		}
	}
	return toks
}

// dropAllLocked empties the cache (a resolver change) and returns its tokens;
// r.mu is held. In-flight resolutions of the old resolver are not cached.
func (r *remoteScopes) dropAllLocked() []string {
	r.gen++
	toks := make([]string, 0, len(r.byToken))
	for tok, e := range r.byToken {
		toks = append(toks, tok)
		if r.inflight > 0 {
			if r.forgot == nil {
				r.forgot = map[string]uint64{}
			}
			r.forgot[e.key] = r.gen
		}
	}
	r.byToken = nil
	return toks
}

// closeTokens closes the client connections bound to each token.
func (t *Traffic) closeTokens(toks []string) {
	if t.sink == nil {
		return
	}
	for _, tok := range toks {
		t.sink.closeScope(tok)
	}
}

// remoteClientRefusal is the error a dial of req's client gets under
// RequireScopeForRemoteClients, nil when the dial may go on: a loopback client,
// the option off, or a non-loopback client naming an active scope.
func (t *Traffic) remoteClientRefusal(req *http.Request, tok string) error {
	if !t.remote.requireScope() {
		return nil
	}
	addr := ""
	if req != nil {
		addr = req.RemoteAddr
	}
	if t.loopbackClient(addr) {
		return nil
	}
	if tok == "" {
		return ErrScopeRequired
	}
	if _, ok := t.resolveScope(tok); !ok {
		return errScopeUnknown
	}
	return nil
}

// loopbackClient reports whether a client remote address ("ip:port") is on
// loopback. An address that does not parse is not (fail closed).
func (t *Traffic) loopbackClient(addr string) bool {
	if fn := t.isLoopback.Load(); fn != nil {
		return (*fn)(addr)
	}
	return isLoopbackAddr(addr)
}

func isLoopbackAddr(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
