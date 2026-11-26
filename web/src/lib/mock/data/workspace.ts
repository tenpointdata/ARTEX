const WS_TREE: Record<string, { name: string; dir: boolean; size: number; content?: string }[]> = {
  "": [
    { name: "t-001", dir: true, size: 0 },
    { name: "transcripts", dir: true, size: 0 },
    {
      name: "notes.md",
      dir: false,
      size: 96,
      content:
        "# Workspace notes\n\n(demo) This is a sample file in the workspace root that can be edited and saved online.\n",
    },
  ],
  "t-001": [
    { name: "i12", dir: true, size: 0 },
    { name: "recon.txt", dir: false, size: 64, content: "acme.com\nwww.acme.com\napi.acme.com\nadmin.acme.com\n" },
  ],
  "t-001/i12": [
    {
      name: "exploit.py",
      dir: false,
      size: 220,
      content:
        "#!/usr/bin/env python3\n# (demo) SQLi PoC — www.acme.com/search?q=\nimport requests\n\nr = requests.get('https://www.acme.com/search', params={'q': \"1' OR '1'='1\"})\nprint(r.status_code, len(r.text))\n",
    },
    {
      name: "response.html",
      dir: false,
      size: 180,
      content: "<!-- (demo) captured response body fragment -->\n<html><body>MySQL error near ''1'='1'</body></html>\n",
    },
  ],
  transcripts: [
    {
      name: "exp1-worker-i12.jsonl",
      dir: false,
      size: 512,
      content: "(demo) Sample raw LLM conversation log, omitted here.",
    },
  ],
};

const wsClean = (p: string) => p.replace(/^\/+|\/+$/g, "");

export function workspaceList(path: string) {
  const key = wsClean(path);
  const items = WS_TREE[key] ?? [];
  const now = Date.parse("2026-11-14T03:00:00Z");
  return {
    path: key,
    entries: items.map((it, i) => ({
      name: it.name,
      path: key ? `${key}/${it.name}` : it.name,
      dir: it.dir,
      size: it.size,
      mtime: now - i * 3_600_000,
    })),
  };
}

export function workspaceRead(path: string) {
  const key = wsClean(path);
  const slash = key.lastIndexOf("/");
  const parent = slash >= 0 ? key.slice(0, slash) : "";
  const name = slash >= 0 ? key.slice(slash + 1) : key;
  const f = (WS_TREE[parent] ?? []).find((x) => x.name === name && !x.dir);
  return { path: key, size: f?.size ?? 0, binary: false, content: f?.content ?? "" };
}

// ── Tool execution history (/commands) ──
