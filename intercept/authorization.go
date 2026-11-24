package intercept

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// The engagement authorization artifact (db.EngagementAuthorization) is
// enforced here, inside the target gate, and nowhere else: every gate entry
// point (CheckTargets with the gate on, CheckUngated with it off, the
// connection-level dial checks and the redirect hops, which all run a
// targetCheck) consults it before its own rules. A task without an artifact is
// gated exactly as before.
//
// A refusal is logged as a decided intercept row like every other gate block,
// so it appears in the intercept history and on the audit chain.

// Block codes of the authorization check. None of them opens an approval
// request: an artifact is changed by an admin, not approved per call.
const (
	// AuthorizationOutOfWindow: the call falls outside the artifact's time window.
	AuthorizationOutOfWindow = "AUTHORIZATION_OUT_OF_WINDOW"
	// AuthorizationRevoked: the newest version of the artifact is a revocation.
	AuthorizationRevoked = "AUTHORIZATION_REVOKED"
	// AuthorizationOutOfScope: a target is not in the artifact's scope.
	AuthorizationOutOfScope = "AUTHORIZATION_OUT_OF_SCOPE"
	// AuthorizationTechniqueDenied: the call uses a technique class the artifact does not allow.
	AuthorizationTechniqueDenied = "AUTHORIZATION_TECHNIQUE_DENIED"
)

// authNow is the clock of the window check; tests replace it.
var authNow = time.Now

// unwrapExtra returns the tool and input an ExecuteExtraTool call runs (as
// extractDeclared reads it), or the call itself.
func unwrapExtra(toolName string, input []byte) (string, []byte) {
	if toolName != executeExtraTool {
		return toolName, input
	}
	var in struct {
		ToolName string          `json:"tool_name"`
		Params   json.RawMessage `json:"params"`
	}
	if json.Unmarshal(input, &in) != nil || in.ToolName == "" || in.ToolName == executeExtraTool {
		return toolName, input
	}
	return in.ToolName, in.Params
}

// authorizeCall is the authorization check of a call that declares no
// target: the window and the technique classes still apply.
func (i *Interceptor) authorizeCall(ctx context.Context, scope TargetScope, toolName string, input []byte) TargetResult {
	if i == nil || i.targets == nil {
		return TargetResult{}
	}
	c := &targetCheck{i: i, ctx: WithCall(ctx, toolName, input), scope: scope, tool: toolName, input: input}
	var err error
	if c.ref, err = scope.ref(); err != nil {
		return TargetResult{Code: TargetGateUnavailable, Reason: err.Error()}
	}
	return c.authorize(nil)
}

// authorize applies the task's artifact, when it has one, to the call and the
// targets: revocation and window first (unless the tool cannot reach a
// target at all), then the technique classes, then the scope of each target.
// c.ref must be set.
func (c *targetCheck) authorize(targets []Target) TargetResult {
	if c.ref.TaskID == 0 || c.i.targets == nil {
		return TargetResult{}
	}
	first := &checkedTarget{}
	if len(targets) > 0 {
		first.Target = targets[0]
	}
	a, err := c.i.targets.Authorization(c.ref)
	if err != nil {
		_, res := c.block(TargetGateUnavailable, "gate_unavailable", first, "the engagement authorization could not be read: "+err.Error())
		return res
	}
	if a == nil {
		return TargetResult{}
	}
	tool, input := unwrapExtra(c.tool, c.input)
	terms := fmt.Sprintf("authorization v%d of task %d", a.Version, a.TaskID)
	if a.Reference != "" {
		terms += " (" + a.Reference + ")"
	}
	if !noTargetTools[tool] {
		now := authNow()
		switch {
		case a.Revoked:
			_, res := c.block(AuthorizationRevoked, "authorization_revoked", first,
				fmt.Sprintf("%s is revoked: no action against targets is authorized", terms))
			return res
		case !a.InForce(now):
			_, res := c.block(AuthorizationOutOfWindow, "authorization_window", first,
				fmt.Sprintf("%s authorizes action from %s until %s; it is now %s", terms,
					a.NotBefore.UTC().Format(time.RFC3339), a.NotAfter.UTC().Format(time.RFC3339), now.UTC().Format(time.RFC3339)))
			return res
		}
	}
	for _, class := range ClassifyTechniques(tool, input) {
		if !a.AllowsTechnique(class) {
			_, res := c.block(AuthorizationTechniqueDenied, "authorization_technique", first,
				fmt.Sprintf("%s does not allow the technique class %s (allowed: %s)", terms, class, strings.Join(a.Techniques, ", ")))
			return res
		}
	}
	for _, t := range targets {
		if !a.CoversHost(t.Kind, t.Host) {
			_, res := c.block(AuthorizationOutOfScope, "authorization_scope", &checkedTarget{Target: t},
				fmt.Sprintf("%s is not in the scope of %s", t, terms))
			return res
		}
	}
	return TargetResult{}
}
