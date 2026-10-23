import importlib.util
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch
import subprocess


ROOT = Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location("check_repository", ROOT / "scripts/check_repository.py")
checks = importlib.util.module_from_spec(spec)
spec.loader.exec_module(checks)


class RepositoryChecksTest(unittest.TestCase):
    def test_conventional_title(self):
        for title in ["ci: настроить репозиторий", "feat(frontend): добавить холст", "feat(backend)!: изменить формат"]:
            with self.subTest(title=title):
                self.assertTrue(checks.valid_title(title))

    def test_invalid_titles(self):
        for title in ["update", "feat: ", "feat: text\nbody", "ci: " + "a" * 100]:
            with self.subTest(title=title):
                self.assertFalse(checks.valid_title(title))

    def test_branch_names(self):
        for branch in ["feat/123-role-dashboard", "fix/124-timeout", "ci/checks", "chore/github-workflow", "copilot/fix-dependency"]:
            with self.subTest(branch=branch):
                self.assertTrue(checks.valid_branch(branch))
        for branch in ["feature/foo", "fix_123", "feat/BadName", "main", "feat/foo/bar", "feat/123_"]:
            with self.subTest(branch=branch):
                self.assertFalse(checks.valid_branch(branch))

    def test_dependabot_branch_exception_is_not_available_to_human_authors(self):
        self.assertTrue(checks.valid_branch("dependabot/github_actions/group-123", "dependabot[bot]"))
        self.assertFalse(checks.valid_branch("dependabot/github_actions/group-123", "contributor"))

    def test_each_new_commit_is_checked(self):
        result = subprocess.CompletedProcess([], 0, "a" * 40 + "\0feat(frontend): добавить холст\n"
                                            + "b" * 40 + "\0WIP\n")
        with patch.object(checks.subprocess, "run", return_value=result) as run:
            errors = checks.commit_title_errors(ROOT, "c" * 40, "d" * 40)
        self.assertEqual(1, len(errors))
        self.assertIn("b" * 12, errors[0])
        self.assertIn("c" * 40 + ".." + "d" * 40, run.call_args.args[0])

    def test_revisions_from_event_cannot_become_git_options(self):
        with patch.object(checks.subprocess, "run") as run:
            self.assertTrue(checks.commit_title_errors(ROOT, "--all", "d" * 40))
        run.assert_not_called()

    def test_empty_copilot_planning_commit_is_allowed(self):
        log = subprocess.CompletedProcess([], 0, "a" * 40 + "\0Initial plan\n")
        empty_diff = subprocess.CompletedProcess([], 0)
        with patch.object(checks.subprocess, "run", side_effect=[log, empty_diff]) as run:
            self.assertEqual([], checks.commit_title_errors(ROOT, "c" * 40, "d" * 40))
        self.assertEqual(["git", "diff", "--quiet", "a" * 40 + "^", "a" * 40, "--"], run.call_args.args[0])

    def test_planning_title_cannot_hide_file_changes(self):
        log = subprocess.CompletedProcess([], 0, "a" * 40 + "\0Initial plan\n")
        for returncode in [1, 128]:
            with self.subTest(returncode=returncode):
                with patch.object(checks.subprocess, "run", side_effect=[log, subprocess.CompletedProcess([], returncode)]):
                    self.assertEqual(1, len(checks.commit_title_errors(ROOT, "c" * 40, "d" * 40)))

    def test_local_and_secret_paths(self):
        for name in [".env", "apps/bot/.env.local", "shared/file.md", "artifacts/report.md",
                     ".agents/skills/file.md", "apps/web/node_modules/a.js", "key.pem", "meeting.m4a"]:
            with self.subTest(name=name):
                self.assertTrue(checks.forbidden_path(name))

    def test_templates_and_nested_media_are_allowed(self):
        for name in [".env.example", "apps/bot/.env.test.example", "docs/a.md", "tests/fixtures/sound.wav"]:
            with self.subTest(name=name):
                self.assertFalse(checks.forbidden_path(name))

    def test_relative_links_and_encoded_spaces(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            source = root / "docs/start.md"
            content = "[a](../README.md) [b](<Project vision.md>) [c](Project%20vision.md#intro)"
            known = {"README.md", "docs/Project vision.md", "docs/start.md"}
            self.assertEqual([], checks.link_errors(root, source, content, known))

    def test_missing_and_local_only_links(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / "local.md").write_text("local")
            errors = checks.link_errors(root, root / "README.md", "[x](local.md) [y](absent.md)", set())
            self.assertEqual(2, len(errors))

    def test_links_cannot_escape_repository(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            errors = checks.link_errors(root, root / "README.md", "[x](../elsewhere.md)", set())
            self.assertEqual(1, len(errors))

    def test_code_remote_urls_and_fragments_are_skipped(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            content = "```md\n[x](absent.md)\n```\n`[x](missing.md)` [a](https://example.com) [b](#anchor)"
            self.assertEqual([], checks.link_errors(root, root / "README.md", content, set()))


if __name__ == "__main__":
    unittest.main()
