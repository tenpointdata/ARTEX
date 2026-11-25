// Rich mock dataset for the Vercel demo (NEXT_PUBLIC_MOCK=1). Shapes track the
// current backend contract in lib/api.ts + lib/types.ts. All data is static and
// self-consistent (one company "Acme", a few tasks, findings, exploration graph,
// sessions/activity, traffic, agents, etc.) so every page looks populated.

export * from "./data/activity";
export * from "./data/agents";
export * from "./data/assets";
export * from "./data/audit";
export * from "./data/conversations";
export * from "./data/coverage";
export * from "./data/findings";
export * from "./data/graph";
export * from "./data/intercept";
export * from "./data/llm";
export * from "./data/mcp-skills-tools";
export * from "./data/records";
export * from "./data/settings";
export * from "./data/tasks";
export * from "./data/tokens";
export * from "./data/traffic";
export * from "./data/workspace";
