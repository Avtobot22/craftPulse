import contextlib
import importlib.util
import io
from pathlib import Path
import unittest
from unittest.mock import patch


ROOT = Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location("github_setup", ROOT / "scripts/github_setup.py")
setup = importlib.util.module_from_spec(spec)
spec.loader.exec_module(setup)


class GitHubSetupTest(unittest.TestCase):
    def invoke(self, argv, api):
        with patch.object(setup.sys, "argv", ["github_setup.py"] + argv), \
                patch.object(setup, "gh_api", side_effect=api) as calls, \
                contextlib.redirect_stdout(io.StringIO()):
            setup.main()
        return calls

    def test_repository_default_is_read_only(self):
        repo = {"full_name": setup.REPOSITORY, "default_branch": "main"}
        calls = self.invoke(["repository"], lambda *args: repo)
        self.assertEqual(1, calls.call_count)
        self.assertEqual(("repos/" + setup.REPOSITORY,), calls.call_args.args)

    def test_wrong_repository_stops_before_mutation(self):
        with self.assertRaisesRegex(RuntimeError, "Unexpected repository"):
            self.invoke(["repository", "--apply"], lambda *args: {
                "full_name": "another/repo", "default_branch": "main",
            })

    def test_apply_requires_admin(self):
        with self.assertRaisesRegex(RuntimeError, "admin permission"):
            self.invoke(["repository", "--apply"], lambda *args: {
                "full_name": setup.REPOSITORY, "default_branch": "main", "permissions": {"admin": False},
            })

    def test_no_mutation_when_settings_already_match(self):
        repo = dict(setup.load("repository-settings.json"), full_name=setup.REPOSITORY,
                    default_branch="main", permissions={"admin": True})
        calls = self.invoke(["repository", "--apply"], lambda *args: repo)
        self.assertTrue(all(len(call.args) == 1 for call in calls.call_args_list))

    def test_protection_requires_successful_ci(self):
        seen = []

        def api(endpoint, *args):
            seen.append((endpoint, args))
            if endpoint.endswith("/rulesets?per_page=100&includes_parents=false"):
                return []
            if endpoint.endswith("/actions/runs?per_page=100"):
                return {"workflow_runs": []}
            return {"full_name": setup.REPOSITORY, "default_branch": "main", "permissions": {"admin": True}}

        with self.assertRaisesRegex(RuntimeError, "Run Repository CI"):
            self.invoke(["protection", "--apply"], api)
        self.assertTrue(all(not args for _, args in seen))

    def protection_api(self, jobs, seen):
        desired = setup.load("main-ruleset.json")

        def api(endpoint, *args):
            seen.append((endpoint, args))
            if endpoint.endswith("/rulesets?per_page=100&includes_parents=false"):
                return [{"id": 42, "name": desired["name"]}]
            if endpoint.endswith("/actions/runs?per_page=100"):
                return {"workflow_runs": [{
                    "id": 123, "path": ".github/workflows/repository.yml",
                    "status": "completed", "conclusion": "success",
                }]}
            if endpoint.endswith("/actions/runs/123/jobs"):
                return {"jobs": jobs}
            if endpoint.endswith("/rulesets/42"):
                return dict(desired, id=42)
            return {"full_name": setup.REPOSITORY, "default_branch": "main", "permissions": {"admin": True}}

        return api

    def test_protection_requires_every_required_job_to_succeed(self):
        for required in ["repository-check", "ci"]:
            for conclusion in [None, "failure", "cancelled", "skipped", "missing"]:
                with self.subTest(job=required, conclusion=conclusion):
                    jobs = [{"name": name, "conclusion": "success"}
                            for name in ["repository-check", "ci"] if name != required]
                    if conclusion != "missing":
                        jobs.append({"name": required, "conclusion": conclusion})
                    seen = []
                    with self.assertRaisesRegex(RuntimeError, required):
                        self.invoke(["protection", "--apply"], self.protection_api(jobs, seen))
                    self.assertTrue(all(not args for _, args in seen))

    def test_protection_accepts_aggregate_success_and_skipped_backend(self):
        jobs = [{"name": name, "conclusion": "success"} for name in ["repository-check", "ci"]]
        jobs.append({"name": "backend", "conclusion": "skipped"})
        seen = []
        self.invoke(["protection", "--apply"], self.protection_api(jobs, seen))
        mutations = [(endpoint, args) for endpoint, args in seen if args]
        self.assertEqual([("repos/" + setup.REPOSITORY + "/rulesets/42",
                           ("PUT", setup.load("main-ruleset.json")))], mutations)

    def test_protection_dry_run_does_not_require_or_change_checks(self):
        seen = []
        self.invoke(["protection"], self.protection_api([], seen))
        self.assertTrue(all(not args and "/actions/" not in endpoint for endpoint, args in seen))

    def test_ruleset_response_can_reorder_but_cannot_add_bypass(self):
        expected = {"bypass_actors": [], "rules": [{"type": "deletion"}, {"type": "non_fast_forward"}]}
        actual = {"id": 1, "bypass_actors": [], "rules": [{"type": "non_fast_forward"}, {"type": "deletion"}]}
        self.assertTrue(setup.matches_expected(actual, expected))
        actual["bypass_actors"] = [{"actor_id": 5}]
        self.assertFalse(setup.matches_expected(actual, expected))


if __name__ == "__main__":
    unittest.main()
