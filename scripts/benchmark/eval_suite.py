#!/usr/bin/env python3
"""Planner/worker evaluation suite: baseline vs candidate prompt arms on a
held-out set of deliberately vulnerable targets, scored per gate with
denominators.

  python3 eval_suite.py run --suite suites/heldout-v1.json \
      --baseline arms/baseline.json --candidate arms/my-candidate.json \
      --subject-model deepseek-chat --replicates 3 --seed 7
  python3 eval_suite.py compare <dir>/baseline.json <dir>/candidate.json

Both arms run under one stamp: the same model chain, seed, per-task budget,
judge and ground truth. Runs are strictly sequential, because the server reads
the active prompt live and a swap would leak into a concurrent task, and the
schedule (target order, arm order within a pair) is drawn from the seed so
neither arm always runs first. The starting prompt bodies are restored when the
suite ends, also on failure. See README.md, "Evaluation suite".
"""
from __future__ import annotations

import argparse
import hashlib
import json
import pathlib
import random
import re
import sys
import time

import arms
import drost
import run_bench
import suite_stats

HERE = pathlib.Path(__file__).parent
# Task states in which a task may still render prompts. Anything else (terminal,
# paused) is safe to leave while arms swap.
IDLE_STATUSES = run_bench.TERMINAL_STATUSES | {"paused"}


def slug(text: str) -> str:
    return re.sub(r"[^a-z0-9-]+", "-", text.lower()).strip("-")[:40]


def load_suite(path: str) -> dict:
    suite = json.loads(pathlib.Path(path).read_text())
    for key in ("suite", "version", "split", "targets"):
        if key not in suite:
            raise SystemExit(f"{path}: suite needs {key!r}")
    ids = [t.get("id") for t in suite["targets"]]
    if len(set(ids)) != len(ids) or not all(isinstance(i, str) and i for i in ids):
        raise SystemExit(f"{path}: target ids must be unique non-empty strings")
    for t in suite["targets"]:
        if t.get("type") == "image":
            for key in ("ground_truth", "image", "port", "container_port"):
                if key not in t:
                    raise SystemExit(f"{path}: image target {t['id']} needs {key!r}")
            env = t.get("env", {})
            if not isinstance(env, dict) or not all(isinstance(k, str) and re.fullmatch(r"[A-Za-z_][A-Za-z0-9_]*", k) and isinstance(v, str) for k, v in env.items()):
                raise SystemExit(f"{path}: image target {t['id']} env must map environment names to strings")
        elif t.get("type") == "drost":
            drost.validate_challenge(t["id"])
        else:
            raise SystemExit(f"{path}: target {t['id']} has unknown type {t.get('type')!r}")
    return suite


def ground_truth_versions(suite: dict, drost_repo: str) -> dict[str, str]:
    """Content-addressed ground-truth version per target: an edited file is a new version."""
    out = {}
    for t in suite["targets"]:
        if t["type"] == "image":
            raw = (HERE / "ground_truth" / f"{t['ground_truth']}.json").read_bytes()
            out[t["id"]] = f"{t['ground_truth']}@{hashlib.sha256(raw).hexdigest()[:12]}"
        else:
            out[t["id"]] = drost.derive_ground_truth(drost_repo, t["id"])["version"]
    return out


def run_argv(target: dict, opts, run_id: str, out_root: pathlib.Path, gt_version: str) -> list[str]:
    """The run_bench argv for one (target, arm) run; every arm gets the same budget flags."""
    argv = ["--run-id", run_id, "--subject-model", opts.subject_model, "--task-timeout", str(opts.task_timeout),
            "--target-host", opts.target_host, "--out-root", str(out_root), "--artex-url", opts.artex_url]
    if opts.llm_profiles:
        argv += ["--llm-profiles", opts.llm_profiles]
    if opts.allow_self_judge:
        argv.append("--allow-self-judge")
    if target["type"] == "drost":
        return argv + ["--corpus-type", "drost", "--drost-repo", opts.drost_repo, "--drost-id", target["id"]]
    argv += ["--corpus", target["ground_truth"], "--image", target["image"], "--port", str(target["port"]),
             "--container-port", str(target["container_port"]), "--gt-version", gt_version]
    for key, value in sorted(target.get("env", {}).items()):
        argv += ["--image-env", f"{key}={value}"]
    return argv


def schedule(target_ids: list[str], arm_names: list[str], replicates: int, seed: int) -> list[tuple[int, str, str]]:
    """(replicate, target, arm) in run order: targets shuffled per replicate, arms
    shuffled per pair, all from the seed, so the order is reproducible."""
    rng = random.Random(seed)
    out = []
    for rep in range(replicates):
        order = list(target_ids)
        rng.shuffle(order)
        for tid in order:
            pair = list(arm_names)
            rng.shuffle(pair)
            out += [(rep, tid, a) for a in pair]
    return out


def assert_idle(api) -> None:
    res = api.get("/api/tasks")
    busy = [f"#{t.get('id')} ({t.get('status')})" for t in res.get("tasks") or []
            if str(t.get("status", "")).lower() not in IDLE_STATUSES]
    if busy:
        raise SystemExit("instance has tasks that may render prompts during arm swaps: " + ", ".join(busy)
                         + "; run the suite on a dedicated bench instance (or pass --allow-busy knowingly)")


def llm_sampling(api, llm_profiles: str) -> list[dict]:
    """The sampling pin (seed, temperature) of each profile heading the task chain, in
    chain order; the active profile when no chain is given. None = not pinned, so the
    provider samples freely and only replicates control variance."""
    profiles = api.get("/api/llm/profiles").get("profiles") or []
    ids = [x.strip() for x in (llm_profiles or "").split(",") if x.strip()]
    chain = ([p for i in ids for p in profiles if str(p.get("id")) == i] if ids
             else [p for p in profiles if p.get("is_default")])
    return [{"id": str(p.get("id")), "seed": (p.get("sampling") or {}).get("seed"),
             "temperature": (p.get("sampling") or {}).get("temperature")} for p in chain]


def stamp_applied(stamp: dict, name: str, applied: dict) -> None:
    """Record what the server reports for an arm; it must not change between that arm's runs.
    lesson_ids / lessons_block_sha differ between arms by design, so they are not suite invariants."""
    seen = stamp["prompt_sha"]
    for r, h in applied["prompt_sha"].items():
        if seen.setdefault(r, h) != h:
            raise SystemExit(f"arm {name}: {r} prompt changed between runs ({seen[r]} -> {h})")
    for key in ("lesson_ids", "lessons_block_sha"):
        if stamp.setdefault(key, applied[key]) != applied[key]:
            raise SystemExit(f"arm {name}: {key} changed between runs ({stamp[key]} -> {applied[key]})")


def write_arm(out_dir: pathlib.Path, stamp: dict, records: list[dict]) -> dict:
    data = {"stamp": stamp, "records": records}
    (out_dir / f"{stamp['role']}.json").write_text(json.dumps(data, indent=2))
    return data


def cmd_run(opts) -> None:
    suite = load_suite(opts.suite)
    target_ids = [t["id"] for t in suite["targets"]]
    if suite["split"] != "held-out" and not opts.allow_dev_split:
        raise SystemExit(f"suite split is {suite['split']!r}; adoption evidence must come from a held-out suite "
                         "(pass --allow-dev-split for exploratory runs)")
    if any(t["type"] == "drost" for t in suite["targets"]) and not opts.drost_repo:
        raise SystemExit("suite has drost targets: pass --drost-repo (or DROST_REPO)")
    base, cand = arms.load_arm(opts.baseline), arms.load_arm(opts.candidate)
    if base["name"] == cand["name"]:
        raise SystemExit("baseline and candidate arms need different names")
    if base["lessons"]:
        raise SystemExit("the baseline arm must not pin lessons: the baseline does not evolve "
                         "(RSI.md, paired evolving/non-evolving comparison)")
    for arm in (base, cand):
        arms.check_held_out(arm, target_ids)

    # Judge configuration and token come from the runner's own validation.
    # An all-drost suite may run flag-only; any image target makes the judge mandatory.
    probe_argv = ["--subject-model", opts.subject_model, "--artex-url", opts.artex_url,
                  *(["--allow-self-judge"] if opts.allow_self_judge else [])]
    if all(t["type"] == "drost" for t in suite["targets"]):
        probe_argv += ["--corpus-type", "drost", "--drost-repo", opts.drost_repo, "--drost-id", target_ids[0]]
    probe = run_bench.build_parser().parse_args(probe_argv)
    token = run_bench.prepare(probe)
    api = run_bench.Api(opts.artex_url, token)
    if not opts.allow_busy:
        assert_idle(api)

    suite_run = opts.run_id or time.strftime("%Y%m%d-%H%M%S")
    out_dir = pathlib.Path(opts.out_root) / f"suite-{suite['suite']}" / suite_run
    out_dir.mkdir(parents=True, exist_ok=True)
    common = {
        "method_version": run_bench.METHOD_VERSION, "subject_model": opts.subject_model,
        "llm_profiles": opts.llm_profiles, "llm_sampling": llm_sampling(api, opts.llm_profiles),
        "judge_model": probe.judge_model or None,
        "task_timeout_seconds": opts.task_timeout, "seed": opts.seed, "suite": suite["suite"],
        "suite_version": suite["version"], "split": suite["split"], "replicates": opts.replicates,
        "ground_truth_versions": ground_truth_versions(suite, opts.drost_repo), "suite_run": suite_run,
    }
    by_name = {base["name"]: ("baseline", base), cand["name"]: ("candidate", cand)}
    stamps = {name: {**common, "role": role, "arm": name, "arm_file": arm["file"],
                     "proposed_from": arm["proposed_from"], "prompt_sha": {}}
              for name, (role, arm) in by_name.items()}
    records: dict[str, list[dict]] = {name: [] for name in by_name}
    targets = {t["id"]: t for t in suite["targets"]}
    plan = schedule(target_ids, list(by_name), opts.replicates, opts.seed)
    switch = arms.PromptSwitch(api, lessons=bool(cand["lessons"]))
    switch.snapshot()
    try:
        for i, (rep, tid, name) in enumerate(plan, 1):
            role, arm = by_name[name]
            print(f"[suite] {i}/{len(plan)} replicate {rep} target {tid} arm {name} ({role})")
            applied = switch.apply(arm)
            stamp_applied(stamps[name], name, applied)
            run_id = f"{suite_run}-r{rep}-{slug(name)}-{slug(tid)}"
            args = run_bench.build_parser().parse_args(
                run_argv(targets[tid], opts, run_id, out_dir / "runs", common["ground_truth_versions"][tid]))
            record = {"target": tid, "arm": name, "replicate": rep, "run_id": run_id, "payload": None, "error": None}
            try:
                record["payload"] = run_bench.run(args, token)
            except SystemExit as exc:
                if "judge" in str(exc) or "grader" in str(exc):
                    raise  # an untrusted grader invalidates every judged result: stop the suite
                record["error"] = str(exc)[:600]
                print(f"[suite] run failed, excluded from the gate denominators: {record['error'][:200]}")
            records[name].append(record)
            write_arm(out_dir, stamps[name], records[name])  # incremental: a crash keeps finished runs
    finally:
        switch.restore()
        print("[suite] starting prompt bodies restored")
    if llm_sampling(api, opts.llm_profiles) != common["llm_sampling"]:
        raise SystemExit("an llm profile's sampling pin changed during the suite; the arms are not comparable")
    report(out_dir, {"stamp": stamps[base["name"]], "records": records[base["name"]]},
           {"stamp": stamps[cand["name"]], "records": records[cand["name"]]}, allow_mismatch=False)


def report(out_dir: pathlib.Path, baseline: dict, candidate: dict, allow_mismatch: bool) -> None:
    cmp = suite_stats.compare(baseline, candidate, allow_mismatch=allow_mismatch)
    (out_dir / "comparison.json").write_text(json.dumps(cmp, indent=2))
    md = suite_stats.render_markdown(cmp, baseline["stamp"])
    (out_dir / "report.md").write_text(md)
    print(md)
    print(f"[out] {out_dir / 'report.md'}")


def cmd_compare(opts) -> None:
    baseline = json.loads(pathlib.Path(opts.baseline_json).read_text())
    candidate = json.loads(pathlib.Path(opts.candidate_json).read_text())
    out_dir = pathlib.Path(opts.out or pathlib.Path(opts.candidate_json).parent)
    out_dir.mkdir(parents=True, exist_ok=True)
    report(out_dir, baseline, candidate, opts.allow_mismatch)


def build_parser() -> argparse.ArgumentParser:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    sub = ap.add_subparsers(dest="cmd", required=True)
    r = sub.add_parser("run", help="run baseline and candidate arms over a suite")
    r.add_argument("--suite", required=True)
    r.add_argument("--baseline", required=True, help="baseline arm file")
    r.add_argument("--candidate", required=True, help="candidate arm file")
    r.add_argument("--subject-model", required=True)
    r.add_argument("--llm-profiles", default="")
    r.add_argument("--replicates", type=int, default=1)
    r.add_argument("--seed", type=int, default=0, help="schedule seed (target and arm order); stamped")
    r.add_argument("--task-timeout", type=int, default=5400, help="per-task budget in seconds, identical for both arms")
    r.add_argument("--target-host", default="127.0.0.1")
    r.add_argument("--artex-url", default=run_bench.os.environ.get("ARTEX_BASE", "http://localhost:8787"))
    r.add_argument("--drost-repo", default=run_bench.os.environ.get("DROST_REPO", ""))
    r.add_argument("--run-id", default="")
    r.add_argument("--out-root", default="benchmark-results")
    r.add_argument("--allow-self-judge", action="store_true")
    r.add_argument("--allow-dev-split", action="store_true")
    r.add_argument("--allow-busy", action="store_true")
    c = sub.add_parser("compare", help="compare two saved arm results")
    c.add_argument("baseline_json")
    c.add_argument("candidate_json")
    c.add_argument("--out", default="")
    c.add_argument("--allow-mismatch", action="store_true", help="report a comparison whose invariants differ (marked)")
    return ap


def main(argv: list[str] | None = None) -> None:
    opts = build_parser().parse_args(argv)
    if opts.cmd == "run":
        if opts.replicates < 1:
            raise SystemExit("--replicates must be at least 1")
        cmd_run(opts)
    else:
        cmd_compare(opts)


if __name__ == "__main__":
    main(sys.argv[1:])
