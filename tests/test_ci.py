"""Exercise the aggregate job's actual shell command with Actions job results."""

import json
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
        dependencies = aggregate.split("    needs:\n", 1)[1].split("    if:", 1)[0]
        names = [line.removeprefix("      - ") for line in dependencies.splitlines()]
        cls.required = ["detect", "repository-check"]
        cls.conditional = [name for name in names if name not in cls.required]

    def run_gate(self, *, results=None, affected=None):
        jobs = {name: {"result": "success"} for name in self.required + self.conditional}
        jobs["detect"]["outputs"] = dict.fromkeys(self.conditional, "true")
        jobs["detect"]["outputs"].update(affected or {})
        for name, result in (results or {}).items():
            jobs[name]["result"] = result
        env = dict(os.environ, JOBS=json.dumps(jobs))
        return subprocess.run([BASH, "--noprofile", "--norc", "-e", "-o", "pipefail", "-c", self.command],
                              env=env, cwd=ROOT, capture_output=True, text=True).returncode

    def test_all_required_jobs_succeeded(self):
        self.assertEqual(0, self.run_gate())

    def test_unaffected_jobs_can_be_skipped(self):
        self.assertEqual(0, self.run_gate(results=dict.fromkeys(self.conditional, "skipped"),
                                          affected=dict.fromkeys(self.conditional, "false")))

    def test_affected_or_unconditional_jobs_cannot_be_skipped(self):
        for name in self.required + self.conditional:
            with self.subTest(job=name):
                self.assertNotEqual(0, self.run_gate(results={name: "skipped"}))

    def test_failures_are_rejected_even_for_unaffected_jobs(self):
        for name in self.required + self.conditional:
            for result in ("failure", "cancelled", ""):
                with self.subTest(job=name, result=result):
                    self.assertNotEqual(0, self.run_gate(results={name: result},
                                                        affected=dict.fromkeys(self.conditional, "false")))

    def test_unknown_detection_output_cannot_skip_a_job(self):
        self.assertNotEqual(0, self.run_gate(results={"openapi": "skipped"},
                                            affected={"openapi": ""}))



if __name__ == "__main__":
    unittest.main()
