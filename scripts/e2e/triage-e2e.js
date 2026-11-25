// Browser end-to-end check of keyboard triage (web/src/lib/triage.ts) against a real
// ARTEX backend: the interception approval queue and the findings "By task" view.
// Run it through scripts/e2e/integrations.sh, which sets up a throwaway Postgres,
// the backend and the web UI:
//
//   ARTEX_E2E_SCENARIO=triage-e2e.js scripts/e2e/integrations.sh
//
// It seeds rows straight into the database with psql (ARTEX_E2E_DSN), so point it
// only at a database you can throw away.
//
// Scenario:
//  - approvals: open the first pending request, press A → it is allowed (decided_by
//    human) and the next request opens; D → denied and the next opens; Esc closes;
//    a verdict key with nothing open decides nothing.
//  - findings, grouped view, status=pending: open the first finding and press C ten
//    times; the tenth verdict empties the group's page, and the reloaded page's first
//    finding opens (it used to collapse).
//  - findings, grouped view, status=all: open the last finding of page 1 and press I;
//    page 2 loads and its first finding opens.
// One PASS/FAIL line per check; exit 1 on any failure.
const path = require("node:path");
const { execFileSync } = require("node:child_process");
const { chromium } = require("playwright");

const base = (process.env.ARTEX_E2E_URL || "http://127.0.0.1:25173").replace(/\/$/, "");
const password = process.env.ARTEX_E2E_PASSWORD;
const dsn = process.env.ARTEX_E2E_DSN;
const shots = process.env.ARTEX_E2E_SHOTS || "";
if (!password) throw new Error("set ARTEX_E2E_PASSWORD");
if (!dsn) throw new Error("set ARTEX_E2E_DSN (a throwaway database)");

const sql = (q) => execFileSync("psql", [dsn, "-At", "-v", "ON_ERROR_STOP=1", "-c", q], { encoding: "utf8" }).trim();

let failed = false;
const check = (name, ok, detail) => {
  console.log(`${ok ? "PASS" : "FAIL"} ${name}${detail !== undefined ? `: ${detail}` : ""}`);
  if (!ok) failed = true;
};
const must = (name, ok, detail) => {
  check(name, ok, detail);
  if (!ok) throw new Error(`required check failed: ${name}`);
};

async function waitFor(fn, timeout = 15000) {
  const end = Date.now() + timeout;
  for (;;) {
    const v = await fn();
    if (v) return v;
    if (Date.now() > end) return v;
    await new Promise((r) => setTimeout(r, 200));
  }
}

(async () => {
  const browser = await chromium.launch();
  const ctx = await browser.newContext({ viewport: { width: 1440, height: 900 } });
  const page = await ctx.newPage();
  const problems = [];
  page.on("pageerror", (e) => problems.push(`pageerror: ${String(e).slice(0, 200)}`));
  const shot = (name) => (shots ? page.screenshot({ path: path.join(shots, `${name}.png`), fullPage: true }) : null);
  const open = (p) => page.goto(base + p, { waitUntil: "networkidle", timeout: 120000 });
  const press = async (key) => {
    // Keys go to the page, not to a focused control.
    await page.evaluate(() => (document.activeElement instanceof HTMLElement ? document.activeElement.blur() : null));
    await page.keyboard.press(key);
  };

  try {
    await page.goto(`${base}/login`, { timeout: 120000 });
    await page.fill("#password", password);
    await page.click("#agree-terms");
    await page.getByRole("button", { name: "Sign in" }).click();
    await page.waitForURL((u) => !u.pathname.startsWith("/login"), { timeout: 30000 });
    must("sign in", true, page.url());

    // ---- approvals ----
    const ids = sql(
      `INSERT INTO intercept_pending (tool_name, tool_input, agent_name)
       SELECT 'Bash', jsonb_build_object('command', 'echo e2e-' || g), 'e2e' FROM generate_series(1,3) g
       RETURNING id`,
    )
      .split("\n")
      .map(Number);
    must("seed 3 pending approvals", ids.length === 3, ids.join(","));
    const status = (id) => sql(`SELECT status || '/' || coalesce(decided_by, '') FROM intercept_pending WHERE id=${id}`);
    await open("/system/intercept/approvals");
    const pendingTable = page.getByRole("table", { name: "Pending approvals" });
    await pendingTable.waitFor({ timeout: 30000 });
    const order = await pendingTable.locator("tbody tr button[aria-label^='Expand approval #']").evaluateAll((els) =>
      els.map((el) => Number((el.getAttribute("aria-label") || "").replace(/\D+/g, ""))),
    );
    must("pending table lists the seeded requests", ids.every((id) => order.includes(id)), order.join(","));
    const [first, second, third] = order;
    const isOpen = (id) => pendingTable.getByRole("button", { name: `Collapse approval #${id}` }).count();

    await press("a");
    await page.waitForTimeout(800);
    check("a verdict key with nothing open decides nothing", status(first) === "pending/", status(first));

    await pendingTable.getByRole("button", { name: `Expand approval #${first}` }).click();
    await page.getByRole("button", { name: /Allow/ }).locator("kbd").first().waitFor({ timeout: 10000 });
    await shot("01-approval-open");
    await press("a");
    check("A allows the open request (human)", await waitFor(() => status(first) === "allowed/human"), status(first));
    check("the next request opens after A", Boolean(await waitFor(async () => (await isOpen(second)) > 0)));
    await press("d");
    check("D denies the open request (human)", await waitFor(() => status(second) === "denied/human"), status(second));
    check("the next request opens after D", Boolean(await waitFor(async () => (await isOpen(third)) > 0)));
    await shot("02-approval-advanced");
    await press("Escape");
    check("Esc closes the request", await waitFor(async () => (await isOpen(third)) === 0));
    check("the last request stays pending", status(third) === "pending/", status(third));

    // ---- findings: grouped view advances across pages ----
    sql(
      `INSERT INTO findings (name, vulnclass, severity, status, created_at)
       SELECT 'E2E finding ' || lpad(g::text, 2, '0'), 'xss', 'medium', 'pending', now() - (g || ' seconds')::interval
       FROM generate_series(1,12) g`,
    );
    const openRow = () =>
      page
        .locator("tr[data-row-key][aria-expanded='true']")
        .first()
        .innerText()
        .then((t) => (t.match(/E2E finding \d+/) || [""])[0])
        .catch(() => "");
    const counts = () => sql(`SELECT status || ':' || count(*) FROM findings GROUP BY status ORDER BY status`);

    await open("/function/findings?view=grouped&status=pending");
    await page.locator("button[aria-expanded]").filter({ hasText: "Unlinked / task deleted" }).first().click();
    const rows = page.locator("tr[data-row-key]");
    await rows.first().waitFor({ timeout: 30000 });
    must("group page holds 10 findings", (await rows.count()) === 10, await rows.count());
    await rows.first().click();
    await page.getByRole("toolbar", { name: "Triage this finding" }).waitFor({ timeout: 10000 });
    const seen = [];
    for (let i = 0; i < 10; i++) {
      const before = await openRow();
      seen.push(before);
      await press("c");
      await waitFor(async () => {
        const now = await openRow();
        return now !== before;
      });
    }
    await page.waitForTimeout(1500);
    const afterPage = await openRow();
    await shot("03-grouped-advanced");
    check("10 verdicts written", counts().includes("confirmed:10"), counts().replace(/\n/g, " "));
    check(
      "the 10th verdict opens the reloaded page's next finding (not collapsed)",
      /E2E finding (11|12)/.test(afterPage),
      afterPage || "(nothing open)",
    );

    await open("/function/findings?view=grouped&status=all");
    await page.locator("button[aria-expanded]").filter({ hasText: "Unlinked / task deleted" }).first().click();
    await rows.first().waitFor({ timeout: 30000 });
    const lastOnPage = rows.nth((await rows.count()) - 1);
    const lastName = ((await lastOnPage.innerText()).match(/E2E finding \d+/) || [""])[0];
    await lastOnPage.click();
    await page.getByRole("toolbar", { name: "Triage this finding" }).waitFor({ timeout: 10000 });
    await press("i");
    const next = await waitFor(async () => {
      const now = await openRow();
      return now && now !== lastName ? now : "";
    });
    await shot("04-grouped-next-page");
    check("a kept verdict on the page's last row opens page 2's first finding", Boolean(next), `${lastName} → ${next}`);
    check(
      "that finding is on page 2",
      Boolean(next) && !(await page.locator("tr[data-row-key]").filter({ hasText: lastName }).count()),
    );
  } catch (e) {
    check("scenario completed", false, String(e).slice(0, 300));
    await shot("99-failure").catch(() => {});
  } finally {
    for (const p of problems) check("no page errors", false, p);
    await browser.close();
    process.exit(failed ? 1 : 0);
  }
})();
