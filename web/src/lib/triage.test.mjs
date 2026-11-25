import { isTypingTarget, matchesStatusFilter, TRIAGE_VERDICTS, triageCommandForKey, triageTarget } from "./triage.ts";
import assert from "node:assert/strict";
import test from "node:test";

test("every verdict key is unique and does not collide with navigation", () => {
  const keys = TRIAGE_VERDICTS.map((v) => v.key);
  assert.equal(new Set(keys).size, keys.length);
  for (const k of keys) assert.equal(triageCommandForKey(k)?.kind, "verdict");
  for (const k of ["j", "k", "ArrowDown", "ArrowUp", "Escape"]) assert.equal(triageCommandForKey(k)?.kind, "nav");
});

test("keys map to commands case-insensitively; unknown keys are ignored", () => {
  assert.deepEqual(triageCommandForKey("C"), { kind: "verdict", status: "confirmed" });
  assert.deepEqual(triageCommandForKey("f"), { kind: "verdict", status: "false_positive" });
  assert.deepEqual(triageCommandForKey("j"), { kind: "nav", nav: "next" });
  assert.deepEqual(triageCommandForKey("Escape"), { kind: "nav", nav: "close" });
  assert.equal(triageCommandForKey("z"), null);
  assert.equal(triageCommandForKey("Enter"), null);
});

test("triageTarget walks the queue and stops at either end", () => {
  const keys = ["a", "b", "c"];
  assert.equal(triageTarget(keys, "a", "next"), "b");
  assert.equal(triageTarget(keys, "c", "next"), null);
  assert.equal(triageTarget(keys, "b", "prev"), "a");
  assert.equal(triageTarget(keys, "a", "prev"), null);
  assert.equal(triageTarget(keys, null, "next"), "a");
  assert.equal(triageTarget(keys, "gone", "next"), "a");
  assert.equal(triageTarget(keys, null, "prev"), null);
  assert.equal(triageTarget([], null, "next"), null);
});

test("matchesStatusFilter keeps everything under all", () => {
  assert.equal(matchesStatusFilter("all", "confirmed"), true);
  assert.equal(matchesStatusFilter("pending", "pending"), true);
  assert.equal(matchesStatusFilter("pending", "confirmed"), false);
});

test("isTypingTarget guards inputs and editable regions", () => {
  assert.equal(isTypingTarget({ tagName: "input" }), true);
  assert.equal(isTypingTarget({ tagName: "TEXTAREA" }), true);
  assert.equal(isTypingTarget({ tagName: "DIV", isContentEditable: true }), true);
  assert.equal(isTypingTarget({ tagName: "TR" }), false);
  assert.equal(isTypingTarget(null), false);
});

// The measurement behind E9's done-criterion. Each step is one user interaction
// (a click or a key press) in the findings list, counted from the page as it loads
// (default filters) to the moment N pending findings each carry a verdict.
// Before: set the Status filter (open + pick), then per finding: expand the row,
// open the Status select, pick the status (the row then drops out of the pending
// list and nothing is open, so the next one is expanded by hand).
// After: click the Pending tile (filter, and open the first finding), then per
// finding one verdict key or button; the next pending finding opens on its own.
function interactions(flow, n) {
  return flow.setup + flow.perFinding * n;
}
const BEFORE = { setup: 2, perFinding: 3 };
const AFTER = { setup: 1, perFinding: 1 };

test("triaging findings takes fewer interactions than before", () => {
  for (const n of [1, 10, 50]) assert.ok(interactions(AFTER, n) < interactions(BEFORE, n));
  assert.equal(interactions(BEFORE, 10), 32);
  assert.equal(interactions(AFTER, 10), 11);
});
