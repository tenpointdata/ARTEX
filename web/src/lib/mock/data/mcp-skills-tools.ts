import type { MCPServer, MCPTool, MissingSkill, SkillCall, SkillItem, Tool } from "@/lib/types";

export const mcpServers: MCPServer[] = [
  {
    id: 1,
    name: "shodan",
    transport: "http",
    url: "https://mcp.shodan.io/sse",
    args: [],
    env: {},
    enabled: true,
    tools: ["host_info", "search", "dns_resolve"],
  },
  {
    id: 2,
    name: "playwright",
    transport: "stdio",
    command: "npx",
    args: ["-y", "@playwright/mcp"],
    env: {},
    enabled: true,
    tools: ["browser_navigate", "browser_click", "browser_snapshot"],
  },
];

export const mcpToolsById: Record<number, MCPTool[]> = {
  1: [
    { name: "host_info", description: "Query Shodan host info for an IP" },
    { name: "search", description: "Shodan search" },
    { name: "dns_resolve", description: "DNS resolution" },
  ],
  2: [
    { name: "browser_navigate", description: "Open a URL in the browser" },
    { name: "browser_click", description: "Click an element" },
    { name: "browser_snapshot", description: "Capture an accessibility snapshot" },
  ],
};

// ── Skills ───────────────────────────────────────────────────────────────────
export const skills: SkillItem[] = [
  {
    name: "api-recon",
    description:
      "Reconnaissance and authorization bypass surface enumeration on REST/GraphQL APIs; use when new API endpoints are discovered.",
    license: "MIT",
    mcps: [],
    files: ["SKILL.md", "scripts/enum.py"],
    calls: 24,
    tasks: 6,
    usage_agents: ["worker", "planner"],
    last_used: "2026-11-14T09:12:00Z",
  },
  {
    name: "playwright-cli",
    description:
      "Drives a browser with Playwright for dynamic crawling and screenshots; use when JS-rendered sites need rendering.",
    mcps: ["playwright"],
    files: ["SKILL.md"],
    calls: 7,
    tasks: 3,
    usage_agents: ["worker"],
    last_used: "2026-11-13T21:40:00Z",
  },
  {
    name: "scopesentry",
    description:
      "Pulls assets from ScopeSentry and merges them into the company scope; use when importing assets in bulk.",
    files: ["SKILL.md", "assets/mapping.md"],
    calls: 0,
    tasks: 0,
    usage_agents: [],
  },
];

export const skillCalls: SkillCall[] = [
  { ts: "2026-11-14T09:12:00Z", agent_key: "worker", task_id: 42, session_id: "", args_len: 128 },
  { ts: "2026-11-14T08:03:00Z", agent_key: "planner", task_id: 42, session_id: "", args_len: 0 },
  { ts: "2026-11-13T17:55:00Z", agent_key: "worker", task_id: 37, session_id: "", args_len: 64 },
];

export const missingSkills: MissingSkill[] = [
  { skill: "jwt-forge", calls: 3, agents: ["worker"], last_used: "2026-11-14T07:20:00Z" },
];

// ── Tools ────────────────────────────────────────────────────────────────────
export const tools: Tool[] = [
  {
    key: "bash",
    system: true,
    description: "Runs shell commands in the Kali environment (subject to scope/destructive gates)",
    schema: { type: "object", properties: { command: { type: "string" } }, required: ["command"] },
    agents: ["worker", "mainagent"],
    enabled: true,
    kind: "builtin",
    calls: 1842,
  },
  {
    key: "report_finding",
    system: true,
    description: "Reports a security finding and links it to related nodes",
    schema: {
      type: "object",
      properties: {
        vulnclass: { type: "string" },
        severity: { type: "string" },
        summary: { type: "string" },
        evidence: { type: "string" },
      },
      required: ["vulnclass", "severity", "summary"],
    },
    agents: ["worker"],
    enabled: true,
    kind: "builtin",
    calls: 96,
  },
  {
    key: "upsert_asset",
    system: true,
    description: "Writes/updates asset nodes",
    schema: { type: "object", properties: { type: { type: "string" }, value: { type: "string" } } },
    agents: ["worker"],
    enabled: true,
    kind: "builtin",
    calls: 417,
  },
  {
    key: "list_goals",
    system: true,
    description: "Lists the current task's goals and completion status",
    schema: { type: "object", properties: {} },
    agents: ["planner"],
    enabled: true,
    kind: "builtin",
    calls: 238,
  },
  {
    key: "nuclei_scan",
    system: false,
    description: "Custom: runs specified templates with nuclei",
    schema: {
      type: "object",
      properties: { target: { type: "string" }, template: { type: "string" } },
      required: ["target"],
    },
    agents: ["worker"],
    enabled: true,
    kind: "command",
    exec: { command: "nuclei", args: ["-u", "{{target}}"] },
    calls: 31,
  },
];

// ── Settings ─────────────────────────────────────────────────────────────────
