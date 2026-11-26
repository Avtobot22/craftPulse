"""Regression checks for integration test assignment and reported coverage."""
import importlib.util
from pathlib import Path
import unittest


path = Path(__file__).resolve().parents[1] / "scripts/ci_shards.py"
spec = importlib.util.spec_from_file_location("ci_shards", path)
shards = importlib.util.module_from_spec(spec)
spec.loader.exec_module(shards)


class ShardAssignmentTest(unittest.TestCase):
    def test_weighted_assignment_is_complete_and_deterministic(self):
        tests = ["TestA", "TestB", "TestC", "TestD"]
        weights = dict(zip(tests, [10, 5, 3, 2]))
        groups = shards.assign(tests, weights, 2)
        self.assertEqual(groups, shards.assign(list(reversed(tests)), weights, 2))
        self.assertEqual(sorted(tests), sorted(test for group in groups for test in group))
        self.assertEqual([10, 10], [sum(weights[test] for test in group) for group in groups])

    def test_new_tests_are_assigned_with_missing_or_invalid_timings(self):
        tests = ["TestOld", "TestNew", "TestOther", "TestLast"]
        groups = shards.assign(tests, {"TestOld": 10, "TestLast": -1}, 4)
        self.assertEqual({test for group in groups for test in group}, set(tests))
        self.assertTrue(all(len(group) == 1 for group in groups))

    def test_rejects_duplicate_names_and_empty_shards(self):
        for tests, count in ((["TestA", "TestA"], 2), (["TestA"], 2), ([], 1)):
            with self.subTest(tests=tests, count=count), self.assertRaises(ValueError):
                shards.assign(tests, {}, count)
