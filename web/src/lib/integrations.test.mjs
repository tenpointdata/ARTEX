import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

import { buildConfig, isMasked, maskedTail, parseList } from "./config-form.ts";
import {
  chunk,
  detectImportFormat,
  evidenceEnabled,
  pushableFindingIds,
  summarizePush,
  summarizeSync,
  trackerDestination,
} from "./integrations.ts";

const fixtures = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../../../scanimport/testdata");
const head = (name) => fs.readFileSync(path.join(fixtures, name), "utf8").slice(0, 4096);

test("detectImportFormat recognizes every backend fixture by content", () => {
  // Names without a telling extension, so the content decides.
  assert.equal(detectImportFormat("scan.xml", head("nmap.xml")), "nmap-xml");
  assert.equal(detectImportFormat("scan.xml", head("burp.xml")), "burp-xml");
  assert.equal(detectImportFormat("out.json", head("zap.json")), "zap-json");
  assert.equal(detectImportFormat("out.json", head("semgrep.sarif")), "sarif");
  assert.equal(detectImportFormat("out.txt", head("nuclei.jsonl")), "nuclei-jsonl");
});

test("detectImportFormat trusts telling extensions and gives up on unknown content", () => {
  assert.equal(detectImportFormat("results.JSONL", ""), "nuclei-jsonl");
  assert.equal(detectImportFormat("x.sarif.json", "{}"), "sarif");
  assert.equal(detectImportFormat("notes.txt", "hello"), null);
  assert.equal(detectImportFormat("other.xml", "<?xml version='1.0'?><report/>"), null);
});

test("buildConfig sends masked secrets back verbatim and an empty box clears", () => {
  const defs = [
    { key: "repo", label: "Repo", kind: "text" },
    { key: "token", label: "Token", kind: "password" },
    { key: "labels", label: "Labels", kind: "list" },
    { key: "test_id", label: "Test", kind: "number" },
    { key: "include_evidence", label: "Evidence", kind: "switch" },
  ];
  const out = buildConfig(defs, {
    repo: " acme/app ",
    token: "__masked__:…a1b2",
    labels: "security, artex  triage",
    test_id: "12",
    extra: "dropped",
  });
  assert.deepEqual(out, {
    repo: "acme/app",
    token: "__masked__:…a1b2",
    labels: ["security", "artex", "triage"],
    test_id: 12,
    include_evidence: false,
  });
  assert.equal(buildConfig(defs, { token: "" }).token, "");
  assert.equal(buildConfig(defs, { include_evidence: true }).include_evidence, true);
});

test("masked helpers and parseList", () => {
  assert.equal(isMasked("__masked__:…xyz"), true);
  assert.equal(isMasked("ghp_real"), false);
  assert.equal(isMasked(undefined), false);
  assert.equal(maskedTail("__masked__:…xyz"), "xyz");
  assert.deepEqual(parseList(["a", " ", "b"]), ["a", "b"]);
});

test("pushableFindingIds keeps positive integer ids once", () => {
  assert.deepEqual(pushableFindingIds(["3", "3", "x", "-1", "0", "1.5", "42"]), [3, 42]);
});

test("chunk splits pushes at the server cap", () => {
  const ids = Array.from({ length: 450 }, (_, i) => i + 1);
  assert.deepEqual(
    chunk(ids, 200).map((c) => c.length),
    [200, 200, 50],
  );
  assert.deepEqual(chunk([], 200), []);
});

test("summarizePush counts created, updated and failed", () => {
  const s = summarizePush([
    { finding_id: 1, created: true },
    { finding_id: 2, created: false },
    { finding_id: 3, created: false, error: "boom" },
    { finding_id: 4, created: false, error: "again" },
  ], 1);
  assert.deepEqual(s, { created: 1, updated: 1, failed: 2, errors: ["#3: boom"] });
});

test("summarizeSync counts each action", () => {
  const s = summarizeSync([
    { finding_id: 1, external_id: "1", action: "pulled" },
    { finding_id: 2, external_id: "2", action: "pulled" },
    { finding_id: 3, external_id: "3", action: "error", error: "x" },
  ]);
  assert.deepEqual(s, { unchanged: 0, pulled: 2, pushed: 0, error: 1 });
});

test("trackerDestination and evidenceEnabled", () => {
  assert.equal(trackerDestination({ kind: "github", config: { repo: "acme/app" } }), "acme/app");
  assert.equal(
    trackerDestination({ kind: "github", config: { repo: "acme/app", base_url: "https://ghe.example.com/api/v3" } }),
    "ghe.example.com/acme/app",
  );
  assert.equal(
    trackerDestination({ kind: "jira", config: { base_url: "https://acme.atlassian.net", project_key: "SEC" } }),
    "acme.atlassian.net · SEC",
  );
  assert.equal(
    trackerDestination({ kind: "defectdojo", config: { base_url: "https://dojo.example.com", test_id: 7 } }),
    "dojo.example.com · test 7",
  );
  assert.equal(evidenceEnabled({ include_evidence: true }), true);
  assert.equal(evidenceEnabled({ include_evidence: "true" }), false);
});
