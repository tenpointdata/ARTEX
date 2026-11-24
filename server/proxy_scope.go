package server

import (
	"context"
	"strconv"

	"github.com/Autumn-27/artex/intercept"
)

// Scoped recording-proxy URLs for clients that serve more than one run. Task
// agents are built per task and get TaskProxyAddr. The chat agent (one per LLM
// profile, shared by every conversation) and the http custom tool (called from
// any run) learn whose call it is only from the context: the run puts its
// intercept.TargetScope there. ScopedProxyAddr maps that scope to the scope
// token of the run, so the recording proxy's upstream dials of the call apply
// the run's own rules as well as the global ones (traffic/scope.go).

// conversationScopeKey is the scope-registry key of conversation id. Task IDs
// never take this form, so the keys of tasks and conversations cannot collide.
func conversationScopeKey(id int64) string {
	return "conversation:" + strconv.FormatInt(id, 10)
}

// ScopedProxyAddr is ProxyAddr for a call running on ctx. With capture on:
//   - a task scope gets TaskProxyAddr of the task, or, when the task is no
//     longer loaded, a URL whose dials are refused (never the global rules);
//   - a conversation scope gets the conversation's scoped URL, whose dials run
//     under the scope of the current turn (each turn re-registers it) with no
//     wait for a decision, as task dials do;
//   - a context without a scope gets the plain URL (global rules only).
//
// With capture off it is ProxyAddr: the global proxy, which records nothing
// and carries no scope.
func (m *Manager) ScopedProxyAddr(ctx context.Context) string {
	if m.traffic == nil || !m.TrafficEnabled() {
		return m.ProxyAddr()
	}
	scope, ok := intercept.TargetScopeFromContext(ctx)
	switch {
	case !ok:
		return m.ProxyAddr()
	case scope.TaskID != "":
		if t, loaded := m.Task(scope.TaskID); loaded {
			return m.TaskProxyAddr(t)
		}
		return m.traffic.EndedScopeURL()
	case scope.ConversationID != 0:
		dial := scope
		dial.WaitBound = 0 // a proxy dial never waits; an unpinned target is refused until approved
		return m.traffic.ScopedProxyURL(conversationScopeKey(scope.ConversationID), func(ctx context.Context) context.Context {
			return intercept.WithTargetScope(ctx, dial)
		})
	}
	return m.ProxyAddr()
}

// forgetConversationScope drops conversation id's proxy scope token: a proxy
// client still naming it is refused from then.
func (m *Manager) forgetConversationScope(id int64) {
	if m.traffic != nil {
		m.traffic.ForgetScope(conversationScopeKey(id))
	}
}
