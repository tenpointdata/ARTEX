package server

import (
	"errors"
	"testing"
	"time"

	pgdb "github.com/Autumn-27/artex/db"
)

type fakeTLSScope struct {
	tasks []pgdb.TaskScopeRules
	err   error
	reads int
}

func (f *fakeTLSScope) ListAssetInterceptRules() ([]pgdb.AssetInterceptRule, error) {
	f.reads++
	return nil, f.err
}
func (f *fakeTLSScope) ActiveTaskScopeRules() ([]pgdb.TaskScopeRules, error) { return f.tasks, f.err }

func TestUpstreamTLSScopeCachesAndFailsClosed(t *testing.T) {
	src := &fakeTLSScope{tasks: []pgdb.TaskScopeRules{{TaskID: 1, Allow: []pgdb.AssetInterceptRule{
		{Enabled: true, Kind: "exact_domain", Pattern: "lab.example"}}}}}
	now := time.Unix(1000, 0)
	p := newUpstreamTLSScope(src)
	p.now = func() time.Time { return now }

	if !p.trusts("lab.example") || p.trusts("other.example") {
		t.Fatal("policy should trust exactly the allow-listed host")
	}
	if src.reads != 1 {
		t.Fatalf("reads %d, want 1 within the TTL", src.reads)
	}
	src.err = errors.New("db down")
	if !p.trusts("lab.example") {
		t.Fatal("a cached read serves until the TTL")
	}
	now = now.Add(upstreamTLSScopeTTL)
	if p.trusts("lab.example") {
		t.Fatal("a failed read must verify every host")
	}
	src.err = nil
	now = now.Add(upstreamTLSScopeTTL)
	if !p.trusts("lab.example") {
		t.Fatal("trust returns once the read succeeds")
	}
}
