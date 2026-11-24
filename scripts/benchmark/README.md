# ARTEX L3 benchmark

Measures whether the product can find and prove real vulnerabilities in a real
application, unprompted, against an executable oracle. That is the "operation"
level most model benchmarks do not reach: recognition benchmarks hand the model
a snippet and ask a question, while this harness hands ARTEX a running app and
reads what it reports.

Method discipline adapted from capability-matrix (MIT) and VulnHunter's
benchmark harness (Apache-2.0).

## Layout

| | |
|---|---|
| `run_bench.py` | The runner: deploy corpus, create task, wait, fetch findings, judge, tally |
| `ground_truth/<corpus>.json` | Version-stamped ground truth (component + class + mechanism, deliberately coarse) |
| `eval_suite.py` | Planner/worker evaluation suite: baseline vs candidate prompt arms over a suite, per-gate comparison |
| `suite_stats.py` | Pure aggregation: per-gate rates with denominators, Wilson/Newcombe intervals, paired table |
| `arms.py`, `arms/*.json` | Prompt-version arms and the server-side prompt switch (apply, read back, restore) |
| `suites/*.json` | Target sets: `heldout-v1` (adoption evidence) and `dev-v1` (proposing and debugging), disjoint |
| `ground_truth/dvga.json` | Damn Vulnerable GraphQL Application 2.2.0 ground truth (v2, 12 entries hand-verified against the 2.2.0 source, each citing its resolver/helper; DoS classes excluded) |
| `benchmark-results/` | Output (gitignored): one directory per run with `report.md` + `results.json` |

## Subject instance

The instance under test is `docker-compose.bench.yml` at the repository root. It
builds from the production `Dockerfile` (the benchmark measures the shipped
image) and runs its own Postgres and named volumes for data and keys, separate from any production
instance on the host:

```bash
cp bench/.env.example bench/.env     # set ARTEX_PG_PASSWORD and an LLM key
docker compose -f docker-compose.bench.yml up --build -d
docker compose -f docker-compose.bench.yml logs artex-bench | grep "Setup token"
```

`bench/.env.example` sets `ARTEX_RSI_LESSONS=off`, so held-out runs measure the
prompts under test and not the lesson library. The runner can mint its token
from the server's key: `docker compose -f docker-compose.bench.yml cp
artex-bench:/app/keys/jwt.key bench/jwt.key`, then `ARTEX_JWT_KEY=bench/jwt.key`
(gitignored). When port 8787 is taken, set `ARTEX_BENCH_PORT` (and
`ARTEX_BENCH_PROXY_PORT`) in the shell and point `ARTEX_BASE` at it. A corpus
published on the host is reached as `--target-host host.docker.internal`, which
the egress filter denies to agents until `bench/.env` allows that one address
(`ARTEX_EGRESS_ALLOW`, see the comment there).

## Scoring

Every ground-truth entry is matched against the scanner's findings by an
independent LLM judge, then graded on four gates (partial answers earn partial
credit):

| Gate | Test | Points |
|---|---|---|
| component | names the right endpoint or feature | 1 |
| class | acceptable vulnerability class | 2 |
| keywords | at least two mechanism-specific terms | 1 |
| proof | PoC describes a route that would actually work | 1 |

Standing rules, enforced by the runner:

1. **Multi-gate scoring, never pass/fail.** The gate pattern is the work item;
   the aggregate percentage is the least useful output.
2. **Discovery candidates.** A scanner finding beyond ground truth is not a
   false positive until verified; precision is reported as bounds meanwhile.
3. **The judge must not be the model under test.** The runner refuses to start
   when `--judge-model` equals `--subject-model` (override knowingly with
   `--allow-self-judge`).
4. **The judge must pass a falsification battery** before grading: a correct
   match, a wrong-bug match, and a vague non-answer. A grader that cannot fail
   the wrong ones does not run.
5. **Invalid never counts.** Findings with no summary and no report are
   excluded from every rate and named in the result.
6. **Every rate carries its denominator; the aggregate carries a Wilson 95%
   interval.** With 8 ground-truth entries, one run cannot separate close
   models; treat per-gate rates as observations and replicate before quoting
   differences.
7. **Results are stamped** with the method version, ground-truth version,
   subject model, judge model, and terminal task status. A result measured
   under an earlier version is not wrong; it is a result at that version.

## Usage

```bash
cd scripts/benchmark

# against a locally deployed ARTEX, with an independent judge endpoint
ARTEX_TOKEN=<jwt or mint via ARTEX_JWT_KEY=/app/keys/jwt.key copy> \
JUDGE_BASE_URL=https://api.example.com JUDGE_MODEL=<other-model> JUDGE_API_KEY=... \
python3 run_bench.py --corpus juice-shop --subject-model glm-5.3
```

Environment contract:

| Variable / flag | Meaning |
|---|---|
| `ARTEX_BASE` / `--artex-url` | ARTEX base URL (default `http://localhost:8787`) |
| `ARTEX_TOKEN` or `ARTEX_JWT_KEY` | API bearer token, or the server's jwt.key to mint one (then also `ARTEX_PWD_VERSION`: the `auth.password_version` settings value, e.g. `docker compose exec postgres psql -U artex -tAc "select value from settings where key='auth.password_version'"`) |
| `JUDGE_BASE_URL`, `JUDGE_MODEL`, `JUDGE_API_KEY` | OpenAI-compatible judge endpoint (independent of the subject) |
| `--subject-model` | model of the task's active profile, for the self-judge guard |
| `--llm-profiles` | comma-separated profile ids for the task chain, head first (e.g. `3,2`); stamped into the result |
| `--target-host` | host the task should target, as reachable from ARTEX (use `172.17.0.1` when ARTEX runs in a container and the corpus runs on the host) |
| `--image` / `--port` / `--corpus-url` | corpus container image, local port, or an existing deployment |
| `--task-timeout` | task `timeout_seconds` (default 5400) |
| `--keep` | keep the corpus container after the run |

The created task's goal bakes the standing quality gates (executed PoC, measured
impact with denominator, proof basis, zero-findings-is-valid), so the benchmark
measures the product as it actually behaves.

## Adding a corpus

Add `ground_truth/<name>.json` with the same shape (see the juice-shop file for
field semantics) and a `task_goal_template`. Pin the image tag in the file's
`version` note. Entries stay coarse on purpose: component + class + mechanism.
Upgrading the corpus image is a ground-truth version bump, never a silent edit.

## DrostBench corpus (drost mode)

[DrostBench v1](https://github.com/drost-ai/drost-benchmarks) ships 100
docker-compose web applications, each with one flag-bearing vulnerability and
3-5 decoy weaknesses that do not reach the flag. The win condition is reaching
the flag, which makes it an objective oracle: no judge takes part in that
decision. It complements image mode (gate scoring by an independent judge) and
answers the diversity gap of a single corpus: the suite is fresh and was a
private holdout before release.

Pin a checkout of the suite, then run one challenge at a time:

```bash
DROST_REPO=/path/to/drost-benchmarks python3 run_bench.py \
    --corpus-type drost --drost-id DBEN-v1-001 \
    --llm-profiles 3,2 --target-host 172.17.0.1 \
    --subject-model deepseek-chat
```

- `--target-host 172.17.0.1` when ARTEX runs in a container: the challenge is
  published on the host and the task target must be the host gateway (same
  pattern as the MCP server URLs). `--llm-profiles` picks the task's model
  chain, head first.
- Black-box web only: this measures the recon/pentest lane, not codeaudit.
- Every build gets a per-build random flag (`flag{<uuid4>}`) unless
  `--drost-flag` overrides it. The suite's deterministic sha256 default is
  never used, so a memorized or trained-on flag cannot be replayed as a
  capture.
- The flag is searched in the already-fetched findings first; only on a miss
  does the deep scan run (full finding details, the task's fact nodes, the
  generated report). Declaring a miss is a claim about all of those.
- Ground truth is derived at run time from the challenge `benchmark.json`
  (name, description, tags). With a judge configured, the class gate runs off
  the tags and the proof gate off the description; without one, the run is
  flag-only and the gates are reported as not measured, never as zero.
- Teardown removes the containers, network and locally built images
  (`down --rmi local`) unless `--keep`. Results are stamped with the challenge
  id and the suite's commit sha.

`--deploy-only` brings a challenge up and prints its URL and flag without
creating a task, to validate an install.

## Evaluation suite (baseline vs candidate prompt arms)

`eval_suite.py` turns single runs into a controlled A/B over a held-out suite,
to meet the measurement half of the `RSI.md` Loop C contract: a baseline arm and
a candidate arm with the same model, seed, budget and judge, compared per gate
with denominators. It adopts nothing: the output is evidence for an owner
decision, and the suite puts the starting prompts back when it ends.

```bash
cd scripts/benchmark
# export the body you want to change, edit it, and name it in an arm file
curl -s -H "Authorization: Bearer $ARTEX_TOKEN" localhost:8787/api/agents/planner \
  | python3 -c 'import json,sys; print(json.load(sys.stdin)["prompt"], end="")' > arms/planner-v2.tmpl
cat > arms/planner-v2.json <<'JSON'
{"name": "planner-v2", "proposed_from": ["DBEN-v1-001"],
 "prompts": {"planner": {"source": "file", "path": "planner-v2.tmpl"}}}
JSON

ARTEX_TOKEN=... JUDGE_BASE_URL=... JUDGE_MODEL=... JUDGE_API_KEY=... DROST_REPO=/path/to/drost-benchmarks \
python3 eval_suite.py run --suite suites/heldout-v1.json \
    --baseline arms/baseline.json --candidate arms/planner-v2.json \
    --subject-model deepseek-chat --llm-profiles 3,2 --target-host 172.17.0.1 \
    --replicates 3 --seed 7 --task-timeout 3600

# re-render or compare two saved arm results (refuses mismatched invariants)
python3 eval_suite.py compare <dir>/baseline.json <dir>/candidate.json
```

What it holds fixed and checks:

- **Arms vary only the editable prompt body** of `goals`, `planner` and `worker`
  (`source`: `current` = the body active when the suite started, `builtin` =
  the in-code default, `file` = a template). Swaps go through the server's own
  versioned prompt API (`PUT /api/agents/{key}/prompt`), so the code-owned
  tails, gates, scope checks, injection defenses and tool allowlists cannot be
  in an arm. Each swap is read back and its sha256 is stamped; a body that
  changes mid-suite aborts it.
- **Held-out discipline.** Adoption evidence comes from a `held-out` suite only
  (`--allow-dev-split` for exploration), and an arm whose `proposed_from` names
  a suite target is refused.
- **Same model, seed, budget, judge.** One stamp covers both arms: method
  version, subject model and llm chain, judge model, per-task timeout, seed,
  replicates, suite version and content-hashed ground-truth versions.
  `compare` refuses two results whose stamps differ (`--allow-mismatch` marks
  the report NOT COMPARABLE).
- **The seed fixes the schedule**: target order per replicate and arm order per
  pair, so neither arm always runs first against a freshly started target. It
  does not seed the LLM: ARTEX has no sampling-seed control, so replicates are
  the variance control.
- **Sequential, dedicated instance.** The server reads the active prompt live,
  so the suite refuses to start while any task is running or queued
  (`--allow-busy` overrides knowingly) and never runs two tasks at once. Run it
  on a bench instance with the RSI lesson library `off` (`ARTEX_RSI_LESSONS`),
  or a lesson learned in one arm leaks into the other. Arms that pin lessons
  need `ARTEX_RSI_LESSONS=bench` instead (see "Benchmark gate").
- **Per-gate results with denominators.** Gates: `detected`, `component`,
  `class`, `keywords`, `proof` (over ground-truth entries of judged runs, with
  the "over detected" view alongside) and `flag_captured` (over drost runs).
  Each carries passes/n and a Wilson 95% interval; the delta carries a Newcombe
  interval; a paired table counts gained and lost observations on the same
  target, replicate and entry. Failed runs are counted and excluded from the
  denominators, never scored as misses. Cost (tokens, tool calls, wall time
  from `GET /api/tasks/{id}/outcome`) is reported per arm, so a gain bought
  with a bigger spend is visible.
- **A judge failure stops the suite**: an untrusted grader invalidates every
  judged result.

Output: `benchmark-results/suite-<suite>/<run-id>/` with `baseline.json` and
`candidate.json` (stamp + per-run records, written after every run), the
per-run directories under `runs/`, `comparison.json` and `report.md`.

## Benchmark gate (E3, RSI Loop C)

The gate turns suite output into an adoptable proposal without letting the loop
adopt anything. `rsi_submit.py` talks to `/api/rsi/*` (same `ARTEX_TOKEN` /
`ARTEX_JWT_KEY` handling as the runner) and has no adopt command: adoption needs
the owner's password (step-up), through the API or the UI, and is not scriptable.
The server needs `ARTEX_RSI_LOOP` on (otherwise proposals answer 409).

```bash
cd scripts/benchmark
# 1. propose: a prompt body (the arm's role spec must be source "file") ...
python3 rsi_submit.py propose --arm arms/planner-v2.json --round 1 --target planner \
    --rationale "planner stops after the first finding on multi-step targets"
#    ... or a lesson set (arm: {"name": "lessons-v1", "lessons": [12, 15]}, prompts all "current")
python3 rsi_submit.py propose --arm arms/lessons-v1.json --round 1 --lessons --rationale "..."

# 2. run the held-out suite for that same arm file against arms/baseline.json
python3 eval_suite.py run --suite suites/heldout-v1.json --baseline arms/baseline.json \
    --candidate arms/planner-v2.json --subject-model deepseek-chat --replicates 3 --seed 7

# 3. attach the evidence; exits 1 unless the gate's verdict passed
python3 rsi_submit.py evidence <proposal_id> benchmark-results/suite-heldout-v1/<run-id>
python3 rsi_submit.py status
```

4. **Owner adoption**: the owner reviews `report.md` and the verdict, then adopts
   the proposal with their password. A prompt body becomes a new `agent_prompts`
   version; a lesson set is adopted through the lesson library's owner actions.
5. **Rollback**: a one-step pointer change back to the previous `agent_prompts`
   version (prompt bodies) or the previous lesson status (lessons); neither
   needs a deploy.

Binding: the server ties evidence to the proposal. For a prompt body,
`candidate.stamp.prompt_sha[target]` must equal the sha256 (first 16 hex) of the
proposed body and every other role must match the baseline. For lessons,
`candidate.stamp.lesson_ids` must equal the proposal's sorted ids and
`baseline.stamp.lesson_ids` must be `[]`. A proposal that does not match the
files it was run from is rejected, so edit the arm file, not the proposal.

Lesson arms. An arm may carry `"lessons": [ids]` (positive integers, deduplicated
and sorted on load). Run the instance with `ARTEX_RSI_LESSONS=bench`: every new
task's planner and worker then get exactly the pinned lessons, none when the list
is empty. The suite pins each arm's set through `/api/rsi/bench-lessons`, reads it
back (a mismatch aborts), stamps `lesson_ids` and `lessons_block_sha` per arm and
restores the starting pin when it ends. The baseline arm must not carry lessons
(it is the non-evolving side of the paired comparison). These two stamps differ
between arms by design, so they are not among the invariants `compare` enforces,
but each must stay constant across one arm's runs. Suites with no lesson arm never
call the endpoint.

What is outside the gate: skills and tool descriptions are adopted only through a
git PR with review, never through this path. The immutable surfaces of `RSI.md`
(gates, scope and authorization checks, injection defenses, tool allowlists and
the code-owned prompt tails, including the lesson block itself) are outside every
arm: a suite can vary only the editable prompt body and the pinned lesson ids.

## What this does not measure

Single-run results are noisy (the fork of VulnHunter measured a 14x spread in
findings across runs of the same commit); replicate before believing a delta.
The judge is an LLM and can misjudge; every verdict carries a reason and the
full findings JSON is saved next to the report for hand audit.
