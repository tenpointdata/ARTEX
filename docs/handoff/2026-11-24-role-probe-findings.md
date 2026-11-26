# Role probe findings (c-80fe4e), 2026-11-24

`scripts/e2e/role-probe.sh` ran twice. Each time every role (reviewer, operator, client_read) signed in
and clicked every enabled control on every console page.

## Results

**1. On the c-92bb52 branch (`ej/20261124-144553-d5978e`, not yet merged; it hides role-lacking
controls).** These are the results that matter.
- **operator: passes.** No 4xx. No request needs a permission it lacks. The traffic sheet's Stream tab
  shows the recorded WebSocket's messages.
- **reviewer: 2 gaps.**
  - Task detail **Overview** tab loads `GET /api/llm/records/by-model`, which needs operate (403).
  - `/system/logs` opened by URL loads `GET /api/logs/stream`, which needs operate (403).

  Opening a traffic row no longer opens the sheet, so there's no Stream view and no 403.
- **client_read: the same gaps as on main** (see the client_read section below). The dashboard's
  requests run on sign-in before the redirect to `/function/findings`, and every non-client page
  opened by URL loads with 403s. The fix is a route guard (card `c-bd9b7a`).
- Sidebar: every link of every role leads to a probed page.

**2. On main (`e9fbefe`, without c-92bb52).** The detailed list below comes from this run. It's the
baseline that c-92bb52 fixes: everything in the reviewer and operator sections below is gone on the
branch except the 2 reviewer gaps above.

---
`scripts/e2e/role-probe.sh` on main at `e9fbefe`. Each role (reviewer, operator, client_read) signed
in and clicked every enabled control on all 28 console pages: about 730 controls for reviewer and
operator, 456 for client_read. The run took about 5 minutes. Seeded: a WebSocket exchange recorded
through the proxy, a paused task, a finding, the `goals` agent, and one row of every config kind.

**How to read this.** Writes are faked in the browser, so each "needs X" line means an enabled control
sent a request the role cannot make. The control shown is the first one that triggered it. GETs fired
by polling are credited to whichever control was clicked last, so trust the page more than the control
for reads.

**What passed:**
- Every sidebar link of every role leads to a page the probe visits. client_read sees only Findings
  and Reports.
- operator: the traffic sheet's Stream tab shows the recorded WebSocket's messages, and no response
  was 4xx.
- The detail pages (task, finding, agent) load for reviewer and operator.

## reviewer (read + review): 8 kinds of 4xx, 36 groups of role-lacking requests

Reads that answer 403 (needs operate):
- `/function/llm-records` loads `GET /api/llm/records` (403). The page is missing from the sidebar but
  can be opened by URL.
- Task detail: the **Overview** tab loads `GET /api/llm/records/by-model`, and the **Credentials** tab
  loads `GET /api/tasks/{id}/credentials`. Both answer 403.
- Traffic: **Export HAR** (`/api/traffic/export/har`). Opening a row loads `GET /api/traffic/exchange`
  (403), so a reviewer can't open the sheet or its Stream view at all.
- Workspace: opening a file (`/api/workspace/read`) and **Download** (`/api/workspace/download`).
- `/system/logs` loads `GET /api/logs/stream` (403).

Enabled controls that need operate:
- Chat **Send message** (`POST /api/conversations`).
- Task detail **Resume** (`POST /api/tasks/{id}/control`).
- Traffic **Run dry-run** (hooks) and **Confirm delete** (`DELETE /api/traffic`).
- Skills: the per-skill MCP checkboxes (`PUT /api/skills/{name}/meta`).

Enabled controls that need admin. Each item below is also enabled for **operator**:
- `/function/llm-records`: the recording switch (`PUT /api/settings`).
- `/function/sync`: **Save & enable** (`POST /api/sync/scopesentry/datasource`).
- `/function/traffic`: **Confirm purge** (`DELETE /api/traffic/all`).
- `/system/agents`: **Create** (`POST /api/agents`), and the tool checkboxes in the agent dialog and on
  the agent detail page (`PUT /api/tools/{name}`, about 50).
- `/system/integrations`: **Add** tracker (`POST /api/integrations/trackers`). Card `c-119206` overlaps.
- `/system/intercept/assets`: the rule switch and the rule dialog's **Save**
  (`/api/asset-intercept/rules/{id}`).
- `/system/llm`: the pool switch, **Load available models**, **Test connection**, and the retry-policy
  **Save** (`/api/llm/*`, `PUT /api/settings`).
- `/system/mcp`: **Add**, the Enable switch, **Delete**, and the visibility checkboxes (`/api/mcp`,
  `/api/visibility/toggle`).
- `/system/notify`: the push master switch and **Save global settings** (`PUT /api/settings`).
- `/system/settings`: every switch, and **Re-detect** (`PUT /api/settings`,
  `POST /api/settings/python/detect`).
- `/system/skills`: the per-agent visibility checkboxes (`POST /api/visibility/skill/toggle`).
- `/system/tools`: **Create** custom tool (`POST /api/tools/custom`).

## operator (read + review + operate)

No 4xx responses. The only failures are the admin-only controls listed above.

## client_read

The sidebar is right, but:
- **On sign-in**, before the console lands on `/function/reports`, the client receives 403s from
  `/api/tasks`, `/api/intercept/pending`, `/api/settings`, `/api/exploration/activity`,
  `/api/tokens/*`, `/api/traffic`, `/api/assets/counts` and `/api/llm/profiles`, and a 400 from
  `/api/stats`. These are the dashboard's own requests: the client loads the dashboard and is then
  redirected.
- **Finding detail** (a page client_read is meant to use) loads
  `GET /api/exploration/findings/{id}/integrations` and `GET /api/integrations/trackers`. Both answer
  403. Card `c-119206` overlaps.
- **Every other page opened by URL** loads with 403s and shows enabled controls. The fix is a route
  guard that sends client_read to `/function/reports` for any page it isn't allowed to see, rather than
  gating each page.
- **Stream view:** the traffic list never loads for client_read (403), so there's no WebSocket row to
  open. A route guard fixes this too.

## Probe artefacts, not product bugs

Faked writes answer `{}`, so some dialogs throw afterwards: chat `localeCompare`, the hooks dry-run
reading `method`, the tracker **Add** reading `base_url`, and the settings **Save** reading `trim`. The
probe lists these as page errors but doesn't count them as failures.
