// Browser login/logout check over the HttpOnly session cookie, run inside the artex
// image with its preinstalled Playwright (no download):
//
//   docker cp scripts/docker-login-probe.js CONTAINER:/tmp/
//   docker exec -e ARTEX_PROBE_PASSWORD=... CONTAINER \
//     sh -c 'NODE_PATH="$(npm root -g)" node /tmp/docker-login-probe.js'
//
// The admin password must already be set (POST /api/auth/init with the setup token).
// Prints one line per check and exits 1 when a required check fails. The replay of
// the old cookie after Sign out is reported, not required: server-side revocation
// lands with card c-a0af33.
const { chromium } = require("playwright");

const base = process.env.ARTEX_PROBE_URL || "http://127.0.0.1:8787";
const password = process.env.ARTEX_PROBE_PASSWORD;
if (!password) throw new Error("set ARTEX_PROBE_PASSWORD");

let failed = false;
const check = (name, ok, detail) => {
  console.log(`${ok ? "PASS" : "FAIL"} ${name}${detail ? `: ${detail}` : ""}`);
  if (!ok) failed = true;
};

(async () => {
  const browser = await chromium.launch();
  const ctx = await browser.newContext();
  const page = await ctx.newPage();
  await page.goto(`${base}/login`);
  await page.fill("#password", password);
  await page.click("#agree-terms");
  await page.getByRole("button", { name: "Sign in" }).click();
  await page.waitForURL((u) => !u.pathname.startsWith("/login"), { timeout: 15000 });
  check("login navigates away from /login", true, page.url());

  const cookies = await ctx.cookies();
  const session = cookies.find((c) => /artex_token$/.test(c.name));
  check("session cookie set", Boolean(session), session && session.name);
  check("cookie HttpOnly", Boolean(session && session.httpOnly));
  check("cookie SameSite=Strict", Boolean(session && session.sameSite === "Strict"));
  const jsCookies = await page.evaluate(() => document.cookie);
  check("token invisible to scripts", !/artex_token/.test(jsCookies));
  const authed = await page.evaluate(async () => (await fetch("/api/tasks")).status);
  check("GET /api/tasks with cookie", authed === 200, String(authed));

  await page.locator('[data-sidebar="menu-button"][data-size="lg"]').last().click();
  await page.getByRole("menuitem", { name: "Sign out" }).click();
  await page.waitForURL((u) => u.pathname.startsWith("/login"), { timeout: 15000 });
  check("Sign out returns to /login", true, page.url());
  const after = await ctx.cookies();
  check("cookie cleared", !after.some((c) => /artex_token$/.test(c.name)));
  const anon = await page.evaluate(async () => (await fetch("/api/tasks")).status);
  check("GET /api/tasks after Sign out", anon === 401, String(anon));

  if (session) {
    const replay = await ctx.request.get(`${base}/api/tasks`, {
      headers: { Cookie: `${session.name}=${session.value}` },
    });
    console.log(`INFO replay of pre-logout cookie: ${replay.status()} (401 once c-a0af33 is merged)`);
  }
  await browser.close();
  process.exit(failed ? 1 : 0);
})().catch((e) => {
  console.error("FAIL", e.message);
  process.exit(1);
});
