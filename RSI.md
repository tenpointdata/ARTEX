# Recursive self-improvement in ARTEX

Status: measurement, shadow memory, and the benchmark gate (E3). There is no
automated harness search and no automatic adoption: edits are proposed (by a
person or a future proposer), measured on a held-out suite, and adopted only by
an owner action. No code in this repository modifies itself.

This file defines the contract any future harness-optimization work must meet,
so the first experiment cannot quietly become an unguarded rewrite loop.

## The three loops

| loop | object | state |
|---|---|---|
| A. within a task | planner, worker, retester, adversarial gate | exists |
| B. across tasks | evidence-gated lessons from closed tasks | shadow mode, PR #104 |
| C. on the harness | prompts, skills, tool guidance | gate built (`rsi/`, `server/rsi_loop.go`), flag off; no proposer |

Loop A produces task gain. Loop B is meant to produce retained gain: a lesson
must survive a closed task and change a later one. Loop C is the only loop that
can produce improver gain, and it is the only one that can silently damage the
product, which is why it goes last.

## Loop C contract

Any PR that proposes, probes, or adopts harness edits must satisfy all of:

1. **Live tasks propose, benchmarks adopt.** A successful bounty task can
   generate a candidate lesson or edit, but it can never be the evidence for
   adoption. Adoption requires held-out benchmark runs, because a live task is
   unreproducible and contaminated by its target.
2. **Owner approval per adoption.** The loop may prepare a proposal branch and
   show the diff. It may not change the active prompt. Adoption is one explicit
   owner action, like a baseline pin.
3. **Immutable surfaces.** The following are never in the search space:
   the adversarial and quality gates, scope and authorization checks,
   prompt-injection defenses, tool allowlists, and the code-owned prompt tails
   (traffic tools, artifact rules, lesson block). Only the editable prompt body,
   skill markdown, and tool descriptions can be proposed.
4. **One-step rollback.** Every adopted prompt change is a new
   `agent_prompts` version, so rollback is a pointer change. Skills are files
   under git; no adoption may bypass review.
5. **Flag off by default.** The loop runs behind a feature flag. Turning it on
   requires an owner decision, and the flag is checked per run.
6. **Bounded edit budget.** A round may propose a bounded number of edits
   (annealed over rounds). A critic proposes only edits that are expected to
   transfer; a pruner rejects edits that only fit the probe task. This follows
   RRSI (arXiv 2609.24972).
7. **Attribution.** Probe results are compared against an unchanged baseline on
   the same task and seed. Regressions are attributed to a specific module
   rather than to the loop as a whole; the modular fault-localization idea is
   from ModularRSI (arXiv 2609.14857). A paired evolving/non-evolving comparison
   guards against misattributed safety regressions, following SEABench
   (arXiv 2609.35596).
8. **No self-approval and no silent state.** The loop cannot set its own status,
   edit its own acceptance criteria, or write to the lesson library as
   `adopted`. Lesson status changes stay owner actions.

A future Loop C PR is acceptable only with:

- a benchmark adapter and a held-out task set, independent of the tasks used to
  propose the edit;
- a baseline run and a candidate run with the same model, seed, budget, and
  judge;
- the judge independent of the subject model, with a falsification battery
  (the existing L3 runner already enforces this);
- per-gate results with denominators, not a single aggregate score;
- a written rollback path and the flag name.

## Benchmark gate (built)

The gate implements the contract above; it has no proposer (critic) of its own.

| rule | where it is enforced |
|---|---|
| 1. live tasks propose, benchmarks adopt | `rsi.Evaluate`: split must be `held-out`; the proposal's and the arm's `proposed_from` must not be in the evaluated targets; the suite run must be registered for the proposal before it ran (`POST .../suite-runs`: suite_run id, suite, version, targets, seed, replicates; held-out only; one open at a time, at most 3 per proposal, each backs exactly one upload, passing or not) and the arms must be exactly that run (every registered target × replicate, run ids as the harness derives them); every run's task must exist on this instance and be newer than the registration, and a task backs at most one evidence upload across all proposals (`rsi_evidence_tasks` ledger); every judged task must match its arm stamp in the server's per-task arm ledger (`task_rsi_arm`, below) |
| 2. owner approval per adoption | `POST /api/rsi/proposals/{id}/adopt` needs the owner password (step-up under the login rate limit) and names the exact passing evidence id; a bearer token alone cannot adopt |
| 3. immutable surfaces | `rsi.ValidateEdit` admits only `prompt_body` for `goals`/`planner`/`worker` and `lesson`; `rsi.ImmutableSurfaces` and git-reviewed surfaces (`skill`, `tool_description`) are refused; a body may not carry a code-owned tail marker (`rsiForbiddenMarkers`), checked on the raw and the rendered template after NFKC, dash, zero-width and whitespace folding. The tails are appended by code after the body, so a body cannot remove them |
| 4. one-step rollback | adoption appends an `agent_prompts` version and records before/after; `POST .../rollback` moves the pointer back, only while the adopted version is still active (an owner edit since is never overwritten); open with the flag off |
| 5. flag off by default | `ARTEX_RSI_LOOP=on` is read on every request; off refuses propose, evidence and adopt. Lesson injection keeps its own flag, `ARTEX_RSI_LESSONS` |
| 6. bounded, annealed budget | `rsi.DefaultBudget`: 4, 2, 1, 1, … proposals per round; rounds move forward one at a time, and at most 4 proposals are open at once. The pruner: paired gain must appear on at least 2 distinct held-out targets |
| 7. attribution | the server recomputes per-gate rates and paired gained/lost from the raw arm records; the candidate may differ from the baseline in exactly the proposed module (prompt sha per role, lesson set); any gate with net paired loss rejects and is reported as a regression of that module, with its targets (a candidate run that crashed where the baseline ran counts its baseline passes as lost); the baseline must not evolve (no lessons) |
| 8. no self-approval, no silent state | status moves only through evidence (server verdict), adopt (owner) and rollback; every transition is a row in the append-only `rsi_adoption_log` (trigger refuses UPDATE/DELETE), written in the same transaction. Registrations are logged (`register`), and a graduated lesson that extraction retires (its evidence withdrawn) or revives as a candidate is logged on its adopted proposal (`lesson_retire`, `lesson_revive`, actor `system`) in the extraction's transaction |

At adoption every role's live prompt must still have the sha the evidence's
baseline ran, and graduating lessons must still render to the block the
candidate ran (`lessons_block_sha`), so stale evidence cannot be adopted
(re-run the suite).
Workflow and commands: `scripts/benchmark/README.md`, "Benchmark gate".

**Per-task arm ledger** (`task_rsi_arm`, migration 0026). When a task's agents
are built (`agentsForTask`), the server records the sha of the body each of
goals/planner/worker runs (the stored prompt, else the built-in default: the
value the harness stamps) and the lesson ids and block sha injected into the
planner and worker. A rebuild with a different arm while the task is live, or a
prompt pointer move on one of those roles while it is live (trigger on
`agents.current_prompt_id`), marks the row drifted, and a drifted task backs no
evidence; a rebuild after the task ended changes nothing. Evidence is checked
record by record against it: prompt sha per role, lesson ids, block sha, and
the task's name, which must end with the record's run id (the run id names the
suite run, replicate, arm and target).

Known limits: the judge verdicts inside the uploaded records come from the
harness, so the server checks their shape, binding and provenance but cannot
re-judge them; the owner reviews the suite's `report.md` before adopting. The
ledger records the template a role was built with; a stored template that
fails to render falls back to the built-in default at run time, which the
ledger does not see (proposal bodies are render-checked at propose time).
Marker checks catch imitation of a tail, not a body that argues against one in
its own words. `proposed_from` is compared as exact target ids. Reject and
rollback need only a session, by decision (2026-11-24, card `c-0d277f`): both
move toward the state the owner last approved (reject closes an unadopted
proposal; rollback restores the version that was live before adoption, and
refuses if an owner edited since), so a stolen session can only undo
improvements, never introduce a change; and rollback must stay one step for an
emergency revert. Both are logged with their actor. The generic prompt editor (`PUT /api/agents/{key}/prompt`)
remains an owner path outside the gate, and the benchmark harness uses it on its
dedicated bench instance to switch arms.

## What the current layers do

- `GET /api/tasks/{id}/outcome` (PR #102) records what a task produced and what
  it cost: findings by disposition and severity, retest verdicts, tokens, tool
  calls, skill misses, wall time.
- `ARTEX_RSI_LESSONS=off|shadow|inject` (PR #104) extracts evidence-gated
  lessons from terminal tasks and, in `inject` mode, appends a code-owned
  data-not-instructions block after the planner and worker system prompts.
  Status stays `candidate`.
- `GET /api/skill-gaps` (PR #106) turns the existing miss ledger into a
  capability backlog.

None of these changes an agent's instructions on its own. Shadow mode exists to
measure whether retrieval helps before any harness edit is attempted.

## Credits

The admission rule (no lesson without cue, action, and evidence) and the
propose/probe/compose shape come from:

> Harness^2: Recursive Agent Harnessing for an Open World.
> Ruiyao Xu, Yanfei Chen, Zhongying CuiZhu, Bhavana Dalvi Mishra, Yifei Ming,
> Han Yu, Rujun Han, Chen-Yu Lee, Tomas Pfister. arXiv:2610.14224, 2026.
> Google Research. Apache-2.0. https://github.com/google-research/harness2

The evidence taxonomy used in the design (task gain, retained gain, improver
gain) and the implementation pointers come from:

> awesome-rsi, lobehub. https://github.com/lobehub/awesome-rsi

Specific ideas referenced above:

- RRSI: Regularized Recursive Self-Improvement of Agent Harnesses,
  arXiv:2609.24972 (annealed edit budget, critic and pruner).
- ModularRSI: Modular and Generalizable Recursive Harness Self-Improvement,
  arXiv:2609.14857 (module-level fault localization from success/failure
  contrast).
- SEABench: Benchmarking Endogenous Misalignment in Self-Evolving Agents,
  arXiv:2609.35596 (paired evolving/non-evolving attribution).
- Agentic Harness Engineering: Observability-Driven Automatic Evolution of
  Coding-Agent Harnesses, arXiv:2604.25850 (observable edits with predictions
  verified later, the same discipline as the benchmark gate here).
- A Safety in Self-Evolving Agents survey, arXiv:2610.00093 (why properties
  must survive accumulation and cross-context reuse, not just entry screening).

Only ideas are borrowed. ARTEX code is independent Go and Python; no source
from harness2 or the referenced projects is copied.
