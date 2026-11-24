#!/usr/bin/env python3
"""Evaluation-suite contract tests; no model, server or Docker calls.

Run: python3 -m unittest discover -s scripts/benchmark -p 'test_*.py' -v
"""
from __future__ import annotations

import json
import pathlib
import tempfile
import unittest
from unittest import mock

import arms
import eval_suite
import run_bench
import suite_stats

HERE = pathlib.Path(__file__).parent


class FakeArtex:
    """In-memory /api/agents prompt store with versioned saves."""

    def __init__(self, start=None):
        self.prompts = dict(start or {"goals": "G0", "planner": "P0", "worker": "W0"})
        self.saves: list[tuple[str, str, str]] = []

    def get(self, path):
        if path == "/api/tasks":
            return {"tasks": []}
        return {"prompt": self.prompts[path.split("/")[3]]}

    def post(self, path, body):
        role = path.split("/")[3]
        self.prompts[role] = f"BUILTIN-{role}"
        self.saves.append((role, "reset", ""))
        return {"version": 1}

    def req(self, method, path, body):
        role = path.split("/")[3]
        self.prompts[role] = body["template"]
        self.saves.append((role, body["note"], body["template"]))
        return {"version": 1}


def write_arm(tmp: pathlib.Path, name: str, prompts: dict, proposed=None) -> str:
    p = tmp / f"{name}.json"
    p.write_text(json.dumps({"name": name, "prompts": prompts, "proposed_from": proposed or []}))
    return str(p)


def payload(gates_by_gt: dict[str, bool], flag=None, status="done", tokens=100):
    verdicts = [{"gt_id": g, "detected": ok, "matched_finding_id": 1 if ok else None,
                 "gates": {k: ok for k in ("component", "class", "keywords", "proof")}, "reason": "x"}
                for g, ok in gates_by_gt.items()]
    result = {} if flag is None else {"flag_captured": flag}
    return {"task_status": status, "verdicts": verdicts, "result": result,
            "task_outcome": {"tokens": {"input_tokens": tokens, "output_tokens": 1}, "tool_calls": 2, "wall_seconds": 3}}


class ArmTests(unittest.TestCase):
    def setUp(self):
        self.tmp = pathlib.Path(tempfile.mkdtemp())
        (self.tmp / "p.tmpl").write_text("PLANNER-CANDIDATE {{.Goal}}")

    def test_shipped_arms_and_suites_load(self):
        for f in (HERE / "arms").glob("*.json"):
            arms.load_arm(str(f))
        for f in (HERE / "suites").glob("*.json"):
            eval_suite.load_suite(str(f))

    def test_held_out_and_dev_splits_are_disjoint(self):
        held = {t["id"] for t in eval_suite.load_suite(str(HERE / "suites/heldout-v1.json"))["targets"]}
        dev = {t["id"] for t in eval_suite.load_suite(str(HERE / "suites/dev-v1.json"))["targets"]}
        self.assertFalse(held & dev)

    def test_shipped_ground_truth_is_versioned(self):
        v = eval_suite.ground_truth_versions(
            {"targets": [{"id": "dvga", "type": "image", "ground_truth": "dvga"}]}, "")
        self.assertRegex(v["dvga"], r"^dvga@[0-9a-f]{12}$")

    def test_roles_outside_planner_worker_are_refused(self):
        path = write_arm(self.tmp, "bad", {"retester": {"source": "builtin"}})
        with self.assertRaisesRegex(SystemExit, "outside the evaluable set"):
            arms.load_arm(path)

    def test_arm_proposed_from_a_suite_target_is_refused(self):
        arm = arms.load_arm(write_arm(self.tmp, "c", {}, proposed=["DBEN-v1-005"]))
        with self.assertRaisesRegex(SystemExit, "not derived from"):
            arms.check_held_out(arm, ["juice-shop", "DBEN-v1-005"])

    def test_apply_and_restore_round_trip(self):
        api = FakeArtex()
        cand = arms.load_arm(write_arm(self.tmp, "cand", {"planner": {"source": "file", "path": "p.tmpl"}}))
        base = arms.load_arm(write_arm(self.tmp, "base", {}))
        sw = arms.PromptSwitch(api)
        sw.snapshot()
        stamp = sw.apply(cand)
        self.assertEqual(api.prompts["planner"], "PLANNER-CANDIDATE {{.Goal}}")
        self.assertEqual(stamp["planner"], arms.sha("PLANNER-CANDIDATE {{.Goal}}"))
        self.assertEqual(api.prompts["worker"], "W0")
        sw.apply(base)
        self.assertEqual(api.prompts["planner"], "P0")
        saves_before = len(api.saves)
        sw.apply(base)  # already active: no new prompt version
        self.assertEqual(len(api.saves), saves_before)
        sw.apply(cand)
        sw.restore()
        self.assertEqual(api.prompts, {"goals": "G0", "planner": "P0", "worker": "W0"})

    def test_builtin_source_and_empty_start(self):
        api = FakeArtex({"goals": "", "planner": "P0", "worker": "W0"})
        arm = arms.load_arm(write_arm(self.tmp, "b", {"planner": {"source": "builtin"}}))
        sw = arms.PromptSwitch(api)
        sw.snapshot()
        sw.apply(arm)
        self.assertEqual(api.prompts["planner"], "BUILTIN-planner")
        self.assertEqual(api.prompts["goals"], "BUILTIN-goals")  # no stored body ran the default
        sw.restore()
        self.assertEqual(api.prompts["planner"], "P0")


class ScheduleTests(unittest.TestCase):
    def test_schedule_is_seeded_complete_and_pairs_arms(self):
        plan = eval_suite.schedule(["a", "b", "c"], ["base", "cand"], 2, seed=7)
        self.assertEqual(plan, eval_suite.schedule(["a", "b", "c"], ["base", "cand"], 2, seed=7))
        self.assertEqual(len(plan), 12)
        for i in range(0, 12, 2):  # both arms run back to back on the same target
            self.assertEqual(plan[i][:2], plan[i + 1][:2])
            self.assertEqual({plan[i][2], plan[i + 1][2]}, {"base", "cand"})
        firsts = {plan[i][2] for i in range(0, 12, 2)}
        self.assertEqual(firsts, {"base", "cand"})  # neither arm always goes first at this seed

    def test_busy_instance_is_refused(self):
        api = mock.Mock()
        api.get.return_value = {"tasks": [{"id": 4, "status": "running"}, {"id": 5, "status": "done"}]}
        with self.assertRaisesRegex(SystemExit, "#4 \\(running\\)"):
            eval_suite.assert_idle(api)


class StatsTests(unittest.TestCase):
    def rec(self, arm, target, rep, p):
        return {"arm": arm, "target": target, "replicate": rep, "payload": p, "error": None if p else "boom"}

    def test_per_gate_rates_carry_denominators_and_exclude_failed_runs(self):
        records = [self.rec("b", "t1", 0, payload({"GT-1": True, "GT-2": False})),
                   self.rec("b", "t2", 0, payload({}, flag=True)),
                   self.rec("b", "t1", 1, None)]
        agg = suite_stats.aggregate(records)
        self.assertEqual(agg["gates"]["detected"], {"passes": 1, "n": 2, "rate": 0.5, "wilson95": [0.0945, 0.9055]})
        self.assertEqual(agg["gates"]["flag_captured"]["n"], 1)
        self.assertEqual((agg["runs_attempted"], agg["runs_failed"]), (3, 1))
        self.assertEqual(agg["cost"]["input_tokens"], 200)
        self.assertEqual(agg["gates_over_detected"]["proof"]["n"], 1)

    def test_unjudged_flag_only_run_has_no_gate_denominator(self):
        p = {"task_status": "done", "verdicts": None, "result": {"flag_captured": False}}
        agg = suite_stats.aggregate([self.rec("b", "d", 0, p)])
        self.assertEqual(agg["gates"]["proof"]["n"], 0)
        self.assertEqual(agg["gates"]["flag_captured"], {"passes": 0, "n": 1, "rate": 0.0, "wilson95": [0.0, 0.7935]})
        self.assertEqual(agg["runs_unjudged"], 1)

    def test_compare_refuses_different_invariants(self):
        stamp = {k: 1 for k in suite_stats.INVARIANT_STAMPS}
        b = {"stamp": {**stamp, "arm": "b"}, "records": []}
        c = {"stamp": {**stamp, "arm": "c", "judge_model": "other"}, "records": []}
        with self.assertRaisesRegex(SystemExit, "judge_model"):
            suite_stats.compare(b, c)
        self.assertEqual(len(suite_stats.compare(b, c, allow_mismatch=True)["stamp_mismatches"]), 1)

    def test_compare_paired_table_and_delta(self):
        stamp = {k: 1 for k in suite_stats.INVARIANT_STAMPS}
        b = {"stamp": {**stamp, "arm": "b"}, "records": [self.rec("b", "t", 0, payload({"GT-1": True, "GT-2": False}))]}
        c = {"stamp": {**stamp, "arm": "c"}, "records": [self.rec("c", "t", 0, payload({"GT-1": True, "GT-2": True}))]}
        cmp = suite_stats.compare(b, c)
        self.assertEqual(cmp["paired"]["detected"], {"both": 1, "neither": 0, "gained": 1, "lost": 0})
        self.assertEqual(cmp["deltas"]["detected"]["delta"], 0.5)
        lo, hi = cmp["deltas"]["detected"]["newcombe95"]
        self.assertLess(lo, 0.5)
        self.assertGreater(hi, 0.5)
        md = suite_stats.render_markdown(cmp, b["stamp"])
        self.assertIn("| detected | 1/2 (0.50", md)
        self.assertIn("| detected | 1 | 0 | 1 | 0 |", md)


class RunTests(unittest.TestCase):
    """The run command end to end with the runner and server faked."""

    def test_run_swaps_arms_records_and_restores_on_failure(self):
        tmp = pathlib.Path(tempfile.mkdtemp())
        (tmp / "p.tmpl").write_text("CAND")
        suite = tmp / "suite.json"
        suite.write_text(json.dumps({"suite": "t", "version": "v1", "split": "held-out", "targets": [
            {"id": "dvga", "type": "image", "ground_truth": "dvga", "image": "x", "port": 1, "container_port": 2}]}))
        base = write_arm(tmp, "base", {})
        cand = write_arm(tmp, "cand", {"planner": {"source": "file", "path": "p.tmpl"}})
        api = FakeArtex()
        seen = []

        def fake_run(args, token):
            seen.append((args.run_id, api.prompts["planner"], args.task_timeout, args.gt_version))
            return payload({"GT-001": api.prompts["planner"] == "CAND"})

        argv = ["run", "--suite", str(suite), "--baseline", base, "--candidate", cand, "--subject-model", "m",
                "--replicates", "2", "--seed", "3", "--task-timeout", "60", "--out-root", str(tmp / "out"), "--run-id", "R"]
        with mock.patch.object(run_bench, "prepare", return_value="tok"), \
                mock.patch.object(run_bench, "Api", return_value=api), \
                mock.patch.object(run_bench, "run", side_effect=fake_run):
            eval_suite.main(argv)
        self.assertEqual(len(seen), 4)
        self.assertEqual({s[2] for s in seen}, {60})  # one budget for both arms
        self.assertEqual({s[1] for s in seen if "-cand-" in s[0]}, {"CAND"})
        self.assertEqual({s[1] for s in seen if "-base-" in s[0]}, {"P0"})
        self.assertEqual(api.prompts["planner"], "P0")  # restored
        cmp = json.loads((tmp / "out/suite-t/R/comparison.json").read_text())
        self.assertEqual(cmp["baseline"]["gates"]["detected"]["passes"], 0)
        self.assertEqual(cmp["candidate"]["gates"]["detected"], {"passes": 2, "n": 2, "rate": 1.0, "wilson95": [0.3424, 1.0]})
        self.assertTrue((tmp / "out/suite-t/R/report.md").is_file())

        api.prompts["planner"] = "P0"
        with mock.patch.object(run_bench, "prepare", return_value="tok"), \
                mock.patch.object(run_bench, "Api", return_value=api), \
                mock.patch.object(run_bench, "run", side_effect=SystemExit("judge battery FAILED on 'positive'")):
            with self.assertRaisesRegex(SystemExit, "judge battery"):
                eval_suite.main(argv[:-1] + ["R2"])
        self.assertEqual(api.prompts["planner"], "P0")  # restored even when the suite aborts

    def test_dev_split_needs_explicit_opt_in(self):
        argv = ["run", "--suite", str(HERE / "suites/dev-v1.json"), "--baseline", str(HERE / "arms/baseline.json"),
                "--candidate", str(HERE / "arms/builtin.json"), "--subject-model", "m"]
        with self.assertRaisesRegex(SystemExit, "held-out suite"):
            eval_suite.main(argv)


if __name__ == "__main__":
    unittest.main()
