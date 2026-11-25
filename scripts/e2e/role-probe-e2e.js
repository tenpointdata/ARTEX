// Role probe: for each non-admin role (reviewer, operator, client_read) sign in to the
// console, visit every page under web/src/app/(main) (detail pages with seeded ids),
// and click every enabled control: buttons, tabs, menus, switches, the controls of the
// sheets and dialogs they open, and the traffic sheet's stream view of a recorded
// WebSocket. Run it through scripts/e2e/role-probe.sh (a throwaway Postgres, the
// backend with its recording proxy, the web UI):
//
//   scripts/e2e/role-probe.sh
//
// Nothing the probe clicks changes the instance: every state-changing /api request
// a probed role makes is answered in the browser with 200 {} and never reaches the
// backend. Reads go through. The checks, per role:
//  - no /api response with status >= 400;
//  - no request, read or write, to a route whose permission (server/rbac_policy.go,
//    parsed by rbac-policy.js) the role lacks: such a request means an enabled control
//    or a page load needs a permission the role does not hold;
//  - every sidebar link the role sees is a page the probe visits.
// The admin session only seeds (role-probe-seed.js). One PASS/FAIL line per check;
// exit 1 on any failure. ARTEX_E2E_ROLES=reviewer,operator narrows the roles;
// ARTEX_E2E_PAGES=traffic narrows the pages to paths containing a substring.
const fs = require("node:fs");
const path = require("node:path");
const { chromium } = require("playwright");
const policy = require("./rbac-policy");
const { seed } = require("./role-probe-seed");
const { inventory, OVERLAY } = require("./role-probe-dom");

const base = (process.env.ARTEX_E2E_URL || "http://127.0.0.1:25173").replace(/\/$/, "");
const password = process.env.ARTEX_E2E_PASSWORD;
const dsn = process.env.ARTEX_E2E_DSN;
const proxy = process.env.ARTEX_E2E_PROXY;
const shots = process.env.ARTEX_E2E_SHOTS || "";
const roles = (process.env.ARTEX_E2E_ROLES || "reviewer,operator,client_read").split(",").filter(Boolean);
const pageFilter = process.env.ARTEX_E2E_PAGES || "";
const MAX_CLICKS_PER_PAGE = Number(process.env.ARTEX_E2E_MAX_CLICKS || 60);
if (!password) throw new Error("set ARTEX_E2E_PASSWORD");
if (!dsn) throw new Error("set ARTEX_E2E_DSN (a throwaway database)");
if (!proxy) throw new Error("set ARTEX_E2E_PROXY (host:port of the backend's recording proxy)");

// requireAuth (server/auth.go) serves these without a session or a role check.
const RBAC_EXEMPT = (p) => p.startsWith("/api/auth/") || p === "/api/health" || p === "/api/ready";
// Controls never clicked: ending the session would end the probe.
const SKIP_CONTROL = /sign out|log ?out/i;

let failed = false;
const check = (name, ok, detail) => {
  console.log(`${ok ? "PASS" : "FAIL"} ${name}${detail !== undefined && detail !== "" ? `: ${detail}` : ""}`);
  if (!ok) failed = true;
};

// The console's pages, from the app router: every page.tsx under web/src/app/(main).
function consolePages() {
  const root = path.join(__dirname, "..", "..", "web", "src", "app", "(main)");
  const out = [];
  const walk = (dir) => {
    for (const e of fs.readdirSync(dir, { withFileTypes: true })) {
      if (e.isDirectory() && !e.name.startsWith("_")) walk(path.join(dir, e.name));
      else if (e.name === "page.tsx") out.push("/" + path.relative(root, dir).split(path.sep).join("/"));
    }
  };
  walk(root);
  return out.sort();
}

async function signIn(page, username, pass) {
  await page.goto(`${base}/login`, { timeout: 120000 });
  await page.fill("#username", username);
  await page.fill("#password", pass);
  await page.click("#agree-terms");
  await page.getByRole("button", { name: "Sign in" }).click();
  await page.waitForURL((u) => !u.pathname.startsWith("/login"), { timeout: 30000 });
}

// Probes one role in its own browser context. Returns the findings for the report.
async function probeRole(browser, role, user, pages) {
  const ctx = await browser.newContext({ viewport: { width: 1500, height: 1000 } });
  await ctx.addInitScript(() => window.localStorage.setItem("artex.locale", "en"));
  const page = await ctx.newPage();
  let where = "sign in";
  const bad = []; // responses >= 400
  const denied = []; // requests the role's permissions do not cover
  const errors = [];
  let perms = null;
  const permOf = (method, p) => (RBAC_EXEMPT(p) ? null : policy.requiredPermission(rules, method, p));
  const judge = (method, url) => {
    const p = new URL(url).pathname;
    const need = permOf(method, p);
    if (perms && need && !perms.includes(need)) denied.push({ need, method, path: p, where });
  };
  await page.route("**/api/**", async (route) => {
    const req = route.request();
    if (req.method() === "GET" || req.method() === "HEAD") {
      judge(req.method(), req.url());
      return route.continue();
    }
    // Sign-in goes through; after it every write, exempt ones (logout, password) too, is faked.
    if (!perms) return route.continue();
    judge(req.method(), req.url());
    return route.fulfill({ status: 200, contentType: "application/json", body: "{}" });
  });
  page.on("response", (r) => {
    const u = new URL(r.url());
    if (!u.pathname.startsWith("/api/") || r.status() < 400) return;
    // Faked writes never reach the server; only real answers are judged here.
    bad.push({ status: r.status(), method: r.request().method(), path: u.pathname + u.search, where });
  });
  page.on("pageerror", (e) => errors.push(`${where}: ${String(e).slice(0, 160)}`));

  await signIn(page, user.username, user.password);
  where = `landing ${new URL(page.url()).pathname}`;
  await page.waitForLoadState("networkidle", { timeout: 30000 }).catch(() => {});
  const session = await page.evaluate(async () => (await fetch("/api/auth/session")).json());
  perms = session.permissions || [];
  check(`${role}: signed in with its role`, session.role === role, `${session.role} [${perms.join(",")}]`);

  // Sidebar links this role sees.
  await page.goto(`${base}/dashboard`, { waitUntil: "networkidle", timeout: 60000 }).catch(() => {});
  const sidebar = await page.$$eval("[data-slot=sidebar] a[href], aside a[href], nav a[href]", (as) =>
    [...new Set(as.map((a) => new URL(a.href).pathname).filter((p) => p !== "/"))].sort());
  const unvisited = sidebar.filter((p) => !pages.some((q) => q.path === p));
  check(`${role}: every sidebar link is a probed page`, unvisited.length === 0, `sidebar=[${sidebar.join(" ")}]${unvisited.length ? ` unprobed=[${unvisited.join(" ")}]` : ""}`);

  let clicks = 0;
  for (const pg of pages) {
    where = `${pg.path} :: load`;
    try {
      await page.goto(base + pg.url, { waitUntil: "networkidle", timeout: 60000 });
    } catch (e) {
      check(`${role}: ${pg.path} loads`, false, String(e).slice(0, 120));
      continue;
    }
    clicks += await explore(page, pg, (w) => { where = w; });
    if (shots) await page.screenshot({ path: path.join(shots, `${role}-${pg.path.replace(/\W+/g, "_")}.png`) }).catch(() => {});
  }
  await ctx.close();
  return { role, sidebar, bad, denied, errors, clicks };
}

// Clicks every enabled control on the page once (deduped by scope and name), the
// overlays they open first. Returns how many controls were clicked.
async function explore(page, pg, setWhere) {
  const seen = new Set();
  let clicks = 0;
  for (let step = 0; step < MAX_CLICKS_PER_PAGE * 2 && clicks < MAX_CLICKS_PER_PAGE; step++) {
    const controls = await page.evaluate(inventory).catch(() => []);
    const next = controls.find((c) => !c.disabled && !seen.has(c.key) && !SKIP_CONTROL.test(c.name));
    if (!next) {
      if (await page.locator(OVERLAY).count()) {
        await page.keyboard.press("Escape");
        await page.waitForTimeout(200);
        if (await page.locator(OVERLAY).count()) await page.goto(base + pg.url, { waitUntil: "networkidle", timeout: 60000 }).catch(() => {});
        continue;
      }
      break;
    }
    seen.add(next.key);
    setWhere(`${pg.path} :: ${next.key}`);
    const el = page.locator(`[data-probe-id="${next.id}"]`).first();
    const before = page.url();
    try {
      if (next.fill) {
        await el.fill(next.type === "number" ? "1" : "probe", { timeout: 1500 });
        await el.press("Tab", { timeout: 1000 }).catch(() => {});
      } else {
        await el.click({ timeout: 1500 });
      }
      clicks++;
    } catch (_) {
      continue; // covered or detached: its key is spent, the loop moves on
    }
    await page.waitForLoadState("networkidle", { timeout: 4000 }).catch(() => {});
    await page.waitForTimeout(250);
    if (page.url() !== before && new URL(page.url()).pathname !== new URL(before).pathname) {
      await page.goto(base + pg.url, { waitUntil: "networkidle", timeout: 60000 }).catch(() => {});
    }
  }
  return clicks;
}

let rules = [];

(async () => {
  rules = policy.load();
  check("rbac policy parsed from server/rbac_policy.go", rules.length > 10, `${rules.length} rules`);
  const browser = await chromium.launch();
  try {
    const admin = await (await browser.newContext()).newPage();
    await signIn(admin, "ARTEX", password);
    const api = (p, init) => admin.evaluate(async ([url, opts]) => {
      const r = await fetch(url, opts);
      return { status: r.status, body: await r.json().catch(() => null) };
    }, [`${p}`, init || {}]);
    const s = await seed({ api, dsn, proxy, log: (m) => console.log(`  ${m}`) });
    check("seed", true, `task ${s.taskId}, finding ${s.findingId}, agent ${s.agentKey}, ws ${s.wsUrl}`);
    const wsListed = await api("/api/traffic?limit=50");
    const exchanges = wsListed.body?.exchanges || [];
    check("a WebSocket exchange is recorded", exchanges.some((e) => e.content_type === "websocket"), `${exchanges.length} exchanges`);

    const DETAIL = {
      "/function/tasks/detail": `?id=${s.taskId}`,
      "/function/findings/detail": `?id=${s.findingId}`,
      "/system/agents/detail": `?key=${encodeURIComponent(s.agentKey)}`,
    };
    const pages = consolePages()
      .filter((p) => !pageFilter || p.includes(pageFilter))
      .map((p) => ({ path: p, url: p + (DETAIL[p] || "") }));
    const unmapped = pages.filter((p) => p.path.endsWith("/detail") && !DETAIL[p.path]).map((p) => p.path);
    check("every detail page has a seeded id", unmapped.length === 0, unmapped.join(" "));
    console.log(`  probing ${pages.length} pages as ${roles.join(", ")}`);

    const results = await Promise.all(roles.map((r) => probeRole(browser, r, s.users[r], pages)));
    for (const r of results) {
      const dedupe = (xs, k) => [...new Map(xs.map((x) => [k(x), x])).values()];
      const bad = dedupe(r.bad, (b) => `${b.status} ${b.method} ${b.path.split("?")[0]}`);
      const denied = dedupe(r.denied, (d) => `${d.method} ${d.path} ${d.where.split(" :: ")[0]}`);
      check(`${r.role}: no /api response >= 400 (${r.clicks} controls clicked)`, bad.length === 0,
        bad.map((b) => `\n    ${b.status} ${b.method} ${b.path}  @ ${b.where}`).join(""));
      check(`${r.role}: no request needs a permission the role lacks`, denied.length === 0,
        denied.map((d) => `\n    needs ${d.need}: ${d.method} ${d.path}  @ ${d.where}`).join(""));
      if (r.errors.length) console.log(`  ${r.role}: ${r.errors.length} page errors (not failures), first: ${r.errors.slice(0, 3).join(" | ")}`);
    }
  } finally {
    await browser.close();
  }
  process.exit(failed ? 1 : 0);
})().catch((e) => {
  console.error(e);
  process.exit(1);
});
