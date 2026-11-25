// Role probes for the E2 integrations UI (c-119206): signed in as a reviewer and as
// a client_read user, every page must hide the controls the server refuses that role
// (server/rbac_policy.go) and must not send a request the server answers with 403.
//
// Called by integrations-e2e.js while its tracker and pushed finding still exist.
// Needs ARTEX_E2E_PG_DSN (psql on PATH) to put the finding's assets in the client's
// company, since an imported finding has no company of its own.
const { execFileSync } = require("node:child_process");

const PASSWORD = "e2e-role-password-42";

// probeRoles(browser, { base, api, check, shot, findingId, findingTitle, tracker, dsn })
//   api: the admin page's JSON fetcher; check: the scenario's reporter.
async function probeRoles(browser, o) {
  const { base, api, check, findingId, tracker, dsn } = o;
  const company = await api("/companies", post({ name: `E2E client ${Date.now()}` }));
  check("roles: company created", company.status < 300, company.status);
  psql(dsn, `UPDATE assets SET company_id = ${Number(company.body.id)}
    WHERE id IN (SELECT e.v::bigint FROM findings f, jsonb_array_elements_text(f.asset_ids) e(v)
                 WHERE f.id = ${Number(findingId)})`);
  const user = async (username, role) => {
    const r = await api("/users", post({ username, password: PASSWORD, role }));
    check(`roles: ${role} user created`, r.status === 201, r.status);
    return r.body;
  };
  await user("e2e-reviewer", "reviewer");
  const client = await user("e2e-client", "client_read");
  const bind = await api(`/users/${client.id}/companies`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ company_ids: [Number(company.body.id)] }),
  });
  check("roles: client bound to the company", bind.status < 300, bind.status);

  await asRole(browser, o, "e2e-reviewer", async (page, open) => {
    await open("/system/integrations");
    await page.getByRole("row", { name: new RegExp(tracker) }).waitFor({ timeout: 30000 });
    await o.shot("10-reviewer-integrations");
    const row = page.getByRole("row", { name: new RegExp(tracker) });
    check("reviewer: no Add tracker", (await page.getByRole("button", { name: "Add tracker" }).count()) === 0);
    check("reviewer: no enable switch", (await row.getByRole("switch").count()) === 0);
    check("reviewer: no Edit/Delete/Sync", (await row.getByRole("button").count()) === 0);
    check("reviewer: tracker name is not a button", (await row.getByRole("button", { name: tracker }).count()) === 0);

    await open("/system/integrations/imports");
    await page.getByText("Past imports").waitFor({ timeout: 30000 });
    check("reviewer: no import form", (await page.locator("#import-file").count()) === 0);
    check("reviewer: past imports listed", (await page.getByRole("row").count()) > 1);

    await open("/function/findings");
    check("reviewer: bulk bar shown for a selected row", await selectFirstRow(page));
    check("reviewer: no bulk Push to tracker", (await pushControls(page)) === 0);

    await open(`/function/findings/detail?id=${findingId}`);
    const card = page.locator("div[data-slot=card]").filter({ hasText: "Tickets" });
    await card.first().waitFor({ timeout: 30000 });
    await o.shot("11-reviewer-detail");
    check("reviewer: Tickets card shown", (await card.count()) >= 1);
    check("reviewer: no Push to tracker on detail", (await pushControls(page)) === 0);
  });

  await asRole(browser, o, "e2e-client", async (page, open) => {
    await open(`/function/findings/detail?id=${findingId}`);
    await page.waitForTimeout(1500);
    await o.shot("12-client-detail");
    const title = (await page.getByText(o.findingTitle).count()) > 0;
    check("client: finding detail loads", title, o.findingTitle);
    check(
      "client: no Tickets card",
      (await page.locator("div[data-slot=card]").filter({ hasText: "Not pushed to a tracker yet." }).count()) +
        (await page.getByRole("columnheader", { name: "Remote state" }).count()) ===
        0,
    );
    check("client: no Push to tracker on detail", (await pushControls(page)) === 0);
    await open("/function/findings");
    await selectFirstRow(page);
    check("client: no bulk Push to tracker", (await pushControls(page)) === 0);
  });
}

// asRole signs username in on a fresh context, runs fn and reports every /api 403.
async function asRole(browser, o, username, fn) {
  const ctx = await browser.newContext({ viewport: { width: 1440, height: 900 } });
  const page = await ctx.newPage();
  const refused = [];
  page.on("response", (r) => {
    if (r.status() === 403 && new URL(r.url()).pathname.startsWith("/api/")) refused.push(r.url());
  });
  const shot = o.shot;
  o.shot = (name) => (shot ? shot(name, page) : null);
  const open = (p) => page.goto(o.base + p, { waitUntil: "networkidle", timeout: 120000 });
  try {
    await page.goto(`${o.base}/login`, { timeout: 120000 });
    await page.fill("#username", username);
    await page.fill("#password", PASSWORD);
    await page.click("#agree-terms");
    await page.getByRole("button", { name: "Sign in" }).click();
    await page.waitForURL((u) => !u.pathname.startsWith("/login"), { timeout: 30000 });
    await fn(page, open);
  } catch (e) {
    o.check(`${username}: probe completed`, false, String(e).split("\n")[0]);
    await o.shot(`zz-${username}-failure`);
  } finally {
    o.check(`${username}: no 403 from /api`, refused.length === 0, refused.join(", "));
    o.shot = shot;
    await ctx.close();
  }
}

// selectFirstRow ticks the first findings row, so the bulk bar renders; false when
// the table offers no selection.
async function selectFirstRow(page) {
  const box = page.locator("tbody tr").first().getByRole("checkbox");
  await page.locator("tbody tr").first().waitFor({ timeout: 30000 });
  if ((await box.count()) === 0) return false;
  await box.first().click();
  return page
    .getByText(/\d+ selected/)
    .first()
    .waitFor({ timeout: 10000 })
    .then(() => true)
    .catch(() => false);
}

const pushControls = (page) => page.getByRole("combobox", { name: "Push to tracker" }).count();

const post = (body) => ({
  method: "POST",
  headers: { "Content-Type": "application/json" },
  body: JSON.stringify(body),
});

function psql(dsn, sql) {
  if (!dsn) throw new Error("set ARTEX_E2E_PG_DSN for the role probes");
  execFileSync("psql", [dsn, "-v", "ON_ERROR_STOP=1", "-qc", sql], { stdio: "pipe" });
}

module.exports = { probeRoles };
