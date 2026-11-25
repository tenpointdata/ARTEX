// Seeds one row of each kind the role probe (role-probe-e2e.js) opens, through the
// admin API (`api`, a fetch bound to an admin browser session) and psql (`dsn`, a
// throwaway database). Tasks, findings and pending approvals go in with SQL so no
// agent ever starts. The WebSocket exchange is recorded through the real proxy.
//
// Returns { users, companyId, taskId, findingId, agentKey, wsUrl }; throws on any
// seed that fails, so the probe never runs against a half-seeded instance.
const { execFileSync } = require("node:child_process");
const { seedWebSocket } = require("./ws-seed");

// Probe users, one per non-admin role. client_read is bound to the seeded company.
const USERS = {
  reviewer: { username: "probe-reviewer", password: "Probe-review-correct-9", role: "reviewer" },
  operator: { username: "probe-operator", password: "Probe-operate-correct-9", role: "operator" },
  client_read: { username: "probe-client", password: "Probe-client-correct-9", role: "client_read" },
};

const sqlRunner = (dsn) => (q) =>
  execFileSync("psql", [dsn, "-qAt", "-v", "ON_ERROR_STOP=1", "-c", q], { encoding: "utf8" }).trim();

const HTTP_EXCHANGES = [
  {
    method: "GET", url: "http://app.probe.invalid/api/items?id=1", status: 200,
    requestHeaders: { Accept: "application/json", Authorization: "Bearer probe" }, requestBody: "",
    responseHeaders: { "Content-Type": "application/json" }, responseBody: '{"items":[1,2,3]}',
  },
  {
    method: "POST", url: "http://app.probe.invalid/login", status: 401,
    requestHeaders: { "Content-Type": "application/json" }, requestBody: '{"u":"a"}',
    responseHeaders: { "Content-Type": "application/json" }, responseBody: '{"error":"no"}',
  },
  {
    method: "GET", url: "http://app.probe.invalid/events", status: 200,
    requestHeaders: { Accept: "text/event-stream" }, requestBody: "",
    responseHeaders: { "Content-Type": "text/event-stream" }, responseBody: "data: hello\n\ndata: world\n\n",
  },
];

async function seed({ api, dsn, proxy, log = () => {} }) {
  const sql = sqlRunner(dsn);
  const post = async (what, p, body, method = "POST") => {
    const r = await api(p, { method, headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) });
    if (r.status >= 300) throw new Error(`seed ${what}: ${method} ${p} -> ${r.status} ${JSON.stringify(r.body).slice(0, 200)}`);
    log(`seeded ${what}`);
    return r.body;
  };

  const company = await post("company", "/api/companies", { name: "Probe Corp", scope: [{ type: "root_domain", value: "probe.invalid" }] });
  const companyId = Number(company.id ?? company.company?.id);
  if (!companyId) throw new Error(`seed company: no id in ${JSON.stringify(company).slice(0, 200)}`);

  const users = {};
  for (const [key, u] of Object.entries(USERS)) {
    const made = await post(`user ${u.username}`, "/api/users", u);
    users[key] = { ...u, id: Number(made.id ?? made.user?.id) };
  }
  await post("client binding", `/api/users/${users.client_read.id}/companies`, { company_ids: [companyId] }, "PUT");

  await post("assets", "/api/assets", {
    assets: [
      { type: "root_domain", domain: "probe.invalid" },
      { type: "subdomain", domain: "www.probe.invalid", record_type: "A", record_value: ["127.0.0.1"] },
    ],
  });
  await post("http traffic", "/api/traffic/ingest", { exchanges: HTTP_EXCHANGES });
  const ws = await seedWebSocket({ proxy, messages: ["probe-hello", "probe-world"] });
  log(`seeded websocket ${ws.url} (${ws.echoed.length} echoes)`);
  await post("workspace dir", "/api/workspace/mkdir", { path: "probe-dir" });
  await post("workspace file", "/api/workspace/write", { path: "probe-dir/notes.txt", content: "probe file" });
  await post("skill", "/api/skills", { name: "probe-skill", description: "A probe skill", instructions: "# Probe\nDo nothing." });
  await post("mcp", "/api/mcp", {
    name: "probe-mcp", transport: "http", url: "http://127.0.0.1:9/mcp", args: [], env: {}, enabled: false, insecure: false, timeout_seconds: 5,
  });
  await post("notify channel", "/api/notify/channels", { name: "probe-webhook", kind: "webhook", enabled: false, config: { url: "http://example.invalid/hook" } });
  await post("llm profile", "/api/llm/profiles", {
    name: "probe-llm", format: "openai", base_url: "http://127.0.0.1:9/v1", model: "probe-model", api_key: "sk-probe-0000",
    rate_per_second: 1, rate_per_minute: 30, context_window_k: 128,
  });
  await post("tamper hook", "/api/traffic/hooks", {
    name: "probe-hook", phase: "request", priority: 10, matchers: {}, source: "package hook\nfunc OnRequest(req *Request) error { return nil }", note: "probe",
  });
  await post("rewrite rule", "/api/traffic/rewrite-rules", {
    name: "probe-rewrite", enabled: false, phase: "request", priority: 1, matchers: {}, actions: [{ type: "set_header", name: "X-Probe", value: "1" }], note: "probe",
  });
  await post("intercept rule", "/api/intercept/rules", {
    name: "probe-rule", enabled: false, priority: 1, match_target: "tool_name", match_type: "string", pattern: "bash", action: "ask", message: "probe",
  });

  // A task that never runs (status created, nothing queued), its finding, and approvals.
  const taskId = Number(sql(
    `WITH e AS (INSERT INTO explorations (goal) VALUES ('probe goal') RETURNING id)
     INSERT INTO tasks (name, description, goal, exploration_id, company_id)
     SELECT 'Probe task', 'role probe', 'probe goal', e.id, ${companyId} FROM e RETURNING id`,
  ));
  const findingId = Number(sql(
    `INSERT INTO findings (task_id, name, vulnclass, severity, status, summary, evidence)
     VALUES (${taskId}, 'Probe finding', 'xss', 'medium', 'pending', 'probe summary', 'probe evidence') RETURNING id`,
  ));
  sql(`INSERT INTO intercept_pending (task_id, agent_name, tool_name, tool_input, reason)
       VALUES ('${taskId}', 'worker', 'bash', '{"command":"curl http://example.invalid"}', 'probe approval')`);
  log(`seeded task ${taskId}, finding ${findingId}, one pending approval`);

  const agents = await api("/api/agents");
  const list = Array.isArray(agents.body) ? agents.body : agents.body?.agents || agents.body?.items || [];
  const agentKey = list.map((a) => a.key || a.name).find(Boolean);
  if (!agentKey) throw new Error(`seed: no agent key in GET /api/agents (${JSON.stringify(agents.body).slice(0, 200)})`);

  return { users, companyId, taskId, findingId, agentKey, wsUrl: ws.url };
}

module.exports = { seed, USERS };
