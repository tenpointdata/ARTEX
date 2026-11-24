# 2026-11-23 security audit: operator-facing changes

Moved out of `HANDOFF.md` Current state on 2026-11-24 to keep that section within its size limit. These changes are on `main` (PR #1).

- Native (non-Linux-root) starts now refuse unless `ARTEX_ALLOW_UNSANDBOXED=1` is set; `dev.sh` sets it.
- A non-loopback `-addr` without TLS is refused. Use `-tls-cert`/`-tls-key` (or
  `ARTEX_TLS_CERT`/`ARTEX_TLS_KEY`), or set `ARTEX_ALLOW_INSECURE_BIND=1`; the Dockerfile sets that env.
- The session is now an HttpOnly, SameSite=Strict cookie (`__Host-` prefixed over TLS), so users log in
  again once.
- The capture proxy verifies upstream TLS; `ARTEX_CAPTURE_INSECURE_UPSTREAM=1` is the opt-out for
  self-signed targets. A non-loopback `-proxy` needs `ARTEX_PROXY_ALLOW_NONLOOPBACK=1`.
- Docker: the artex container has NET_ADMIN, and `egress.sh` denies the executor uid access to metadata,
  the host gateway and the API port (`ARTEX_EGRESS_ALLOW`/`_DENY`/`_FILTER`). GHCR compose needs
  `ARTEX_IMAGE_TAG`.
- Cloud-metadata and link-local addresses are always denied for platform dials, redirects and WebFetch.
  Since 2026-11-23 (branch `ej/20261123-152404-f61d73`, card `c-485de0`) this includes the capture
  proxy's own upstream dials: go-mitmproxy's upstream is a loopback gate (`traffic/upstream_gate.go`)
  that dials with `Interceptor.DialSOCKS5`.
- **2026-11-24** (card `c-5a6f96`, commit `75184c3`): task agents' proxy URLs now carry a task scope token,
  so capture-proxy upstream dials (and SOCKS5 with the token as password) also apply that task's
  block rules and, with its gate on, its pins/approvals; an ended task's token is refused.
  **2026-11-24** (card `c-eb32cc`, commit `ac7c35e`, branch `ej/20261124-075745-5a4987`, which also
  carries the not-yet-merged `c-5a6f96` branch): chat/conversation agents and `use_recording_proxy`
  HTTP tools now get their run's scoped URL (`Manager.ScopedProxyAddr`, `server/proxy_scope.go`);
  conversations have their own token, forgotten on delete; an unloaded task's call gets a refused
  URL. The transparent listener is global-only by decision. **2026-11-24** (card `c-15b39d`, branch
  `ej/20261124-110649-dbd854`, stacked on PR #13's branch): stdio MCP servers, the Playwright browser
  included, are spawned per run with their capture-proxy `--proxy-server`/proxy env rewritten to a
  per-run loopback **scope relay** (`traffic/scope_relay.go`, `Manager.scopeMCPProxy`), so browser
  dials carry the run's scope; listing tools from the UI (no run) still uses the global rules. **2026-11-24**
  (card `c-a1c0fc`, branch `ej/20261124-114610-cae888`, which carries c-15b39d): a relay serves at most 256
  connections at once; a prepareMCP closer is released at once when its MCP fails to start; tests cover
  the hook (PG), two concurrent scopes, the keep-alive host change, wss, an in-flight request across the
  relay's close (tombstone), the cap, and a real headless Chrome (opt-in `ARTEX_TEST_CHROME`). This is on
  main (landed with #22). Re-verified 2026-11-24 12:30: the PG hook test ran against a throwaway Postgres
  (not skipped) and passed, the Chrome test passed, relay tests pass with -race. A full live task is `c-517c2d`. Operators' own clients keep the global rules. go-mitmproxy's keep-alive host pinning is worked around
  (`unpinHostChange`; upstream report `c-9b48b7`). The global-egress-proxy bypass is accepted and
  documented (`docs/OPS.md` §6). `scripts/verify.sh` passes.
  Block rules apply even with the gate off. Fuzzy allow rules match on name and address boundaries.
- Main-agent writes carry origin `mainagent` and are framed as agent-authored. Norma compaction
  summaries are framed as untrusted data.
- Per-run executor HOME; MCP servers run as uid 10002 (`ARTEX_EXEC_MCP_UID`). The workspace file
  manager uses `os.Root`.
- **2026-11-24 — per-task executor uids (card `c-13f293`, opt-in).** `-exec-uid-range LO-HI` (e.g.
  `20000-29999`, passed to `start.sh`/the container command) gives each task and chat session its own
  uid while it is live. Its run dirs are `<uid>:<uid>` 2770. Processes take the identity of the run dir
  they start in: the bash shim, custom tools and file tools all read it from the dir's owner. The uid is
  released (dirs closed, processes reaped, files back to root) on a terminal status, a task delete or a
  conversation delete. `egress.sh` filters the range. Code: `server/execuid.go`,
  `server/execuid_unix.go`. Single-uid mode is still the default.
  Verified: Linux-root tests (`server/execuid_root_linux_test.go`, where task A gets EACCES on
  `tasks/B/i1` and `.artex-home`), the full server and agent suites as root in `golang:1.26.11-bookworm`,
  and egress rules with iptables in a NET_ADMIN container.
  Spill dirs follow the key too (card `c-dd160a`, `server/execuid_spill_unix.go`). A live key's
  `<data>/spill/{tasks,sessions}/<id>` is `root:<uid>` 0750 with files `root:<uid>` 0640, so the
  shared exec gid no longer reads another task's spills. It is regrouped on acquire, closed to
  `root:root` 0700 at the start of release, and closed at start-up when the key holds no uid. Verified
  by `TestExecUIDRangeSpillIsolationAsRoot`, where task A's shell and file tool get EACCES on B's spill,
  plus the full server suite as root in Docker.
  Root tests in Docker: build the test binary into a 0755 dir (`go test -c -o /t/server.test`). The
  default 0700 go-build dir makes the isolation self-tests fail on `main` too.
