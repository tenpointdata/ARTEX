import { liveEventsPath, parseLiveEvent, touches } from "./live-events.ts";
import assert from "node:assert/strict";
import test from "node:test";

test("parseLiveEvent reads the server's frames", () => {
  assert.deepEqual(parseLiveEvent('{"kind":"finding","op":"insert","id":7,"task":"3"}'), {
    kind: "finding",
    op: "insert",
    id: 7,
    task: "3",
  });
  assert.deepEqual(parseLiveEvent('{"kind":"resync"}'), { kind: "resync" });
});

test("parseLiveEvent drops malformed and unknown frames", () => {
  assert.equal(parseLiveEvent("not json"), null);
  assert.equal(parseLiveEvent("null"), null);
  assert.equal(parseLiveEvent('{"kind":"secret"}'), null);
  assert.deepEqual(parseLiveEvent('{"kind":"task","op":"drop","id":"x"}'), { kind: "task" });
});

test("liveEventsPath carries the company", () => {
  assert.equal(liveEventsPath(), "/api/events/stream");
  assert.equal(liveEventsPath("12"), "/api/events/stream?company=12");
});

test("a resync touches every kind", () => {
  assert.equal(touches({ kind: "resync" }, ["task"]), true);
  assert.equal(touches({ kind: "approval" }, ["task", "finding"]), false);
  assert.equal(touches({ kind: "finding" }, ["task", "finding"]), true);
});
