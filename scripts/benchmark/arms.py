#!/usr/bin/env python3
"""Prompt-version arms for baseline/candidate evaluation.

An arm names, per role, which editable prompt body the role runs with:

    {"name": "planner-breadth-v2",
     "proposed_from": ["DBEN-v1-001"],          # targets the edit was derived from
     "prompts": {"planner": {"source": "file", "path": "prompts/planner-v2.tmpl"},
                 "worker":  {"source": "current"}}}

source: "current" = the body active when the suite started (the default for any
role the arm omits), "builtin" = the in-code default (POST .../prompt/reset),
"file" = a template file relative to the arm file.

An arm may also pin recorded lessons: "lessons": [12, 15] (lesson ids). The server
must run with ARTEX_RSI_LESSONS=bench, where every new task gets exactly the pinned
lessons and none when the list is empty. Baseline arms never carry lessons.

Only the editable body (section [A], stored in agent_prompts) can be varied,
through the server's own versioned prompt API. The code-owned tails, the
adversarial and quality gates, scope checks, injection defenses and tool
allowlists are outside this API and therefore outside every arm (RSI.md
immutable surfaces). Every swap is a new agent_prompts version and the suite
restores the starting bodies when it ends, so rollback is a pointer change.
"""
from __future__ import annotations

import hashlib
import json
import pathlib

# The orchestration roles a planner/worker evaluation may vary. Chat agents
# (auto, pentest), the retester and custom agents are deliberately excluded.
ROLES = ("goals", "planner", "worker")
SOURCES = ("current", "builtin", "file")


def sha(text: str) -> str:
    return hashlib.sha256(text.encode()).hexdigest()[:16]


def load_arm(path: str) -> dict:
    """Load and validate an arm file; file sources are read eagerly."""
    p = pathlib.Path(path)
    arm = json.loads(p.read_text())
    if not isinstance(arm.get("name"), str) or not arm["name"].strip():
        raise SystemExit(f"{path}: arm needs a non-empty name")
    prompts = arm.get("prompts") or {}
    unknown = set(prompts) - set(ROLES)
    if unknown:
        raise SystemExit(f"{path}: roles {sorted(unknown)} are outside the evaluable set {list(ROLES)}")
    resolved = {}
    for role in ROLES:
        spec = prompts.get(role) or {"source": "current"}
        source = spec.get("source")
        if source not in SOURCES:
            raise SystemExit(f"{path}: {role}.source must be one of {SOURCES}")
        entry = {"source": source}
        if source == "file":
            tmpl = (p.parent / str(spec.get("path") or "")).resolve()
            if not tmpl.is_file():
                raise SystemExit(f"{path}: {role} template not found: {tmpl}")
            entry["text"] = tmpl.read_text()
            if not entry["text"].strip():
                raise SystemExit(f"{path}: {role} template is empty")
            entry["path"] = str(spec["path"])
        resolved[role] = entry
    proposed = arm.get("proposed_from") or []
    if not isinstance(proposed, list) or not all(isinstance(x, str) for x in proposed):
        raise SystemExit(f"{path}: proposed_from must be a list of target ids")
    return {"name": arm["name"], "proposed_from": proposed, "prompts": resolved,
            "lessons": _lesson_ids(path, arm.get("lessons")), "file": str(p)}


def _lesson_ids(path: str, raw) -> list[int]:
    if raw is None:
        return []
    if not isinstance(raw, list) or not all(isinstance(x, int) and not isinstance(x, bool) and x > 0 for x in raw):
        raise SystemExit(f"{path}: lessons must be a list of positive integer lesson ids")
    return sorted(set(raw))


def check_held_out(arm: dict, target_ids: list[str]) -> None:
    """RSI.md rule 1: the evaluation set is independent of the tasks used to propose the edit."""
    overlap = sorted(set(arm["proposed_from"]) & set(target_ids))
    if overlap:
        raise SystemExit(f"arm {arm['name']!r} was proposed from {overlap}, which are in this suite; "
                         "evaluate it on targets it was not derived from")


class PromptSwitch:
    """Applies arms through the server's prompt API and restores the start state.

    With lessons=True (some arm pins lessons) it also drives /api/rsi/bench-lessons;
    with lessons=False that endpoint is never called, so suites without lesson arms
    run on any instance, whatever ARTEX_RSI_LESSONS says.
    """

    LESSONS_PATH = "/api/rsi/bench-lessons"

    def __init__(self, api, lessons: bool = False):
        self.api = api
        self.lessons = lessons
        self.start: dict[str, str] = {}
        self.start_lessons: list[int] = []
        self.builtin: dict[str, str] = {}

    def current(self, role: str) -> str:
        return str(self.api.get(f"/api/agents/{role}").get("prompt") or "")

    def snapshot(self) -> None:
        self.start = {role: self.current(role) for role in ROLES}
        if self.lessons:
            self.start_lessons = sorted(self.api.get(self.LESSONS_PATH).get("lesson_ids") or [])

    def _pin(self, ids: list[int]) -> dict:
        return self.api.req("PUT", self.LESSONS_PATH, {"lesson_ids": ids})

    def _put(self, role: str, text: str, note: str) -> None:
        self.api.req("PUT", f"/api/agents/{role}/prompt", {"template": text, "note": note})

    def _builtin(self, role: str) -> str:
        if role not in self.builtin:
            self.api.post(f"/api/agents/{role}/prompt/reset", {})
            self.builtin[role] = self.current(role)
        return self.builtin[role]

    def apply(self, arm: dict) -> dict:
        """Make every role run the arm's body (and, with lessons on, its pinned lessons).

        Returns what the server reports back: {"prompt_sha": {role: sha},
        "lesson_ids": sorted ids ([] when lessons are off), "lessons_block_sha": str|None}.
        """
        stamp = {}
        for role in ROLES:
            spec = arm["prompts"][role]
            have = self.current(role)
            if spec["source"] == "file":
                want = spec["text"]
            elif spec["source"] == "current" and self.start[role]:
                want = self.start[role]
            else:  # builtin, or "current" when no stored body existed (the agent ran its default)
                want = self._builtin(role)
                have = self.current(role)
            if have != want:
                self._put(role, want, f"bench arm {arm['name']}")
            got = self.current(role)
            if got != want:
                raise SystemExit(f"{role}: server did not activate the arm body for {arm['name']!r}")
            stamp[role] = sha(got)
        applied = {"prompt_sha": stamp, "lesson_ids": [], "lessons_block_sha": None}
        if self.lessons:
            want_ids = list(arm.get("lessons") or [])
            self._pin(want_ids)
            back = self.api.get(self.LESSONS_PATH)
            got_ids = sorted(back.get("lesson_ids") or [])
            if got_ids != want_ids:
                raise SystemExit(f"server pinned lessons {got_ids}, arm {arm['name']!r} wants {want_ids}")
            applied.update(lesson_ids=got_ids, lessons_block_sha=back.get("block_sha"))
        return applied

    def restore(self) -> None:
        try:
            for role, text in self.start.items():
                if self.current(role) == text:
                    continue
                if text:
                    self._put(role, text, "bench restore")
                else:  # no stored body at start: the built-in default is the equivalent state
                    self.api.post(f"/api/agents/{role}/prompt/reset", {})
        finally:
            if self.lessons:
                self._pin(self.start_lessons)
