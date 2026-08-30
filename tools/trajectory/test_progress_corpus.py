#!/usr/bin/env python3
"""Tests for progress_corpus. Run: python3 -m unittest discover tools/trajectory"""

import json
import os
import subprocess
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

import progress_corpus as pc  # noqa: E402

HERE = os.path.dirname(os.path.abspath(__file__))
DATA = os.path.join(HERE, "testdata")


def fixture(name):
    return os.path.join(DATA, name)


class SemanticSourceWins(unittest.TestCase):
    """The semantic frame is preferred wherever it exists.

    The proxy is easier to parse and present in the same export, so a fallback
    chosen by convenience would silently mix two definitions of progress in one
    corpus."""

    def test_prefers_todo_progress_over_complete_step(self):
        summary, _ = pc.extract(fixture("semantic.json"))
        self.assertEqual(summary["progress_source"], "todo_progress")
        self.assertEqual(summary["progress_fidelity"], "semantic")
        # The fixture carries a complete_step call the proxy would have counted.
        self.assertEqual(summary["progress_events"], 2)
        self.assertEqual(summary["plan_revisions"], 1)


class LegacyFallbackIsLabelled(unittest.TestCase):
    def test_proxy_fidelity_is_part_of_the_sample(self):
        summary, _ = pc.extract(fixture("legacy_proxy.json"))
        self.assertEqual(summary["progress_source"], "complete_step")
        self.assertEqual(summary["progress_fidelity"], "proxy")

    def test_require_semantic_refuses_a_proxy_sample(self):
        code = pc.main([fixture("legacy_proxy.json"), "--require-semantic"])
        self.assertEqual(code, 2)

    def test_require_semantic_accepts_a_semantic_sample(self):
        code = pc.main([fixture("semantic.json"), "--require-semantic", "--json"])
        self.assertEqual(code, 0)


class GapArithmetic(unittest.TestCase):
    """Gaps are measured from the previous advance, not from the run's start."""

    def _at(self, tokens):
        return {"seq": 0, "at": tokens / 1e5, "kind": "model_round",
                "text": f"model_round · hit 0 · miss {int(tokens)} · out 0 · src=executor"}

    def test_gaps_are_differences_between_consecutive_advances(self):
        rows = [
            self._at(1e6),
            {"seq": 1, "at": 10.0, "kind": "outcome_progress",
             "text": "todo advance · content 1 · plan 0 · progress 1"},
            self._at(1e6),
            {"seq": 2, "at": 20.0, "kind": "outcome_progress",
             "text": "todo advance · content 2 · plan 0 · progress 2"},
            self._at(1e7),
            {"seq": 3, "at": 30.0, "kind": "outcome_progress",
             "text": "todo advance · content 3 · plan 0 · progress 3"},
        ]
        with tempfile.TemporaryDirectory() as tmp:
            path = os.path.join(tmp, "t.json")
            with open(path, "w", encoding="utf-8") as fh:
                json.dump({"span": 100.0, "rows": rows}, fh)
            summary, _ = pc.extract(path)
        self.assertEqual(summary["progress_events"], 3)
        self.assertEqual(summary["max_tokens_between_progress"], 1e7)
        self.assertEqual(summary["p50_tokens_between_progress"], 1e6)
        self.assertEqual(summary["max_to_p50_ratio"], 10.0)


class ForwardCompatibility(unittest.TestCase):
    def test_unknown_fields_and_kinds_are_ignored(self):
        rows = [
            {"seq": 1, "at": 0.0, "kind": "model_round", "unknown": 7,
             "text": "model_round · hit 0 · miss 1k · out 1 · src=executor"},
            {"seq": 2, "at": 1.0, "kind": "a_kind_from_the_future", "text": "whatever"},
            {"seq": 3, "at": 2.0, "kind": "outcome_progress",
             "text": "todo advance · content 1 · plan 0 · progress 1"},
        ]
        with tempfile.TemporaryDirectory() as tmp:
            path = os.path.join(tmp, "t.json")
            with open(path, "w", encoding="utf-8") as fh:
                json.dump({"span": 5.0, "rows": rows, "future": {"a": 1}}, fh)
            summary, _ = pc.extract(path)
        self.assertEqual(summary["progress_events"], 1)

    def test_a_todo_line_that_stopped_parsing_is_an_error(self):
        """Silently reading a renamed frame as absent would make every later
        sample look like it had no semantic progress at all."""
        with self.assertRaises(pc.TrajectoryError):
            pc.extract(fixture("broken_todo.json"))

    def test_a_file_that_is_not_a_trajectory_is_an_error(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = os.path.join(tmp, "t.json")
            with open(path, "w", encoding="utf-8") as fh:
                json.dump({"nope": True}, fh)
            with self.assertRaises(pc.TrajectoryError):
                pc.extract(path)


class TransitionExport(unittest.TestCase):
    def test_transitions_carry_scalars_only(self):
        with tempfile.TemporaryDirectory() as tmp:
            out = os.path.join(tmp, "t.jsonl")
            code = pc.main([fixture("semantic.json"), "--transitions", out, "--json"])
            self.assertEqual(code, 0)
            with open(out, encoding="utf-8") as fh:
                records = [json.loads(line) for line in fh]
        self.assertEqual(len(records), 4)
        for record in records:
            self.assertEqual(
                set(record), {"at", "cumulative_tokens", "kind", "content_rev",
                              "plan_rev", "progress_rev"})


class CommandLine(unittest.TestCase):
    def test_runs_as_a_script(self):
        result = subprocess.run(
            [sys.executable, os.path.join(HERE, "progress_corpus.py"), fixture("semantic.json")],
            capture_output=True, text=True, check=False)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("todo_progress (semantic)", result.stdout)


if __name__ == "__main__":
    unittest.main()
