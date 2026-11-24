#!/usr/bin/env python3
"""Submit a Loop C proposal and its suite evidence to the ARTEX benchmark gate (E3).

  python3 rsi_submit.py propose --arm arms/planner-v2.json --round 1 --target planner --rationale "..."
  python3 rsi_submit.py propose --arm arms/lessons-v1.json --round 1 --lessons --rationale "..."
  python3 rsi_submit.py evidence <proposal_id> benchmark-results/suite-<suite>/<run-id>
  python3 rsi_submit.py status

Needs ARTEX_TOKEN or ARTEX_JWT_KEY (and ARTEX_BASE or --artex-url). Adoption is deliberately
not here: it needs the owner's password (step-up), through the API or UI. See README.md,
"Benchmark gate (E3, RSI Loop C)".
"""
from __future__ import annotations

import argparse
import json
import os
import pathlib
import sys

import arms
import run_bench


def make_api(url: str) -> run_bench.Api:
    """Token handling is run_bench.prepare's (ARTEX_TOKEN, else a JWT minted from ARTEX_JWT_KEY)."""
    probe = run_bench.build_parser().parse_args([
        "--corpus-type", "drost", "--drost-repo", "-", "--drost-id", "-", "--subject-model", "rsi-submit",
        "--judge-url", "", "--judge-model", "", "--artex-url", url])  # no judge needed to submit
    return run_bench.Api(url, run_bench.prepare(probe))


def proposal_body(arm: dict, round_no: int, rationale: str, target: str | None) -> dict:
    """The POST /api/rsi/proposals body for an arm: one prompt body, or its lesson set."""
    body = {"round": round_no, "proposed_from": arm["proposed_from"], "rationale": rationale}
    if target:
        spec = arm["prompts"][target]
        if spec["source"] != "file":
            raise SystemExit(f"--target {target}: the arm's {target} spec must be source 'file' (got {spec['source']!r})")
        if arm["lessons"]:
            raise SystemExit("a prompt_body proposal cannot also pin lessons; submit them as separate arms")
        return {**body, "surface": "prompt_body", "target": target, "body": spec["text"]}
    if not arm["lessons"]:
        raise SystemExit("--lessons needs an arm with a non-empty 'lessons' list")
    if any(s["source"] != "current" for s in arm["prompts"].values()):
        raise SystemExit("a lesson proposal varies only lessons: every prompt role must be source 'current'")
    return {**body, "surface": "lesson", "lesson_ids": arm["lessons"]}


def evidence_body(run_dir: str) -> dict:
    d = pathlib.Path(run_dir)
    try:
        return {"baseline": json.loads((d / "baseline.json").read_text()),
                "candidate": json.loads((d / "candidate.json").read_text())}
    except (OSError, ValueError) as exc:
        raise SystemExit(f"{run_dir}: need readable baseline.json and candidate.json from an eval_suite run ({exc})")


def cmd_propose(api, opts) -> int:
    arm = arms.load_arm(opts.arm)
    res = api.post("/api/rsi/proposals", proposal_body(arm, opts.round, opts.rationale, opts.target))
    print(json.dumps(res, indent=2))
    return 0


def cmd_evidence(api, opts) -> int:
    ev = api.post(f"/api/rsi/proposals/{opts.proposal_id}/evidence", evidence_body(opts.run_dir)).get("evidence") or {}
    print(json.dumps(ev, indent=2))
    return 0 if ev.get("passed") is True else 1


def cmd_status(api, opts) -> int:
    print(json.dumps(api.get("/api/rsi/status"), indent=2))
    return 0


def build_parser() -> argparse.ArgumentParser:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--artex-url", default=os.environ.get("ARTEX_BASE", "http://localhost:8787"))
    sub = ap.add_subparsers(dest="cmd", required=True)
    p = sub.add_parser("propose", help="register a candidate arm as a proposal")
    p.add_argument("--arm", required=True, help="candidate arm file")
    p.add_argument("--round", type=int, required=True)
    p.add_argument("--rationale", required=True)
    what = p.add_mutually_exclusive_group(required=True)
    what.add_argument("--target", choices=arms.ROLES, help="propose this role's prompt body (a source: file spec)")
    what.add_argument("--lessons", action="store_true", help="propose the arm's lesson set")
    p.set_defaults(fn=cmd_propose)
    e = sub.add_parser("evidence", help="attach an eval_suite run to a proposal; exits 1 unless it passed")
    e.add_argument("proposal_id", type=int)
    e.add_argument("run_dir", help="eval_suite output directory holding baseline.json and candidate.json")
    e.set_defaults(fn=cmd_evidence)
    s = sub.add_parser("status", help="show the gate's state")
    s.set_defaults(fn=cmd_status)
    return ap


def main(argv: list[str] | None = None, api=None) -> int:
    opts = build_parser().parse_args(argv)
    return opts.fn(api or make_api(opts.artex_url), opts)


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
