# E4 — Methodology-driven coverage (c-c0bb07)

Moved from HANDOFF.md Current state on 2026-11-24 to keep it under the size limit.

**2026-11-24 (c-c0bb07, E4)** — Methodology-driven coverage: OWASP WSTG 4.2 (97 tests) and the OWASP
API Security Top 10 2023 are typed checklists in `methodology/` (data in `methodology/data/*.json`,
embedded and validated). A task's applicable checklists come from its goal and description (no
column). `db/coverage_methodology.go` scores them from the coverage ledger, findings (vulnclass)
and intents citing an item (`payload.methodology_item`). The planner gets a `[Methodology coverage]`
block each round, and `add_intent` takes `methodology_item`, which is validated and stored as the
canonical item id. `GET /api/tasks/{id}/coverage` returns `methodologies`, and the task overview's
coverage card shows a filterable table of the items. Verified by the methodology, agent and
db unit tests, a Postgres integration test (`TestMethodologyCoverage`), `tsc` and `scripts/verify.sh`.
The UI was not checked in a browser.
