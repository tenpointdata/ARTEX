# Integrations: findings out, scanner data in

Two operator-driven paths, both behind the normal API auth.

## Findings out: ticket trackers

Package `ticketing` holds one adapter per tracker (`github`, `jira`, `defectdojo`). Every
request goes through `notify.Do`: the same dial guard as notifications (loopback, link-local
and cloud metadata refused unless `ARTEX_NOTIFY_ALLOW_LOCAL=1`), no cross-host redirects, and
error text with URLs and credentials redacted. Tracker configs are sealed at rest
(`ticket_trackers.config`), masked on readback, and a change of `base_url`/`repo`/`test_id`
without re-entering the token is refused (`notify.PrepareConfigUpdateKeys`).

| Kind | Config | Dedup key carried as |
|---|---|---|
| `github` | `repo` (owner/name), `token`, optional `base_url`, `labels` | hidden `<!-- artex:key=… -->` marker in the body |
| `jira` | `base_url`, `project_key`, `token` (+ `email` for Cloud basic auth), optional `issue_type`, `done_transition`, `reopen_transition` | label |
| `defectdojo` | `base_url`, `api_key`, `test_id`, optional `found_by` | `unique_id_from_tool` |

Any tracker config may also set `include_evidence: true` to copy the finding's evidence into
the ticket (credential headers redacted, 8 KiB cap). It is off by default because captured
traffic can carry session material.

API:

- `GET /api/integrations/meta` — kinds with their secret and destination keys.
- `GET|POST /api/integrations/trackers`, `PATCH|DELETE /api/integrations/trackers/{id}`.
- `POST /api/integrations/trackers/{id}/push` `{"finding_ids":[…]}` — create or update tickets.
  Idempotent: a stored link is reused, otherwise the tracker is searched by dedup key.
- `POST /api/integrations/trackers/{id}/sync[?limit=N]` — reconcile the tracker's tickets, at most
  `limit` per call (default 100, max 500), oldest sync first. The reply is
  `{"results":[…],"more":bool}`; `more` means the pass has links left and the next call continues it.
- `GET /api/exploration/findings/{id}/integrations` — a finding's tickets and import provenance.

Status mapping: pending/in_progress/confirmed → open; resolved/fixed → closed as fixed;
false_positive, risk_accepted, duplicate → closed with that resolution; ignored → closed as
won't fix. Reading back, an open ticket reopens a closed finding as `confirmed`. A sync compares
each side with the state both agreed on last time (`finding_tickets`); when both changed, the
tracker wins. A ticket status changes the finding's disposition only, never its verification.

**One run per tracker at a time.** Push, sync, and a destination change of one tracker run under
the PostgreSQL advisory lock `ticket_sync:<id>` (`db.WithTryAdvisoryLock`), so two requests, a
request and the background run, or two ARTEX instances on one database never reconcile the same
links at once. A busy lock answers **409** (the background run skips that round); it is never
waited on.

**Cursor.** Sync walks a tracker's links in passes (`db.TicketSyncCursor`): a pass visits every
link synced before it started exactly once, oldest first. A link synced during the pass waits for
the next one, and a link that keeps failing (its `synced_at` does not move) is passed over instead
of blocking the rest. The cursor lives in memory; a restart starts a new pass.

**Background sync** (off by default). The setting `ticket_sync_interval_min` (`PUT /api/settings`,
0–1440, 0 = off) makes the server sync every enabled tracker that often, one capped batch per
tracker per run; a tracker with links left in its pass continues on the next 30-second tick. The
runner re-reads the setting every tick, so a change applies without a restart.

**Push updates (webhooks).** `POST /api/hooks/trackers/{id}` lets GitHub or Jira report a ticket
change at once instead of waiting for the next sync. It is the one tracker route outside the
session check (`requireAuth` exempts `/api/hooks/trackers/`); the caller proves itself with an
HMAC-SHA256 of the raw body under the tracker's `webhook_secret` (optional config key, at least 16
characters; sealed, masked and scrubbed like the token). The check is constant-time and runs before
the body is parsed (`ticketing.WebhookReceiver`).
- **GitHub:** a repository webhook, content type JSON, *Issues* events, the secret in GitHub's
  secret field. The signature is `X-Hub-Signature-256`. Deliveries for another repository are ignored.
- **Jira Cloud:** an admin webhook (*Issue updated*) registered with a secret; Jira signs it as
  `X-Hub-Signature: sha256=…`. Issues outside `project_key` are ignored. Jira Connect JWT and
  secrets in the URL are not supported (a URL secret ends up in proxy and access logs).
- **DefectDojo:** no webhooks; use sync.

A delivery is only a hint: its payload names the ticket, then the ordinary sync of that one link
reads the state from the tracker's API under the tracker lock. A forged state cannot be injected
and a replay can at most cause one more read; repeated delivery ids are also dropped for 10 minutes.
Replies: 200 `{"status":"ok","synced":n}`, `ignored` (unknown ticket, other event, tracker
disabled), `duplicate`; 202 `deferred` when a push or sync of the tracker holds the lock (GitHub
does not retry a 2xx, so the change lands with the running or next sync; a manual redelivery is
accepted); 401 for an unknown
tracker, no secret, or a missing or bad signature (all the same, so tracker ids are not revealed);
413 over 1 MiB or a body slower than 10 s; 503 past 4 concurrent deliveries. Refusals hash the body
like a real check, so timing does not reveal configured trackers either. Delivery ids over 128 bytes
are not remembered (such a delivery is still verified and synced). A delivery that pulls a status (or fails) is
written to the audit chain as `ticket.webhook`. The tracker sheet shows the URL to register.

The dedup key is `ticketing.DedupKey` over the finding's class, name and asset ids, or over the
import key for an imported finding.

## Scanner data in: imports

`POST /api/integrations/imports?format=<f>[&task_id=N][&asset_ids=1,2][&file_name=x]` with the raw file as the
body (32 MiB cap). Formats: `nmap-xml`, `nuclei-jsonl`, `zap-json`, `burp-xml`, `sarif`
(Semgrep and other SARIF 2.1.0 tools). `GET /api/integrations/imports` lists past imports.

- Assets go through the same `AssetStore` upserts as `POST /api/assets`, and each one is linked to
  the import (`scan_import_assets`). Owning an asset does not authorize testing it: the scope gate
  still decides at action time.
- Each result at low severity or above becomes a finding with `status=pending`,
  `verification_status=unverified`, POE 0, `worker=import:<tool>`, and a row in
  `finding_import_sources` (import id, tool, rule, record pointer, dedup key). Informational
  results are counted, not stored.
- **Imports never promote.** A re-import of a result with a known dedup key leaves the existing
  finding exactly as it is: severity, status and verification included.
- Summary and name are built from typed, sanitized fields (tool, rule token, severity, host). The
  scanner's free text (titles, descriptions, matched values) is target-shaped, so it is kept only as
  quoted strings in the evidence under an untrusted-data header. Credentials in URLs are stripped and
  Burp request/response bodies are not imported.

## Code-security lane: SAST and SCA in the same graph and report (E10)

Code scanners feed the same records as runtime testing. Run Semgrep for SAST, and Trivy,
osv-scanner, Grype or another SARIF-emitting tool for SCA (dependencies), and import the SARIF
into the engagement's task:

```sh
semgrep scan --sarif -o semgrep.sarif ./src
curl -H "Authorization: Bearer $TOKEN" --data-binary @semgrep.sarif \
  "http://localhost:8787/api/integrations/imports?format=sarif&task_id=12&asset_ids=345&file_name=semgrep.sarif"
```

- **Lane.** Every imported finding has a lane: `runtime` (nmap, nuclei, ZAP, Burp), `sast` or
  `sca`. A SARIF result is `sca` when its tool only scans dependencies (osv-scanner, grype, ...),
  its rule id is an advisory id (CVE, GHSA, PYSEC, RUSTSEC, GO-, SNYK-, ...) or its file is a
  manifest or lockfile; otherwise `sast`. A Trivy file can hold both. The lane, the repo-relative
  path and the line are stored in `finding_import_sources` (migration 0024).
- **Exploration graph.** With `task_id`, the import writes one exploration **fact** for the scan
  run (format, tool, version, file hash, counts by lane; typed values only), and every new finding
  becomes a **finding node** of the task that the fact `yields`, with `findings.node_id` set, the
  same as a worker's finding. A task that is archived or being archived refuses the import (409).
- **Asset graph.** `asset_ids` (at most 20, and only with `task_id`) anchors the code findings to
  the runtime assets the scanned code serves, such as the application's `http_service`. A SAST
  finding and a runtime finding on the same application then share the asset. Each anchor must
  already belong to the task, either in its declared scope or carrying the task in `task_ids`;
  otherwise the request gets 400 and nothing is written. An anchor decides which client companies
  see the finding, so an arbitrary asset id is never accepted. The import fact itself anchors
  nothing, because an import tests no runtime asset and must not raise coverage. Runtime findings
  use the anchors only when no imported asset matched their host.
- **Dedup per task.** A task import's dedup key includes the task, so a result already known
  from another engagement still reaches this task's graph and report. Re-importing into the same
  task adds nothing.
- **Report.** `GET /api/report?task=N` lists code and runtime findings together, sorted by
  severity. Each imported finding has a **Source** line (lane, tool, rule, `path:line`, import
  id). In the summary, "Confirmed findings" leaves out imported claims; they get their own count
  and a per-lane breakdown.
- **Goals.** Imported findings are leads, not results. `graph_overview.findings_total` does not
  count them, `finding_list` marks them `imported: <lane>`, and `prove_goal` refuses them because
  they have no verified traffic. To report a code finding as proven, reproduce it at runtime and
  record that as a worker finding.

The in-run agent tool `ingest_scan_output` (`agent/scan_ingest.go`) is separate: it writes facts
for the planner during an exploration and creates no findings.
