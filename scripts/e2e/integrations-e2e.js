// Browser end-to-end check of the E2 integrations UI against a real ARTEX backend
// and a fake GitHub Issues API on loopback. scripts/e2e/integrations.sh sets the
// whole thing up (throwaway Postgres, backend, web UI, Playwright); run this file
// directly only against an instance you started yourself:
//
//   ARTEX_E2E_URL=http://127.0.0.1:25173 ARTEX_E2E_PASSWORD=... \
//     NODE_PATH=<dir with playwright> node scripts/e2e/integrations-e2e.js
//
// The backend must run with ARTEX_NOTIFY_ALLOW_LOCAL=1 (the fake tracker is on
// 127.0.0.1) and its admin password must be set. Use a database you can throw away:
// the scenario creates a tracker, an import and findings.
//
// Scenario: add a GitHub tracker; reopen it (token masked); save without
// retyping the token; a destination change without the token is refused; import a
// Nuclei file; follow Past imports → its findings (import_id filter, chip clears);
// push them to the tracker; the finding detail shows the ticket and provenance and a
// re-push updates; close an issue on the tracker and Sync pulls the finding to fixed;
// delete the tracker (with its confirmation). One PASS/FAIL line per check; exit 1
// on any failure. Screenshots go to ARTEX_E2E_SHOTS when set.
const path = require("node:path");
const { chromium } = require("playwright");
const { startFakeGitHub } = require("./fake-github");
const { probeRoles } = require("./roles-e2e");

const base = (process.env.ARTEX_E2E_URL || "http://127.0.0.1:25173").replace(/\/$/, "");
const password = process.env.ARTEX_E2E_PASSWORD;
const shots = process.env.ARTEX_E2E_SHOTS || "";
const testdata = path.join(__dirname, "..", "..", "scanimport", "testdata");
const SECRET = "ghp_e2e_secret_token_ABC123";
const TRACKER = "Fake GH";
if (!password) throw new Error("set ARTEX_E2E_PASSWORD");

let failed = false;
const check = (name, ok, detail) => {
  console.log(`${ok ? "PASS" : "FAIL"} ${name}${detail !== undefined ? `: ${detail}` : ""}`);
  if (!ok) failed = true;
};
const must = (name, ok, detail) => {
  check(name, ok, detail);
  if (!ok) throw new Error(`required check failed: ${name}`);
};

(async () => {
  const gh = await startFakeGitHub(Number(process.env.ARTEX_E2E_FAKE_GH_PORT || 0));
  const browser = await chromium.launch();
  const ctx = await browser.newContext({ viewport: { width: 1440, height: 900 } });
  const page = await ctx.newPage();
  const problems = [];
  page.on("pageerror", (e) => problems.push(`pageerror: ${String(e).slice(0, 200)}`));
  const shot = (name, on = page) =>
    shots ? on.screenshot({ path: path.join(shots, `${name}.png`), fullPage: true }) : null;
  // JSON from the API with the browser's session cookie.
  const api = (p, init) =>
    page.evaluate(
      async ([url, opts]) => {
        const r = await fetch(url, opts);
        return { status: r.status, body: await r.json().catch(() => null) };
      },
      [`/api${p}`, init || {}],
    );
  const toast = async (re) => {
    const t = page.locator("[data-sonner-toast]").filter({ hasText: re }).first();
    await t.waitFor({ timeout: 20000 });
    return (await t.innerText()).replace(/\s+/g, " ");
  };
  const open = (p) => page.goto(base + p, { waitUntil: "networkidle", timeout: 120000 });

  try {
    // ---- sign in ----
    await page.goto(`${base}/login`, { timeout: 120000 });
    await page.fill("#password", password);
    await page.click("#agree-terms");
    await page.getByRole("button", { name: "Sign in" }).click();
    await page.waitForURL((u) => !u.pathname.startsWith("/login"), { timeout: 30000 });
    must("sign in", true, page.url());

    // ---- add a GitHub tracker ----
    await open("/system/integrations");
    await page.getByRole("button", { name: "Add tracker" }).click();
    const sheet = page.getByRole("dialog");
    await page.waitForSelector("#tracker-name");
    if (!(await page.locator("#cfg-repo").count())) {
      await sheet.getByRole("combobox").first().click();
      await page.getByRole("option", { name: "GitHub Issues" }).click();
    }
    await page.waitForSelector("#cfg-repo");
    await page.fill("#tracker-name", TRACKER);
    await page.fill("#cfg-repo", "acme/app");
    await page.fill("#cfg-token", SECRET);
    await page.fill("#cfg-base_url", gh.url);
    await page.fill("#cfg-labels", "security, artex");
    await sheet.getByRole("button", { name: "Add", exact: true }).click();
    must("add tracker", Boolean(await toast(`Added: ${TRACKER}`)));
    const row = page.getByRole("row", { name: new RegExp(TRACKER) });
    await row.waitFor();
    await shot("01-tracker-added");
    const trackers = (await api("/integrations/trackers")).body?.trackers ?? [];
    const tracker = trackers.find((t) => t.name === TRACKER);
    must("tracker listed by the API", Boolean(tracker), tracker && `id ${tracker.id}`);
    check("token masked on readback", !JSON.stringify(tracker.config).includes(SECRET));

    // ---- masked edit keeps the token ----
    await row.getByRole("button", { name: "Edit" }).click();
    await page.waitForSelector("#cfg-token");
    await page.waitForTimeout(500);
    check("token not shown in the edit sheet", !(await page.inputValue("#cfg-token")).includes(SECRET));
    await sheet.getByRole("button", { name: "Save", exact: true }).click();
    check("save without retyping the token", Boolean(await toast(`Saved: ${TRACKER}`)));

    // ---- a destination change without the token is refused ----
    await row.getByRole("button", { name: "Edit" }).click();
    await page.waitForSelector("#cfg-token");
    await page.waitForTimeout(500);
    await page.fill("#cfg-repo", "acme/other");
    await sheet.getByRole("button", { name: "Save", exact: true }).click();
    await page.locator("[data-sonner-toast][data-type=error]").first().waitFor({ timeout: 20000 });
    const kept = (await api("/integrations/trackers")).body.trackers.find((t) => t.id === tracker.id);
    check("destination change without the token refused", kept?.config?.repo === "acme/app", kept?.config?.repo);
    await page.keyboard.press("Escape");

    // ---- import a ZAP file (findings the import filter must leave out), then a Nuclei file ----
    await open("/system/integrations/imports");
    const runImport = async (file, format) => {
      await page.setInputFiles("#import-file", path.join(testdata, file));
      await page.waitForTimeout(500);
      check(`${file}: format detected`, format.test(await page.getByRole("combobox", { name: "Format" }).innerText()));
      await page.getByRole("button", { name: "Import", exact: true }).click();
      const text = await toast(`Imported ${file}`);
      const created = Number((text.match(/(\d+) new findings/) || [])[1] || 0);
      must(`${file}: import created findings`, created > 0, text);
      await page.locator("[data-sonner-toast]").first().waitFor({ state: "detached", timeout: 30000 }).catch(() => {});
      return created;
    };
    const otherCount = await runImport("zap.json", /ZAP/);
    const newCount = await runImport("nuclei.jsonl", /Nuclei/);
    await shot("02-imported");
    const latest = (await api("/integrations/imports?limit=1")).body;
    const importId = latest.imports[0].id;
    check("imports list pages (limit=1, total)", latest.imports.length === 1 && latest.total >= 1, `total ${latest.total}`);

    // ---- Past imports → that import's findings ----
    await page.locator(`#import-${importId}`).getByRole("link", { name: `Show the ${newCount} new findings of import #${importId}` }).click();
    await page.waitForURL((u) => u.searchParams.get("import_id") === String(importId), { timeout: 30000 });
    await page.waitForLoadState("networkidle");
    check("import filter chip shown", await page.getByText(`Import #${importId}`).isVisible());
    const filtered = (await api(`/exploration/findings?page=1&limit=200&import_id=${importId}`)).body;
    check("API: import_id filter returns the new findings", filtered.total === newCount, `${filtered.total}/${newCount}`);
    const boxes = page.getByRole("checkbox", { name: "Select this vulnerability" });
    await boxes.first().waitFor({ timeout: 30000 });
    check(
      "findings page lists exactly the import's findings",
      (await boxes.count()) === newCount,
      `${await boxes.count()} rows, import ${newCount}, other import ${otherCount}`,
    );
    await shot("03-findings-by-import");

    // ---- push the import's findings to the tracker ----
    for (let i = 0; i < (await boxes.count()); i++) await boxes.nth(i).click();
    await page.getByRole("combobox", { name: "Push to tracker" }).click();
    await page.getByRole("option", { name: TRACKER }).click();
    const pushed = await toast(`${TRACKER}: `);
    check("push created one ticket per finding", pushed.includes(`${newCount} created`) && gh.issues.size === newCount, pushed);
    check("tracker called with credentials", gh.log.some((l) => l.startsWith("POST") && l.endsWith("auth=yes")));

    // ---- chip clears the filter ----
    await page.getByRole("button", { name: "Clear the import filter" }).click();
    await page.waitForURL((u) => !u.searchParams.has("import_id"), { timeout: 15000 });
    await page.waitForLoadState("networkidle");
    await page.waitForTimeout(1000);
    const all = await boxes.count();
    check("clearing the chip shows every finding again", all === newCount + otherCount, `${all} rows`);

    // ---- finding detail: ticket, provenance, idempotent re-push ----
    const findingId = filtered.items[0].id;
    await open(`/function/findings/detail?id=${findingId}`);
    const card = page.locator("div[data-slot=card]").filter({ hasText: "Imported from a scanner" });
    await card.waitFor({ timeout: 30000 });
    const ticketLink = card.locator(`a[href^="${gh.url}"]`);
    check("detail shows the ticket link", (await ticketLink.count()) === 1, await ticketLink.first().getAttribute("href"));
    await shot("04-detail");
    await card.getByRole("combobox", { name: "Push to tracker" }).click();
    await page.getByRole("option", { name: TRACKER }).click();
    const repushed = await toast(/1 updated/);
    check("re-push updates, never duplicates", gh.issues.size === newCount, repushed);

    // ---- sync pulls a closed issue to fixed ----
    // A human closes the finding's issue on the tracker as completed.
    const ticket = (await api(`/exploration/findings/${findingId}/integrations`)).body.tickets[0];
    gh.close(Number(ticket.external_id), "completed");
    await open("/system/integrations");
    await page.getByRole("row", { name: new RegExp(TRACKER) }).getByRole("button", { name: "Sync" }).click();
    await page.getByText(`Last sync: ${TRACKER}`).waitFor({ timeout: 30000 });
    await shot("05-synced");
    const status = (await api(`/exploration/findings/${findingId}`)).body.status;
    check("sync pulled the closed issue to fixed", status === "fixed", status);

    // ---- role probes: reviewer and client_read see no control the server refuses ----
    const shown = (await api(`/exploration/findings/${findingId}`)).body;
    const findingTitle = shown.name || shown.vulnclass;
    await probeRoles(browser, {
      base,
      api,
      check,
      shot,
      findingId,
      findingTitle,
      tracker: TRACKER,
      dsn: process.env.ARTEX_E2E_PG_DSN,
    });

    // ---- delete with confirmation ----
    await page.getByRole("row", { name: new RegExp(TRACKER) }).getByRole("button", { name: "Delete" }).click();
    const confirm = page.getByRole("alertdialog");
    await confirm.waitFor();
    await confirm.getByRole("button", { name: "Delete" }).click();
    check("delete tracker", Boolean(await toast(`Deleted: ${TRACKER}`)));
    const left = (await api("/integrations/trackers")).body.trackers.filter((t) => t.id === tracker.id);
    check("tracker gone", left.length === 0);
  } catch (e) {
    check("scenario completed", false, String(e).split("\n")[0]);
    await shot("zz-failure");
  } finally {
    for (const p of problems) console.log(`INFO ${p}`);
    await browser.close();
    await gh.stop();
  }
  process.exit(failed ? 1 : 0);
})();
