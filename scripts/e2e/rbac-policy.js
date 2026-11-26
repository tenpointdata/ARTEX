// The server's RBAC policy (server/rbac_policy.go), read from the Go source so the
// e2e checks cannot drift from it. requiredPermission(method, path) mirrors the Go
// function of the same name: the first matching rule wins, else reads need "read"
// and anything else "operate". A role's permissions come from GET /api/auth/status.
//
// Only the rbacPolicy table is parsed; its rows must keep the literal form
//   {<method>, "<path>", <true|false>, perm<Name>},
// where <method> is methodAny, methodMutate or http.Method<Verb>. load() throws when
// the table cannot be found or a row does not parse, rather than checking against
// a partial policy.
const fs = require("node:fs");
const path = require("node:path");

const SOURCE = path.join(__dirname, "..", "..", "server", "rbac_policy.go");

function parseMethod(tok) {
  if (tok === "methodAny") return "*";
  if (tok === "methodMutate") return "MUTATE";
  const m = /^http\.Method([A-Za-z]+)$/.exec(tok);
  if (!m) throw new Error(`rbac-policy: unknown method token ${tok}`);
  return m[1].toUpperCase();
}

function parsePerm(tok) {
  const m = /^perm([A-Za-z]+)$/.exec(tok);
  if (!m) throw new Error(`rbac-policy: unknown permission token ${tok}`);
  return m[1].replace(/([a-z])([A-Z])/g, "$1_$2").toLowerCase(); // permClientRead -> client_read
}

function load(file = SOURCE) {
  const src = fs.readFileSync(file, "utf8");
  const start = src.indexOf("var rbacPolicy = []policyRule{");
  if (start < 0) throw new Error("rbac-policy: rbacPolicy table not found");
  const end = src.indexOf("\n}\n", start);
  const rules = [];
  for (const raw of src.slice(start, end).split("\n").slice(1)) {
    const line = raw.replace(/\/\/.*$/, "").trim();
    if (!line) continue;
    const m = /^\{\s*([\w.]+)\s*,\s*"([^"]+)"\s*,\s*(true|false)\s*,\s*(\w+)\s*\},?$/.exec(line);
    if (!m) throw new Error(`rbac-policy: cannot parse rule line: ${raw.trim()}`);
    rules.push({ method: parseMethod(m[1]), path: m[2], prefix: m[3] === "true", need: parsePerm(m[4]) });
  }
  if (rules.length === 0) throw new Error("rbac-policy: empty rbacPolicy table");
  return rules;
}

// pathSegments cleans p like path.Clean and splits it into segments.
function pathSegments(p) {
  const out = [];
  for (const s of p.split("/")) {
    if (s === "" || s === ".") continue;
    if (s === "..") out.pop();
    else out.push(s);
  }
  return out;
}

function methodMatches(rule, m) {
  if (rule.method === "*") return true;
  if (rule.method === "MUTATE") return m !== "GET" && m !== "HEAD";
  if (rule.method === "GET") return m === "GET" || m === "HEAD";
  return rule.method === m;
}

function pathMatches(rule, segs) {
  const pat = pathSegments(rule.path);
  for (let i = 0; i < pat.length; i++) {
    const p = pat[i];
    if (p.startsWith("{") && p.endsWith("...}")) return true;
    if (i >= segs.length) return false;
    if (!(p.startsWith("{") && p.endsWith("}")) && p !== segs[i]) return false;
  }
  return segs.length === pat.length || rule.prefix;
}

// matchRule is the rule that decides (method, urlPath), or null when the default applies.
function matchRule(rules, method, urlPath) {
  const m = method.toUpperCase();
  const segs = pathSegments(urlPath);
  return rules.find((r) => methodMatches(r, m) && pathMatches(r, segs)) || null;
}

function requiredPermission(rules, method, urlPath) {
  const r = matchRule(rules, method, urlPath);
  if (r) return r.need;
  const m = method.toUpperCase();
  return m === "GET" || m === "HEAD" ? "read" : "operate";
}

module.exports = { load, matchRule, requiredPermission, pathSegments };
