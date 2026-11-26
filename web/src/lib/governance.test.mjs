import {
  authorizationStatus,
  can,
  formatScopeText,
  fromLocalInput,
  guessScopeKind,
  isForbidden,
  landingPath,
  parseScopeText,
  toLocalInput,
} from "./governance.ts";
import assert from "node:assert/strict";
import test from "node:test";

test("can reads the session's permission set", () => {
  const reviewer = { permissions: ["client_read", "read", "review"] };
  assert.equal(can(reviewer, "review"), true);
  assert.equal(can(reviewer, "operate"), false);
  assert.equal(can(reviewer, "admin"), false);
  assert.equal(can({}, "read"), false);
  assert.equal(can(null, "read"), false);
});

test("isForbidden matches only a 403", () => {
  assert.equal(isForbidden({ status: 403 }), true);
  assert.equal(isForbidden({ status: 401 }), false);
  assert.equal(isForbidden(new Error("x")), false);
  assert.equal(isForbidden(null), false);
});

test("scope text round-trips and guesses kinds", () => {
  assert.equal(guessScopeKind("203.0.113.0/24"), "cidr");
  assert.equal(guessScopeKind("203.0.113.7"), "ip");
  assert.equal(guessScopeKind("2001:db8::1"), "ip");
  assert.equal(guessScopeKind("example.com"), "domain");
  const scope = parseScopeText(
    "example.com\nhost:api.example.com, 10.0.0.0/8\n\n2001:db8::1\nexample.com\nDOMAIN:x.org",
  );
  assert.deepEqual(scope, [
    { kind: "domain", value: "example.com" },
    { kind: "host", value: "api.example.com" },
    { kind: "cidr", value: "10.0.0.0/8" },
    { kind: "ip", value: "2001:db8::1" },
    { kind: "domain", value: "x.org" },
  ]);
  assert.deepEqual(parseScopeText(formatScopeText(scope)), scope);
  assert.equal(formatScopeText(null), "");
});

test("datetime-local conversion keeps the instant", () => {
  const iso = "2026-12-01T09:30:00.000Z";
  assert.equal(fromLocalInput(toLocalInput(iso)), iso);
  assert.equal(toLocalInput(""), "");
  assert.equal(fromLocalInput("nonsense"), "");
});

test("authorizationStatus matches the server's InForce window", () => {
  const a = { revoked: false, not_before: "2026-12-01T00:00:00Z", not_after: "2026-12-15T00:00:00Z" };
  assert.equal(authorizationStatus(null, "2026-12-02T00:00:00Z"), "none");
  assert.equal(authorizationStatus(a, "2026-11-30T23:59:59Z"), "not_yet");
  assert.equal(authorizationStatus(a, "2026-12-01T00:00:00Z"), "in_force");
  assert.equal(authorizationStatus(a, "2026-12-15T00:00:00Z"), "expired");
  assert.equal(authorizationStatus({ ...a, revoked: true }, "2026-12-02T00:00:00Z"), "revoked");
});

test("landingPath sends a client_read session to its summary", () => {
  assert.equal(landingPath({ permissions: ["client_read", "read"] }), "/function/tasks");
  assert.equal(landingPath({ permissions: ["client_read"] }), "/function/reports");
  assert.equal(landingPath(null), "/function/reports");
});
