# Upstream report: go-mitmproxy keep-alive host pinning (card c-9b48b7)

**Status (2026-11-24):** drafted and tested, **not filed**. Filing is outward-facing and waits on
Ian's go-ahead. The bug is present in every release through **v1.9.5**; upstream `main` is
`07e0ca5` = v1.9.5. No existing upstream issue or PR covers it (searched "keep-alive",
"ServerConn host", "dial"). ARTEX pins v1.9.2 and keeps `unpinHostChange` (`traffic/scope.go`).

Patch: [`go-mitmproxy-keepalive-host-pinning.patch`](go-mitmproxy-keepalive-host-pinning.patch)
(applies to upstream `main` 07e0ca5; adds `proxy/keepalive_host_test.go`). Verified: the new test
fails without the fix (`expected B, but got ""`), passes with it, and `go test ./...` passes.

## Why not "redial" in place

The card proposed redialling when the canonical address changes. Closing the stale
`ServerConn` calls `wrapServerConn.Close`, which for a non-TLS client half-closes the client's
read side (`proxy/entry.go`, `CloseRead`), so the client connection would die. The patch instead
sends the request through the attacker's separate client (the same path upstream already takes
when an addon rewrites the host), limited to connections that carry absolute-form plain HTTP
proxy requests. Requests inside a CONNECT tunnel, including the new plain-HTTP-in-tunnel mode,
are untouched: there the CONNECT fixed the server.

## Issue text (for `gh issue create -R lqqyt2423/go-mitmproxy`)

Title: `Keep-alive plain HTTP proxy connection sends requests for a second host to the first host's server`

> **Version:** v1.9.5 (also v1.9.2; `main` at 07e0ca5)
>
> **What happens:** When a client reuses one keep-alive connection to the proxy for plain HTTP
> requests to two different hosts (`GET http://a.example/` then `GET http://b.example/`), the
> second request is sent over the upstream connection that was dialled for `a.example`.
> `attacker.attack` only calls `ConnContext.dialFn` while `ServerConn == nil`, and
> `initHttpDialFn` records `ServerConn.Address` for the first host only, so every later request
> on that client connection reuses it regardless of its own host.
>
> **Impact:** wrong responses (host B's request answered by host A), and a host-based policy
> enforced in `getUpstreamConn` / upstream proxy selection is applied only to the first request
> on a connection.
>
> **Reproduce:** two local HTTP servers answering "A" and "B"; an `http.Transport` with
> `Proxy` set to go-mitmproxy and `MaxConnsPerHost: 1`; request A, then B. B's body is not "B".
> A test is included in the PR.
>
> **Proposed fix:** when a plain HTTP proxy connection's request names a different canonical
> address than `ServerConn.Address`, send it with the separate client (as already done when an
> addon changes the host). Closing and redialling `ServerConn` is not safe because
> `wrapServerConn.Close` half-closes the client connection.

## PR

Branch from upstream `main`, apply the patch, commit
`fix(proxy): do not reuse a keep-alive upstream connection for a different host`, and open a PR
that references the issue (body: the "Proposed fix" paragraph plus the test result).

## After a fixed release

Bump `github.com/lqqyt2423/go-mitmproxy` in `go.mod`, delete `unpinHostChange` and its call in
`traffic/pace.go` (and `absoluteForm` if nothing else uses it), keep
`TestProxyKeepAliveHostChange` and `TestProxiedRedirectToMetadataRefused`, run `scripts/verify.sh`.
