# ARTEX operations

## 1. Startup

Docker (recommended):

```bash
cp .env.example .env          # set POSTGRES_PASSWORD; optionally an LLM key
docker compose up -d --build
docker compose logs artex | grep "Setup token"
```

Open `http://localhost:8787`. The first visit to `/setup` asks for the one-time setup
token printed at startup (`[auth] no admin password is set. Setup token for the /setup
page: ...` in `server/auth.go`).

From source (no embedded UI):

```bash
go run ./cmd/artex                       # backend only, 127.0.0.1:8787
cd web && npm run dev                    # UI on :5173, /api proxied to the backend
# or: ./dev.sh                           # both, plus the traffic proxy on :8788
```

The process prints one `[config] snapshot {...}` line at startup. It is the authoritative
record of the effective configuration: config path, DSN source and **redacted** DSN,
data/skill/key directories, listen address, proxy, noise profile, untrusted-output
boundary and trusted proxies. No password or token value is ever logged there.

## 2. Configuration precedence

| Setting | Resolution order | Code |
| --- | --- | --- |
| Config file | `ARTEX_CONFIG` → `./config.json` → `config.json` next to the binary | `config.Path` |
| PostgreSQL DSN | `ARTEX_PG_DSN` → `ARTEX_PG_HOST` / `ARTEX_PG_PORT` / `ARTEX_PG_USER` / `ARTEX_PG_PASSWORD` / `ARTEX_PG_DBNAME` / `ARTEX_PG_SSLMODE` → `database.dsn` → assembled `host/port/user/password/dbname` | `config.PostgresDSN` |
| Skill directory | `ARTEX_SKILL_DIR` → `skill_dir` → `<binary dir>/skills` | `config.SkillDir` |
| JWT / secret key directory | `ARTEX_JWT_KEY_DIR` → `<binary dir>` (never the browsable data dir) | `config.KeyDir` |
| Secret encryption key | `ARTEX_SECRET_KEY` → `<key dir>/secret.key` → newly generated key file | `db.InitSecrets` |
| Noise profile | `ARTEX_NOISE_PROFILE` (`off` default; `quiet`, `balanced`, `aggressive`), also settable in UI | `egress.ProfileFromEnv` |
| Untrusted-output boundary | on by default; `ARTEX_UNTRUSTED_BOUNDARY=0\|false\|off\|no` disables it | `agent/untrusted_boundary.go` |
| Trusted proxies for login rate limit | `ARTEX_TRUSTED_PROXIES` (comma-separated IPs/CIDRs; empty trusts none) | `server.ParseTrustedProxies` |
| Target user agent | `ARTEX_TARGET_USER_AGENT` (applied by non-`off` noise profiles) | `egress.ProfileByName` |
| Budgets | `ARTEX_TASK_BUDGET_TOKENS` / `_TURNS`, `ARTEX_INTENT_BUDGET_TOKENS` / `_TURNS` | `agent/budget.go` |
| Live model judge test | `ARTEX_REVIEW_LIVE_CONFIG` (opt-in integration test only) | `server/intercept_live_test.go` |

Agent profiles, providers, notifications and other runtime settings live in the database
and are edited in **System → LLM / Settings / Notify**, not in `config.json`.

## 3. Backup and restore

```bash
scripts/backup.sh                       # database dump only, 14 rotations
scripts/backup.sh --with-data --keep 30 # also archive the data/ bind mount
```

The dump runs inside the `postgres` compose service, so no host `psql` is needed. Restore:

```bash
gunzip -c backups/artex-db-<ts>.sql.gz | docker compose exec -T postgres \
  psql -U "$POSTGRES_USER" -d "$POSTGRES_DB"
```

Back up the key directory as well: without it, JWTs cannot be verified and sealed
secrets cannot be decrypted. Keep it outside the database backup but under the same
access controls.

## 4. Key rotation

- **`jwt.key`** — signing key for the 7-day HS256 session tokens. Replacing it
  invalidates every existing session; all users must log in again. Existing key files
  are never overwritten by startup (`loadOrCreateJWTKey`).
- **`secret.key`** (or `ARTEX_SECRET_KEY`) — AES-256-GCM key for API keys, MCP env,
  task credentials and captured auth headers. Startup never overwrites an existing key.
  If it changes, values sealed under the old key cannot be decrypted (the code returns
  an error rather than leaking ciphertext as plaintext). Re-enter those secrets in the
  UI, or keep the previous key available until the migration is complete.

Plan a short maintenance window for either rotation.

## 5. Diagnostics and local verification

- `GET /api/health` — liveness, build version and provider health.
- `GET /api/logs`, `GET /api/logs/stream` — live backend log sink; the UI shows it under
  **System → Logs**. The startup snapshot line is here too.
- `GET /api/events/stream` — the dashboard's live push (task status, findings, approval
  queue). Triggers on `findings`, `intercept_pending` and `tasks` (migration 0027) `NOTIFY
  artex_changes` at commit; the server holds one pooled connection `LISTEN`ing on it once a
  console is open (visible as `LISTEN artex_changes` in `pg_stat_activity`). Frames carry ids
  only; client_read sessions get only their company's findings and tasks. When the stream is
  down the dashboard falls back to polling every 5s.
- `scripts/verify.sh` — reproduces the CI checks locally (gofmt, build, vet, tests,
  web type-check and unit tests, shellcheck, python checks). `--full` adds `-race` and
  the web static build; `--go-only` / `--web-only` narrow the run.
- `./scripts/backup.sh --help` — backup options.

When Postgres is not configured, database-dependent tests skip themselves; that is
expected. Set `ARTEX_PG_DSN` to run them. CI uses `go test -p 1 ./...` because test
packages share a Postgres service and use suffixed per-package databases
(`SuffixTestDatabase`).

## 6. Capture-proxy upstream checks

The recording proxy (go-mitmproxy) has no dial hook, so its upstream is a loopback gate
(`traffic/upstream_gate.go`) that dials with `Interceptor.DialSOCKS5`.

- **Global rules** (asset block rules, the metadata/link-local deny) apply to every upstream dial.
- **Task rules.** Task agents get a proxy URL with a task scope token in its userinfo
  (`http://artex-scope:scope-…@127.0.0.1:8788`; Bash gets `artex-pace:scope-…`). The proxy binds each
  client connection that presents a token to its task. Upstream dials then also apply that task's
  block rules and, when the task's target gate is on, its pins and approvals. Dials never wait: an
  unpinned host opens an approval request and is refused until someone approves it. A token whose
  task was deleted or archived is refused. A client without a token (an operator's browser or
  `curl`) gets the global rules only. The SOCKS5 listener takes the same token as its RFC 1929
  password. The token only adds rules; it is not a credential to protect. Code:
  `traffic/scope.go`, `egress/proxycred.go`, `Manager.TaskProxyAddr`.
- **Conversation rules.** A conversation's chat agent gets a URL with the conversation's own token,
  resolved per turn from the turn's target scope (`Manager.ScopedProxyAddr`), so its WebFetch and
  Bash dials apply the conversation's block rules and, with its gate on, its pins and approvals,
  without waiting. The token is dropped when the conversation is deleted.
- **Custom HTTP tools** with `use_recording_proxy` use the URL of the calling task or conversation
  (`ScopedProxyAddr` on the call's context). A call whose task is no longer loaded gets a URL
  whose dials are all refused, never the global-only one. A call with no run scope (an editor
  test) gets the global rules.
- **Global rules only, by decision:** the transparent TCP listener. A redirected connection has no
  credential to carry a token, and mapping it to a run would need per-run uids or cgroups plus
  netfilter owner matching set up by the operator. The listener is opt-in and operator-configured;
  treat it as a global-rules path and do not point a run's tools at it when that run's own block
  rules or gate must hold.
- **Browser and other stdio MCP servers.** A stdio MCP is spawned per run (each worker run,
  planner round, main-agent turn and chat turn). The stored row names the plain proxy URL (the
  seeded browser gets `--proxy-server` from `syncBrowserMCPProxy`), but every spawn replaces a
  `--proxy-server` flag or an `HTTP_PROXY`/`HTTPS_PROXY`/`ALL_PROXY` variable naming the recording
  proxy with a **scope relay** of the run: a loopback listener (`http://127.0.0.1:<port>`, no
  credential, because Chromium drops a userinfo from `--proxy-server`) whose connections the proxy
  binds to the run's token. The relay closes with the run; a relay of a deleted conversation or an
  ended task is refused. If the relay cannot start, the server is not started for that run.
  A reference is matched by port and host (a loopback name stands for any loopback address; no
  scheme reads as http); `PLAYWRIGHT_MCP_PROXY_SERVER` counts too. A form that cannot be rewritten
  (Chromium's `http=…;https=…` list) is logged and keeps the global rules. A relay is an
  unauthenticated loopback listener, like the proxy itself; using another run's relay only adds
  that run's rules. A reference to another proxy is left as stored. Listing an MCP's tools from the UI (no run)
  spawns it with the stored URL: global rules. Code: `traffic/scope_relay.go`,
  `Manager.scopeMCPProxy`.
- **Global egress proxy.** When one is set, go-mitmproxy forwards through it and the gate is not on
  the path. Neither global nor task rules then apply to the proxy's dials: the operator's proxy
  reaches the target. That is the intended behaviour for a remote egress proxy. **Do not run that
  proxy on the ARTEX host** (or anywhere that can reach cloud metadata) unless it has its own deny
  for 169.254.0.0/16, `fd00:ec2::254` and the metadata names. Otherwise a redirect to metadata
  goes through it.
