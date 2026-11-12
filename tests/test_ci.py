"""Exercise the aggregate job's actual shell command with Actions job results."""

import os
from pathlib import Path
import shutil
import subprocess
import textwrap
import unittest


ROOT = Path(__file__).resolve().parents[1]
BASH = shutil.which("bash")
if os.name == "nt":
    # Windows' system bash launches WSL. Use the existing Git for Windows shell.
    git = shutil.which("git")
    candidate = Path(git).parents[1] / "bin/bash.exe" if git else None
    BASH = str(candidate) if candidate and candidate.is_file() else None


@unittest.skipUnless(BASH, "bash is required to exercise the GitHub Actions shell command")
class AggregateCITest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        workflow = (ROOT / ".github/workflows/repository.yml").read_text(encoding="utf-8")
        aggregate = workflow.split("\n  ci:\n", 1)[1]
        cls.command = textwrap.dedent(aggregate.split("        run: |\n", 1)[1])

    def run_gate(self, **overrides):
        env = dict(os.environ, DETECT="success", REPOSITORY="success", BACKEND="success",
                   BACKEND_CHANGED="true", DATASET="success", DATASET_CHANGED="true",
                   EVENT_NAME="pull_request")
        env.update(overrides)
        return subprocess.run([BASH, "--noprofile", "--norc", "-e", "-o", "pipefail", "-c", self.command],
                              env=env, cwd=ROOT, capture_output=True, text=True).returncode

    def test_all_required_jobs_succeeded(self):
        self.assertEqual(0, self.run_gate())

    def test_docs_only_pr_can_skip_backend(self):
        self.assertEqual(0, self.run_gate(BACKEND="skipped", BACKEND_CHANGED="false",
                                          DATASET="skipped", DATASET_CHANGED="false"))

    def test_dataset_only_pr_can_skip_backend(self):
        self.assertEqual(0, self.run_gate(BACKEND="skipped", BACKEND_CHANGED="false"))

    def test_backend_only_pr_can_skip_dataset(self):
        self.assertEqual(0, self.run_gate(DATASET="skipped", DATASET_CHANGED="false"))

    def test_required_checks_cannot_be_skipped_failed_or_cancelled(self):
        for job in ["DETECT", "REPOSITORY", "BACKEND", "DATASET"]:
            for result in ["failure", "cancelled", "skipped", ""]:
                with self.subTest(job=job, result=result):
                    self.assertNotEqual(0, self.run_gate(**{job: result}))

    def test_failed_detection_cannot_masquerade_as_unaffected_backend(self):
        self.assertNotEqual(0, self.run_gate(DETECT="failure", BACKEND="skipped", BACKEND_CHANGED=""))

    def test_unknown_detection_output_cannot_skip_backend(self):
        self.assertNotEqual(0, self.run_gate(BACKEND="skipped", BACKEND_CHANGED=""))

    def test_manual_run_cannot_skip_backend(self):
        self.assertNotEqual(0, self.run_gate(BACKEND="skipped", BACKEND_CHANGED="false",
                                           EVENT_NAME="workflow_dispatch"))

    def test_unknown_detection_output_cannot_skip_dataset(self):
        self.assertNotEqual(0, self.run_gate(DATASET="skipped", DATASET_CHANGED=""))

    def test_manual_run_cannot_skip_dataset(self):
        self.assertNotEqual(0, self.run_gate(DATASET="skipped", DATASET_CHANGED="false",
                                           EVENT_NAME="workflow_dispatch"))


if __name__ == "__main__":
    unittest.main()
