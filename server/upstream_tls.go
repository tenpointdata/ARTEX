package server

import (
	"log"
	"sync"
	"time"

	pgdb "github.com/Autumn-27/artex/db"
	"github.com/Autumn-27/artex/traffic"
)

// upstreamTLSScopeTTL is how long one read of the scope rules serves the
// capture proxy's TLS decisions. A rule added to a task applies within it.
const upstreamTLSScopeTTL = 5 * time.Second

// upstreamTLSScopeSource is what the policy reads: the global asset block
// rules and the rules of the tasks that can still run.
type upstreamTLSScopeSource interface {
	ListAssetInterceptRules() ([]pgdb.AssetInterceptRule, error)
	ActiveTaskScopeRules() ([]pgdb.TaskScopeRules, error)
}

// upstreamTLSScope is the capture proxy's trust policy: a host's certificate
// is not verified only when an allow rule of an active task names it and no
// global or task block rule hits it (pgdb.HostInTaskAllowScope). The proxy
// cannot tell which task a request belongs to, so any active task's scope
// counts. A failed read verifies every host until the next read succeeds.
type upstreamTLSScope struct {
	src upstreamTLSScopeSource
	now func() time.Time

	mu      sync.Mutex
	loading bool
	loaded  time.Time
	ok      bool
	global  []pgdb.AssetInterceptRule
	tasks   []pgdb.TaskScopeRules
	errLast time.Time
}

func newUpstreamTLSScope(src upstreamTLSScopeSource) *upstreamTLSScope {
	return &upstreamTLSScope{src: src, now: time.Now}
}

// trusts is a traffic.UpstreamTrustFunc. A stale snapshot is refreshed by
// one caller, outside the lock; the others decide on the snapshot they find,
// so a slow database never holds up a handshake (before the first read
// succeeds, that snapshot verifies every host).
func (p *upstreamTLSScope) trusts(host string) bool {
	p.mu.Lock()
	now := p.now()
	if !p.loading && now.Sub(p.loaded) >= upstreamTLSScopeTTL {
		p.loading = true
		p.mu.Unlock()
		global, tasks, err := p.read()
		p.mu.Lock()
		p.store(now, global, tasks, err)
	}
	ok, global, tasks := p.ok, p.global, p.tasks
	p.mu.Unlock()
	return ok && pgdb.HostInTaskAllowScope(host, global, tasks)
}

func (p *upstreamTLSScope) read() ([]pgdb.AssetInterceptRule, []pgdb.TaskScopeRules, error) {
	global, err := p.src.ListAssetInterceptRules()
	if err != nil {
		return nil, nil, err
	}
	tasks, err := p.src.ActiveTaskScopeRules()
	return global, tasks, err
}

// store records a read; p.mu is held.
func (p *upstreamTLSScope) store(now time.Time, global []pgdb.AssetInterceptRule, tasks []pgdb.TaskScopeRules, err error) {
	p.loading, p.loaded = false, now
	if err != nil {
		p.ok, p.global, p.tasks = false, nil, nil
		if now.Sub(p.errLast) >= time.Minute {
			p.errLast = now
			log.Printf("[traffic] upstream TLS scope unavailable, verifying every host: %v", err)
		}
		return
	}
	p.ok, p.global, p.tasks = true, global, tasks
}

var _ traffic.UpstreamTrustFunc = (*upstreamTLSScope)(nil).trusts
