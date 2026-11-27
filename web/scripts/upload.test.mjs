import ts from "typescript";

import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import { test } from "node:test";
import { fileURLToPath } from "node:url";
import vm from "node:vm";

const web = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const compiled = new Map();
function compile(file) {
  if (!compiled.has(file))
    compiled.set(
      file,
      ts.transpileModule(fs.readFileSync(file, "utf8"), {
        compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 },
      }).outputText,
    );
  return compiled.get(file);
}
const MiB = 1024 * 1024;

function setup(mock = false) {
  const requests = [];
  const forms = [];
  const sandbox = {
    FormData: class {
      fields = [];
      constructor() {
        forms.push(this);
      }
      append(name, value) {
        this.fields.push([name, value]);
      }
    },
    fetch: async (url, init) => {
      requests.push({ url, init });
      return { ok: true, json: async () => ({ attachments: [] }) };
    },
  };
  const modules = new Map();
  function load(file) {
    if (modules.has(file)) return modules.get(file);
    const moduleExports = {};
    modules.set(file, moduleExports);
    vm.runInNewContext(compile(file), {
      ...sandbox,
      exports: moduleExports,
      require: (name) => requireFrom(file, name),
    });
    return moduleExports;
  }
  function requireFrom(file, name) {
    if (name === "@/lib/mock/enabled") return { MOCK: mock };
    if (name === "@/lib/mock/handler") return { mockHandle: () => assert.fail("unexpected mock request") };
    // English passthrough: assertions below match the source (English) messages.
    if (name === "@/i18n")
      return { t: (key, vars) => key.replace(/\{(\w+)\}/g, (m, k) => (vars && k in vars ? String(vars[k]) : m)) };
    // The api barrel and its per-domain modules (relative imports only).
    if (name.startsWith("./")) return load(path.resolve(path.dirname(file), name) + ".ts");
    throw new Error("Unexpected import: " + name);
  }
  return { api: load(path.join(web, "src/lib/api.ts")).api, requests, forms };
}

for (const scope of ["task", "session", "staging"]) {
  test(scope + ": rejects oversized single and combined files before FormData or fetch", async () => {
    const s = setup();
    for (const sizes of [[512 * MiB + 1], [256 * MiB, 256 * MiB + 1]]) {
      const files = sizes.map((size, i) => ({ name: i + ".apk", size }));
      await assert.rejects(s.api.chatUpload(scope, "42", files), /512 MiB.*combined size/);
    }
    assert.equal(s.forms.length, 0);
    assert.equal(s.requests.length, 0);
  });

  test(scope + ": accepts ordinary files and exactly 512 MiB, including combined files", async () => {
    for (const sizes of [[129 * MiB], [512 * MiB], [256 * MiB, 256 * MiB]]) {
      const s = setup();
      const files = sizes.map((size, i) => ({ name: i + ".apk", size }));
      const result = await s.api.chatUpload(scope, "42", files);
      assert.equal(result.attachments.length, 0);
      assert.equal(s.requests.length, 1);
      assert.equal(s.requests[0].url, "/api/chat/upload?scope=" + scope + "&id=42");
      assert.equal(s.requests[0].init.method, "POST");
      assert.equal(s.requests[0].init.body, s.forms[0]);
      assert.deepEqual(
        s.forms[0].fields,
        files.map((file) => ["file", file]),
      );
    }
  });
}

test("mock uploads enforce the same combined limit", async () => {
  const s = setup(true);
  await assert.rejects(s.api.chatUpload("staging", "42", [{ name: "app.apk", size: 512 * MiB + 1 }]), /512 MiB/);
  const result = await s.api.chatUpload("staging", "42", [{ name: "app.apk", size: 512 * MiB }]);
  assert.equal(result.attachments[0].size, 512 * MiB);
  assert.equal(s.requests.length, 0);
});
