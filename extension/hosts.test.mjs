// Run with: node --test extension/hosts.test.mjs
import assert from "node:assert/strict";
import fs from "node:fs";
import test from "node:test";
import vm from "node:vm";

const ctx = { URL };
ctx.globalThis = ctx;
vm.runInNewContext(fs.readFileSync(new URL("./hosts.js", import.meta.url), "utf8"), ctx);
const { isSkippedUrl, parseHostList, originPattern, DEFAULT_SKIP_HOSTS } = ctx.ArtexHosts;

test("default SSO/IdP hosts are skipped, including subdomains of wildcard entries", () => {
  for (const u of [
    "https://accounts.google.com/o/oauth2/auth?client_id=x",
    "https://login.microsoftonline.com/common/oauth2/v2.0/authorize",
    "https://acme.okta.com/app/x/sso/saml",
    "https://acme.auth0.com/authorize",
    "https://sso.onelogin.com/",
    "https://login.live.com/",
    "https://api-1234.duosecurity.com/frame",
    "https://appleid.apple.com/auth/authorize",
    "https://ACCOUNTS.GOOGLE.COM./x",
  ]) {
    assert.equal(isSkippedUrl(u, []), true, u);
  }
});

test("ordinary and look-alike hosts are captured", () => {
  for (const u of [
    "https://app.example.com/login",
    "https://notokta.com/",
    "https://okta.com.evil.example/",
    "https://mail.google.com/",
    "https://google.com/",
  ]) {
    assert.equal(isSkippedUrl(u, []), false, u);
  }
});

test("operator-supplied patterns extend the list", () => {
  const extra = parseHostList("# corp idp\nsso.corp.example\n*.idp.example, Login.Other.Example.\n\n");
  assert.deepEqual([...extra], ["sso.corp.example", "*.idp.example", "login.other.example"]);
  assert.equal(isSkippedUrl("https://sso.corp.example/saml", extra), true);
  assert.equal(isSkippedUrl("https://a.b.idp.example/", extra), true);
  assert.equal(isSkippedUrl("https://idp.example/", extra), true);
  assert.equal(isSkippedUrl("https://app.corp.example/", extra), false);
});

test("unparseable URLs are skipped, never kept", () => {
  assert.equal(isSkippedUrl("not a url", []), true);
  assert.equal(isSkippedUrl("", []), true);
});

test("origin pattern is per host and http(s) only", () => {
  assert.equal(originPattern("https://app.example.com:8443/a?b=1"), "https://app.example.com/*");
  assert.equal(originPattern("http://10.0.0.5/x"), "http://10.0.0.5/*");
  assert.equal(originPattern("chrome://extensions"), null);
  assert.equal(originPattern("file:///etc/passwd"), null);
  assert.equal(originPattern("nope"), null);
});

test("default list has the documented providers", () => {
  assert.ok(DEFAULT_SKIP_HOSTS.length >= 8);
});
