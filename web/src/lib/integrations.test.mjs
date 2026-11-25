import { buildConfig, isMasked, maskedTail, parseList } from "./config-form.ts";
import {
  anchorAssetIdsParam,
  chunk,
  codeLocation,
  detectImportFormat,
  evidenceEnabled,
  importFindingsHref,
  importIdParam,
  laneCountEntries,
  laneParam,
  pushableFindingIds,
  summarizePush,
  summarizeSync,
  trackerDestination,
  webhookURL,
} from "./integrations.ts";
import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

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
  const s = summarizePush(
    [
      { finding_id: 1, created: true },
      { finding_id: 2, created: false },
      { finding_id: 3, created: false, error: "boom" },
      { finding_id: 4, created: false, error: "again" },
    ],
    1,
  );
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

test("importIdParam keeps only a positive integer", () => {
  assert.equal(importIdParam("42"), "42");
  assert.equal(importIdParam(" 7 "), "7");
  for (const bad of [null, undefined, "", "0", "-1", "1.5", "abc", "1e3", "01"]) assert.equal(importIdParam(bad), "");
});

test("importFindingsHref links to the findings list filtered by import", () => {
  assert.equal(importFindingsHref(12), "/function/findings?import_id=12");
});

test("laneParam keeps only a known lane", () => {
  for (const lane of ["runtime", "sast", "sca"]) assert.equal(laneParam(lane), lane);
  assert.equal(laneParam(" sast "), "sast");
  for (const bad of [null, undefined, "", "all", "SAST", "dast"]) assert.equal(laneParam(bad), "");
});

test("codeLocation renders path:line and tolerates a missing line or path", () => {
  assert.equal(codeLocation({ location: "src/app.go", line: 42 }), "src/app.go:42");
  assert.equal(codeLocation({ location: "src/app.go" }), "src/app.go");
  assert.equal(codeLocation({ location: "go.mod", line: 0 }), "go.mod");
  assert.equal(codeLocation({ line: 7 }), "");
  assert.equal(codeLocation(null), "");
  assert.equal(codeLocation(undefined), "");
});

test("laneCountEntries lists non-zero lanes in a stable order", () => {
  assert.deepEqual(laneCountEntries({ sca: 1, sast: 3 }), [
    { lane: "sast", count: 3 },
    { lane: "sca", count: 1 },
  ]);
  assert.deepEqual(laneCountEntries({ runtime: 0, sast: 0 }), []);
  assert.deepEqual(laneCountEntries(undefined), []);
});

test("anchorAssetIdsParam joins unique positive integer ids", () => {
  assert.equal(anchorAssetIdsParam([3, 1, 3, 2]), "3,1,2");
  assert.equal(anchorAssetIdsParam([0, -1, 1.5, 4]), "4");
  assert.equal(anchorAssetIdsParam([]), "");
});

test("webhookURL joins the origin and the tracker id", () => {
  assert.equal(webhookURL("https://artex.example.com/", 7), "https://artex.example.com/api/hooks/trackers/7");
  assert.equal(webhookURL("http://localhost:8787", 12), "http://localhost:8787/api/hooks/trackers/12");
});
