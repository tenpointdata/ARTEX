# Engagement governance

Three mechanisms keep each engagement within what the client signed: console **roles**, a per-task
**authorization artifact** that the target gate enforces, and a **hash-chained audit log** over the
intercept history. All of the schema is in `db/migrations/0016_engagement_governance.sql`.

## Roles (RBAC)

| Role | Can |
|---|---|
| `admin` | everything, including users, authorization artifacts and guardrail configuration (intercept rules, asset rules, target-gate exemptions, judge, tool config, settings) |
| `operator` | run engagements: create, start and steer tasks, chat, tools, traffic; decide approvals |
| `reviewer` | read everything except secrets, raw LLM records and logs; decide approvals; triage findings |
| `client_read` | read the report, the findings, the stats and a task's outcome and coverage. Nothing else. **The role is global for now:** a `client_read` user sees every client's findings, so give it only to someone entitled to all of them (per-client binding is a follow-up) |

- The bootstrap login `ARTEX` (set on `/setup`) is always `admin` and serves as the break-glass account.
  Admins add other users with `POST /api/users`.
- `requireAuth` reads the policy from the declarative table in `server/rbac_policy.go`; the first
  matching rule wins. Without a matching rule, `GET` needs `read` and every other method needs
  `operate`, and `client_read` is refused. Each refusal answers 403 and is appended to the audit chain
  as `rbac.denied`.
- Reviewers can't read raw traffic exchanges, blobs, HAR exports or the traffic stream, and can't read
  conversation messages: those carry the credentials the agents used.
- `POST /api/auth/change-password` changes only the `ARTEX` password, and only an `ARTEX` session can
  call it. An admin changes a console user's password with `PATCH /api/users/{id}`.
- The names `system`, `anonymous`, `human`, `rule`, `model`, `target`, `db` and `admin` are reserved,
  because the audit trail uses them for automatic actors.
- A user's session token carries the user's version, and any password, role or disabled change bumps
  that version. Old sessions stop working on their next request.

## Authorization artifact

`POST /api/tasks/{id}/authorization` (admin) stores the task's rules of engagement as a new version:

```json
{
  "reference": "SOW-2026-114",
  "authorized_by": "Jane Doe, CISO",
  "scope": [{"kind": "domain", "value": "example.com"}, {"kind": "cidr", "value": "203.0.113.0/24"}],
  "techniques": ["recon", "scanning", "fuzzing"],
  "not_before": "2026-12-01T00:00:00Z",
  "not_after": "2026-12-15T00:00:00Z",
  "note": "no testing of the payment flow"
}
```

`GET /api/tasks/{id}/authorization` returns every version, the one in force, and the list of technique
classes. `POST /api/tasks/{id}/authorization/revoke` adds a version that authorizes nothing. Versions
can't be edited; the database rejects an `UPDATE`.

The **existing target gate** enforces the newest version, and nothing else does. The check is in
`intercept/authorization.go`, and every gate entry point calls it before its own rules: `CheckTargets`
when the gate is on, `CheckUngated` when it is off, and the connection-level dial checks and redirect
hops. A task without an artifact is gated exactly as before.

| Check | Refused with | Applies to |
|---|---|---|
| Revoked | `AUTHORIZATION_REVOKED` | every tool that can reach a target |
| Outside `not_before`..`not_after` (this is scope expiry) | `AUTHORIZATION_OUT_OF_WINDOW` | every tool that can reach a target, including commands that declare no target |
| Technique class not listed | `AUTHORIZATION_TECHNIQUE_DENIED` | calls classified into a class (`intercept/technique.go`) |
| Target outside the scope | `AUTHORIZATION_OUT_OF_SCOPE` | each declared target, and each connection the platform dials (WebFetch, custom tools, SOCKS5, redirects) |

**Undeclared connections.** A command can build its target at run time, for example
`curl "https://$(cat host)"`. The scope check then sees only what goes through the platform's dialer.
For a task with an artifact, turn the target gate on as well, so that undeclared targets still need a
pin, and run with the Docker egress filter.

Tools that cannot reach a target are never refused, even when the artifact can't be read: these are the
`noTargetTools` such as `Read`, `record_fact` and `report_finding`, so agents can still write up an
engagement after it expires. One exception is `spawn_task`, which stays subject to revocation and the
window. A child task inherits its caller's current artifact as its first version, and a spawn under a
revoked or expired artifact is refused.
Each refusal is a decided intercept row (`decided_by = system`). It shows up in
`/api/intercept/history` and on the audit chain.

**Scope matching.** A `domain` entry covers the name and every name below it. A `host` entry covers
exactly one name. `ip` and `cidr` entries cover addresses and prefixes. Names are never resolved for
this check, so a name has to be listed by name even when it resolves inside a listed CIDR. An empty
scope leaves the targets to the gate's other checks.

**Technique classes:** `recon`, `scanning`, `fuzzing`, `exploitation`, `credential_attack`,
`post_exploitation`, `denial_of_service`. An empty list allows every class. The classifier reads the
tool name and the command line. It is a declared check, not a sandbox: a renamed binary or a script
that wraps a scanner goes unseen. A call that matches no class is not limited by the list.

## Hash-chained audit log

`audit_chain` is append-only. Each entry stores `seq`, `at`, `actor`, `kind`, `subject`, `payload`,
`prev_hash` and `hash`, where `hash = sha256(len:seq; len:prev_hash; len:at; len:actor; len:kind; len:subject; len:payload;)`.
The SQL function `audit_append` and the Go verifier (`db/audit_chain.go`) both compute it.

- Row triggers write an entry for every insert, update and delete of `intercept_pending` (the intercept
  history), `artex_users` (without the password hash) and `engagement_authorizations`. The entry is
  written in the same transaction as the change, so no writer can skip it.
- The application appends `auth.login`, `auth.login_failed` and `rbac.denied`.
- `UPDATE`, `DELETE` and `TRUNCATE` on `audit_chain` fail.
- `GET /api/intercept/history/chain?after_seq=&kind=&subject=&limit=` lists entries. `kind=row.`
  is a prefix filter.
- `GET /api/intercept/history/chain/verify` recomputes every hash. It also checks that `seq` has no
  gaps and that the head row names the last entry, then compares every intercept history row with
  its newest entry. Rows written before 0016 are counted as `rows_unchained`.

Verification reads everything in one repeatable-read snapshot with `TimeZone = 'UTC'`, and the row
trigger pins UTC too, so concurrent appends and session time zones don't produce false alarms.

**Cost.** Each append locks the head row until its transaction commits, so every intercept write
queues behind any open transaction that already wrote one. A long one, such as a task archive, stalls
the others until it commits.

**Limits.** A database superuser can disable the triggers and rewrite the whole chain consistently.
The hashes catch partial edits, removals and edits made behind the triggers' back, but a full rewrite
can only be caught by comparing `head_hash` with a copy kept outside the database. Record the
`head_seq` and `head_hash` that verify returns, for example in the engagement report.
