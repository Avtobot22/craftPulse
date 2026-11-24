
import base64
from datetime import datetime
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch


PROFILE = Path(__file__).resolve().parents[1] / "deploy/profile-d"
sys.path.insert(0, str(PROFILE))
try:
    import rollout
    import rollout_host
    from prepare import generated_values
finally:
    sys.path.remove(str(PROFILE))


FIRST, SECOND, BROKEN = "a" * 40, "b" * 40, "c" * 40
OPENSSL = os.environ.get("CRAFTPULSE_TEST_OPENSSL") or shutil.which("openssl")


def manifest(sha, schema, adapter=False):
    names = ["craftpulse-server", "craftpulse-frontend"]
    if adapter:
        names.append("craftpulse-adapter-git")
    return {
        "schema_version": 1,
        "commit_sha": sha,
        "images": {name: f"ghcr.io/avtobot22/{name}@sha256:{sha[0] * 64}" for name in names},
        "core_schema_version": schema,
        "adapter_schema_versions": {"craftpulse-adapter-git": 3} if adapter else {},
        "adapter_contract_version": "1" if adapter else None,
    }


def request(sha, schema):
    raw = {**generated_values(), "oidc_client_secrets": {"stand": "TEST-OIDC-SECRET"}}
    release = manifest(sha, schema)
    adapters, files = rollout.process_files(raw, release, "openssl")
    return {"action": "deploy", "sha": sha, "adapters": adapters, "files": files, "manifest": release}


class SimulatedStand(rollout_host.Stand):
    def __init__(self, root):
        super().__init__(root)
        self.version = 0
        self.events = []
        self.running = None

    def prepare(self, release, files):
        self.events.append(("files", release["commit_sha"]))
        self.assert_private(files)

    @staticmethod
    def assert_private(files):
        if any("age" in name or "ca-key" in name for name in files):
            raise AssertionError("operator's keys crossed SSH")

    def fetch(self, release):
        self.events.append(("pull", release["commit_sha"]))

    def schema(self, release, database):
        return self.version

    def compose(self, release, label, *args, **kwargs):
        self.events.append((label, args))
        if label == "migrate craftpulse":
            self.version = max(self.version, release["core_schema_version"])

    def backup(self, release, database):
        self.events.append(("backup", self.version))

    def switch(self, release, previous):
        self.running = release["commit_sha"]
        self.events.append(("running", self.running))

    def check_installation(self, release):
        self.events.append(("check", release["commit_sha"]))
        if release["commit_sha"] == BROKEN:
            raise rollout.RolloutError("installation check failed")


class RolloutTest(unittest.TestCase):
    def test_manifest_rejects_a_tag_and_other_release(self):
        data = manifest(FIRST, 1, adapter=True)
        self.assertEqual(rollout.validate_manifest(data, FIRST), data)
        with self.assertRaisesRegex(rollout.RolloutError, "requested release"):
            rollout.validate_manifest(data, SECOND)
        data["images"]["craftpulse-server"] = "ghcr.io/avtobot22/craftpulse-server:latest"
        with self.assertRaisesRegex(rollout.RolloutError, "invalid image digest"):
            rollout.validate_manifest(data, FIRST)

    def test_deploy_migration_failed_gate_and_manual_rollback(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            stand = SimulatedStand(root)
            stand.apply(request(FIRST, 1))
            stand.apply(request(SECOND, 2))
            events = stand.events
            migrations = [(i, event) for i, event in enumerate(events) if event[0] == "migrate craftpulse"]
            self.assertEqual(len(migrations), 2)
            for previous_schema, (index, (_, args)) in enumerate(migrations):
                self.assertEqual(events[index - 1], ("backup", previous_schema))
                self.assertEqual(args, ("run", "--rm", "--no-deps", "-T", "migrate"))

            with self.assertRaisesRegex(rollout.RolloutError, "installation check failed"):
                stand.apply(request(BROKEN, 3))
            self.assertEqual((stand.running, stand.version), (SECOND, 3))
            self.assertIsNone(stand.state["pending"])
            stand.apply({"action": "rollback", "sha": FIRST})
            self.assertEqual((stand.running, stand.version), (FIRST, 3))
            self.assertEqual(sum(e[0] == "migrate craftpulse" for e in stand.events), 3)
            journal = (root / "journal.jsonl").read_text(encoding="utf-8")
            self.assertNotIn("TEST-OIDC-SECRET", journal)
            events = [json.loads(line) for line in journal.splitlines()]
            self.assertEqual([(e["sha"], e["outcome"], e["rollback"]) for e in events[-4:]], [
                (BROKEN, "failed", False), (SECOND, "passed", True), (FIRST, "started", True), (FIRST, "passed", True),
            ])
            self.assertEqual(events[-1]["images"], manifest(FIRST, 1)["images"])

    def test_readiness_failure_restores_previous_images_without_migrating_again(self):
        with tempfile.TemporaryDirectory() as directory:
            stand = SimulatedStand(Path(directory))
            stand.apply(request(FIRST, 1))
            switch = stand.switch

            def fail_readiness(release, previous):
                switch(release, previous)
                if release["commit_sha"] == SECOND:
                    raise rollout.RolloutError("api did not pass release version and readiness checks")

            with patch.object(stand, "switch", side_effect=fail_readiness):
                with self.assertRaisesRegex(rollout.RolloutError, "readiness checks"):
                    stand.apply(request(SECOND, 2))
            self.assertEqual((stand.running, stand.version), (FIRST, 2))
            self.assertEqual(sum(e[0] == "migrate craftpulse" for e in stand.events), 2)
            self.assertNotIn(("check", SECOND), stand.events)

    def test_recovers_interrupted_switch_from_state(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            stand = SimulatedStand(root)
            stand.apply(request(FIRST, 1))
            stand.state["pending"] = manifest(SECOND, 2)
            stand.save()
            restarted = SimulatedStand(root)
            restarted.recover()
            self.assertEqual(restarted.running, FIRST)
            self.assertIsNone(restarted.state["pending"])

    def test_worker_api_and_adapters_switch_in_order(self):
        with tempfile.TemporaryDirectory() as directory:
            stand = rollout_host.Stand(Path(directory))
            release = {**manifest(FIRST, 1, adapter=True), "adapters": ["git"], "snapshot": FIRST}
            calls = []
            with patch.object(stand, "compose", side_effect=lambda _, label, *args: calls.append((label, args))), \
                 patch.object(stand, "verify", side_effect=lambda _, service: calls.append(("verify", service))):
                stand.switch(release, None)
            self.assertEqual([label for label, _ in calls], [
                "restart worker", "restart api", "restart adapter-git", "verify", "start frontend", "verify", "verify",
            ])
            for _, args in calls[:3]:
                self.assertIn("--no-deps", args)
            self.assertEqual([args for label, args in calls if label == "verify"], ["adapter-git", "worker", "api"])

    def test_process_file_allowlist_rejects_operator_key_and_path_traversal(self):
        files = request(FIRST, 1)["files"]
        self.assertEqual(rollout_host.decode_files(files, [])["oidc/stand"], (65532, b"TEST-OIDC-SECRET\n"))
        for name in ("age-key.txt", "adapter-ca.key", "oidc/../../payload"):
            with self.subTest(name=name), self.assertRaisesRegex(rollout.RolloutError, "process files"):
                rollout_host.decode_files({**files, name: "YQ=="}, [])

    @unittest.skipUnless(OPENSSL, "OpenSSL is required to verify generated certificates")
    def test_certificates_trust_only_installation_ca_and_last_one_year(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            for name in ("installation", "foreign"):
                subprocess.run([OPENSSL, "req", "-x509", "-newkey", "ed25519", "-nodes", "-days", "730",
                                "-subj", f"/CN={name}", "-keyout", str(root / f"{name}.key"),
                                "-out", str(root / f"{name}.pem"), "-addext", "basicConstraints=critical,CA:TRUE"],
                               check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
            raw = {**generated_values(), "oidc_client_secrets": {"stand": "OIDC"},
                   "adapter_ca_cert": (root / "installation.pem").read_text(),
                   "adapter_ca_key": (root / "installation.key").read_text(),
                   "adapters": {"git": {"owner_password": "owner", "app_password": "app",
                                       "master_key": base64.b64encode(bytes(32)).decode(),
                                       "service_key": "service"}}}
            names, files = rollout.process_files(raw, manifest(FIRST, 1, adapter=True), OPENSSL)
            self.assertEqual(names, ["git"])
            process_files = rollout_host.decode_files(files, names)
            self.assertNotIn(raw["adapter_ca_key"].encode(), [value for _, value in process_files.values()])
            self.assertEqual(process_files["adapter-ca.pem"][1], (root / "installation.pem").read_bytes())
            leaf = root / "leaf.pem"
            leaf.write_bytes(process_files["adapters/git/tls-cert.pem"][1])
            for name, passed in (("installation", True), ("foreign", False)):
                proc = subprocess.run([OPENSSL, "verify", "-verify_hostname", "adapter-git",
                                       "-CAfile", str(root / f"{name}.pem"), str(leaf)],
                                      stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, check=False)
                self.assertEqual(proc.returncode == 0, passed)
            dates = subprocess.check_output([OPENSSL, "x509", "-in", str(leaf), "-noout", "-dates"], text=True)
            start, end = [datetime.strptime(line.split("=", 1)[1], "%b %d %H:%M:%S %Y GMT")
                          for line in dates.splitlines()]
            self.assertEqual((end - start).days, 365)
