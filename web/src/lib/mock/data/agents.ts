import type { Agent, AgentDetail, PromptVar, PromptVersion } from "@/lib/types";

const T = (iso: string) => iso; // readability helper for timestamps

export const agents: Agent[] = [
  {
    id: "1001",
    key: "retester",
    name: "Finding retest",
    role: "assistant",
    builtin: false,
    enabled: true,
    description: "Started manually from the finding detail; saves an independent retest conclusion",
    max_turns: 0,
    mcp_count: 0,
    skill_count: 0,
    tool_count: 2,
  },
  {
    id: "1",
    key: "goals",
    name: "Goal decomposer",
    role: "goals",
    builtin: true,
    enabled: true,
    description: "Decomposes task goals into explorable sub-goals",
    max_turns: 8,
    mcp_count: 0,
    skill_count: 1,
    tool_count: 3,
  },
  {
    id: "2",
    key: "planner",
    name: "Planner",
    role: "planner",
    builtin: true,
    enabled: true,
    description: "Reads exploration routes, judges goal completion, generates intents",
    max_turns: 0,
    mcp_count: 1,
    skill_count: 2,
    tool_count: 9,
  },
  {
    id: "3",
    key: "mainagent",
    name: "Main agent",
    role: "mainagent",
    builtin: true,
    enabled: true,
    description: "Human-in-the-loop conversations; injects hints / high-priority intents",
    max_turns: 0,
    web_search: true,
    mcp_count: 2,
    skill_count: 3,
    tool_count: 11,
  },
  {
    id: "4",
    key: "worker",
    name: "Work Agent",
    role: "worker",
    builtin: true,
    enabled: true,
    description: "Claims intents → runs Kali tools → writes back to the graph",
    max_turns: 40,
    run_seconds: 1800,
    web_search: false,
    interactive_shell: true,
    mcp_count: 2,
    skill_count: 3,
    tool_count: 14,
  },
  {
    id: "5",
    key: "recon-bot",
    name: "Recon bot (custom)",
    role: "custom",
    builtin: false,
    enabled: true,
    description: "Periodically runs passive reconnaissance on new assets",
    max_turns: 12,
    mcp_count: 1,
    skill_count: 1,
    tool_count: 4,
  },
];

const promptVars: PromptVar[] = [
  { name: "Goal", description: "Current task goal", example: "Gain admin access to acme.com", source: "runtime" },
  {
    name: "AssetSummary",
    description: "Asset graph overview",
    example: "6 subdomains / 3 IPs / 4 apps / 4 endpoints",
    source: "distilled",
  },
  {
    name: "RouteHint",
    description: "Exploration route hints",
    example: "Prioritize the admin and API authorization bypass surfaces",
    source: "exploration",
  },
];

const promptVersions: PromptVersion[] = [
  {
    version: 3,
    ts: T("2026-11-13T10:00:00Z"),
    note: "Strengthen authorization bypass probing guidance",
    template_text: "You are the ARTEX planner...",
  },
  {
    version: 2,
    ts: T("2026-11-08T10:00:00Z"),
    note: "Initial minor tuning",
    template_text: "You are the ARTEX planner (v2)...",
  },
];

export function agentDetail(key: string): AgentDetail {
  const a = agents.find((x) => x.key === key) ?? agents[1];
  return {
    agent: a,
    prompt: `You are the ARTEX "${a.name}".\nGoal: {{.Goal}}\nAsset overview: {{.AssetSummary}}\nRoute hint: {{.RouteHint}}\nProceed with the exploration based on the above and write the results back to the graph via tools.`,
    variables: promptVars,
    versions: promptVersions,
    visibility: { mcp: [1, 2], skill: ["api-recon", "playwright-cli"] },
    wrapup_prompt: "",
    wrapup_default: "Time/steps are running out; summarize the confirmed findings and mark intent final states.",
    wrapup_max_turns: 0,
    wrapup_max_turns_default: 3,
    task_timeout_wrapup_supported: a.key === "worker" || a.key === "planner",
    task_timeout_wrapup_default: "Task timed out; wrap up immediately and persist the current conclusions.",
    task_timeout_wrapup_max_turns_default: 2,
  };
}

// ── MCP ──────────────────────────────────────────────────────────────────────
