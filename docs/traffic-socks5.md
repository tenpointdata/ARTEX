# Opt-in SOCKS5 Inbound Proxy & Raw TCP Recording (P4.1a)

ARTEX ships an opt-in **RFC 1928 SOCKS5 `CONNECT`** inbound listener in `traffic/` that records raw TCP connections separately from HTTP traffic.

## Opt-in Configuration (Off by Default)

| Environment Variable | Default | Behavior |
|---|---|---|
| `ARTEX_SOCKS5_ENABLED` | unset (off) | Set to `1` / `true` / `on` / `yes` to start the SOCKS5 listener on `127.0.0.1:1080` (loopback). Explicit `0` / `false` / `off` / `no` forces it off even if `ARTEX_SOCKS5_ADDR` is set. |
| `ARTEX_SOCKS5_ADDR` | `127.0.0.1:1080` | Explicit TCP bind address; setting a non-empty value also enables the listener unless `ARTEX_SOCKS5_ENABLED` is explicitly false. |
| `ARTEX_SOCKS5_PASSWORD` | unset | Operator password (RFC 1929). When set, a client without a task scope token must present it, and no-auth is refused. |
| `ARTEX_SOCKS5_USER` | `artex` | Operator username paired with `ARTEX_SOCKS5_PASSWORD` (ignored, with a log line, when no password is set). Both are 1-255 bytes. |

- **Flag off**: no SOCKS5 listener is started and the HTTP MITM recording proxy is byte-identical.
- **Requires traffic recorder**: the server only starts the SOCKS5 listener when the traffic recorder is enabled (inside the traffic proxy block in `server/manager.go`). If traffic capture is disabled, setting `ARTEX_SOCKS5_ENABLED` / `ARTEX_SOCKS5_ADDR` does not open a listener.
- **Agents unchanged**: workers and tools keep `HTTP_PROXY`, `HTTPS_PROXY`, and `ALL_PROXY` pointing to the existing HTTP proxy; ARTEX does not silently switch agents to SOCKS5.
- **Authentication is enforced** (`traffic/socks5_auth.go`, the policy is the comment at its top). A client is admitted only as:
  1. a **task scope token** as the RFC 1929 password (a live token from `Traffic.ScopedProxyURL`): the connection runs on that task's rules; a token that is not live (an ended task) is refused with RFC 1929 status `0x01`, never downgraded;
  2. the **operator credential** (`ARTEX_SOCKS5_USER` / `ARTEX_SOCKS5_PASSWORD`): the global rules. The pace marker user `artex-pace` may be used as the username with the operator password to ask for pacing;
  3. **open loopback mode**, only when the listener is bound to a loopback address *and* no operator password is configured: the pre-audit behaviour for a local developer listener. A loopback peer may use no-auth, or the pace marker (`artex-pace` / `artex-pace` or an empty password). Any other username/password gets `0x01`.

  A listener bound beyond loopback with no operator password admits scope-token clients only (a warning is logged at start), so exposing it no longer exposes an open proxy. Once an operator password is set, no-auth (`0x00`) is refused even from loopback, since a co-resident process could otherwise reach the global rules unauthenticated. Comparisons are constant-time. The target gate and egress governor still apply to every connection. Existing clients that sent arbitrary credentials (for example `curl --proxy socks5h://x:y@...`) must now send the operator password or a scope token.

## Protocol Scope (P4.1a)

- **Commands**: `CONNECT` (`0x01`) by default. `BIND` (`0x02`) and unknown commands return RFC 1928 reply `0x07` (*Command not supported*). `UDP ASSOCIATE` (`0x03`) also returns `0x07` unless `ARTEX_SOCKS5_UDP_ENABLED=1` is set (see [docs/traffic-socks5-udp.md](traffic-socks5-udp.md)).
- **Addressing**: IPv4 (`0x01`), domain name (`0x03`), and IPv6 (`0x04`). Unknown address types return `0x08` (*Address type not supported*).
- **Authentication**:
  - **No Auth** (`0x00`) is accepted only in open loopback mode (above) from loopback peers; otherwise the greeting is answered `0x05 0xFF`.
  - **Username/Password** (`0x02`, RFC 1929) is verified as described above; failure status is `0x01` and the connection is closed.
- **Relay**: Full-duplex, half-close aware (`CloseWrite` on EOF in each direction).

## Policy Enforcement Before Dialing

Before opening any outbound TCP socket, the listener invokes the same controls used by the HTTP path:

1. **Egress governor (`egress.Do(egress.Current(), ...)`)**: enforces the active noise profile (`off` is a zero-delay passthrough). If waiting for a slot/token exceeds `PaceTimeout`, the connection is refused before dialing with SOCKS5 reply `0x02` (*Connection not allowed by ruleset*).
2. **Target gate (`intercept.Interceptor.DialSOCKS5` / `intercept.PinnedDialer`)**: enforces global asset block rules (before and after fresh DNS resolution) and any enabled `intercept.TargetScope` pins/exemptions, dialing only verified addresses. Blocked targets return `0x02` before dialing; DNS lookup failures return `0x04` (*Host unreachable*).

## Raw TCP Storage (SQLite)

Each completed relayed connection writes one row to the `raw_tcp_streams` table in the SQLite traffic index (`traffic/traffic.go`, `indexSchema` — no PostgreSQL migration):

- Columns: `id` (`tcp-<ms>-<seq>`), `protocol` (`'tcp'`), `host`, `port`, `started_at`, `ended_at`, `duration_ms`, `c2s_bytes`, `s2c_bytes`, `truncated`, `c2s_preview`, `c2s_blob`, `s2c_preview`, `s2c_blob`.
- **No HTTP exchange row**: raw TCP streams never insert into `exchanges`.
- **Preview & blob spilling**:
  - Directional payloads up to `PreviewBytes` (default `8 KiB`) stay inline in `c2s_preview` / `s2c_preview`.
  - Payloads above `PreviewBytes` keep the leading `PreviewBytes` inline and spill the captured bytes (up to `MaxBytesPerDir`) atomically (temp file + rename) into `_blobs/sha256/` with references registered in `blob_refs` under the stream's `tcp-*` ID.
  - Exceeding `MaxBytesPerDir` sets `truncated = 1` while continuing to count exact total relayed `c2s_bytes` / `s2c_bytes`.

## Operational Bounds

| Parameter | Default |
|---|---|
| `MaxConcurrent` | `64` active streams (excess connections receive `0x01`) |
| `MaxBytesPerDir` | `4 MiB` captured per direction |
| `PreviewBytes` | `8 KiB` inline preview per direction |
| `IdleTimeout` | `2m` per-direction read/write deadline |
| `HandshakeTimeout` | `10s` for greeting, auth, and `CONNECT` request |
| `DialTimeout` | `30s` outbound TCP connect timeout |
| `PaceTimeout` | `30s` egress governor wait bound |

Closing `Traffic` stops accepting new SOCKS5 connections, closes active sockets, and waits for all handler goroutines to exit before closing SQLite.
