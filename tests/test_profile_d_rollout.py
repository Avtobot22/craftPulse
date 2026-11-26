
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
    def test_ac01_install_example_matches_two_oidc_secret_files(self):
        settings = json.loads((PROFILE / "rollout.example.json").read_text(encoding="utf-8"))
        for org, subject in (("A", "17"), ("B", "24")):
            settings["install"][f"ORG_ORG_{org}_OIDC_CLIENT_ID"] = f"client-{org.lower()}"
            settings["install"][f"ORG_ORG_{org}_ADMIN_SUBJECT"] = subject
        bundle = {**generated_values(), "oidc_client_secrets": {"org-a": "TEST-SECRET-A", "org-b": "TEST-SECRET-B"}}

        adapters, encoded = rollout.process_files(bundle, manifest(FIRST, 1), "openssl")
        files = rollout_host.decode_files(encoded, adapters)
        config = rollout_host.install_config(settings, files)

        self.assertEqual(config["INSTALL_ORGANIZATIONS"], "org-a,org-b")
        self.assertEqual(files["oidc/org-a"], (65532, b"TEST-SECRET-A\n"))
        self.assertEqual(files["oidc/org-b"], (65532, b"TEST-SECRET-B\n"))
        self.assertEqual(config, settings["install"])
        self.assertEqual(config["ORG_ORG_A_OIDC_ISSUER"], "https://gitlab.defects.team")
        self.assertEqual(config["ORG_ORG_B_OIDC_ISSUER"], "https://gitlab.defects.team")

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
            self.assertEqual(
                [args for label, args in events if label == "apply observability configuration"],
                [("up", "-d", "--no-deps", "--wait", "--wait-timeout", "180",
                  "victoriametrics", "victorialogs", "grafana", "cadvisor")] * 2,
            )
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

    def test_source_network_policy_reaches_core_and_adapters(self):
        settings = {
            "tls_dir": "/etc/craftpulse/tls",
            "installation_networks": "172.31.84.0/27",
            "source_private_networks": "10.0.0.0/24",
        }
        for adapters in ([], ["git"]):
            with self.subTest(adapters=adapters):
                release = manifest(FIRST, 1, adapter=bool(adapters))
                services = rollout_host.overrides(settings, adapters, release, "/tmp/secrets")["services"]
                for service in ("api", "worker", *(f"adapter-{name}" for name in adapters)):
                    env = services[service]["environment"]
                    self.assertEqual(env["INSTALLATION_NETWORKS"], settings["installation_networks"])
                    self.assertEqual(env["SOURCE_PRIVATE_NETWORKS"], settings["source_private_networks"])

    def test_renewed_tls_files_trigger_adapter_restart_for_same_release(self):
        with tempfile.TemporaryDirectory() as directory:
            stand = rollout_host.Stand(Path(directory))
            release = {**manifest(FIRST, 1, adapter=True), "adapters": ["git"]}
            previous = {**release, "snapshot": "previous"}
            candidate = {**release, "snapshot": "candidate"}
            for snapshot in (previous, candidate):
                secrets = stand.path(snapshot) / "secrets"
                adapter = secrets / "adapters/git"
                adapter.mkdir(parents=True)
                (secrets / "adapter-ca.pem").write_bytes(b"same CA")
                for filename in rollout.ADAPTER_FILES:
                    (adapter / filename).write_bytes(filename.encode())

            with patch.object(stand, "compose", return_value=b"running-container"):
                self.assertFalse(stand.adapter_changed(candidate, previous, "git"))
                for filename in ("tls-cert.pem", "tls-key.pem"):
                    with self.subTest(filename=filename):
                        credential = stand.path(candidate) / "secrets/adapters/git" / filename
                        credential.write_bytes(b"renewed")
                        self.assertTrue(stand.adapter_changed(candidate, previous, "git"))
                        credential.write_bytes(filename.encode())

    def test_adapter_resources_follow_profile_budget(self):
        release = manifest(FIRST, 1, adapter=True)
        release["images"]["craftpulse-adapter-gitlab"] = release["images"]["craftpulse-adapter-git"]
        config = rollout_host.overrides({"tls_dir": "/etc/craftpulse/tls"}, ["git", "gitlab"], release, "/tmp/secrets")
        services = config["services"]
        self.assertEqual((services["adapter-git"]["mem_limit"], services["adapter-git"]["cpus"]), ("256m", 1.0))
        self.assertEqual(services["adapter-gitlab"]["mem_limit"], "48m")
        self.assertEqual(services["adapter-gitlab"]["cpus"], "${CONTAINER_CPUS:-4}")

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
