# CLAUDE.md — ARTEX

@AGENTS.md

The project overview, where things live, how to build/run/test, the handoff policy, and the
project conventions are all in `AGENTS.md` (imported above). Read `HANDOFF.md` first for the
current state and triage.

Key reminders specific to this repo:

- **Authorized-use invariants are load-bearing.** The scope/target gate, interception approval
  gate, budgets, noise governor, untrusted-output boundary, and audit trail keep autonomous
  action inside its authorization. Never weaken, bypass, or route around them. The `RSI.md`
  "immutable surfaces" must never enter an automated search space.
- **Fix migrations forward** — a published `db/migrations/*.sql` is never edited.
- **Run `scripts/verify.sh`** before committing; `--full` for the race + web build, `--security`
  for `govulncheck`.
