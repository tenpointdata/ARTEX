import { flattenRoutes, routeNeed } from "./route-access.ts";
import assert from "node:assert/strict";
import { test } from "node:test";

const rules = flattenRoutes([
  {
    items: [
      { url: "/dashboard" },
      { url: "/chat", need: "operate" },
      { url: "/function/tasks" },
      { url: "/function/findings", need: "client_read" },
      { url: "/function/reports", need: "client_read" },
    ],
  },
  {
    items: [
      { url: "/system/intercept" },
      { url: "/system/intercept/approvals", need: "review", subItems: [{ url: "/system/logs", need: "operate" }] },
      { url: "/system/users", need: "admin" },
    ],
  },
]);

test("exact routes take their own need", () => {
  assert.equal(routeNeed(rules, "/chat"), "operate");
  assert.equal(routeNeed(rules, "/system/users"), "admin");
  assert.equal(routeNeed(rules, "/function/findings"), "client_read");
});

test("trailing slash, query and encoding are ignored", () => {
  assert.equal(routeNeed(rules, "/chat/"), "operate");
  assert.equal(routeNeed(rules, "/function/findings/?id=3"), "client_read");
  assert.equal(routeNeed(rules, "/%63hat"), "operate");
});

test("child paths inherit the longest matching parent", () => {
  assert.equal(routeNeed(rules, "/function/findings/detail"), "client_read");
  assert.equal(routeNeed(rules, "/function/tasks/detail/"), "read");
  assert.equal(routeNeed(rules, "/system/intercept/approvals/x"), "review");
  assert.equal(routeNeed(rules, "/system/intercept/assets"), "read");
});

test("sub-items count and a prefix must end at a segment", () => {
  assert.equal(routeNeed(rules, "/system/logs"), "operate");
  assert.equal(routeNeed(rules, "/chatty"), "read");
});

test("uncovered paths need read", () => {
  assert.equal(routeNeed(rules, "/"), "read");
  assert.equal(routeNeed(rules, "/function/unknown"), "read");
  assert.equal(routeNeed([], "/x"), "read");
});
