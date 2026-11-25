package server

import (
	"net/http"
	"path"
	"strings"

	"github.com/Autumn-27/artex/db"
)

// Role-based access control: the declarative policy. requireAuth (rbac.go)
// resolves each authenticated API request to one required permission with
// requiredPermission and refuses it unless the caller's role holds it.
//
// Permissions are sets, not a ladder, though conceptually
// client_read < read < review < operate < admin.

type rbacPerm string

const (
	permClientRead rbacPerm = "client_read" // the client-facing summary: report, findings, stats, outcome
	permRead       rbacPerm = "read"        // everything else that only reads
	permReview     rbacPerm = "review"      // human decisions: intercept approvals, finding triage
	permOperate    rbacPerm = "operate"     // runs and changes engagements; reads secrets and raw traffic logs
	permAdmin      rbacPerm = "admin"       // users, authorizations and guardrail configuration
)

// allPermissions is every permission, least privileged first.
var allPermissions = []rbacPerm{permClientRead, permRead, permReview, permOperate, permAdmin}

// rolePermissions is what each role holds. The legacy ARTEX login is admin.
var rolePermissions = map[string][]rbacPerm{
	db.RoleAdmin:      allPermissions,
	db.RoleOperator:   {permClientRead, permRead, permReview, permOperate},
	db.RoleReviewer:   {permClientRead, permRead, permReview},
	db.RoleClientRead: {permClientRead},
}

// roleHas reports whether role holds p. An unknown role holds nothing.
func roleHas(role string, p rbacPerm) bool {
	for _, q := range rolePermissions[role] {
		if q == p {
			return true
		}
	}
	return false
}

// Rule methods besides the HTTP verbs. GET also matches HEAD, as in ServeMux.
const (
	methodAny    = "*"
	methodMutate = "MUTATE" // anything but GET and HEAD
)

// policyRule requires Need for requests whose method matches Method and whose
// path matches Path. Path uses ServeMux syntax: {name} matches one segment,
// {name...} the rest. Prefix also matches every path below Path.
type policyRule struct {
	Method string
	Path   string
	Prefix bool
	Need   rbacPerm
}

// rbacPolicy is checked in order; the first matching rule wins. Requests that
// match none fall to defaultPermission.
var rbacPolicy = []policyRule{
	// --- admin: users, engagement authorizations, guardrail configuration ---
	{methodAny, "/api/users", true, permAdmin},
	{methodMutate, "/api/engagements", true, permAdmin},
	{methodMutate, "/api/tasks/{id}/authorization", true, permAdmin},
	{methodMutate, "/api/intercept/rules", true, permAdmin},
	{methodMutate, "/api/asset-intercept/rules", true, permAdmin},
	{methodMutate, "/api/target-gate/exemptions", true, permAdmin},
	{methodMutate, "/api/intercept/judge", true, permAdmin},
	{methodMutate, "/api/intercept/tool-config", true, permAdmin},
	{methodMutate, "/api/tasks/{id}/intercept-rules", true, permAdmin},
	{http.MethodPost, "/api/intercept/pending/{id}/always-allow", false, permAdmin}, // creates a rule
	{methodMutate, "/api/settings", true, permAdmin},                                // includes the noise governor
	// Platform configuration that decides what the agents can run and where
	// data goes: tool catalog and allowlists, MCP servers, agent prompts and
	// visibility, LLM endpoints and keys, notification sinks, sync sources.
	{methodMutate, "/api/tools", true, permAdmin},
	{methodMutate, "/api/mcp", true, permAdmin},
	{methodMutate, "/api/agents", true, permAdmin},
	{methodMutate, "/api/visibility", true, permAdmin},
	{methodMutate, "/api/llm", true, permAdmin},
	{methodMutate, "/api/notify/channels", true, permAdmin},
	// Ticket trackers (E2) are sinks like notification channels: sealed
	// credentials and a destination findings leave to. Push and sync stay operate.
	{http.MethodPost, "/api/integrations/trackers", false, permAdmin},
	{http.MethodPatch, "/api/integrations/trackers/{id}", false, permAdmin},
	{http.MethodDelete, "/api/integrations/trackers/{id}", false, permAdmin},
	{methodMutate, "/api/sync/scopesentry/datasource", true, permAdmin},
	// Bulk destruction of evidence.
	{http.MethodDelete, "/api/traffic/all", false, permAdmin},
	{http.MethodPost, "/api/gc", false, permAdmin},

	// --- review: human decisions ---
	{http.MethodPost, "/api/intercept/pending/{id}/decide", false, permReview},
	{http.MethodPost, "/api/intercept/pending/batch-decide", false, permReview},
	{http.MethodPatch, "/api/exploration/findings/{id}", false, permReview},
	{http.MethodPost, "/api/exploration/findings/bulk-status", false, permReview},
	{http.MethodPost, "/api/exploration/findings/bulk-report-state", false, permReview},

	// --- operate, even to read: secrets, raw LLM traffic, server logs, files ---
	{http.MethodGet, "/api/tasks/{id}/credentials", true, permOperate},
	{http.MethodGet, "/api/workspace/download", false, permOperate},
	{http.MethodGet, "/api/workspace/read", false, permOperate},
	{http.MethodGet, "/api/llm/records", true, permOperate},
	{http.MethodGet, "/api/logs", true, permOperate},
	{http.MethodGet, "/api/settings/nodes", false, permOperate}, // node host names (E7)
	// Raw recorded traffic carries the Authorization headers and cookies the
	// agents used; conversation messages echo them. Host and list summaries stay read.
	{http.MethodGet, "/api/traffic/exchange", false, permOperate},
	{http.MethodGet, "/api/traffic/blob", false, permOperate},
	{http.MethodGet, "/api/traffic/export", true, permOperate},
	{http.MethodGet, "/api/traffic/stream", false, permOperate},
	{http.MethodGet, "/api/conversations/{id}/messages", true, permOperate},

	// --- read: the audit chain, for reviewers ---
	{http.MethodGet, "/api/intercept/history/chain", true, permRead},

	// --- read: live retests across every client ---
	{http.MethodGet, "/api/exploration/findings/retests/active", false, permRead},
	// A finding's tracker tickets and raw scanner record are operator data, not
	// the client summary: kept out of the client_read findings prefix below.
	{http.MethodGet, "/api/exploration/findings/{id}/integrations", false, permRead},

	// --- client_read: the allowlist for the client-facing role. Each route
	// must also have a binder in clientBinders (client_scope.go), which
	// narrows it to the user's companies. ---
	{http.MethodGet, "/api/report", true, permClientRead},
	{http.MethodGet, "/api/exploration/findings", true, permClientRead},
	{http.MethodGet, "/api/stats", false, permClientRead},
	{http.MethodGet, "/api/tasks/{id}/outcome", false, permClientRead},
	{http.MethodGet, "/api/tasks/{id}/coverage", false, permClientRead},
	// The console's pickers: the companies and tasks the caller may see.
	{http.MethodGet, "/api/me/companies", false, permClientRead},
	{http.MethodGet, "/api/me/tasks", false, permClientRead},
}

// defaultPermission applies when no rule matches: reads need read, anything
// else operate.
func defaultPermission(method string) rbacPerm {
	if method == http.MethodGet || method == http.MethodHead {
		return permRead
	}
	return permOperate
}

// requiredPermission resolves a request to the permission it needs. route is
// the path of the ServeMux pattern the request matched when known (so the
// policy sees exactly the route the mux will serve), else the request path.
func requiredPermission(method, route string) rbacPerm {
	segs := pathSegments(route)
	for _, rule := range rbacPolicy {
		if rule.methodMatches(method) && rule.pathMatches(segs) {
			return rule.Need
		}
	}
	return defaultPermission(method)
}

func (r policyRule) methodMatches(m string) bool {
	switch r.Method {
	case methodAny:
		return true
	case methodMutate:
		return m != http.MethodGet && m != http.MethodHead
	case http.MethodGet:
		return m == http.MethodGet || m == http.MethodHead
	}
	return r.Method == m
}

func (r policyRule) pathMatches(segs []string) bool {
	pat := pathSegments(r.Path)
	for i, p := range pat {
		if strings.HasPrefix(p, "{") && strings.HasSuffix(p, "...}") {
			return true
		}
		if i >= len(segs) {
			return false
		}
		if !(strings.HasPrefix(p, "{") && strings.HasSuffix(p, "}")) && p != segs[i] {
			return false
		}
	}
	return len(segs) == len(pat) || r.Prefix
}

// pathSegments cleans p (dot segments, doubled and trailing slashes) and
// splits it into segments.
func pathSegments(p string) []string {
	c := strings.Trim(path.Clean("/"+p), "/")
	if c == "" {
		return nil
	}
	return strings.Split(c, "/")
}

// routePath is the path part of a ServeMux pattern ("GET host/a/{b}" -> "/a/{b}").
func routePath(pattern string) string {
	if i := strings.Index(pattern, "/"); i >= 0 {
		return pattern[i:]
	}
	return ""
}
