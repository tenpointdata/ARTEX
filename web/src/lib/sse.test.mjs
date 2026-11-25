import { sseUrl } from "./sse.ts";
import assert from "node:assert/strict";
import test from "node:test";

test("sseUrl keeps a stream on the page's origin so the session cookie is sent", () => {
  const path = "/api/exploration/activity/stream?task=7&since=12";
  assert.equal(sseUrl(path), path);
});

test("sseUrl refuses a URL the session cookie would not reach", () => {
  assert.throws(() => sseUrl("http://localhost:8787/api/logs/stream"));
  assert.throws(() => sseUrl("//example.test/api/logs/stream"));
  assert.throws(() => sseUrl("api/logs/stream"));
});
