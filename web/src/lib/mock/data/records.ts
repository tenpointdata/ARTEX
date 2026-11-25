import type { LLMTask } from "@/lib/types";

const T = (iso: string) => iso; // readability helper for timestamps

export const commandRecords = [
  {
    id: 1,
    exploration_id: 1,
    worker: "worker-1",
    tool: "bash",
    command: JSON.stringify({ command: "curl -s 'https://www.acme.com/search?q=test'" }),
    output: "HTTP/1.1 200 OK\nContent-Length: 12034\n<html>… search results page …</html>",
    is_error: false,
    created_at: T("2026-11-03T02:10:00Z"),
  },
  {
    id: 2,
    exploration_id: 1,
    worker: "worker-1",
    tool: "bash",
    command: JSON.stringify({ command: "sqlmap -u 'https://www.acme.com/search?q=1' --batch --dbs" }),
    output: "[*] available databases [2]:\n[*] acme_prod\n[*] information_schema\nback-end DBMS: MySQL >= 5.7",
    is_error: false,
    created_at: T("2026-11-03T02:12:30Z"),
  },
  {
    id: 3,
    exploration_id: 1,
    worker: "worker-2",
    tool: "bash",
    command: JSON.stringify({ command: "ffuf -u https://api.acme.com/FUZZ -w common.txt -mc 200,401,403" }),
    output: "v1       [Status: 200]\nhealth   [Status: 200]\norders   [Status: 401]\nadmin    [Status: 403]",
    is_error: false,
    created_at: T("2026-11-03T02:15:00Z"),
  },
  {
    id: 4,
    exploration_id: 1,
    worker: "worker-2",
    tool: "bash",
    command: JSON.stringify({ command: "nmap -sV -p- 203.0.113.10" }),
    output: "PORT    STATE SERVICE VERSION\n22/tcp  open  ssh     OpenSSH 8.9\n443/tcp open  https   nginx 1.24",
    is_error: false,
    created_at: T("2026-11-03T02:18:00Z"),
  },
  {
    id: 5,
    exploration_id: 1,
    worker: "worker-1",
    tool: "bash",
    command: JSON.stringify({ command: "curl -s https://admin.acme.com/login --data 'user=admin&pass=admin123'" }),
    output: "curl: (28) Operation timed out after 10000 ms",
    is_error: true,
    created_at: T("2026-11-03T02:20:00Z"),
  },
];

// ── LLM recordings (/llm/records, /llm/records/{id}) ──
export const llmRecords = [
  {
    id: 1,
    ts: T("2026-11-03T02:10:02Z"),
    model: "claude-opus-4-8",
    profile_name: "Default",
    session_id: "exp1-worker-i12",
    task_id: "t-001",
    worker: "worker-1",
    latency_ms: 3210,
    input_tokens: 8421,
    output_tokens: 512,
    cache_read: 6000,
    cache_write: 1200,
    status: "ok",
  },
  {
    id: 2,
    ts: T("2026-11-03T02:11:40Z"),
    model: "claude-opus-4-8",
    profile_name: "Default",
    session_id: "exp1-planner",
    task_id: "t-001",
    worker: "planner",
    latency_ms: 4180,
    input_tokens: 12044,
    output_tokens: 733,
    cache_read: 9000,
    cache_write: 1500,
    status: "ok",
  },
  {
    id: 3,
    ts: T("2026-11-03T02:15:20Z"),
    model: "claude-opus-4-8",
    profile_name: "Default",
    session_id: "exp1-worker-i18",
    task_id: "t-001",
    worker: "worker-2",
    latency_ms: 2890,
    input_tokens: 7311,
    output_tokens: 421,
    cache_read: 5200,
    cache_write: 900,
    status: "ok",
  },
  {
    id: 4,
    ts: T("2026-11-03T02:16:05Z"),
    model: "claude-opus-4-8",
    profile_name: "Default",
    session_id: "exp1-worker-i18",
    task_id: "t-001",
    worker: "worker-2",
    latency_ms: 900,
    input_tokens: 7600,
    output_tokens: 0,
    cache_read: 5200,
    cache_write: 0,
    status: "error",
    error: "429 Too Many Requests (retried with backoff)",
  },
];

// Distinct tasks with counts, derived from llmRecords above (task picker).
const llmTaskCounts: Record<string, number> = {};
for (const r of llmRecords) if (r.task_id) llmTaskCounts[r.task_id] = (llmTaskCounts[r.task_id] ?? 0) + 1;
export const llmTasks: LLMTask[] = Object.entries(llmTaskCounts).map(([task_id, count]) => ({ task_id, count }));

export function llmRecordDetail(id: number, records = llmRecords) {
  const item = records.find((r) => r.id === id) ?? records[0];
  return {
    ...item,
    request_body: JSON.stringify(
      {
        model: item.model,
        system: "You are the 'executor' of an authorized penetration testing system… (omitted)",
        messages: [
          {
            role: "user",
            content:
              "Start executing this intent from the system prompt: do only it, produce only facts, stop when done.",
          },
        ],
        tools: ["bash", "insert_assets", "record_fact", "report_finding"],
      },
      null,
      2,
    ),
    response_body: JSON.stringify(
      {
        stop_reason: item.status === "error" ? "error" : "tool_use",
        content: [
          { type: "text", text: "Probe injection on search?q=; verify with an error-based payload first." },
          { type: "tool_use", name: "bash", input: { command: "curl -s 'https://www.acme.com/search?q=1%27'" } },
        ],
        usage: { input_tokens: item.input_tokens, output_tokens: item.output_tokens },
      },
      null,
      2,
    ),
    // Raw HTTP: the request contains the full tool schema dropped by the normalized view; the response is raw SSE frames.
    raw_request: JSON.stringify({
      model: item.model,
      max_tokens: 8192,
      stream: true,
      system: [
        {
          type: "text",
          text: "You are the 'executor' of an authorized penetration testing system… (omitted)",
          cache_control: { type: "ephemeral" },
        },
      ],
      messages: [
        {
          role: "user",
          content:
            "Start executing this intent from the system prompt: do only it, produce only facts, stop when done.",
        },
      ],
      tools: [
        {
          name: "bash",
          description: "Runs a shell command in the target environment and returns its output.",
          input_schema: { type: "object", properties: { command: { type: "string" } }, required: ["command"] },
        },
      ],
      thinking: { type: "enabled" },
    }),
    raw_response: [
      `event: message_start`,
      `data: {"type":"message_start","message":{"id":"msg_01mock","model":"${item.model}","usage":{"input_tokens":${item.input_tokens},"output_tokens":1}}}`,
      ``,
      `event: content_block_delta`,
      `data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Probe injection on search?q=."}}`,
      ``,
      `event: content_block_start`,
      `data: {"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"toolu_01mock","name":"bash"}}`,
      ``,
      `event: content_block_delta`,
      `data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\\"command\\":\\"curl -s 'https://www.acme.com/search?q=1%27'\\"}"}}`,
      ``,
      `event: message_delta`,
      `data: {"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":${item.output_tokens}}}`,
      ``,
      `event: message_stop`,
      `data: {"type":"message_stop"}`,
      ``,
    ].join("\n"),
  };
}
