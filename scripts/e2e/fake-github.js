// In-memory fake of the GitHub Issues REST API subset ticketing/github.go uses:
//   GET   /search/issues?q=..."artex:key=..."   (find an existing ticket by its key)
//   POST  /repos/{owner}/{repo}/issues          (create)
//   GET   /repos/{owner}/{repo}/issues/{n}      (read state for sync)
//   PATCH /repos/{owner}/{repo}/issues/{n}      (update / close / reopen)
// Listens on loopback only. The server's notify SSRF guard refuses loopback
// unless ARTEX_NOTIFY_ALLOW_LOCAL=1, which the e2e runner sets.
//
// Usage from a script: const gh = await startFakeGitHub(0); gh.url; gh.issues; gh.close(n, reason); await gh.stop();
// Standalone: node scripts/e2e/fake-github.js [port]
const http = require("node:http");

function startFakeGitHub(port = 0) {
  const issues = new Map(); // number -> issue
  const log = []; // "METHOD path auth=yes|no"
  let base = "";

  const send = (res, code, obj) => {
    const body = JSON.stringify(obj);
    res.writeHead(code, { "Content-Type": "application/json", "Content-Length": Buffer.byteLength(body) });
    res.end(body);
  };
  const readBody = (req) =>
    new Promise((resolve) => {
      let data = "";
      req.on("data", (c) => (data += c));
      req.on("end", () => {
        try {
          resolve(data ? JSON.parse(data) : {});
        } catch {
          resolve({});
        }
      });
    });
  const labels = (names) => (names || []).map((name) => ({ name }));

  const server = http.createServer(async (req, res) => {
    const url = new URL(req.url, "http://fake");
    log.push(`${req.method} ${url.pathname} auth=${req.headers.authorization ? "yes" : "no"}`);
    const one = url.pathname.match(/^\/repos\/[^/]+\/[^/]+\/issues\/(\d+)$/);
    const many = url.pathname.match(/^\/repos\/([^/]+\/[^/]+)\/issues$/);
    if (req.method === "GET" && url.pathname === "/search/issues") {
      const key = (url.searchParams.get("q") || "").match(/"(artex:key=[^"]+)"/);
      const items = key ? [...issues.values()].filter((i) => i.body.includes(key[1])) : [];
      return send(res, 200, { total_count: items.length, items });
    }
    if (req.method === "GET" && one && issues.has(Number(one[1]))) return send(res, 200, issues.get(Number(one[1])));
    if (req.method === "POST" && many) {
      const b = await readBody(req);
      const number = Math.max(0, ...issues.keys()) + 1;
      const issue = {
        number,
        html_url: `${base}/${many[1]}/issues/${number}`,
        title: b.title || "",
        body: b.body || "",
        state: "open",
        state_reason: null,
        labels: labels(b.labels),
      };
      issues.set(number, issue);
      return send(res, 201, issue);
    }
    if (req.method === "PATCH" && one && issues.has(Number(one[1]))) {
      const b = await readBody(req);
      const issue = issues.get(Number(one[1]));
      for (const k of ["title", "body", "state", "state_reason"]) if (k in b) issue[k] = b[k];
      if ("labels" in b) issue.labels = labels(b.labels);
      return send(res, 200, issue);
    }
    send(res, 404, { message: "Not Found" });
  });

  return new Promise((resolve, reject) => {
    server.once("error", reject);
    server.listen(port, "127.0.0.1", () => {
      base = `http://127.0.0.1:${server.address().port}`;
      resolve({
        url: base,
        issues,
        log,
        // A human closes an issue on the tracker (reason: completed | not_planned).
        close(number, reason = "completed") {
          const issue = issues.get(number);
          if (!issue) throw new Error(`no issue #${number}`);
          issue.state = "closed";
          issue.state_reason = reason;
        },
        stop: () => new Promise((r) => server.close(() => r())),
      });
    });
  });
}

module.exports = { startFakeGitHub };

if (require.main === module) {
  startFakeGitHub(Number(process.argv[2] || 0)).then((gh) => console.log(`fake GitHub API on ${gh.url}`));
}
