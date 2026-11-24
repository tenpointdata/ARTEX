import assert from "node:assert/strict";
import test from "node:test";
import { classifyMarkdownImage } from "./markdown-image.ts";

const base = "http://localhost:8787";

test("same-origin and relative images load inline", () => {
  assert.deepEqual(classifyMarkdownImage("/api/evidence/1.png", base), {
    kind: "inline",
    src: "http://localhost:8787/api/evidence/1.png",
  });
  assert.equal(classifyMarkdownImage("shot.png", base).kind, "inline");
  assert.equal(classifyMarkdownImage("http://localhost:8787/a.png", base).kind, "inline");
});

test("data:image and same-origin blob: load inline", () => {
  assert.equal(classifyMarkdownImage("data:image/png;base64,AAAA", base).kind, "inline");
  assert.equal(classifyMarkdownImage("blob:http://localhost:8787/abc", base).kind, "inline");
});

test("remote images never load: they become an operator-clicked link", () => {
  assert.deepEqual(classifyMarkdownImage("https://evil.example/p.png?id=1", base), {
    kind: "remote",
    href: "https://evil.example/p.png?id=1",
    host: "evil.example",
  });
  assert.equal(classifyMarkdownImage("http://localhost:9999/x.png", base).kind, "remote"); // other port
  assert.equal(classifyMarkdownImage("//evil.example/p.png", base).kind, "remote"); // protocol-relative
  assert.equal(classifyMarkdownImage("HTTPS://EVIL.example/p.png", base).kind, "remote");
});

test("non-image data, foreign blobs, script and junk schemes are blocked", () => {
  assert.equal(classifyMarkdownImage("data:text/html,<script>1</script>", base).kind, "blocked");
  assert.equal(classifyMarkdownImage("blob:https://evil.example/abc", base).kind, "blocked");
  assert.equal(classifyMarkdownImage("javascript:alert(1)", base).kind, "blocked");
  assert.equal(classifyMarkdownImage("ftp://evil.example/x.png", base).kind, "blocked");
  assert.equal(classifyMarkdownImage("", base).kind, "blocked");
  assert.equal(classifyMarkdownImage(undefined, base).kind, "blocked");
});
