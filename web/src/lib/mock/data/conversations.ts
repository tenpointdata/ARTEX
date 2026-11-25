import type { Activity, Conversation } from "@/lib/types";

const T = (iso: string) => iso; // readability helper for timestamps

export const conversations: Conversation[] = [
  {
    id: 1,
    agent_key: "mainagent",
    title: "Discussion on the acme admin entry point",
    llm_profile_id: 1,
    pinned: true,
    pinned_at: T("2026-11-14T04:00:00Z"),
    created_at: T("2026-11-13T10:00:00Z"),
    updated_at: T("2026-11-14T03:40:00Z"),
  },
  {
    id: 2,
    agent_key: "mainagent",
    title: "Mapping the API authorization bypass surface",
    llm_profile_id: 2,
    created_at: T("2026-11-12T16:00:00Z"),
    updated_at: T("2026-11-12T17:20:00Z"),
  },
  {
    id: 3,
    agent_key: "mainagent",
    title: "Internal lateral movement · DMZ→domain controller attack path planning",
    llm_profile_id: 1,
    created_at: T("2026-11-13T15:45:00Z"),
    updated_at: T("2026-11-14T00:25:00Z"),
  },
];

export const conversationMessages: Record<number, Activity[]> = {
  1: [
    {
      seq: 1,
      worker: "user",
      ts: T("2026-11-13T10:00:00Z"),
      kind: "user",
      summary: "Where is the admin console, and where should we start?",
    },
    {
      seq: 2,
      worker: "mainagent",
      ts: T("2026-11-13T10:00:04Z"),
      kind: "thinking",
      summary:
        "First confirm the admin.acme.com login page's stack and protections (captcha/rate limit), then decide between default credentials and dictionaries.",
    },
    {
      seq: 3,
      worker: "mainagent",
      ts: T("2026-11-13T10:00:06Z"),
      kind: "tool_use",
      tool: "bash",
      tool_use_id: "c11",
      summary: "curl -s https://admin.acme.com/login | grep -Eo 'element-ui|captcha|csrf'",
    },
    {
      seq: 4,
      worker: "mainagent",
      ts: T("2026-11-13T10:00:08Z"),
      kind: "tool_result",
      tool: "bash",
      tool_use_id: "c11",
      summary: "element-ui\ncsrf",
      detail: "Element-UI admin console; has a CSRF token but no captcha.",
    },
    {
      seq: 5,
      worker: "mainagent",
      ts: T("2026-11-13T10:00:20Z"),
      kind: "tool_use",
      tool: "bash",
      tool_use_id: "c12",
      summary:
        "for i in 1 2 3 4 5; do curl -s -o /dev/null -w '%{http_code}\\n' -d 'username=x&password=y' https://admin.acme.com/login; done",
    },
    {
      seq: 6,
      worker: "mainagent",
      ts: T("2026-11-13T10:00:24Z"),
      kind: "tool_result",
      tool: "bash",
      tool_use_id: "c12",
      summary: "401\\n401\\n401\\n401\\n401",
      detail: "5 consecutive attempts with no lockout/captcha → no rate limit; brute-force is viable.",
    },
    {
      seq: 7,
      worker: "mainagent",
      ts: T("2026-11-13T10:00:30Z"),
      kind: "text",
      summary:
        "Conclusion: Element-UI admin console, no captcha, no rate limit. Entry order: (1) run the default credential list first (admin/admin, admin/admin123, admin/123456); (2) fall back to the top1000 dictionary. I have injected 'admin default credential testing' as a high-priority intent for the work agent.",
    },
    { seq: 8, worker: "user", ts: T("2026-11-14T03:40:00Z"), kind: "user", summary: "Did it work?" },
    {
      seq: 9,
      worker: "mainagent",
      ts: T("2026-11-14T03:40:05Z"),
      kind: "text",
      summary:
        "It worked — admin/admin123 logged straight into the console; recorded as a High finding and proved the 'Gain admin access' goal is met. Next, pivot to reading user data (goal 2).",
    },
  ],
  2: [
    {
      seq: 1,
      worker: "user",
      ts: T("2026-11-12T16:00:00Z"),
      kind: "user",
      summary: "Help me map api.acme.com's authorization bypass surface.",
    },
    {
      seq: 2,
      worker: "mainagent",
      ts: T("2026-11-12T16:00:05Z"),
      kind: "thinking",
      summary:
        "First probe the API's authentication scheme and a few key endpoint behaviors, then assess IDOR / JWT risks.",
    },
    {
      seq: 3,
      worker: "mainagent",
      ts: T("2026-11-12T16:00:10Z"),
      kind: "tool_use",
      tool: "bash",
      tool_use_id: "c21",
      summary: "curl -s https://api.acme.com/v1/users/me -H 'Authorization: Bearer <self>' | jq '{id,role}'",
    },
    {
      seq: 4,
      worker: "mainagent",
      ts: T("2026-11-12T16:00:12Z"),
      kind: "tool_result",
      tool: "bash",
      tool_use_id: "c21",
      summary: '{ "id": 42, "role": "user" }',
      detail: "A regular user token works.",
    },
    {
      seq: 5,
      worker: "mainagent",
      ts: T("2026-11-12T16:00:30Z"),
      kind: "tool_use",
      tool: "bash",
      tool_use_id: "c22",
      summary: "echo '<jwt>' | cut -d. -f1 | base64 -d; # inspect the algorithm",
    },
    {
      seq: 6,
      worker: "mainagent",
      ts: T("2026-11-12T16:00:31Z"),
      kind: "tool_result",
      tool: "bash",
      tool_use_id: "c22",
      summary: '{"alg":"HS256","typ":"JWT"}',
      detail: "HS256 symmetric signing; a weak key can be cracked offline to forge tokens.",
    },
    {
      seq: 7,
      worker: "mainagent",
      ts: T("2026-11-12T16:01:00Z"),
      kind: "text",
      summary:
        "Two authorization bypass surfaces: (1) /v1/orders?id= is an auto-increment numeric id → typical IDOR; change the id to try reading others' orders; (2) the JWT uses HS256; try john/hashcat to crack the key → any sub can be forged to bypass authorization. I have dispatched one intent for each. Doing IDOR first is recommended (low cost, direct impact).",
    },
  ],
  3: [
    {
      seq: 1,
      worker: "user",
      ts: T("2026-11-13T15:45:00Z"),
      kind: "user",
      summary: "Got the shell; how do we attack the internal network next?",
    },
    {
      seq: 2,
      worker: "mainagent",
      ts: T("2026-11-13T15:45:05Z"),
      kind: "thinking",
      summary:
        "www-data is insufficient for internal lateral movement; check local privilege escalation first, then do alive-host discovery with the DMZ as pivot after getting root.",
    },
    {
      seq: 3,
      worker: "mainagent",
      ts: T("2026-11-13T15:45:10Z"),
      kind: "text",
      summary:
        "Escalate first: sudo -l has passwordless python3, so go straight to root. Then use the DMZ as a springboard for internal alive-host scanning (fscan), focusing on the domain controller and CI/CD servers.",
    },
    {
      seq: 4,
      worker: "user",
      ts: T("2026-11-13T21:05:00Z"),
      kind: "user",
      summary: "The internal scan found 3 hosts; Jenkins 8080 is open — promising?",
    },
    {
      seq: 5,
      worker: "mainagent",
      ts: T("2026-11-13T21:05:08Z"),
      kind: "tool_use",
      tool: "bash",
      tool_use_id: "c31",
      summary: "proxychains curl -s http://10.10.10.20:8080/script -o /dev/null -w '%{http_code}'",
    },
    {
      seq: 6,
      worker: "mainagent",
      ts: T("2026-11-13T21:05:12Z"),
      kind: "tool_result",
      tool: "bash",
      tool_use_id: "c31",
      summary: "200",
      detail: "/script is directly accessible without authentication.",
    },
    {
      seq: 7,
      worker: "mainagent",
      ts: T("2026-11-13T21:05:20Z"),
      kind: "text",
      summary:
        "Promising — the Script Console is unauthenticated and gives direct Groovy RCE. I have injected a hint for the worker to hit this box first. Once taken, watch the credentials store; it may hold domain accounts.",
    },
    {
      seq: 8,
      worker: "user",
      ts: T("2026-11-13T23:25:00Z"),
      kind: "user",
      summary: "Got the domain account; svc_deploy is in Domain Admins — go straight for the domain controller?",
    },
    {
      seq: 9,
      worker: "mainagent",
      ts: T("2026-11-13T23:25:10Z"),
      kind: "text",
      summary:
        "Yes. svc_deploy + Domain Admins = psexec straight to DC01. Run secretsdump afterward to export all domain hashes and wrap up. Note that secretsdump triggers interception rules; I have pre-allowed it for you.",
    },
    {
      seq: 10,
      worker: "user",
      ts: T("2026-11-14T00:22:00Z"),
      kind: "user",
      summary: "Target taken; summarize the attack chain.",
    },
    {
      seq: 11,
      worker: "mainagent",
      ts: T("2026-11-14T00:22:12Z"),
      kind: "text",
      summary:
        "Full kill chain:\n1. .git leak → white-box source + hardcoded credentials\n2. shop Fastjson RCE → DMZ reverse shell\n3. sudo escalation → root foothold\n4. internal discovery → DC01/FS01/Jenkins\n5. unauthenticated Jenkins → exported domain credentials svc_deploy\n6. Kerberoasting → cracked svc_sql\n7. svc_deploy ∈ Domain Admins → psexec DC01 → secretsdump all domain hashes\n\nAll four goals met; 17 findings (7 High/4 Medium/2 Low + 8 internal-chain).",
    },
  ],
};

// ── Asset test coverage (/tasks/{id}/coverage) ──
