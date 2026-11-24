package agent

import (
	"context"
	"testing"
)

type proxyCtxKey struct{}

// The resolver, when set, decides the chat run's proxy from its context (one
// chat agent serves many conversations); without it the SetProxy address holds.
func TestChatAgentProxyResolver(t *testing.T) {
	c := NewChatAgent(nil, "", t.TempDir(), nil, 0)
	c.SetProxy("http://127.0.0.1:8080", "")
	ctx := context.WithValue(context.Background(), proxyCtxKey{}, "http://artex-scope:scope-x@127.0.0.1:8080")
	if got := c.proxy(ctx); got != "http://127.0.0.1:8080" {
		t.Fatalf("no resolver: %q", got)
	}
	c.SetProxyResolver(func(ctx context.Context) string { s, _ := ctx.Value(proxyCtxKey{}).(string); return s })
	if got := c.proxy(ctx); got != "http://artex-scope:scope-x@127.0.0.1:8080" {
		t.Fatalf("resolver: %q", got)
	}
}
