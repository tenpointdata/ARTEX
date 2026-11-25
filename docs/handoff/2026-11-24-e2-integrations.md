# E2 integrations: state as of 2026-11-24

Moved out of HANDOFF.md Current state to keep that section under 16 KB.

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
  `/system/integrations` and its imports page gate by role since c-4ee114; the finding-detail Tickets/Push
  controls and the client_read check remain (`c-119206`).
- **Browser e2e:** `scripts/e2e/integrations.sh` sets up a throwaway Postgres, the backend, a Next production
  build and Playwright, then runs `integrations-e2e.js` against `fake-github.js`. It passed 24 checks
  locally on 2026-11-24. It is not a CI job yet (`c-1198a2`, after c-cc25b6).
- Migration `0015_integrations.sql` is unreleased, and its checksum is enforced, so do not edit it. Fix forward.
- Local `./server` failures that are environment, not code: `TestConnectWithoutPasswordRealPG` (trust-auth Postgres)
  and the SOCKS5 test that needs a `127.0.0.2` alias.
