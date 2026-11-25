# E2 integrations and E10 code-security lane — hand-off detail (2026-11-24)

Moved out of HANDOFF.md Current state by c-d059ae to keep it under the size limit. Still true unless HANDOFF.md says otherwise.

**2026-11-24 — E10 code-security lane (card `c-72dd9d`, in review)** on `ej/20261124-150533-1ba29e`,
which merges the E2 backend branch `ej/20261124-055326-6bcfa9` (so E2 lands with it). SAST and SCA
results come in as SARIF through the E2 import API and now share the runtime record:
- `scanimport` gives each finding a lane (`runtime`/`sast`/`sca`, from the tool, an advisory-style
  rule id, or a manifest or lockfile; `scanimport/lane.go`).
- `POST /api/integrations/imports?format=sarif&task_id=N&asset_ids=…` writes one exploration fact
  for the scan run that `yields` a finding node per new finding (`findings.node_id` set), anchored to
  the runtime assets the code serves, which must already be in the task's scope or asset set
  (`db/code_findings.go`, `server/scan_import_lane.go`). The fact itself anchors nothing (coverage).
  Task imports dedup per task.
- `GET /api/report` lists code and runtime findings together, with a Source line
  (lane, tool, rule, `path:line`, import id) and a per-lane summary (`report/source.go`).
- `findings_total` skips imported claims and `finding_list` marks them; `prove_goal` already refused them.
- Migration `0024_code_security_lane.sql` adds lane/location/line and fact/anchor columns. E2's
  `0015_integrations.sql` landed on main, so the renumber to 0023 was dropped when c-d059ae merged
  this branch; 0024 follows main's `0023_distributed_workers.sql`.
- An independent review found four anchor and dedup problems (client cross-visibility, inflated
  coverage, a second task missing known results, task deletion owning foreign assets); all are
  fixed in `d483374b`.
- E2 × E5 fix: `GET /api/exploration/findings/{id}/integrations` now needs `read`, not client_read.
- Verified against Postgres 16: `TestCodeScanJoinsTaskGraphAndReport` (semgrep + trivy/osv SARIF
  plus a runtime finding in one task report), the `db`, `agent`, `report` and `scanimport` suites, and
  `scripts/verify.sh`. The `./server` suite passes except `TestManagerSOCKS5UDP…` (no `127.0.0.2` alias on macOS).
- Docs: `docs/integrations.md` § Code-security lane. Follow-ups: `c-d059ae` (UI and exports),
  `c-c978e3` (SAST-guided runtime verification), `c-5c4009` (tracker RBAC).

**2026-11-24** — **E2 integrations** (card `c-6def05`, branch `ej/20261124-055326-6bcfa9`, landing with
E10's branch; UI `c-597eb6` on `ej/20261124-132955-7aef3f`). See `docs/integrations.md`.
- Findings out: package `ticketing` (GitHub Issues, Jira, DefectDojo) over `notify.Do` (SSRF guard,
  redaction); tracker configs sealed at rest and masked; `/api/integrations/trackers` (CRUD, `push`,
  `sync`); status round-trips, tracker wins on conflict, verification never touched.
- Scanner data in: package `scanimport` (Nmap XML, Nuclei JSONL, ZAP JSON, Burp XML, SARIF);
  `POST /api/integrations/imports` writes assets with provenance and unverified POE-0 findings; a
  re-import never edits or promotes. Migration `0015_integrations.sql` (on main).


**2026-11-24** — **E2 integrations** (backend `c-6def05`, UI `c-597eb6`, follow-ups `c-2d91eb`, all in review)
are on branch `ej/20261124-150516-d78c96`. It merges the UI branch `ej/20261124-132955-7aef3f`, which carries
the backend `ej/20261124-055326-6bcfa9`, so landing it lands all of E2. See `docs/integrations.md`.
- **Findings out:** package `ticketing` (GitHub Issues, Jira, DefectDojo) goes through `notify.Do` (the SSRF
  guard and redaction). Tracker configs are sealed and masked, and a destination change needs the credentials again.
  `/api/integrations/trackers` has CRUD, `push` and `sync`. Status round-trips, and the tracker wins a conflict.
- **Scanner data in:** package `scanimport` reads Nmap, Nuclei, ZAP, Burp and SARIF. `POST /api/integrations/imports`
  writes assets with provenance and unverified POE-0 findings (`finding_import_sources`), and never edits on re-import.
  `GET /api/integrations/imports?limit=&offset=` returns `{imports, total}`.
- **UI:** System → Integrations (the trackers table and a masked-credential sheet) and Scanner imports (format
  detection, Past imports with a pager). Each import's New findings count links to
  `/function/findings?import_id=N`. `import_id` is in the shared findings filter (`db.FindingFilter.ImportID`),
  so it applies to the list, groups, asset tree and export, and the page shows a chip that clears it.
  The findings bulk bar and the detail page have Push to tracker; the detail has a Tickets card.
- **RBAC (with E5):** `GET /api/exploration/findings/{id}/integrations` needs `read`, not `client_read`.
  Tracker create/edit/delete need `admin`, the same as notification channels. Push, sync and import need `operate`.
  The E2 pages do not yet gate their controls by role (`c-119206`).
- **Browser e2e:** `scripts/e2e/integrations.sh` sets up a throwaway Postgres, the backend, a Next production
  build and Playwright, then runs `integrations-e2e.js` against `fake-github.js`. It passed 24 checks
  locally on 2026-11-24. It is not a CI job yet (`c-1198a2`, after c-cc25b6).
- Migration `0015_integrations.sql` is unreleased, and its checksum is enforced, so do not edit it. Fix forward.
- Local `./server` failures that are environment, not code: `TestConnectWithoutPasswordRealPG` (trust-auth Postgres)
  and the SOCKS5 test that needs a `127.0.0.2` alias.

