import { allows, filterGroups, parsePermissions } from "./permissions.ts";
import assert from "node:assert/strict";
import test from "node:test";

const groups = [
  {
    id: 1,
    items: [
      { id: "findings", url: "/function/findings" },
      { id: "cost", url: "/function/cost", requires: "read" },
      {
        id: "parent",
        subItems: [
          { id: "open", url: "/a" },
          { id: "admin-only", url: "/b", requires: "admin" },
        ],
      },
    ],
  },
  { id: 2, items: [{ id: "logs", url: "/system/logs", requires: "operate" }] },
];

const ids = (gs) => gs.flatMap((g) => g.items.flatMap((i) => [i.id, ...(i.subItems ?? []).map((s) => s.id)]));

test("parsePermissions keeps only known permissions", () => {
  assert.deepEqual(parsePermissions(["read", "bogus", 3, "client_read"]), ["client_read", "read"]);
  assert.deepEqual(parsePermissions("read"), []);
  assert.deepEqual(parsePermissions(undefined), []);
});

test("gated entries are hidden while permissions are unknown", () => {
  assert.equal(allows(null, { requires: "read" }), false);
  assert.equal(allows(null, {}), true);
});

test("client_read does not see the cost entry or gated groups", () => {
  const got = filterGroups(groups, ["client_read"]);
  assert.deepEqual(ids(got), ["findings", "parent", "open"]);
  assert.equal(got.length, 1, "a group left empty is dropped");
});

test("an internal role sees the entries its permissions allow", () => {
  assert.deepEqual(ids(filterGroups(groups, ["client_read", "read", "review", "operate"])), [
    "findings",
    "cost",
    "parent",
    "open",
    "logs",
  ]);
  assert.deepEqual(ids(filterGroups(groups, ["client_read", "read", "review", "operate", "admin"])), [
    "findings",
    "cost",
    "parent",
    "open",
    "admin-only",
    "logs",
  ]);
});

test("a parent whose sub-entries are all hidden is dropped", () => {
  const only = [{ id: 1, items: [{ id: "p", subItems: [{ id: "x", url: "/x", requires: "admin" }] }] }];
  assert.deepEqual(filterGroups(only, ["read"]), []);
});
