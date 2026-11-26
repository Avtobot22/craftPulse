import io
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
import urllib.error
from unittest.mock import Mock, patch

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "scripts"))
import oci_release as release


SHA = "a" * 40
DIGEST = "sha256:" + "b" * 64


class ReleaseTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        for name in ["backend/cmd/adapter-git/main.go", "backend/cmd/adapter-test/main.go", "backend/Dockerfile",
                     "backend/adapter.Dockerfile", "backend/ci.Dockerfile", "frontend/Dockerfile", "backend/migrations/core/00001_init.sql",
                     "backend/migrations/core/00019_latest.sql", "backend/migrations/adapter/00002_latest.sql"]:
            path = self.root / name
            path.parent.mkdir(parents=True, exist_ok=True)
            path.touch()
        contract = self.root / "backend/pkg/adaptersdk/errors.go"
        contract.parent.mkdir(parents=True)
        contract.write_text('const CurrentContractVersion = "1"')
        release.write_json(self.root / "frontend/package-lock.json", {"packages": {
            "": {"name": "frontend"}, "node_modules/react": {"version": "19.3.1", "license": "MIT"}}})

    def test_discovery_includes_new_adapters_without_workflow_edits(self):
        path = self.root / "backend/cmd/adapter-future/main.go"
        path.parent.mkdir()
        path.touch()
        names = [image["name"] for image in release.plan(self.root)]
        self.assertEqual(names, ["craftpulse-server", "craftpulse-frontend", "craftpulse-ci", "craftpulse-adapter-future", "craftpulse-adapter-git"])
        self.assertNotIn("craftpulse-adapter-test", names)

    def test_bake_keeps_each_image_target_tag_revision_and_cache_scope(self):
        images = [
            {"name": "craftpulse-ci", "context": "backend", "dockerfile": "backend/ci.Dockerfile"},
            {"name": "craftpulse-ci-trivy-default", "context": "backend",
             "dockerfile": "backend/ci.Dockerfile", "target": "trivy-default"},
        ]
        with patch.dict(release.os.environ, {"CRAFTPULSE_BUILDKIT_CACHE": "gha",
                                             "CRAFTPULSE_BUILDKIT_CACHE_WRITE": "1"}):
            with patch.object(release, "run") as run:
                release.build_profile_images(images, SHA, self.root)
        args, kwargs = run.call_args
        self.assertEqual(args[0], ["docker", "buildx", "bake", "--file", "-", "--load", "--progress", "plain"])
        self.assertEqual(kwargs["cwd"], self.root)
        definition = json.loads(kwargs["input_text"])
        self.assertEqual(definition["group"]["default"]["targets"], [image["name"] for image in images])
        for image in images:
            name = image["name"]
            target = definition["target"][name]
            self.assertEqual(target["context"], "backend")
            self.assertEqual(target["dockerfile"], "ci.Dockerfile")
            self.assertEqual(target.get("target"), image.get("target"))
            self.assertEqual(target["tags"], [name + ":check"])
            self.assertEqual(target["platforms"], ["linux/amd64"])
            self.assertEqual(target["args"], {"COMMIT_SHA": SHA})
            self.assertEqual(target["labels"], {"org.opencontainers.image.revision": SHA})
            self.assertIn("scope=craftpulse-amd64-" + name + ",", target["cache-from"][0])
            self.assertIn("scope=craftpulse-amd64-" + name + ",", target["cache-to"][0])

    def test_profile_smoke_failure_fails_the_group(self):
        profile = self.root / "backend/pkg/secrets/profiles/gitleaks-default/profile.json"
        profile.parent.mkdir(parents=True)
        profile.touch()
        failure = subprocess.CalledProcessError(1, ["check"])
        with patch.object(release, "build_profile_images") as build:
            with patch.object(release, "check_profile_image", side_effect=[None, failure]) as check:
                with self.assertRaises(subprocess.CalledProcessError):
                    release.check_ci_images(SHA, self.root)
        names = ["craftpulse-ci", "craftpulse-ci-gitleaks-default"]
        self.assertEqual([image["name"] for image in build.call_args.args[0]], names)
        self.assertCountEqual([call.args[0]["name"] for call in check.call_args_list], names)

    def test_profile_discovery_pins_dependency_and_hashes_policy(self):
        directory = self.root / "backend/pkg/secrets/profiles/gitleaks-default"
        directory.mkdir(parents=True)
        release.write_json(directory / "profile.json", {"gitleaks_version": "8.30.1", "min_fragment_length": 8})
        (directory / "rules.toml").write_text("# rules\n")
        (self.root / "backend/go.mod").write_text("require github.com/zricethezav/gitleaks/v8 v8.30.1\n")
        images = release.plan(self.root)
        self.assertEqual(next(i for i in images if i.get("target"))["name"], "craftpulse-ci-gitleaks-default")
        metadata = release.metadata(self.root, images)
        self.assertEqual(metadata["ci_tool_versions"], {"gitleaks": "8.30.1"})
        original = metadata["rule_hashes"]["gitleaks-default"]
        (directory / "rules.toml").write_text("# changed rules\n")
        self.assertNotEqual(release.metadata(self.root, images)["rule_hashes"]["gitleaks-default"], original)
        (self.root / "backend/go.mod").write_text("require github.com/zricethezav/gitleaks/v8 v8.29.0\n")
        with self.assertRaisesRegex(release.ReleaseError, "versions disagree"):
            release.metadata(self.root, images)

    def test_go_profile_versions_match_image_recipe_and_rules_are_hashed(self):
        directory = self.root / release.PROFILES / "go-default"
        directory.mkdir(parents=True)
        tools = {"golangci-lint": "2.15.0", "lizard": "1.26.0", "jscpd": "5.4.2"}
        release.write_json(directory / "profile.json", {"profile_id": "go-default", "tools": tools})
        (directory / "golangci.yml").write_text('version: "2"\n')
        (directory / "jscpd.json").write_text("{}\n")
        (directory / "lizard-requirements.txt").write_text("lizard==1.26.0 --hash=sha256:" + "0" * 64 + "\n")
        recipe = ("ARG GOLANGCI_LINT_VERSION=2.15.0\n"
                  "ADD https://registry.npmjs.org/jscpd-linux-x64-musl/-/jscpd-linux-x64-musl-5.4.2.tgz /jscpd.tgz\n"
                  "ADD https://registry.npmjs.org/jscpd-linux-arm64-musl/-/jscpd-linux-arm64-musl-5.4.2.tgz /jscpd.tgz\n")
        (self.root / "backend/ci.Dockerfile").write_text(recipe)
        images = release.plan(self.root)
        self.assertIn({"name": "craftpulse-ci-go-default", "context": "backend", "dockerfile": "backend/ci.Dockerfile",
                       "target": "go-default"}, images)
        metadata = release.metadata(self.root, images)
        self.assertEqual(metadata["ci_tool_versions"], tools)
        original = metadata["rule_hashes"]["go-default"]
        (directory / "golangci.yml").write_text('version: "2"\nlinters: {default: none}\n')
        self.assertNotEqual(release.metadata(self.root, images)["rule_hashes"]["go-default"], original)
        (self.root / "backend/ci.Dockerfile").write_text(recipe.replace("arm64-musl-5.4.2", "arm64-musl-5.4.1"))
        with self.assertRaisesRegex(release.ReleaseError, "versions disagree"):
            release.metadata(self.root, images)

    def test_trivy_profile_pins_version_and_database_digest(self):
        directory = self.root / release.TRIVY_PROFILE
        directory.mkdir(parents=True)
        release.write_json(directory / "profile.json", {"profile_id": "trivy-default", "tools": {"trivy": "0.76.2"}})
        (directory / "trivy.yaml").write_text("{}\n")
        (directory / "trivyignore").write_text("KSV-0125\n")
        database = "ghcr.io/aquasecurity/trivy-db@sha256:" + "5" * 64
        recipe = ("ADD https://github.com/aquasecurity/trivy/releases/download/v0.76.2/trivy_0.76.2_Linux-64bit.tar.gz /trivy.tgz\n"
                  "ADD https://github.com/aquasecurity/trivy/releases/download/v0.76.2/trivy_0.76.2_Linux-ARM64.tar.gz /trivy.tgz\n"
                  f"ARG TRIVY_DB_REPOSITORY={database}\n")
        (self.root / "backend/ci.Dockerfile").write_text(recipe)
        images = release.plan(self.root)
        self.assertEqual([i["name"] for i in images if i.get("target")], ["craftpulse-ci-trivy-default"])
        metadata = release.metadata(self.root, images)
        self.assertEqual(metadata["ci_tool_versions"], {"trivy": "0.76.2", "trivy-db": database})
        original = metadata["rule_hashes"]["trivy-default"]
        (directory / "trivyignore").write_text("")
        self.assertNotEqual(release.metadata(self.root, images)["rule_hashes"]["trivy-default"], original)
        (self.root / "backend/ci.Dockerfile").write_text(recipe.replace("@sha256:" + "5" * 64, ":2"))
        with self.assertRaisesRegex(release.ReleaseError, "not pinned by digest"):
            release.metadata(self.root, images)
        (self.root / "backend/ci.Dockerfile").write_text(recipe.replace("v0.76.2/trivy_0.76.2_Linux-ARM64", "v0.76.1/trivy_0.76.1_Linux-ARM64"))
        with self.assertRaisesRegex(release.ReleaseError, "Trivy versions disagree"):
            release.metadata(self.root, images)

    def test_docs_profile_pins_spectral_in_lockfile_and_goldmark_in_go_mod(self):
        directory = self.root / release.DOCS_PROFILE
        directory.mkdir(parents=True)
        release.write_json(directory / "profile.json", {"tools": {"craftpulse-docs": "1", "goldmark": "1.8.7", "spectral": "6.17.1"}})
        (directory / "spectral.yaml").write_text('extends: ["spectral:oas"]\n')
        lock = {"packages": {"": {}, "node_modules/@stoplight/spectral-cli": {"version": "6.17.1"}}}
        release.write_json(directory / "package-lock.json", lock)
        (self.root / "backend/go.mod").write_text("require (\n\tgithub.com/yuin/goldmark v1.8.7\n)\n")
        images = release.plan(self.root)
        self.assertEqual([i["name"] for i in images if i.get("target")], ["craftpulse-ci-git-markdown-openapi"])
        self.assertEqual(release.PROFILE_CHECKS["git-markdown-openapi"], ["scripts/check_docs_profile.py"])
        metadata = release.metadata(self.root, images)
        self.assertEqual(metadata["ci_tool_versions"], {"spectral": "6.17.1", "goldmark": "1.8.7"})
        original = metadata["rule_hashes"]["git-markdown-openapi"]
        (directory / "spectral.yaml").write_text('extends: ["spectral:oas", "spectral:asyncapi"]\n')
        self.assertNotEqual(release.metadata(self.root, images)["rule_hashes"]["git-markdown-openapi"], original)
        (self.root / "backend/go.mod").write_text("require github.com/yuin/goldmark v1.8.6\n")
        with self.assertRaisesRegex(release.ReleaseError, "docs profile and image tool versions disagree"):
            release.metadata(self.root, images)

    def test_python_and_js_profiles_pin_tools_in_requirements_and_lockfile(self):
        recipe = ("ADD https://registry.npmjs.org/jscpd-linux-x64-musl/-/jscpd-linux-x64-musl-5.4.2.tgz /jscpd.tgz\n"
                  "ADD https://registry.npmjs.org/jscpd-linux-arm64-musl/-/jscpd-linux-arm64-musl-5.4.2.tgz /jscpd.tgz\n")
        (self.root / "backend/ci.Dockerfile").write_text(recipe)
        python = self.root / release.PROFILES / "python-default"
        python.mkdir(parents=True)
        release.write_json(python / "profile.json", {"tools": {"ruff": "0.17.4", "lizard": "1.26.0", "jscpd": "5.4.2"}})
        (python / "ruff.toml").write_text("[lint]\n")
        (python / "jscpd.json").write_text("{}\n")
        (python / "requirements.txt").write_text("lizard==1.26.0 --hash=sha256:" + "0" * 64 + "\n"
                                                 "ruff==0.17.4 --hash=sha256:" + "1" * 64 + " --hash=sha256:" + "2" * 64 + "\n")
        js = self.root / release.PROFILES / "js-default"
        js.mkdir(parents=True)
        js_tools = {"eslint": "10.14.0", "typescript-eslint": "8.72.2", "typescript": "6.0.3", "lizard": "1.26.0", "jscpd": "5.4.2"}
        release.write_json(js / "profile.json", {"tools": js_tools})
        (js / "eslint.config.mjs").write_text("export default [];\n")
        (js / "jscpd.json").write_text("{}\n")
        (js / "lizard-requirements.txt").write_text("lizard==1.26.0 --hash=sha256:" + "0" * 64 + "\n")
        lock = {"packages": {"": {"name": "craftpulse-js-default"}, "node_modules/eslint": {"version": "10.14.0"},
                             "node_modules/typescript-eslint": {"version": "8.72.2"}, "node_modules/typescript": {"version": "6.0.3"},
                             "node_modules/eslint/node_modules/typescript": {"version": "5.0.0"}}}
        release.write_json(js / "package-lock.json", lock)
        images = release.plan(self.root)
        self.assertEqual([i["name"] for i in images if i.get("target")], ["craftpulse-ci-python-default", "craftpulse-ci-js-default"])
        metadata = release.metadata(self.root, images)
        self.assertEqual(metadata["ci_tool_versions"], {**js_tools, "ruff": "0.17.4"})
        self.assertEqual(release.PROFILE_CHECKS["js-default"], ["scripts/check_code_profile.py", "--profile", "js-default"])
        original = metadata["rule_hashes"]["js-default"]
        (js / "eslint.config.mjs").write_text('export default [{ rules: { "no-undef": "error" } }];\n')
        self.assertNotEqual(release.metadata(self.root, images)["rule_hashes"]["js-default"], original)
        lock["packages"]["node_modules/typescript"]["version"] = "6.0.2"
        release.write_json(js / "package-lock.json", lock)
        with self.assertRaisesRegex(release.ReleaseError, "js-default profile and image tool versions disagree"):
            release.metadata(self.root, images)
        lock["packages"]["node_modules/typescript"]["version"] = "6.0.3"
        release.write_json(js / "package-lock.json", lock)
        (js / "lizard-requirements.txt").write_text("lizard==1.25.0 --hash=sha256:" + "0" * 64 + "\n")
        release.write_json(js / "profile.json", {"tools": {**js_tools, "lizard": "1.25.0"}})
        with self.assertRaisesRegex(release.ReleaseError, "pin lizard at different versions"):
            release.metadata(self.root, images)

    def test_sast_profile_version_rule_hash_and_sources_reach_the_manifest(self):
        directory = self.root / release.SAST_PROFILE
        (directory / "rules").mkdir(parents=True)
        release.write_json(directory / "profile.json", {"profile_id": "opengrep-default", "tools": {"opengrep": "1.33.1"}})
        sources = [{"id": "go-eval", "file": "rules/go.yaml", "source": "craftPulse", "source_version": "1", "license": "LicenseRef-craftPulse"}]
        release.write_json(directory / "sources.json", {"rules": sources})
        (directory / "rules/go.yaml").write_text("rules: []\n")
        recipe = ("ADD https://github.com/opengrep/opengrep/releases/download/v1.33.1/opengrep_musllinux_x86 /opengrep\n"
                  "ADD https://github.com/opengrep/opengrep/releases/download/v1.33.1/opengrep_musllinux_aarch64 /opengrep\n")
        (self.root / "backend/ci.Dockerfile").write_text(recipe)
        images = release.plan(self.root)
        self.assertIn("craftpulse-ci-opengrep-default", [image["name"] for image in images])
        metadata = release.metadata(self.root, images)
        self.assertEqual(metadata["ci_tool_versions"]["opengrep"], "1.33.1")
        self.assertEqual(metadata["rule_sources"], {"opengrep-default": sources})
        original = metadata["rule_hashes"]["opengrep-default"]
        (directory / "rules/go.yaml").write_text("rules: [{id: go-eval}]\n")
        self.assertNotEqual(release.metadata(self.root, images)["rule_hashes"]["opengrep-default"], original)
        (self.root / "backend/ci.Dockerfile").write_text(recipe.replace("v1.33.1/opengrep_musllinux_aarch64", "v1.33.0/opengrep_musllinux_aarch64"))
        with self.assertRaisesRegex(release.ReleaseError, "Opengrep versions disagree"):
            release.metadata(self.root, images)

    def test_core_and_adapter_versions_come_from_distinct_migrations(self):
        metadata = release.metadata(self.root, release.plan(self.root))
        self.assertEqual(metadata["core_schema_version"], 19)
        self.assertEqual(metadata["adapter_schema_versions"], {"craftpulse-adapter-git": 2})
        self.assertEqual(metadata["adapter_contract_version"], "1")

    def test_no_adapters_means_empty_adapter_metadata(self):
        (self.root / "backend/cmd/adapter-git/main.go").unlink()
        (self.root / "backend/cmd/adapter-git").rmdir()
        metadata = release.metadata(self.root, release.plan(self.root))
        self.assertEqual(metadata["adapter_schema_versions"], {})
        self.assertIsNone(metadata["adapter_contract_version"])
        self.assertEqual(metadata["ci_tool_versions"], {})
        self.assertEqual(metadata["rule_hashes"], {})

    def test_incomplete_adapter_is_not_silently_omitted(self):
        (self.root / "backend/cmd/adapter-git/main.go").unlink()
        with self.assertRaisesRegex(release.ReleaseError, "invalid adapter"):
            release.plan(self.root)

    def test_missing_license_remains_unknown_and_nested_npm_names_survive(self):
        inventory = release.npm_licenses({"packages": {
            "node_modules/@scope/pkg": {"version": "2", "dev": True},
            "node_modules/a/node_modules/b": {"version": "3", "license": "Apache-2.0"},
            "node_modules/local": {"link": True, "resolved": "tools/local"}}})
        self.assertEqual(inventory[0], {"name": "@scope/pkg", "version": "2", "license": "NOASSERTION", "dev": True})
        self.assertEqual(inventory[1]["name"], "b")

    def test_sbom_keeps_go_and_os_licenses_and_unknown(self):
        inventory = release.image_licenses({"artifacts": [
            {"name": "go/module", "version": "1", "type": "go-module", "licenses": [{"spdxExpression": "MIT"}]},
            {"name": "alpine-package", "version": "2", "type": "apk", "licenses": []}]})
        self.assertEqual(inventory[0]["licenses"], ["NOASSERTION"])
        self.assertEqual(inventory[1]["licenses"], ["MIT"])

    @patch.object(release, "run")
    def test_scan_failure_scans_remaining_images_and_blocks_publish(self, run):
        images = release.plan(self.root)
        output = self.root / "reports"
        output.mkdir()

        def execute(args, **kwargs):
            if args[0].endswith("trivy"):
                (Path(args[args.index("--output") + 1])).write_text('{"Results": []}')
                if "craftpulse-server" in args[-1]:
                    raise subprocess.CalledProcessError(1, args)
            else:
                release.write_json(Path(args[-1].split("=", 1)[1]), {"artifacts": []})

        run.side_effect = execute
        with self.assertRaisesRegex(release.ReleaseError, "Trivy blocked"):
            release.scan(images, {image["name"]: image["name"] for image in images}, output, self.root / "tools")
        self.assertEqual(len(list(output.glob("*.trivy.json"))), len(images))
        self.assertFalse(any(call.args[0][:2] == ["docker", "push"] for call in run.call_args_list))

    @patch.object(release, "run")
    def test_existing_sha_tags_are_never_pushed(self, run):
        images = release.plan(self.root)
        registry = Mock()
        registry.resolve.return_value = DIGEST
        release.publish_images(images, SHA, {}, {image["name"]: DIGEST for image in images}, registry)
        run.assert_not_called()

    @patch.object(release, "run")
    def test_tag_appearing_during_build_requires_fresh_scan(self, run):
        image = release.plan(self.root)[0]
        registry = Mock()
        registry.resolve.return_value = DIGEST
        with self.assertRaisesRegex(release.ReleaseError, "tag appeared"):
            release.publish_images([image], SHA, {image["name"]: "tag"}, {image["name"]: None}, registry)
        run.assert_not_called()

    @patch.object(release, "run")
    def test_published_content_must_match_scanned_image(self, run):
        image = release.plan(self.root)[0]
        registry = Mock()
        registry.resolve.side_effect = [None, DIGEST]
        run.side_effect = ["scanned-id", None, None, "other-id"]
        with self.assertRaisesRegex(release.ReleaseError, "differs from scanned"):
            release.publish_images([image], SHA, {image["name"]: "tag"}, {image["name"]: None}, registry)

    @patch.object(release, "scan")
    @patch.object(release, "build")
    @patch.object(release, "run")
    @patch.object(release, "Registry")
    def test_rerun_reuses_original_manifest_bytes_and_licenses(self, registry_type, run, build, scan):
        images = release.plan(self.root)
        metadata = release.metadata(self.root, images)
        original = {"schema_version": 1, "commit_sha": SHA, **metadata,
                    "images": {image["name"]: f"ghcr.io/avtobot22/{image['name']}@{DIGEST}" for image in images},
                    "licenses": {"images": {"original": ["MIT"]}, "frontend_npm": ["original"]}}
        original_bytes = json.dumps(original, indent=4).encode()
        registry_type.return_value.resolve.return_value = DIGEST
        output = self.root / "output"

        def execute(args, **kwargs):
            if args[:2] == ["git", "rev-parse"]:
                return SHA
            if args[0].endswith("oras") and args[1] == "pull":
                original_dir = Path(args[args.index("--output") + 1])
                original_dir.mkdir(parents=True)
                (original_dir / "release-manifest.json").write_bytes(original_bytes)
            if args[:3] == ["docker", "image", "inspect"]:
                return json.dumps({"org.opencontainers.image.revision": SHA})

        run.side_effect = execute
        scan.return_value = {"changed": ["Apache-2.0"]}
        with patch.dict("os.environ", {"GITHUB_REPOSITORY": release.REPOSITORY, "GITHUB_ACTOR": "actor", "RELEASE_TOKEN": "secret"}):
            release.release(SHA, output, self.root / "tools", self.root)
        self.assertEqual((output / "release-manifest.json").read_bytes(), original_bytes)
        build.assert_not_called()
        self.assertFalse(any(call.args[0][1] == "push" for call in run.call_args_list if len(call.args[0]) > 1))

    def test_stored_manifest_with_changed_registry_digest_is_rejected(self):
        images = release.plan(self.root)
        metadata = release.metadata(self.root, images)
        original = {"schema_version": 1, "commit_sha": SHA, **metadata, "images": {}}
        with self.assertRaisesRegex(release.ReleaseError, "disagree"):
            release.validate_original(original, SHA, images, {image["name"]: DIGEST for image in images}, metadata)

    @patch.object(release, "publish_images")
    @patch.object(release, "scan")
    @patch.object(release, "smoke_adapter")
    @patch.object(release, "build")
    @patch.object(release, "run")
    @patch.object(release, "Registry")
    def test_adapter_runtime_failure_blocks_scanning_and_publication(self, registry_type, run, build, smoke, scan, publish):
        registry_type.return_value.resolve.return_value = None
        smoke.side_effect = subprocess.CalledProcessError(255, ["ssh", "-G"])

        def execute(args, **kwargs):
            if args[:2] == ["git", "rev-parse"]:
                return SHA
            if args[:3] == ["docker", "image", "inspect"]:
                return json.dumps({"org.opencontainers.image.revision": SHA})

        run.side_effect = execute
        with patch.dict("os.environ", {"GITHUB_REPOSITORY": release.REPOSITORY, "GITHUB_ACTOR": "actor", "RELEASE_TOKEN": "secret"}):
            with self.assertRaises(subprocess.CalledProcessError):
                release.release(SHA, self.root / "output", self.root / "tools", self.root)
        smoke.assert_called_once_with(f"ghcr.io/avtobot22/craftpulse-adapter-git:{SHA}")
        scan.assert_not_called()
        publish.assert_not_called()

    @patch.object(release.urllib.request, "urlopen")
    def test_registry_only_404_means_absent(self, urlopen):
        token_response = io.BytesIO(b'{"token":"opaque"}')
        missing = urllib.error.HTTPError("https://ghcr.io/manifest", 404, "not found", {}, None)
        urlopen.side_effect = [token_response, missing]
        self.assertIsNone(release.Registry("actor", "secret").resolve("craftpulse-server", SHA))
        for status in [401, 403, 429, 500, 503]:
            with self.subTest(status=status):
                urlopen.side_effect = [io.BytesIO(b'{"token":"opaque"}'), urllib.error.HTTPError("https://ghcr.io/manifest", status, "error", {}, None)]
                with self.assertRaises(urllib.error.HTTPError):
                    release.Registry("actor", "secret").resolve("craftpulse-server", SHA)

    @patch.object(release.urllib.request, "urlopen")
    def test_invalid_tool_checksum_does_not_install_executable(self, urlopen):
        config = self.root / ".github/release-tools.json"
        config.parent.mkdir()
        release.write_json(config, {"trivy": {"url": "https://github.com/tool.tar.gz", "sha256": "0" * 64}})
        urlopen.return_value = io.BytesIO(b"untrusted")
        with self.assertRaisesRegex(release.ReleaseError, "checksum"):
            release.install_tools(self.root / "tools", self.root)
        self.assertFalse((self.root / "tools/trivy").exists())

    @patch.object(release.subprocess, "run")
    def test_build_subprocess_does_not_inherit_publication_token(self, subprocess_run):
        with patch.dict("os.environ", {"RELEASE_TOKEN": "secret", "GH_TOKEN": "other", "GITHUB_TOKEN": "third"}):
            release.run(["docker", "build", "."])
        environment = subprocess_run.call_args.kwargs["env"]
        self.assertFalse({"RELEASE_TOKEN", "GH_TOKEN", "GITHUB_TOKEN"} & environment.keys())


if __name__ == "__main__":
    unittest.main()
