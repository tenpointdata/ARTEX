import { intlLocale, t } from "@/i18n";

import type { TreeNode } from "./types";

export function fmtTime(ts?: string) {
  if (!ts) return t("Never called");
  return new Date(ts).toLocaleString(intlLocale, {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  });
}

// Backend returns dirs as "scripts/" (trailing slash) and files as "scripts/a.py".
export function buildTree(entries: string[]): TreeNode[] {
  const root: TreeNode[] = [];
  const dirMap = new Map<string, TreeNode>();

  function ensureDir(dirPath: string, parentNodes: TreeNode[]): TreeNode[] {
    if (dirMap.has(dirPath)) return dirMap.get(dirPath)!.children;
    const parts = dirPath.split("/");
    let nodes = root;
    let cur = "";
    for (const part of parts) {
      cur = cur ? `${cur}/${part}` : part;
      if (!dirMap.has(cur)) {
        const d: TreeNode = { name: part, path: cur, type: "dir", children: [] };
        nodes.push(d);
        dirMap.set(cur, d);
      }
      nodes = dirMap.get(cur)!.children;
    }
    return nodes;
  }

  for (const entry of [...entries].sort()) {
    if (entry.endsWith("/")) {
      // Explicit directory entry — ensure the node exists (may already be created)
      ensureDir(entry.slice(0, -1), root);
    } else {
      // File — ensure parent dirs exist, then push file node
      const parts = entry.split("/");
      let nodes = root;
      let cur = "";
      for (let i = 0; i < parts.length - 1; i++) {
        cur = cur ? `${cur}/${parts[i]}` : parts[i];
        if (!dirMap.has(cur)) {
          const d: TreeNode = { name: parts[i], path: cur, type: "dir", children: [] };
          nodes.push(d);
          dirMap.set(cur, d);
        }
        nodes = dirMap.get(cur)!.children;
      }
      const fname = parts[parts.length - 1];
      if (fname) nodes.push({ name: fname, path: entry, type: "file", children: [] });
    }
  }
  sortNodes(root);
  return root;
}

// sortNodes orders every level like a file explorer: directories before files,
// then case-insensitive by name. Recurses into children.
function sortNodes(nodes: TreeNode[]): void {
  nodes.sort((a, b) => {
    if (a.type !== b.type) return a.type === "dir" ? -1 : 1;
    return a.name.localeCompare(b.name, undefined, { sensitivity: "base" });
  });
  for (const n of nodes) {
    if (n.children.length > 0) sortNodes(n.children);
  }
}
