import type { EngineMode, Task } from "./tasks";

// ---- Agents ----
export interface Agent {
  id: string;
  key: string; // built-ins are goals/planner/mainagent/worker; custom agents use user-defined keys
  name: string;
  description?: string;
  role: string;
  builtin: boolean;
  enabled: boolean;
  llm_profile_id?: number | null; // bound LLM profile; null/absent = follow task/session/global
  max_turns?: number; // 0 = unlimited
  run_seconds?: number; // worker per-run wall-clock cap (seconds); 0 = unlimited
  web_search?: boolean; // whether web search is enabled (gated by the global system switch)
  interactive_shell?: boolean; // whether the interactive shell is enabled (persistent PTY session tool family)
  // P3 post-trigger handling policy (only meaningful for custom agents)
  trigger_run_mode?: "serial" | "parallel"; // queue serially / each trigger gets its own concurrent session
  trigger_merge_mode?: "by_task" | "all" | "none"; // serial only: merge per task / merge all / no merge
  trigger_max_parallel?: number; // parallel only: per-agent concurrency cap; 0 = unlimited
  // Binding counts (returned only by the list endpoint): visible MCP / visible Skill / bound tools
  mcp_count?: number;
  skill_count?: number;
  tool_count?: number;
}

export interface PromptVar {
  name: string;
  description: string;
  example: string;
  source: "exploration" | "runtime" | "distilled";
}

export interface PromptVersion {
  version: number;
  ts: string;
  note: string;
  template_text: string;
}

export interface AgentDetail {
  agent: Agent;
  prompt: string;
  variables: PromptVar[];
  versions: PromptVersion[];
  visibility: { mcp: number[]; skill: string[] };
  // Bindable LLM profile candidates (for the "default model" dropdown); the current binding is agent.llm_profile_id
  llm_profiles?: { id: number; name: string; model: string; is_default: boolean }[];
  wrapup_prompt?: string; // saved wrap-up prompt (empty = built-in default)
  wrapup_default?: string; // built-in default wrap-up prompt (placeholder / restore default)
  wrapup_max_turns?: number; // saved wrap-up turn count (0 = built-in default)
  wrapup_max_turns_default?: number; // built-in default wrap-up turn count (for the "0 = default N" hint)
  // Task-level timeout wrap-up prompt (worker/planner only; this section shows only when task_timeout_wrapup_supported=true)
  task_timeout_wrapup_supported?: boolean;
  task_timeout_wrapup_prompt?: string;
  task_timeout_wrapup_default?: string;
  task_timeout_wrapup_max_turns?: number;
  task_timeout_wrapup_max_turns_default?: number;
}

// ---- MCP ----
export interface MCPServer {
  id: number;
  name: string;
  transport: "stdio" | "http" | "sse";
  command?: string;
  args: string[];
  env: Record<string, string>;
  url?: string;
  enabled: boolean;
  insecure?: boolean; // http: skip TLS cert verification (self-signed servers)
  timeout_seconds?: number; // http/sse: per-request timeout in seconds (default 120)
  last_connect_at?: string | null; // last connect attempt (ISO time)
  last_connect_ok?: boolean | null; // outcome of the last connect attempt
  last_connect_err?: string | null; // failure reason when last_connect_ok is false
  tools?: string[]; // mcp_tools_cache (names only, for the count)
}

export interface MCPTool {
  name: string;
  description: string;
}

// ---- Skills ----
// Fields align with the agentskills.io open specification.
// description covers both "what the skill does" and "when to use it".
export interface SkillItem {
  name: string; // unique key = directory name
  description?: string; // required per spec; covers what + when to use
  license?: string; // optional: SPDX identifier or free text
  compatibility?: string; // optional: environment requirements
  mcps?: string[]; // MCP server names this skill unlocks on load
  files: string[]; // files in the skill directory
  // Call stats (the skill_usage ledger). A skill never called: calls=0, last_used absent.
  calls: number;
  tasks: number; // number of tasks that loaded it (chat sessions excluded)
  usage_agents: string[]; // agent keys that loaded it
  last_used?: string;
}

// SkillCall is one Skill() invocation (recent-call list for a single skill).
export interface SkillCall {
  ts: string;
  agent_key: string;
  task_id: number; // 0 = non-task context (chat session)
  session_id: string;
  args_len: number;
}

// MissingSkill is a skill that was named but does not exist — the "wanted but missing" gap.
export interface MissingSkill {
  skill: string;
  calls: number;
  agents: string[];
  last_used?: string;
}

// ---- Tools (built-in tool catalog) ----
// key + handler live in Go; only these fields are page-editable. system tools lock
// the key and the parameter *structure* (name/type/required) — the per-param
// description/default and the agent binding are what move.
export interface Tool {
  key: string;
  system: boolean;
  description: string;
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  schema: Record<string, unknown>; // full JSON-Schema (object with properties)
  agents: string[]; // bound agent keys
  enabled: boolean;
  kind?: "builtin" | "shell" | "command" | "script" | "http"; // custom tool kind
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  exec?: Record<string, unknown>; // custom tool execution spec (kind!=builtin)
  deferred?: boolean; // schema deferred (SearchExtraTools/ExecuteExtraTool)
  calls?: number; // persistent runtime invocation count (older APIs may omit it)
}

// ---- Stats ----
export interface Stats {
  assets: number;
  engine_mode: EngineMode;
  llm_configured: boolean;
  roe_enabled: boolean;
  findings_confirmed: number;
  active_task?: Partial<Task>;
}
