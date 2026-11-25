import type { Activity } from "@/lib/types";

const T = (iso: string) => iso; // readability helper for timestamps

export const activityPlanner: Activity[] = [
  // planner — the full decision chain each round: read the situation → pull worker outputs for comparison → judge goals → dispatch direction via add_intent
  // ── Round 0 · Goal decomposition and the first intents (2026-11-12 09:12) ──
  {
    seq: 1000,
    worker: "planner",
    ts: T("2026-11-12T09:12:10Z"),
    kind: "round",
    summary: "Round 0 · Goal decomposition",
  },
  {
    seq: 1001,
    worker: "planner",
    ts: T("2026-11-12T09:12:25Z"),
    kind: "thinking",
    summary:
      "graph_overview: goals are already decomposed by the goal layer — g1 'Gain admin access to acme.com', g2 'Read user sensitive data', both open; facts=0, recent_facts is empty (the graph is essentially blank, with no worker output yet). Assets in scope are just the root domain acme.com + one /24 segment. Initial attack-surface read: five entries — www / admin / api / shop / vpn. There is no fact in the graph, so I can only do [ultra-light read-only probing] to make the initial intents concrete, and never do the actual work in a plan.",
  },
  {
    seq: 1002,
    worker: "planner",
    ts: T("2026-11-12T09:12:40Z"),
    kind: "tool_use",
    tool: "bash",
    tool_use_id: "p0-probe",
    summary: "curl -sI https://acme.com; curl -sI https://admin.acme.com",
    detail:
      "The opening situation is nearly blank; do ≤3 read-only probes to calibrate the initial intent directions (no enumeration/validation — that is the worker's job).",
  },
  {
    seq: 1003,
    worker: "planner",
    ts: T("2026-11-12T09:12:52Z"),
    kind: "tool_result",
    tool: "bash",
    tool_use_id: "p0-probe",
    summary:
      "acme.com → 302 → www; Server: nginx/1.24.0\nadmin.acme.com → 200, X-Powered-By absent, body matches element-ui/vue fingerprints",
    detail:
      "admin is a standalone admin login page; the five entries are confirmed valid, so the initial intent descriptions can be made precise.",
  },
  {
    seq: 1004,
    worker: "planner",
    ts: T("2026-11-12T09:13:00Z"),
    kind: "tool_use",
    tool: "add_intent",
    tool_use_id: "p0-ai",
    summary: "add_intent × 4 (batch-dispatch the first exploration directions)",
    detail:
      "intents=[\n  { summary: 'Full subdomain enumeration of acme.com + port/service identification of alive hosts to map the attack surface', asset_ids: [acme.com], priority: 6 },\n  { summary: 'admin.acme.com admin console default/weak credential testing (Element-UI login, no captcha)', asset_ids: [admin], priority: 9 },\n  { summary: 'SQL injection probing of the www.acme.com/search q parameter', asset_ids: [www], priority: 8 },\n  { summary: 'api.acme.com order API authorization bypass (IDOR) testing', asset_ids: [api], priority: 8 },\n]\nparent_ids all empty (brand-new top-level directions, fall back to linking the origin fact).",
  },
  {
    seq: 1005,
    worker: "planner",
    ts: T("2026-11-12T09:13:02Z"),
    kind: "tool_result",
    tool: "add_intent",
    tool_use_id: "p0-ai",
    summary: "4 intents created → ids=[i1, i2, i4, i5], linked into the frontier",
    detail:
      "Deliberately keeping the routes diverse: i1 reconnaissance + i2 (g1 credential route) + i4/i5 (g2 data route), two independent mechanisms in parallel to avoid premature convergence.",
  },
  {
    seq: 1006,
    worker: "planner",
    ts: T("2026-11-12T09:13:05Z"),
    kind: "text",
    summary:
      "This round derived the first 4 intents: i1 asset reconnaissance, i2 admin credentials, i4 injection, i5 API authorization bypass. g1 goes through i2 and g2 through i4/i5; both goals have independent entries in progress. Escalate along lineage once workers produce facts.",
  },

  // ── Round 1 · Attack surface escalation (2026-11-13 14:12) ──
  {
    seq: 1100,
    worker: "planner",
    ts: T("2026-11-13T14:12:00Z"),
    kind: "round",
    summary: "Round 1 · Attack surface escalation",
  },
  {
    seq: 1101,
    worker: "planner",
    ts: T("2026-11-13T14:12:15Z"),
    kind: "thinking",
    summary:
      "Comparing recent_facts: workG returned faGit '.git is downloadable; git-dumper recovered the backend source' and faCreds 'config.php hardcodes sa/Acme@2021 (suspected shared internally)'; work#6's fingerprinting returned fa3 'shop matched Fastjson 1.2.24 (known deserialization RCE)'. Two [materially new mechanisms]: (1) the hardcoded DB password may be reused across the internal network; (2) Fastjson has a mature RCE playbook. This opens an entry [essentially different] from the existing credential/injection routes — breaking into the DMZ directly via RCE on shop. Pull faGit details first to verify the artifacts.",
  },
  {
    seq: 1102,
    worker: "planner",
    ts: T("2026-11-13T14:12:28Z"),
    kind: "tool_use",
    tool: "node_detail",
    tool_use_id: "p1-nd",
    summary: "node_detail(faGit) — verifying workG's source recovery artifacts",
    detail:
      "Before add_intent, check what the worker actually recovered, to avoid guessing directions from fingerprints.",
  },
  {
    seq: 1103,
    worker: "planner",
    ts: T("2026-11-13T14:12:35Z"),
    kind: "tool_result",
    tool: "node_detail",
    tool_use_id: "p1-nd",
    summary:
      "faGit evidence: git-dumper recovered config.php / db.php / order_api.php; config.php contains $db=['host'=>'10.10.10.5','user'=>'sa','pass'=>'Acme@2021']",
    detail:
      "The hardcoding points to the internal 10.10.10.5 (FS01), corroborating faCreds' 'shared internally' inference — the credentials are worth reusing once the RCE lands.",
  },
  {
    seq: 1104,
    worker: "planner",
    ts: T("2026-11-13T14:12:50Z"),
    kind: "tool_use",
    tool: "add_intent",
    tool_use_id: "p1-ai",
    summary: "add_intent × 2 (Fastjson breakthrough chain; serial steps, dispatch the executable step first)",
    detail:
      "intents=[\n  { summary: 'Deep-dive shop.acme.com component fingerprinting + precise Fastjson 1.2.24 CVE correlation (confirm the gadget chain)', asset_ids: [shop], parent_ids: [fa3], priority: 7 },\n  { summary: 'Build a Fastjson deserialization payload to trigger the shop RCE and obtain a reverse shell', asset_ids: [shop], parent_ids: [fa3], priority: 9 },\n]\nNote: this is a strongly dependent serial chain; TodoWrite already recorded the three steps 'fingerprint → RCE → foothold escalation', and this round dispatches only the first two (escalation depends on the shell output, dispatch next round).",
  },
  {
    seq: 1105,
    worker: "planner",
    ts: T("2026-11-13T14:12:52Z"),
    kind: "tool_result",
    tool: "add_intent",
    tool_use_id: "p1-ai",
    summary: "2 intents created → ids=[i6, i7], linked into lineage via fa3",
  },
  {
    seq: 1106,
    worker: "planner",
    ts: T("2026-11-13T14:13:00Z"),
    kind: "text",
    summary:
      "Situation escalated: source code + hardcoded internal credentials + Fastjson RCE form a brand-new breakthrough route. The goal layer has been asked to register g3 'Break through the DMZ from the outside and establish an internal foothold', dispatching i6 (fingerprint/CVE correlation) and i7 (trigger the RCE). The DMZ escalation step depends on the shell and is recorded as a todo for the next round.",
  },

  // ── Round 2 · Foothold established, pivoting internal (2026-11-13 20:25) ──
  {
    seq: 1200,
    worker: "planner",
    ts: T("2026-11-13T20:25:00Z"),
    kind: "round",
    summary: "Round 2 · Foothold established, pivoting internal",
  },
  {
    seq: 1201,
    worker: "planner",
    ts: T("2026-11-13T20:25:18Z"),
    kind: "thinking",
    summary:
      "Comparing lineage: i7 returned fa4 'reverse shell www-data@dmz-web01(10.0.20.15), in the DMZ segment'; the escalation chain returned fa5 'sudo -l matched NOPASSWD python3 → escalation to root possible' and fa6 'the foothold's second NIC connects directly to the internal 10.10.10.0/24'. g3's acceptance conditions (break through the DMZ + establish a foothold) are directly satisfied by fa4+fa6. Verify fa6's internal reachability first, then prove_goal.",
  },
  {
    seq: 1202,
    worker: "planner",
    ts: T("2026-11-13T20:25:30Z"),
    kind: "tool_use",
    tool: "node_detail",
    tool_use_id: "p2-nd",
    summary: "node_detail(fa6) — confirming dual NICs and internal reachability",
  },
  {
    seq: 1203,
    worker: "planner",
    ts: T("2026-11-13T20:25:38Z"),
    kind: "tool_result",
    tool: "node_detail",
    tool_use_id: "p2-nd",
    summary:
      "fa6 evidence: ip a → eth0 10.0.20.15 (DMZ) / eth1 10.10.10.15 (internal); ping 10.10.10.10 succeeds, crossing the DMZ boundary confirmed",
  },
  {
    seq: 1204,
    worker: "planner",
    ts: T("2026-11-13T20:25:50Z"),
    kind: "tool_use",
    tool: "prove_goal",
    tool_use_id: "p2-pg",
    summary: "prove_goal(g3, evidence=fa6)",
    detail:
      "reason: i7's RCE gained the DMZ foothold (fa4), fa5 escalated to root, and fa6 proved the second NIC connects directly to the internal network and can ping DC01 — the acceptance for 'Break through the DMZ and establish an internal foothold' is met.",
  },
  {
    seq: 1205,
    worker: "planner",
    ts: T("2026-11-13T20:25:52Z"),
    kind: "tool_result",
    tool: "prove_goal",
    tool_use_id: "p2-pg",
    summary: "goal g3 → met (evidence fa6 linked); g1/g2/g4 still unmet, the task continues",
  },
  {
    seq: 1206,
    worker: "planner",
    ts: T("2026-11-13T20:26:10Z"),
    kind: "tool_use",
    tool: "add_intent",
    tool_use_id: "p2-ai",
    summary: "add_intent × 4 (expand internal lateral movement with the foothold as pivot)",
    detail:
      "intents=[\n  { summary: 'Use dmz-web01 as pivot for host discovery and port identification on the internal 10.10.10.0/24', asset_ids: [dmz-web01], parent_ids: [fa6], priority: 8 },\n  { summary: 'Internal lateral movement: reuse the hardcoded password sa/Acme@2021 for credential reuse and password spraying', parent_ids: [faCreds], priority: 8 },\n  { summary: 'Unauthenticated /script Groovy execution (RCE) on the internal Jenkins(10.10.10.20)', parent_ids: [fa6], priority: 9 },\n  { summary: 'Kerberoast domain service accounts and crack them offline', parent_ids: [fa6], priority: 8 },\n]\nHost discovery is a prerequisite (dispatched first); the other three are parallel lateral exploration dimensions.",
  },
  {
    seq: 1207,
    worker: "planner",
    ts: T("2026-11-13T20:26:12Z"),
    kind: "tool_result",
    tool: "add_intent",
    tool_use_id: "p2-ai",
    summary: "4 intents created → ids=[i9, i10, i11, i12]",
  },
  {
    seq: 1208,
    worker: "planner",
    ts: T("2026-11-13T20:26:20Z"),
    kind: "text",
    summary:
      "g3 met; the attack surface moves from external to internal. Registered g4 'Move laterally internally and take the internal target domain controller DC01', dispatching i9 (host discovery) + three parallel lateral tracks i10/i11/i12. Credential reuse is anchored directly on faCreds — the password leaked from external source code feeds internal lateral movement.",
  },

  // ── Round 3 · Target-met retrospective (2026-11-14 00:25) ──
  {
    seq: 1300,
    worker: "planner",
    ts: T("2026-11-14T00:25:00Z"),
    kind: "round",
    summary: "Round 3 · Target-met retrospective",
  },
  {
    seq: 1301,
    worker: "planner",
    ts: T("2026-11-14T00:25:16Z"),
    kind: "thinking",
    summary:
      "Comparing this round's worker output: i9→fa7 'alive internally: DC01/FS01/JENKINS'; i11→fa8 'Jenkins /script unauthenticated Groovy' + fa9 'exported the plaintext password of acme\\svc_deploy from the Jenkins credentials store'; i12→fa10 'Kerberoasted the svc_sql TGS, hashcat cracked Sql@2020'. svc_deploy is in Domain Admins; i13 used it to take DC01 with psexec and secretsdump. g4's acceptance (take DC01) should be met — verify the finding evidence chain before stamping it.",
  },
  {
    seq: 1302,
    worker: "planner",
    ts: T("2026-11-14T00:25:30Z"),
    kind: "tool_use",
    tool: "list_findings",
    tool_use_id: "p3-lf",
    summary: "list_findings — verifying the confirmed evidence of DC01's fall",
  },
  {
    seq: 1303,
    worker: "planner",
    ts: T("2026-11-14T00:25:40Z"),
    kind: "tool_result",
    tool: "list_findings",
    tool_use_id: "p3-lf",
    summary:
      "F-09 'Full control of domain controller DC01 / secretsdump exported all domain hashes' critical (evidence: psexec svc_deploy@10.10.10.10 + NTDS.dit dump); the upstream chain fa9→fa10→i13 is complete",
  },
  {
    seq: 1304,
    worker: "planner",
    ts: T("2026-11-14T00:25:52Z"),
    kind: "tool_use",
    tool: "prove_goal",
    tool_use_id: "p3-pg",
    summary: "prove_goal(g4, evidence=F-09)",
    detail:
      "reason: Jenkins leaked svc_deploy (Domain Admin) → psexec controlled DC01 → secretsdump on all domain hashes; F-09 is confirmed and 'Take the internal target domain controller DC01' is met.",
  },
  {
    seq: 1305,
    worker: "planner",
    ts: T("2026-11-14T00:25:54Z"),
    kind: "tool_result",
    tool: "prove_goal",
    tool_use_id: "p3-pg",
    summary: "goal g4 → met; system hint: g2 is still open, not the last goal, so the task is not fully wrapped up",
  },
  {
    seq: 1306,
    worker: "planner",
    ts: T("2026-11-14T00:26:10Z"),
    kind: "text",
    summary:
      "g4 wrapped up and the external-internal chain is fully connected (external RCE → DMZ → internal → domain controller). Comparing uncovered directions one by one: g2 'Read user sensitive data' still lacks the final step, but the existing i3 (admin user APIs) + i5 (API IDOR) already cover that direction and both are running. No [essentially different uncovered new direction] this round — 0 new intents per the restraint principle; wait for i3/i5 output.",
  },

  // ── Round 4 · Admin goal met, converging on g2 (2026-11-14 03:55) ──
  {
    seq: 1400,
    worker: "planner",
    ts: T("2026-11-14T03:55:00Z"),
    kind: "round",
    summary: "Round 4 · Admin goal met, converging on g2",
  },
  {
    seq: 1401,
    worker: "planner",
    ts: T("2026-11-14T03:55:14Z"),
    kind: "thinking",
    summary:
      "Comparing recent_facts: work#2 used Acme@2021 from faCreds to break into the admin console (i2 done); admin access is in hand — g1's acceptance is met, so prove_goal stamps it first. Then a [quantitative acceptance check]: g2 'Read user sensitive data' currently only has i3/i5 touching the API surface without actually obtaining bulk data; measured as not met, prove_goal is [forbidden] — instead raise priorities and supplement data-retrieval intents.",
  },
  {
    seq: 1402,
    worker: "planner",
    ts: T("2026-11-14T03:55:26Z"),
    kind: "tool_use",
    tool: "prove_goal",
    tool_use_id: "p4-pg",
    summary: "prove_goal(g1, evidence=fa1)",
    detail:
      "reason: i2 used the externally leaked hardcoded password Acme@2021 to log into the admin console (Element-UI) and obtained admin access; g1 is met.",
  },
  {
    seq: 1403,
    worker: "planner",
    ts: T("2026-11-14T03:55:28Z"),
    kind: "tool_result",
    tool: "prove_goal",
    tool_use_id: "p4-pg",
    summary: "goal g1 → met; remaining unmet: g2",
  },
  {
    seq: 1404,
    worker: "planner",
    ts: T("2026-11-14T03:55:40Z"),
    kind: "thinking",
    summary:
      "g2 quantitative check: the goal's 'Read user sensitive data' = actually obtaining bulk user records. findings in graph_overview has no 'user data exported' entry yet, and i3/i5 are both running with no retrieval results back — measured as not met. Marking met early on 'the console is entered, broadly done' is strictly forbidden; keep dispatching intents to close the gap.",
  },
  {
    seq: 1405,
    worker: "planner",
    ts: T("2026-11-14T03:55:52Z"),
    kind: "tool_use",
    tool: "add_intent",
    tool_use_id: "p4-ai",
    summary: "add_intent × 2 (converge on g2 data retrieval, raise priority)",
    detail:
      "intents=[\n  { summary: 'Logged into the admin console; enumerate user management/export APIs and pull user sensitive fields in bulk', asset_ids: [admin], parent_ids: [fa1], priority: 9 },\n  { summary: 'api.acme.com order IDOR: iterate order_id to pull user orders/PII in bulk', asset_ids: [api], parent_ids: [fa2], priority: 9 },\n]\nNote: i3/i5 are already running; this round escalates with more precise retrieval descriptions + raised priorities (→9) instead of duplicating synonymous intents.",
  },
  {
    seq: 1406,
    worker: "planner",
    ts: T("2026-11-14T03:55:54Z"),
    kind: "tool_result",
    tool: "add_intent",
    tool_use_id: "p4-ai",
    summary: "2 intents updated → ids=[i3, i5] (priority raised to 9, descriptions refined to 'bulk data retrieval')",
  },
  {
    seq: 1407,
    worker: "planner",
    ts: T("2026-11-14T03:55:58Z"),
    kind: "text",
    summary:
      "g1 met (3 of 4 already met: g1/g3/g4). g2 strictly follows quantitative acceptance — merely entering the console does not count; bulk user data must be obtained before stamping it; i3/i5 are raised to priority 9 and refined into retrieval intents, and the whole task wraps up once workers report back.",
  },
];
