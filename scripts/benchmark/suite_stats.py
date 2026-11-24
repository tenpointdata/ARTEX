#!/usr/bin/env python3
"""Per-gate aggregation and baseline/candidate comparison for evaluation suites.

Pure functions over run records; no server, model or Docker calls. A run record
is {"target", "arm", "replicate", "payload", "error"} where payload is the
results.json dict written by run_bench.run() (None when the run failed).

Rules carried over from the L3 runner:
- every rate is reported as passes / denominator, never as a bare percentage;
- a failed or unjudged run is excluded from the gate denominators and counted
  separately, never scored as a miss;
- gates are counted over ground-truth entries (so arms with different
  detection counts stay comparable); "over detected" is reported alongside.
"""
from __future__ import annotations

import math

GATES = ("detected", "component", "class", "keywords", "proof")
FLAG_GATE = "flag_captured"

# Stamps that must be identical between arms for a comparison to mean anything:
# same model, budget, judge, method and ground truth (RSI.md Loop C contract).
INVARIANT_STAMPS = ("method_version", "subject_model", "llm_profiles", "judge_model",
                    "task_timeout_seconds", "seed", "suite", "suite_version", "replicates",
                    "ground_truth_versions")


def wilson(passes: int, n: int, z: float = 1.96) -> tuple[float, float]:
    if n == 0:
        return (0.0, 0.0)
    p = passes / n
    denom = 1 + z * z / n
    center = (p + z * z / (2 * n)) / denom
    half = z * math.sqrt(p * (1 - p) / n + z * z / (4 * n * n)) / denom
    return (max(0.0, center - half), min(1.0, center + half))


def rate(passes: int, n: int) -> dict:
    lo, hi = wilson(passes, n)
    return {"passes": passes, "n": n, "rate": round(passes / n, 4) if n else None,
            "wilson95": [round(lo, 4), round(hi, 4)]}


def newcombe_diff(a: dict, b: dict, z: float = 1.96) -> list[float] | None:
    """Newcombe hybrid-score 95% interval for rate(a) - rate(b) (independent samples).

    Gate observations within one run are not independent, so treat it as a
    guide; the paired table is the sharper read."""
    if not a["n"] or not b["n"]:
        return None
    p1, p2 = a["passes"] / a["n"], b["passes"] / b["n"]
    l1, u1 = wilson(a["passes"], a["n"], z)
    l2, u2 = wilson(b["passes"], b["n"], z)
    d = p1 - p2
    lo = d - math.sqrt((p1 - l1) ** 2 + (u2 - p2) ** 2)
    hi = d + math.sqrt((u1 - p1) ** 2 + (p2 - l2) ** 2)
    return [round(max(-1.0, lo), 4), round(min(1.0, hi), 4)]


def gate_observations(record: dict) -> dict[str, bool]:
    """{"<gt_id>/<gate>": passed} for one judged run; {} when unjudged."""
    payload = record.get("payload") or {}
    obs: dict[str, bool] = {}
    for v in payload.get("verdicts") or []:
        g = v.get("gates") or {}
        obs[f"{v['gt_id']}/detected"] = v.get("detected") is True
        for k in GATES[1:]:
            obs[f"{v['gt_id']}/{k}"] = g.get(k) is True
    flag = (payload.get("result") or {}).get(FLAG_GATE)
    if flag is not None:
        obs[f"flag/{FLAG_GATE}"] = flag is True
    return obs


def aggregate(records: list[dict]) -> dict:
    """Per-gate pass counts with denominators for one arm's records."""
    counts = {g: [0, 0] for g in (*GATES, FLAG_GATE)}
    per_target: dict[str, dict] = {}
    statuses: dict[str, int] = {}
    cost = {"input_tokens": 0, "output_tokens": 0, "tool_calls": 0, "wall_seconds": 0, "runs_with_outcome": 0}
    failed = unjudged = 0
    for r in records:
        tgt = per_target.setdefault(r["target"], {g: [0, 0] for g in (*GATES, FLAG_GATE)})
        payload = r.get("payload")
        if payload is None:
            failed += 1
            continue
        status = str(payload.get("task_status") or "?")
        statuses[status] = statuses.get(status, 0) + 1
        if payload.get("verdicts") is None:
            unjudged += 1
        for key, passed in gate_observations(r).items():
            gate = key.split("/", 1)[1]
            for bucket in (counts, tgt):
                bucket[gate][1] += 1
                bucket[gate][0] += 1 if passed else 0
        outcome = payload.get("task_outcome") or {}
        if outcome:
            cost["runs_with_outcome"] += 1
            tokens = outcome.get("tokens") or {}
            cost["input_tokens"] += int(tokens.get("input_tokens") or 0)
            cost["output_tokens"] += int(tokens.get("output_tokens") or 0)
            cost["tool_calls"] += int(outcome.get("tool_calls") or 0)
            cost["wall_seconds"] += int(outcome.get("wall_seconds") or 0)
    return {
        "runs_attempted": len(records),
        "runs_failed": failed,
        "runs_unjudged": unjudged,
        "task_statuses": statuses,
        "gates": {g: rate(*counts[g]) for g in counts},
        "gates_over_detected": {g: rate(counts[g][0], counts["detected"][0]) for g in GATES[1:]},
        "per_target": {t: {g: rate(*c[g]) for g in c} for t, c in sorted(per_target.items())},
        "cost": cost,
    }


def paired(baseline: list[dict], candidate: list[dict]) -> dict:
    """Discordant-pair table per gate over observations both arms produced for
    the same (target, replicate, gt entry, gate)."""
    def index(records):
        out = {}
        for r in records:
            for key, passed in gate_observations(r).items():
                out[(r["target"], r["replicate"], key)] = passed
        return out
    b, c = index(baseline), index(candidate)
    table: dict[str, dict[str, int]] = {}
    for key in sorted(b.keys() & c.keys()):
        gate = key[2].split("/", 1)[1]
        row = table.setdefault(gate, {"both": 0, "neither": 0, "gained": 0, "lost": 0})
        bp, cp = b[key], c[key]
        row["both" if bp and cp else "neither" if not (bp or cp) else "gained" if cp else "lost"] += 1
    return table


def stamp_mismatches(baseline_stamp: dict, candidate_stamp: dict) -> list[str]:
    return [f"{k}: baseline={baseline_stamp.get(k)!r} candidate={candidate_stamp.get(k)!r}"
            for k in INVARIANT_STAMPS if baseline_stamp.get(k) != candidate_stamp.get(k)]


def compare(baseline: dict, candidate: dict, allow_mismatch: bool = False) -> dict:
    """Compare two arm results ({"stamp", "records"}); refuses mismatched invariants."""
    mismatches = stamp_mismatches(baseline["stamp"], candidate["stamp"])
    if mismatches and not allow_mismatch:
        raise SystemExit("arms are not comparable (same model, seed, budget and judge required):\n  "
                         + "\n  ".join(mismatches))
    agg_b, agg_c = aggregate(baseline["records"]), aggregate(candidate["records"])
    deltas = {}
    for g in agg_b["gates"]:
        rb, rc = agg_b["gates"][g], agg_c["gates"][g]
        deltas[g] = {
            "delta": round(rc["rate"] - rb["rate"], 4) if rb["n"] and rc["n"] else None,
            "newcombe95": newcombe_diff(rc, rb),
        }
    return {
        "baseline": {"arm": baseline["stamp"].get("arm"), **agg_b},
        "candidate": {"arm": candidate["stamp"].get("arm"), **agg_c},
        "deltas": deltas,
        "paired": paired(baseline["records"], candidate["records"]),
        "stamp_mismatches": mismatches,
    }


def _fmt(r: dict) -> str:
    if not r["n"]:
        return "n/a (0)"
    return f"{r['passes']}/{r['n']} ({r['rate']:.2f}, {r['wilson95'][0]:.2f}–{r['wilson95'][1]:.2f})"


def render_markdown(cmp: dict, stamp: dict) -> str:
    b, c = cmp["baseline"], cmp["candidate"]
    lines = [
        f"# Evaluation suite `{stamp.get('suite')}` ({stamp.get('suite_version')}): "
        f"`{b['arm']}` vs `{c['arm']}`",
        "",
        f"- method `{stamp.get('method_version')}`, subject `{stamp.get('subject_model')}`, "
        f"llm chain `{stamp.get('llm_profiles') or 'default'}`, judge `{stamp.get('judge_model') or 'none (flag-only)'}`",
        f"- budget {stamp.get('task_timeout_seconds')}s per task, seed {stamp.get('seed')}, "
        f"replicates {stamp.get('replicates')}",
        f"- runs (baseline/candidate): attempted {b['runs_attempted']}/{c['runs_attempted']}, "
        f"failed {b['runs_failed']}/{c['runs_failed']}, unjudged {b['runs_unjudged']}/{c['runs_unjudged']}",
    ]
    if cmp["stamp_mismatches"]:
        lines.append("- **NOT COMPARABLE, forced with --allow-mismatch:** " + "; ".join(cmp["stamp_mismatches"]))
    lines += ["", "## Per-gate pass rates (passes/denominator, rate, Wilson 95%)", "",
              "| gate | baseline | candidate | delta | Newcombe 95% |", "|---|---|---|---|---|"]
    for g, d in cmp["deltas"].items():
        lines.append(f"| {g} | {_fmt(b['gates'][g])} | {_fmt(c['gates'][g])} | "
                     f"{d['delta'] if d['delta'] is not None else 'n/a'} | {d['newcombe95'] or 'n/a'} |")
    lines += ["", "## Paired observations (same target, replicate, ground-truth entry)", "",
              "| gate | both pass | neither | candidate gained | candidate lost |", "|---|---|---|---|---|"]
    for g, row in cmp["paired"].items():
        lines.append(f"| {g} | {row['both']} | {row['neither']} | {row['gained']} | {row['lost']} |")
    lines += ["", "## Per target", "", "| target | gate | baseline | candidate |", "|---|---|---|---|"]
    for t in sorted(b["per_target"].keys() | c["per_target"].keys()):
        for g in (*GATES, FLAG_GATE):
            rb = b["per_target"].get(t, {}).get(g, {"n": 0})
            rc = c["per_target"].get(t, {}).get(g, {"n": 0})
            if rb["n"] or rc["n"]:
                lines.append(f"| {t} | {g} | {_fmt(rb)} | {_fmt(rc)} |")
    cb, cc = b["cost"], c["cost"]
    lines += ["", "## Cost (sums over runs with an outcome record)", "",
              "| arm | runs | input tokens | output tokens | tool calls | wall s |", "|---|---|---|---|---|---|",
              f"| baseline | {cb['runs_with_outcome']} | {cb['input_tokens']} | {cb['output_tokens']} | {cb['tool_calls']} | {cb['wall_seconds']} |",
              f"| candidate | {cc['runs_with_outcome']} | {cc['input_tokens']} | {cc['output_tokens']} | {cc['tool_calls']} | {cc['wall_seconds']} |",
              "", "Gate observations within one run are correlated: treat intervals as a guide and replicate "
              "before claiming a difference. A rate without its denominator must not be quoted.", ""]
    return "\n".join(lines)
