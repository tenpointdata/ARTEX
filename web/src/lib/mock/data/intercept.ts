import type { InterceptApprovalRow, InterceptAudit, InterceptPending, InterceptRule } from "@/lib/types";

const T = (iso: string) => iso; // readability helper for timestamps

export const interceptRules: InterceptRule[] = [
  {
    id: 1,
    name: "Destructive commands require approval",
    enabled: true,
    priority: 10,
    match_target: "tool_input",
    match_type: "regex",
    pattern: "rm\\s+-rf|mkfs|dd\\s+if=",
    action: "ask",
    message: "Destructive command detected; manual approval required",
    timeout_enabled: true,
    timeout_seconds: 120,
    timeout_action: "deny",
    created_at: T("2026-11-08T10:00:00Z"),
    updated_at: T("2026-11-08T10:00:00Z"),
  },
  {
    id: 2,
    name: "Outbound traffic denied outright",
    enabled: true,
    priority: 20,
    match_target: "tool_input",
    match_type: "string",
    pattern: "exfil",
    action: "deny",
    message: "Suspected data exfiltration",
    timeout_enabled: false,
    timeout_seconds: 0,
    timeout_action: "deny",
    created_at: T("2026-11-09T10:00:00Z"),
    updated_at: T("2026-11-09T10:00:00Z"),
  },
];

export const interceptPending: InterceptPending[] = [
  {
    id: 101,
    rule_id: 1,
    task_id: "t-acme-web",
    agent_name: "work#3",
    tool_name: "bash",
    tool_input: { command: "mysqldump -h 203.0.113.10 -uroot acme_prod > /tmp/dump.sql" },
    status: "pending",
    reason: "Data exfiltration gate: full database export requires manual approval",
    created_at: T("2026-11-14T03:58:00Z"),
  },
];

export const interceptHistory: InterceptApprovalRow[] = [
  {
    id: 90,
    rule_id: 1,
    task_id: "t-acme-web",
    agent_name: "work#1",
    tool_name: "bash",
    tool_input: { command: "dd if=/dev/zero of=/tmp/x" },
    status: "denied",
    reason: "Destructive commands require approval",
    created_at: T("2026-11-13T20:00:00Z"),
    decided_at: T("2026-11-13T20:01:00Z"),
    conv_title: "",
    conv_agent_key: "",
    rule_name: "Destructive commands require approval",
  },
  {
    id: 91,
    task_id: "t-acme-api",
    agent_name: "work#2",
    tool_name: "bash",
    tool_input: { command: "rm -rf /var/www/html" },
    status: "denied",
    reason: "[model] Delete a target production file (D4)",
    created_at: T("2026-11-13T18:00:00Z"),
    decided_at: T("2026-11-13T18:00:05Z"),
    conv_title: "",
    conv_agent_key: "",
    rule_name: "",
  },
  {
    id: 92,
    rule_id: 2,
    task_id: "t-acme-web",
    agent_name: "work#13",
    tool_name: "bash",
    tool_input: { command: "impacket-secretsdump acme.local/svc_deploy@10.10.10.10 -just-dc" },
    status: "allowed",
    reason: "Outbound traffic denied outright",
    created_at: T("2026-11-14T00:05:00Z"),
    decided_at: T("2026-11-14T00:06:00Z"),
    conv_title: "",
    conv_agent_key: "",
    rule_name: "Outbound traffic denied outright",
  },
  {
    id: 93,
    task_id: "t-acme-web",
    agent_name: "work#7",
    tool_name: "bash",
    tool_input: { command: 'mysql -e "DROP TABLE users_bak_0921"' },
    status: "pending",
    reason: "[model] Suspected backup table; cannot confirm whether it is production data",
    created_at: T("2026-11-13T15:14:00Z"),
    conv_title: "",
    conv_agent_key: "",
    rule_name: "",
  },
];

// Approval detail fixtures use harmless report-writing examples.
interceptHistory.unshift({
  id: 94,
  task_id: "t-acme-web",
  agent_name: "work#1",
  tool_name: "Write",
  tool_input: {
    path: "reports/summary.md",
    content:
      "# Check summary\n\nThis round of verification is complete; existing evidence and follow-up suggestions are organized.",
  },
  status: "allowed",
  decision_source: "model",
  decided_by: "model",
  reason: "[model] Write the existing check conclusions into a local report without modifying business data.",
  created_at: T("2026-11-14T08:00:00Z"),
  decided_at: T("2026-11-14T08:00:02Z"),
  conv_title: "",
  conv_agent_key: "",
  rule_name: "",
});

export const interceptDetails: Record<number, InterceptAudit> = {
  94: {
    run_id: "demo-run-report",
    tool_use_id: "call-write-report",
    correlation: "exact",
    input_digest: "a3b458eca3b458eca3b458eca3b458eca3b458eca3b458eca3b458eca3b458ec1234",
    user_message:
      "Please organize the completed checks and write the conclusions and evidence index into reports/summary.md.\nKeep the items pending verification; do not modify business data.",
    context: [
      { kind: "user", text: "Summarize this round's existing evidence and generate a check summary." },
      {
        kind: "tool_use",
        tool: "Read",
        tool_use_id: "call-read-evidence",
        text: '{"path":"reports/evidence-index.json"}',
      },
      {
        kind: "tool_result",
        tool: "Read",
        tool_use_id: "call-read-evidence",
        text: "Read 3 evidence index entries.\nRecords contain request time, result summary, and local file location.",
      },
    ],
    captured_at: T("2026-11-14T08:00:00Z"),
    initial_action: "allow",
    initial_reason: "[model] Write the existing check conclusions into a local report without modifying business data.",
    effective_action: "allow",
    config_digest: "b4c569fdb4c569fdb4c569fdb4c569fdb4c569fdb4c569fdb4c569fdb4c569fd1234",
    profile_id: 1,
    execution_status: "succeeded",
    output: "Successfully wrote reports/summary.md\n68 characters written in total.",
    execution_ended_at: T("2026-11-14T08:00:03Z"),
  },
  93: {
    run_id: "demo-run-review",
    tool_use_id: "call-pending-review",
    correlation: "exact",
    input_digest: "c5d670aec5d670aec5d670aec5d670aec5d670aec5d670aec5d670aec5d670ae1234",
    user_message: "Check the backup table's purpose; confirm before anything involving deletion.",
    context: [],
    captured_at: T("2026-11-13T15:14:00Z"),
    initial_action: "ask",
    initial_reason: "[model] Suspected backup table; cannot confirm whether it is production data.",
    profile_id: 1,
    execution_status: "not_started",
  },
};

// ── Conversations (chat) ─────────────────────────────────────────────────────
