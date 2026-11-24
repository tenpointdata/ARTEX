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
- `POST /api/integrations/trackers/{id}/sync` — reconcile every ticket of the tracker.
- `GET /api/exploration/findings/{id}/integrations` — a finding's tickets and import provenance.

Status mapping: pending/in_progress/confirmed → open; resolved/fixed → closed as fixed;
false_positive, risk_accepted, duplicate → closed with that resolution; ignored → closed as
won't fix. Reading back, an open ticket reopens a closed finding as `confirmed`. A sync compares
each side with the state both agreed on last time (`finding_tickets`); when both changed, the
tracker wins. A ticket status changes the finding's disposition only, never its verification.

The dedup key is `ticketing.DedupKey` over the finding's class, name and asset ids, or over the
import key for an imported finding.

## Scanner data in: imports

`POST /api/integrations/imports?format=<f>[&task_id=N][&file_name=x]` with the raw file as the
body (32 MiB cap). Formats: `nmap-xml`, `nuclei-jsonl`, `httpx-jsonl`, `ffuf-json` (assets only),
`zap-json`, `burp-xml`, `sarif` (Semgrep and other SARIF 2.1.0 tools). URLs are canonical (no
userinfo, fragment or default port), so one service reported by two scanners is one asset. `GET /api/integrations/imports` lists past imports.

- Assets go through the same `AssetStore` upserts as `POST /api/assets`, and each one is linked to
  the import (`scan_import_assets`). Owning an asset does not authorize testing it: the scope gate
  still decides at action time.
- Each result at low severity or above becomes a finding with `status=pending`,
  `verification_status=unverified`, POE 0, `worker=import:<tool>`, and a row in
  `finding_import_sources` (import id, tool, rule, record pointer, dedup key). Informational
  results are counted, not stored.
- **Imports never promote.** A re-import of a result with a known dedup key leaves the existing
  finding exactly as it is: severity, status and verification included. It only adds a
  `finding_import_links` row, so every import that reported a finding is listed (`imports` on the
  provenance). A ZAP instance's parameter is part of the key.
- Summary and name are built from typed, sanitized fields (tool, rule token, severity, host). The
  scanner's free text (titles, descriptions, matched values) is target-shaped, so it is kept only as
  quoted strings in the evidence under an untrusted-data header. Credentials in URLs are stripped and
  Burp request/response bodies are not imported.

The in-run agent tool `ingest_scan_output` (`agent/scan_ingest.go`) uses the same `scanimport`
parsers (with informational results kept) but writes assets and facts for the planner during an
exploration and creates no findings.
