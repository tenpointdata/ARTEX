# Engagement governance

Three mechanisms keep each engagement within what the client signed: console **roles**, a per-task
**authorization artifact** that the target gate enforces, and a **hash-chained audit log** over the
intercept history. The schema is in `db/migrations/0016_engagement_governance.sql`, with the audit-at-commit
triggers in 0018 and client bindings in 0019.

## Roles (RBAC)

| Role | Can |
|---|---|
| `admin` | everything, including users, authorization artifacts and guardrail configuration (intercept rules, asset rules, target-gate exemptions, judge, tool config, settings) |
| `operator` | run engagements: create, start and steer tasks, chat, tools, traffic; decide approvals |
| `reviewer` | read everything except secrets, raw LLM records and logs; decide approvals; triage findings |
| `client_read` | read the report, the findings (with their captured evidence), the stats and a task's outcome and coverage, **of the companies the user is bound to**. Nothing else. See [Client binding](#client-binding) |

- The bootstrap login `ARTEX` (set on `/setup`) is always `admin` and serves as the break-glass account.
  Admins add other users with `POST /api/users`.
- `requireAuth` reads the policy from the declarative table in `server/rbac_policy.go`; the first
  matching rule wins. Without a matching rule, `GET` needs `read` and every other method needs
  `operate`, and `client_read` is refused. Each refusal answers 403 and is appended to the audit chain
  as `rbac.denied`.
- Reviewers can't read raw traffic exchanges, blobs, HAR exports or the traffic stream, and can't read
  conversation messages: those carry the credentials the agents used.
- `POST /api/auth/change-password` (old + new password) changes the caller's own password: the `ARTEX`
  password for an `ARTEX` session, the user's own for a console user. It is rate-limited like login.
  For a console user the change applies only if the user's version is still the one in the caller's
  session (else 409), and it bumps the version, which ends every session of that user including the
  current one. An admin can also set a console user's password with `PATCH /api/users/{id}`.
- Open SSE streams re-check the session on every event. The first check of a request reads the users
  table; the re-checks inside a stream use a 2-second cache that `UpdateUser`, `DeleteUser` and a
  password change invalidate at once, so only a change made outside this process (psql, another
  instance) can take up to 2 s to end a stream.
- The names `system`, `anonymous`, `human`, `rule`, `model`, `target`, `db` and `admin` are reserved,
  because the audit trail uses them for automatic actors.
- A user's session token carries the user's version, and any password, role or disabled change bumps
  that version. Old sessions stop working on their next request.

### Client binding

A `client_read` user sees only the companies (clients) it is bound to. An admin sets the bindings with
`PUT /api/users/{id}/companies {"company_ids":[…]}` (an empty list withdraws them all) and reads them
with `GET /api/users/{id}/companies`. They are stored in `artex_user_companies` (migration 0019), and
every change is appended to the audit chain as `row.artex_user_companies`. Bindings apply to
`client_read` only; the other roles stay global.

- A client account with no binding is refused (403) everywhere.
- **Findings** routes (list, groups, asset tree, stats, export) are narrowed to one bound company by the
  same rule as the sidebar's company filter: a finding belongs to the company that owns one of its
  live assets, or, when it has none, to the companies its task is scoped to. `?company=` picks the
  company, and may be left out when the user has only one. A company that is not bound answers 404.
- A **finding** by id (detail, lineage, retests, captured traffic) answers 404 unless it belongs to a
  bound company. An export of selected ids answers 404 if any of them does not.
- **Task** routes (`/api/report`, `/api/stats`, `/api/exploration/findings?task=`,
  `/api/tasks/{id}/outcome|coverage`) need an explicit task id (`?task=active` is refused), and the
  task must be scoped to a bound company; otherwise 404. A client's report counts only its own
  companies' assets, and its stats leave out the global traffic and asset totals.
- A task belongs to a company only through a company scope row an operator made (at task creation or
  by hand). Rows an agent added with `add_task_scope` do not count, so agent output cannot widen what
  a client sees.
- Each refusal is appended to the audit chain as `rbac.client_scope_denied`.
- Every route the policy opens to `client_read` must have a binder in `server/client_scope.go`; a route
  without one is refused, and `TestClientBindersCoverClientRoutes` fails until it has one.
- A task scoped to two clients is fully visible to both, including its report and lineage graph.

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
So an artifact requires the target gate: storing a version turns the task's gate on in the same
transaction, `PATCH /api/tasks/{id}` with `target_gate: false` answers 409 while the task has an
artifact, and a child spawned under an artifact is gated. Run with the Docker egress filter as well.

**Expiry and revocation pause the task.** The scheduler checks every 5 s for running or queued tasks
whose newest version is revoked or past `not_after`, and pauses them (cause `authorization_ended`); a
revocation through the API pauses at once. Such a task can't be resumed through the control API until
an admin stores a version that has not ended. A window that has not started yet does not pause: the
gate refuses calls until it starts.

**Caching.** The gate reads the artifact through a per-task cache (`db.CachedAuthorization`, 2 s).
`AddAuthorization` invalidates it after commit, so a version stored through the API applies to the next
call; a row changed outside this process is seen within 2 s.

**Archives.** A task archive carries the artifact's versions in its package. Archiving leaves the rows
in place with the task stub, so the gate still applies them after a restore. A restore re-creates
archived versions missing from the database (audit actor `archive-restore`), keeps a database version
that differs from the archive, and reports both as restore warnings. Deleting an archive deletes the
task and its versions; the audit chain keeps their history.

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
  written in the same transaction as the change, so no writer can skip it and a rolled-back change
  leaves none. Since migration 0018 these are deferred constraint triggers: the entry records the row
  as written, but it is appended during `COMMIT`.
- The application appends `auth.login`, `auth.login_failed` and `rbac.denied`.
- `UPDATE`, `DELETE` and `TRUNCATE` on `audit_chain` fail.
- `GET /api/intercept/history/chain?after_seq=&kind=&subject=&limit=` lists entries. `kind=row.`
  is a prefix filter.
- `GET /api/intercept/history/chain/verify` recomputes every hash. It also checks that `seq` has no
  gaps and that the head row names the last entry, then compares every intercept history row with
  its newest entry. Rows written before 0016 are counted as `rows_unchained`.

Verification reads everything in one repeatable-read snapshot with `TimeZone = 'UTC'`, and the row
trigger pins UTC too, so concurrent appends and session time zones don't produce false alarms.

**Cost.** Each append locks the head row until its transaction commits. Row entries are appended at
commit (migration 0018), so a transaction holds the head only while its own entries are written, and
it takes no other lock after it. A long transaction, such as a task archive deleting intercept rows,
no longer stalls gate decisions, logins or `rbac.denied` entries, and an intercept write followed by
another lock cannot deadlock on the head. Commits that carry audited rows are still serialized, one
at a time, for as long as their appends take; `AppendAudit` events are single statements. Side effects:
chain order is commit order; a row entry's actor is the transaction's `artex.actor` at commit (writers
set it once, first); a transaction cannot `ALTER` or `TRUNCATE` an audited table after writing to it;
and `SET CONSTRAINTS ALL IMMEDIATE` would bring back the old behaviour for that transaction.
`db/audit_chain_concurrency_test.go` covers the archive stall, the lock-order case and rollback.

**Limits.** A database superuser can disable the triggers and rewrite the whole chain consistently.
The hashes catch partial edits, removals and edits made behind the triggers' back, but a full rewrite
can only be caught by comparing `head_hash` with a copy kept outside the database. Record the
`head_seq` and `head_hash` that verify returns, for example in the engagement report.
