
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


def manifest(sha, schema, adapter=False, adapter_name="git"):
    names = ["craftpulse-server", "craftpulse-frontend"]
    if adapter:
        names.append(rollout.ADAPTER + adapter_name)
    return {
        "schema_version": 1,
        "commit_sha": sha,
        "images": {name: f"ghcr.io/avtobot22/{name}@sha256:{sha[0] * 64}" for name in names},
        "core_schema_version": schema,
        "adapter_schema_versions": {rollout.ADAPTER + adapter_name: 3} if adapter else {},
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
            with patch.object(stand, "compose", side_effect=lambda _, label, *args, **kw: calls.append((label, args))), \
                 patch.object(stand, "verify", side_effect=lambda _, service: calls.append(("verify", service))):
                stand.switch(release, None)
            self.assertEqual([label for label, _ in calls], [
                "restart worker", "restart api", "restart adapter-git", "verify", "start frontend",
                "verify", "verify", "apply observability configuration",
            ])
            for _, args in calls[:3]:
                self.assertIn("--no-deps", args)
            self.assertEqual([args for label, args in calls if label == "verify"], ["adapter-git", "worker", "api"])
            self.assertEqual(calls[-1][1], ("up", "-d", "--no-deps", "--wait", "--wait-timeout", "180",
                                            "victoriametrics", "victorialogs", "grafana", "cadvisor"))

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
                retired = stand.path(previous) / "secrets/adapters/git" / rollout.PREVIOUS_SERVICE_KEY
                retired.write_bytes(b"old-key")
                self.assertTrue(stand.adapter_changed(candidate, previous, "git"))

    def test_adapter_resources_follow_profile_budget(self):
        release = manifest(FIRST, 1, adapter=True)
        release["images"]["craftpulse-adapter-gitlab"] = release["images"]["craftpulse-adapter-git"]
        config = rollout_host.overrides({"tls_dir": "/etc/craftpulse/tls"}, ["git", "gitlab"], release, "/tmp/secrets")
        services = config["services"]
        self.assertEqual((services["adapter-git"]["mem_limit"], services["adapter-git"]["cpus"]), ("256m", 1.0))
        self.assertEqual(services["adapter-gitlab"]["mem_limit"], "48m")
        self.assertEqual(services["adapter-gitlab"]["cpus"], "${CONTAINER_CPUS:-4}")

    def test_jira_cloud_compose_uses_release_and_disjoint_runtime_and_migration_credentials(self):
        settings = {
            "tls_dir": "/etc/craftpulse/tls",
            "source_private_networks": "10.0.0.0/24",
            "adapter_master_key_versions": {"jiracloud": 2},
        }
        release = manifest(FIRST, 1, adapter=True, adapter_name="jiracloud")
        services = rollout_host.overrides(settings, ["jiracloud"], release, "/run/test")["services"]
        adapter = services["adapter-jiracloud"]
        migration = services["adapter-jiracloud-migrate"]
        self.assertEqual(adapter["image"], release["images"]["craftpulse-adapter-jiracloud"])
        self.assertEqual(migration["image"], adapter["image"])
        self.assertEqual((adapter["mem_limit"], adapter["memswap_limit"]), ("48m", "48m"))
        self.assertEqual(adapter["environment"]["GOMEMLIMIT"], "36MiB")
        self.assertEqual(adapter["environment"]["SOURCE_PRIVATE_NETWORKS"], "")
        self.assertEqual(adapter["environment"]["ADAPTER_METRICS_ADDR"], ":9090")
        self.assertEqual(adapter["restart"], "unless-stopped")
        self.assertEqual(adapter["healthcheck"]["test"], ["CMD", "/usr/local/bin/adapter", "healthcheck"])
        self.assertNotIn("ports", adapter)
        self.assertEqual(migration["command"], ["migrate"])
        self.assertEqual(migration["networks"], ["data"])
        for service, user, password in ((adapter, "adapter_jiracloud", "app-password"),
                                         (migration, "adapter_jiracloud_owner", "owner-password")):
            self.assertEqual(service["environment"]["DATABASE_URL"],
                             f"postgres://{user}@postgres:5432/adapter_jiracloud?sslmode=disable")
            self.assertEqual(service["environment"]["ADAPTER_MASTER_KEY_VERSION"], "2")
            self.assertIn(f"/run/test/adapters/jiracloud/{password}:/run/secrets/db-password:ro", service["volumes"])
        for name in ("api", "worker"):
            self.assertEqual(services[name]["environment"]["BUILTIN_ADAPTERS"], "jira_cloud=https://adapter-jiracloud:8443")
            self.assertEqual(services[name]["environment"]["ADAPTER_CA_FILE"], "/run/secrets/adapter-ca.pem")
            self.assertIn("/run/test/adapters/jiracloud/service-key:/run/secrets/adapter-keys/jira_cloud:ro",
                          services[name]["volumes"])

    def test_required_jira_cloud_cannot_be_omitted_from_bundle(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            config = root / "config"
            config.mkdir()
            (config / "rollout.json").write_text('{"required_adapters": ["jiracloud"]}')
            stand = rollout_host.Stand(root, config)
            release = {**manifest(FIRST, 1, adapter=True, adapter_name="jiracloud"),
                       "adapters": [], "snapshot": FIRST}
            with self.assertRaisesRegex(rollout.RolloutError, "missing from the release or SOPS bundle"):
                stand.prepare(release, {})

    def test_rollout_can_adopt_existing_stand_data_volumes(self):
        settings = {"tls_dir": "/etc/craftpulse/tls"}
        release = manifest(FIRST, 1)
        self.assertNotIn("volumes", rollout_host.overrides(settings, [], release, "/run/test"))
        settings["stand_data_volumes"] = {"postgres": "craftpulse-product_database",
                                         "objects": "craftpulse-product_object-store"}
        config = rollout_host.overrides(settings, [], release, "/run/test")
        self.assertEqual(config["volumes"], {
            "postgres": {"name": "craftpulse-product_database", "external": True},
            "objects": {"name": "craftpulse-product_object-store", "external": True},
        })
        settings["stand_data_volumes"]["objects"] = "craftpulse-product_database"
        with self.assertRaisesRegex(rollout.RolloutError, "distinct existing"):
            rollout_host.overrides(settings, [], release, "/run/test")

    def test_rollout_snapshots_collection_provisioning(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            config = root / "config"
            (config / "observability").mkdir(parents=True)
            for name in ("scrape.yaml", "grafana-alerts.yaml", "grafana-collection.json"):
                shutil.copyfile(PROFILE / "observability" / name, config / "observability" / name)
            settings = json.loads((PROFILE / "rollout.example.json").read_bytes())
            settings = {
                **settings, "required_adapters": [], "tls_dir": str(root),
                "check_profiles": str(PROFILE / "check-profiles.local.yaml"),
                "backup_recipient": "age1test",
                "stand_data_volumes": {"postgres": "existing-db", "objects": "existing-store"},
            }
            (config / "rollout.json").write_text(json.dumps(settings))
            for name in ("tls-cert.pem", "tls-key.pem"):
                (root / name).touch()
            stand = rollout_host.Stand(root, config)
            release = {**manifest(FIRST, 1), "adapters": [], "snapshot": FIRST}
            orgs = settings["install"]["INSTALL_ORGANIZATIONS"].split(",")
            bundle = {
                **generated_values(),
                "oidc_client_secrets": dict.fromkeys(orgs, "TEST-OIDC-SECRET"),
            }
            adapters, encoded = rollout.process_files(bundle, release, "openssl")
            files = rollout_host.decode_files(encoded, adapters)

            def render(label, args):
                if label == "check existing stand data volume":
                    return b""
                override = json.loads(Path(args[args.index("--profile") - 1]).read_bytes())
                return json.dumps({**override, "name": "craftpulse-profile-d"}).encode()

            with patch.object(rollout_host, "materialize_files"), patch.object(rollout_host, "run", side_effect=render):
                stand.prepare(release, files)

            def legacy_running(label, args):
                if label == "check existing stand data volume":
                    return b"abc123 craftpulse-product\n"
                return render(label, args)

            with patch.object(rollout_host, "materialize_files"), \
                 patch.object(rollout_host, "run", side_effect=legacy_running):
                candidate = {**release, "snapshot": "legacy-running"}
                with self.assertRaisesRegex(rollout.RolloutError, "in use outside the rollout project"):
                    stand.prepare(candidate, files)

            snapshot = stand.path(release)
            services = json.loads((snapshot / "compose.json").read_bytes())["services"]
            for service, name, target in (
                ("victoriametrics", "scrape.yaml", "/etc/victoriametrics/scrape.yaml"),
                ("grafana", "grafana-alerts.yaml", "/etc/grafana/provisioning/alerting/craftpulse.yaml"),
                ("grafana", "grafana-collection.json", "/etc/grafana/craftpulse-dashboards/collection.json"),
            ):
                self.assertIn(f"{snapshot / name}:{target}:ro", services[service]["volumes"])
                self.assertEqual((snapshot / name).read_bytes(), (config / "observability" / name).read_bytes())

            with patch.object(os, "fchown"):
                rollout_host.materialize_files(snapshot / "secrets", files)
            stand.state["current"] = release
            settings["stand_data_volumes"]["objects"] = "different-store"
            (config / "rollout.json").write_text(json.dumps(settings))
            candidate = {**manifest(SECOND, 1), "adapters": [], "snapshot": SECOND}
            with patch.object(rollout_host, "materialize_files"), patch.object(rollout_host, "run", side_effect=render):
                with self.assertRaisesRegex(rollout.RolloutError, "volumes differ from the active release"):
                    stand.prepare(candidate, files)

    def test_jira_cloud_db_migrates_before_switch_and_rollback_uses_previous_digest(self):
        with tempfile.TemporaryDirectory() as directory:
            stand = rollout_host.Stand(Path(directory))
            releases = [{**manifest(sha, 1, adapter=True, adapter_name="jiracloud"),
                         "adapters": ["jiracloud"], "snapshot": sha} for sha in (FIRST, SECOND)]
            schema = {"craftpulse": 1, "adapter_jiracloud": 0}
            calls = []
            observability = []

            def compose(release, label, *args, **kwargs):
                if label == "migrate adapter_jiracloud":
                    schema["adapter_jiracloud"] = 3
                if label == "apply observability configuration":
                    observability.append(release["snapshot"])
                if label == "restart adapter-jiracloud":
                    calls.append(release["images"]["craftpulse-adapter-jiracloud"])
                else:
                    calls.append(label)
                return b""

            with patch.object(stand, "compose", side_effect=compose), \
                 patch.object(stand, "schema", side_effect=lambda _, db: schema[db]), \
                 patch.object(stand, "backup"), patch.object(stand, "verify") as verify:
                stand.migrate(releases[0])
                self.assertLess(calls.index("prepare database for jiracloud"), calls.index("migrate adapter_jiracloud"))
                stand.switch(releases[0], None)
                stand.switch(releases[1], releases[0])
                stand.switch(releases[0], releases[1])
            self.assertEqual([value for value in calls if "@sha256:" in value],
                             [release["images"]["craftpulse-adapter-jiracloud"]
                              for release in (releases[0], releases[1], releases[0])])
            self.assertEqual([args[0]["commit_sha"] for args, _ in verify.call_args_list
                              if args[1] == "adapter-jiracloud"], [FIRST, SECOND, FIRST])
            self.assertEqual(observability, [FIRST, SECOND, FIRST])

    def test_jira_cloud_rollback_rejects_old_keys_before_switching(self):
        for rotated in ("master key", "service key", "retired service key"):
            with self.subTest(rotated=rotated), tempfile.TemporaryDirectory() as directory:
                stand = rollout_host.Stand(Path(directory))
                for i, sha in enumerate((FIRST, SECOND)):
                    release = {**manifest(sha, 1, adapter=True, adapter_name="jiracloud"),
                               "adapters": ["jiracloud"], "snapshot": sha}
                    secret_dir = stand.path(release) / "secrets"
                    keys = secret_dir / "adapters/jiracloud"
                    keys.mkdir(parents=True)
                    key_version = 1 + i if rotated == "master key" else 1
                    (keys / "master-key").write_bytes(bytes([key_version] * 32))
                    (keys / "service-key").write_text(sha if rotated == "service key" else "service")
                    if rotated == "retired service key" and sha == FIRST:
                        (keys / rollout.PREVIOUS_SERVICE_KEY).write_bytes(b"retired")
                    settings = {"tls_dir": "/etc/craftpulse/tls",
                                "adapter_master_key_versions": {"jiracloud": key_version}}
                    compose = rollout_host.overrides(settings, release["adapters"], release, secret_dir)
                    rollout_host.write_json(stand.path(release) / "compose.json", compose)
                    stand.state["releases"][sha] = release
                stand.state["current"] = stand.state["releases"][SECOND]
                with patch.object(stand, "switch") as switch:
                    with self.assertRaisesRegex(rollout.RolloutError, rotated):
                        stand.apply({"action": "rollback", "sha": FIRST})
                    switch.assert_not_called()

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
                   "adapters": {"jiracloud": {"owner_password": "owner", "app_password": "app",
                                       "master_key": base64.b64encode(bytes(32)).decode(),
                                        "service_key": "service", "previous_service_key": "rotating-in"}}}
            names, files = rollout.process_files(raw, manifest(FIRST, 1, adapter=True, adapter_name="jiracloud"), OPENSSL)
            self.assertEqual(names, ["jiracloud"])
            process_files = rollout_host.decode_files(files, names)
            self.assertNotIn(raw["adapter_ca_key"].encode(), [value for _, value in process_files.values()])
            self.assertEqual(process_files["adapter-ca.pem"][1], (root / "installation.pem").read_bytes())
            leaf = root / "leaf.pem"
            leaf.write_bytes(process_files["adapters/jiracloud/tls-cert.pem"][1])
            for name, passed in (("installation", True), ("foreign", False)):
                proc = subprocess.run([OPENSSL, "verify", "-verify_hostname", "adapter-jiracloud",
                                       "-CAfile", str(root / f"{name}.pem"), str(leaf)],
                                      stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, check=False)
                self.assertEqual(proc.returncode == 0, passed)
            dates = subprocess.check_output([OPENSSL, "x509", "-in", str(leaf), "-noout", "-dates"], text=True)
            start, end = [datetime.strptime(line.split("=", 1)[1], "%b %d %H:%M:%S %Y GMT")
                          for line in dates.splitlines()]
            self.assertEqual((end - start).days, 365)

            stand = rollout_host.Stand(root)
            release = {**manifest(FIRST, 1, adapter=True, adapter_name="jiracloud"), "snapshot": FIRST}
            secrets_dir = stand.path(release) / "secrets"
            with patch.object(os, "fchown"):
                rollout_host.materialize_files(secrets_dir, process_files)
            for name in ("master-key", "service-key", rollout.PREVIOUS_SERVICE_KEY):
                self.assertEqual((secrets_dir / "adapters/jiracloud" / name).stat().st_mode & 0o777, 0o400)
            settings = {"tls_dir": str(root)}
            services = rollout_host.overrides(settings, names, release, secrets_dir)["services"]
            key_env = "ADAPTER_PREVIOUS_SERVICE_KEY_FILE"
            self.assertEqual(services["adapter-jiracloud"]["environment"][key_env], "/run/secrets/previous-service-key")
            self.assertIn(f"{secrets_dir}/adapters/jiracloud/previous-service-key:/run/secrets/previous-service-key:ro",
                          services["adapter-jiracloud"]["volumes"])
            for name in ("api", "worker", "adapter-jiracloud-migrate"):
                self.assertNotIn(key_env, services[name]["environment"])
                self.assertFalse(any("previous-service-key" in volume for volume in services[name]["volumes"]))
            version = json.dumps({"commit_sha": FIRST, "contract_version": "1", "schema_version": 3}).encode()
            with patch.object(stand, "address", return_value="127.0.0.1"), \
                 patch.object(rollout_host, "probe_http", side_effect=[version, b""]) as probe:
                stand.verify(release, "adapter-jiracloud")
            self.assertEqual([args[3] for args, _ in probe.call_args_list], ["/version", "/readyz"])
            context = probe.call_args.args[4]
            self.assertTrue(context.check_hostname)
            self.assertEqual(len(context.get_ca_certs()), 1)
