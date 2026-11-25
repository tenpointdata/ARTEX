package agent

// Per-role default output caps (c-83ae7b).
//
// A profile's max_tokens of 0 does not mean "no cap": norma's harness then sends
// its own default (32768) on every request, for every provider format including
// ollama, and retries a max_tokens truncation up to three times with a resume
// nudge. A degenerate repetition loop could therefore run 4×32k output tokens per
// turn: 10–20 minutes on a single-slot local model (starving every other task),
// wasted spend on a paid one.
//
// Each role now applies its own default when the profile leaves the cap at 0. An
// explicit profile cap (> 0) always wins, in either direction, so an operator who
// needs longer replies can raise it per profile.
//
// The numbers come from llm_usage (2,612 recorded calls, local DB, 2026-11-24):
// the largest single reply was 3,511 tokens for the worker (p99 1,604), 2,972 for
// the planner (p99 2,337), 5,823 for unlabelled calls (chat). 8192 leaves ≥1.4×
// headroom over every observed reply and >2× over the agent roles, and a legitimate
// reply that does hit it is still continued by the harness's truncation resume.
// The runaway bound drops from 4×32768 to 4×8192 output tokens per turn.
//
// Roles that already bound themselves are not listed: the goals decomposer
// (GoalsMaxTokens, c-7361b4), the cold-digest Compactor (1500, compaction.go),
// and the context-window summariser (norma's ProviderSummarizer clamps to its own
// compactMaxOutputTokens and only ever lowers it to the role cap).

// ReplyRole names an agent role for output-cap defaults.
type ReplyRole string

const (
	ReplyRolePlanner   ReplyRole = "planner"
	ReplyRoleWorker    ReplyRole = "worker"
	ReplyRoleMainAgent ReplyRole = "mainagent"
	ReplyRoleChat      ReplyRole = "chat"
)

// DefaultReplyCaps is the per-reply output cap each role sends when its profile
// sets max_tokens to 0. Exported so the API/UI can state the effective default.
var DefaultReplyCaps = map[ReplyRole]int{
	ReplyRolePlanner:   8192,
	ReplyRoleWorker:    8192,
	ReplyRoleMainAgent: 8192,
	ReplyRoleChat:      8192,
}

// ReplyCap returns the output cap a role sends: the profile's cap when it is set
// (> 0), else the role's default. An unknown role with no profile cap returns 0,
// i.e. norma's own default; callers should use a listed role.
func ReplyCap(role ReplyRole, profileMax int) int {
	if profileMax > 0 {
		return profileMax
	}
	return DefaultReplyCaps[role]
}

// resolveReplyCap applies ReplyCap to a per-run resolver (nil = no profile cap).
func resolveReplyCap(role ReplyRole, fn func() int) int {
	profileMax := 0
	if fn != nil {
		profileMax = fn()
	}
	return ReplyCap(role, profileMax)
}
