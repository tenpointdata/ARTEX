import { t } from "@/i18n";
import type { Tool } from "@/lib/types";

// ---- tool scope ----

// SDK tools are intentionally not seeded into the DB (they apply to every agent
// and have no per-agent binding). We hardcode them here so they still appear in
// the scope dialog.
function sdkTool(key: string, description: string): Tool {
  return { key, system: true, description, schema: {}, agents: [], enabled: true, kind: "builtin" };
}

const SDK_EXEC: Tool[] = [
  sdkTool("Bash", t("Execute commands in a shell")),
  sdkTool("WebFetch", t("Make HTTP/HTTPS requests (with proxy support)")),
  sdkTool("web_search", t("Web search")),
  sdkTool("shell_open", t("Open a persistent PTY interactive session")),
  sdkTool("shell_send", t("Send input to an interactive session")),
  sdkTool("shell_read", t("Read output from an interactive session")),
  sdkTool("shell_close", t("Close an interactive session")),
  sdkTool("shell_list", t("List all interactive sessions")),
];

const SDK_WRITE: Tool[] = [
  sdkTool("Write", t("Write files")),
  sdkTool("Edit", t("Edit files (exact replacement)")),
  sdkTool("MultiEdit", t("Edit files in batch")),
];

const SDK_KEYS = new Set([...SDK_EXEC, ...SDK_WRITE].map((tool) => tool.key));

export function groupTools(dbTools: Tool[]) {
  const sys: Tool[] = [],
    custom: Tool[] = [];
  for (const tool of dbTools) {
    if (SDK_KEYS.has(tool.key)) continue; // already covered by hardcoded groups
    if (tool.system) sys.push(tool);
    else custom.push(tool);
  }
  return [
    { label: t("Execution"), tools: SDK_EXEC },
    { label: t("Write/Edit"), tools: SDK_WRITE },
    { label: t("System tools"), tools: sys },
    { label: t("Custom tools"), tools: custom },
  ].filter((g) => g.tools.length > 0);
}
