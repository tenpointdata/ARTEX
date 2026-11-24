#!/usr/bin/env python3
"""rsi_submit request-body tests with a fake Api; no server calls.

Run: python3 -m unittest discover -s scripts/benchmark -p 'test_*.py' -v
"""
from __future__ import annotations

import contextlib
import io
import json
import pathlib
import tempfile
import unittest
from unittest import mock

import arms
import rsi_submit
from test_eval_suite import write_arm


class FakeApi:
    def __init__(self, evidence=None):
        self.calls: list[tuple[str, str, dict | None]] = []
        self.evidence = evidence or {"id": 3, "passed": True, "verdict": {}}

    def get(self, path):
        self.calls.append(("GET", path, None))
        return {"status": "ok"}

    def post(self, path, body):
        self.calls.append(("POST", path, body))
        return {"evidence": self.evidence} if path.endswith("/evidence") else {"proposal": {"id": 7}}


class SubmitTests(unittest.TestCase):
    def setUp(self):
        self.tmp = pathlib.Path(tempfile.mkdtemp())
        (self.tmp / "p.tmpl").write_text("PLANNER-V2 {{.Goal}}\n")

    def run_cli(self, argv, api):
        out = io.StringIO()
        with contextlib.redirect_stdout(out):
            code = rsi_submit.main(argv, api=api)
        return code, out.getvalue()

    def test_prompt_body_proposal_body(self):
        arm = write_arm(self.tmp, "c", {"planner": {"source": "file", "path": "p.tmpl"}}, proposed=["DBEN-v1-001"])
        api = FakeApi()
        code, out = self.run_cli(["propose", "--arm", arm, "--round", "2", "--target", "planner",
                                  "--rationale", "breadth"], api)
        self.assertEqual(code, 0)
        self.assertEqual(json.loads(out)["proposal"]["id"], 7)
        body = {"round": 2, "proposed_from": ["DBEN-v1-001"], "rationale": "breadth", "surface": "prompt_body",
                "target": "planner", "body": "PLANNER-V2 {{.Goal}}\n"}
        self.assertEqual(api.calls, [("POST", "/api/rsi/proposals", body)])
        self.assertEqual(arms.sha(api.calls[0][2]["body"]), arms.sha("PLANNER-V2 {{.Goal}}\n"))  # what the server binds

    def test_lesson_proposal_body_uses_sorted_ids(self):
        arm = write_arm(self.tmp, "l", {}, lessons=[9, 4])
        api = FakeApi()
        self.run_cli(["propose", "--arm", arm, "--round", "1", "--lessons", "--rationale", "r"], api)
        self.assertEqual(api.calls[0][2], {"round": 1, "proposed_from": [], "rationale": "r",
                                           "surface": "lesson", "lesson_ids": [4, 9]})

    def test_propose_refuses_mismatched_arms(self):
        files = write_arm(self.tmp, "f", {"planner": {"source": "file", "path": "p.tmpl"}})
        cur = write_arm(self.tmp, "cur", {}, lessons=[1])
        empty = write_arm(self.tmp, "e", {})
        cases = [(files, ["--lessons"], "non-empty 'lessons'"), (files, ["--target", "worker"], "must be source 'file'"),
                 (cur, ["--target", "planner"], "must be source 'file'"), (empty, ["--lessons"], "non-empty 'lessons'")]
        for arm, flags, msg in cases:
            with self.assertRaisesRegex(SystemExit, msg):
                self.run_cli(["propose", "--arm", arm, "--round", "1", "--rationale", "r", *flags], FakeApi())
        mixed = write_arm(self.tmp, "m", {"planner": {"source": "file", "path": "p.tmpl"}}, lessons=[1])
        with self.assertRaisesRegex(SystemExit, "cannot also pin lessons"):
            self.run_cli(["propose", "--arm", mixed, "--round", "1", "--rationale", "r", "--target", "planner"], FakeApi())
        with self.assertRaisesRegex(SystemExit, "every prompt role must be source 'current'"):
            self.run_cli(["propose", "--arm", mixed, "--round", "1", "--rationale", "r", "--lessons"], FakeApi())

    def test_target_and_lessons_are_exclusive_and_required(self):
        arm = write_arm(self.tmp, "c", {})
        for flags in ([], ["--target", "planner", "--lessons"]):
            with contextlib.redirect_stderr(io.StringIO()), self.assertRaises(SystemExit):
                rsi_submit.main(["propose", "--arm", arm, "--round", "1", "--rationale", "r", *flags], api=FakeApi())

    def test_evidence_posts_both_arm_files_and_exits_on_verdict(self):
        base = {"stamp": {"role": "baseline", "lesson_ids": []}, "records": []}
        cand = {"stamp": {"role": "candidate", "lesson_ids": [4]}, "records": [{"target": "t"}]}
        (self.tmp / "baseline.json").write_text(json.dumps(base))
        (self.tmp / "candidate.json").write_text(json.dumps(cand))
        api = FakeApi()
        code, out = self.run_cli(["evidence", "3", str(self.tmp)], api)
        self.assertEqual((code, json.loads(out)["passed"]), (0, True))
        self.assertEqual(api.calls, [("POST", "/api/rsi/proposals/3/evidence", {"baseline": base, "candidate": cand})])
        code, _ = self.run_cli(["evidence", "3", str(self.tmp)], FakeApi({"id": 4, "passed": False, "verdict": {}}))
        self.assertEqual(code, 1)

    def test_evidence_needs_both_files(self):
        with self.assertRaisesRegex(SystemExit, "baseline.json and candidate.json"):
            self.run_cli(["evidence", "3", str(self.tmp)], FakeApi())

    def test_status_gets_status_and_there_is_no_adopt_command(self):
        api = FakeApi()
        code, out = self.run_cli(["status"], api)
        self.assertEqual((code, api.calls), (0, [("GET", "/api/rsi/status", None)]))
        with contextlib.redirect_stderr(io.StringIO()), self.assertRaises(SystemExit):
            rsi_submit.main(["adopt", "3"], api=FakeApi())

    def test_make_api_reuses_run_bench_token_handling(self):
        with mock.patch.object(rsi_submit.run_bench, "prepare", return_value="tok") as prep:
            api = rsi_submit.make_api("http://x:1/")
        self.assertEqual((api.base, api.token), ("http://x:1", "tok"))
        self.assertTrue(prep.called)


if __name__ == "__main__":
    unittest.main()
