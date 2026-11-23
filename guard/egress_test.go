package guard

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Autumn-27/artex/intercept"
)

// With the gate off, every guard with an interceptor runs the always-on
// egress check, and its block stops the call with the target block text,
// before the rules (the store-less interceptor would panic on a rule read).
// With the gate on the gate decides and the check is not run.
func TestGateOffRunsAlwaysOnEgressCheck(t *testing.T) {
	blocked := intercept.TargetResult{Code: intercept.TargetRuleBlocked, Host: "169.254.169.254", Port: 80,
		Reason: "169.254.169.254:80 reaches 169.254.169.254, a link-local or cloud instance-metadata address"}
	const metadata = `{"url":"http://169.254.169.254/latest/meta-data/"}`
	offCtxs := map[string]context.Context{
		"no scope": context.Background(),
		"flag off": intercept.WithTargetScope(context.Background(), intercept.TargetScope{TaskID: "7", Enabled: func() bool { return false }}),
	}
	for name, g := range map[string]*Guard{
		"full":        NewWithInterceptor(intercept.New(nil)),
		"target-only": NewTargetOnly(intercept.New(nil)),
	} {
		gate := &fakeGate{t: t, ungated: func(tool string, input []byte) intercept.TargetResult {
			if tool == "WebFetch" && strings.Contains(string(input), "169.254.169.254") {
				return blocked
			}
			return intercept.TargetResult{}
		}}
		g.gate = gate
		for scopeName, ctx := range offCtxs {
			if block, msg := pre(g, ctx, "WebFetch", metadata); !block || msg != TargetBlockMessage(blocked) {
				t.Errorf("%s, %s: block=%v msg=%q, want the target block", name, scopeName, block, msg)
			}
		}
		if gate.ungateds != 2 || gate.checks != 0 {
			t.Errorf("%s: %d gate-off checks, %d gate checks; want 2 and 0", name, gate.ungateds, gate.checks)
		}
	}

	// The target-only guard reads no rule: a call the check passes runs.
	g := NewTargetOnly(intercept.New(nil))
	gate := &fakeGate{t: t}
	g.gate = gate
	for scopeName, ctx := range offCtxs {
		if block, msg := pre(g, ctx, "WebFetch", fetchApp); block {
			t.Errorf("%s: a call the check passes was blocked: %s", scopeName, msg)
		}
	}
	// Gate on: the gate decides, the gate-off check is not run.
	pre(g, gateOn(context.Background(), "7", time.Minute), "WebFetch", fetchApp)
	if gate.ungateds != 2 || gate.checks != 1 {
		t.Errorf("%d gate-off checks, %d gate checks; want 2 (gate off) and 1 (gate on)", gate.ungateds, gate.checks)
	}
}
