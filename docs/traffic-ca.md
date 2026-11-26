# MITM certificate authority

The recording proxy re-signs HTTPS with a CA stored in `<data>/traffic/_ca/`. Anything that trusts that CA
trusts whatever its key signs, so ARTEX keeps the CA small and short-lived. The code is `traffic/ca.go`
(`GenerateCA`, `LoadOrCreateCA`, wired through go-mitmproxy's `Options.NewCaFunc` in `traffic.Open`).

## What a new CA looks like

| Property | Value |
|---|---|
| Key | ECDSA P-256 (leaf signing through go-mitmproxy needs no RSA) |
| Validity | 365 days; `ARTEX_CA_VALIDITY_DAYS` (1-3650) |
| Constraints | path length 0 (signs leaves only), EKU `serverAuth` only |
| Leaves | fresh P-256 key per host, valid 7 days (backdated 24h for clock skew), `serverAuth` only; cached until a day before expiry |
| Files | `mitmproxy-ca.pem` (key + cert, mode 0600), `mitmproxy-ca-cert.pem` and `.cer` (cert only, world-readable). Same names as before. |

Before this, go-mitmproxy generated an RSA-2048 root valid for three years that could sign any name, and used the
root's own key as every leaf's key.

## Optional name constraints

`ARTEX_CA_PERMITTED_DNS=example.com,.lab.test` bakes **critical DNS name constraints** into a newly generated root:
`example.com` permits itself and its subdomains, `.lab.test` subdomains only. A client that trusts the CA then rejects
any leaf for another name, so a stolen CA key cannot impersonate other sites to it. Full constraints are off by default
because a general MITM proxy must sign arbitrary hosts; **where the engagement's domains are known, set this**.

- With constraints, HTTPS to a host outside them fails the handshake (the CA refuses to issue; the log says why).
- IP-address leaves are not constrained by a DNS list.
- It applies only when the CA is generated. An existing CA is never altered; a log line says the variable was ignored.

## Existing installs and rotation

An existing `mitmproxy-ca.pem` (RSA or ECDSA) loads unchanged, so installed trust keeps working. At start ARTEX logs a
warning when it is RSA-2048 or expires within 30 days (or when constraints are requested but it has none).

To rotate: stop ARTEX, delete `mitmproxy-ca.pem`, `mitmproxy-ca-cert.pem` and `mitmproxy-ca-cert.cer` in
`<data>/traffic/_ca/`, set the variables above if wanted, start ARTEX, and install the new `mitmproxy-ca-cert.pem` wherever
the old one was trusted (agent containers pick it up from `CACertPath`). A malformed CA file is an error; it is never
overwritten.
