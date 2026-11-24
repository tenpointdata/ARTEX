# go-mitmproxy v1.9.2, patched for ARTEX

Vendored from `github.com/lqqyt2423/go-mitmproxy@v1.9.2` (MIT, see `LICENSE`) and wired in with a
`replace` in ARTEX's `go.mod`. Only the packages ARTEX builds are kept (`proxy`, `cert`,
`internal/helper`); tests, the web UI, examples and commands were dropped.

The one change: per-host upstream TLS verification (ARTEX card c-8d54d5). Upstream has only the
global `Options.SslInsecure`.

- `proxy/upstream_tls.go` (new): `Options.UpstreamInsecure func(host string) bool` decides per
  upstream host; `Options.UpstreamRootCAs` sets the roots (nil = system). Verification runs in
  `tls.Config.VerifyConnection` and matches crypto/tls's default (chain, server-auth usage, name).
  In `serverTlsHandshake` a connection is exempt only when both the client's SNI and the dialled
  host are exempt, and an IP target without SNI is checked against the dialled IP. The wss dialer
  checks the dialled host. The shared client (used after a rewrite changes the host or scheme) only
  sees `ConnectionState.ServerName`, which is empty for an IP, so it refuses IP targets
  (fails closed).
- `proxy/proxy.go`: the two options. `proxy/attacker.go` (shared client, `serverTlsHandshake`) and
  `proxy/websocket.go` (wss dialer): use `upstreamTLSConfig`. `SslInsecure` still governs an https
  upstream proxy (`helper.GetProxyConn`) and is the fallback when `UpstreamInsecure` is nil.

To move to a newer upstream release: re-copy the three packages, re-apply the diff above, and
re-run `go test ./traffic/`. Drop the fork if upstream gains an equivalent hook.

The vendored files are `gofmt`ed (ARTEX CI formats every tracked `.go` file); that is whitespace only.
