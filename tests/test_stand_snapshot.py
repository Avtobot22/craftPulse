import copy
import json
from pathlib import Path
import sys
import tempfile
import unittest
from unittest.mock import Mock, patch


ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "deploy/stand/snapshot"))
import stand_snapshot as snapshot
import stand_sources as sources
import stand_storage as storage


class ManifestTest(unittest.TestCase):
    def setUp(self):
        self.manifest = sources.read_json(snapshot.MANIFEST)

    def test_record_and_release_reproduce_checked_in_manifest(self):
        release = {key: value for key, value in self.manifest["craftpulse"].items() if key != "url"}
        runtime = sources.read_json(snapshot.MANIFEST.with_name("instance.json"))
        self.assertEqual(self.manifest, sources.build_manifest(release, runtime))
        self.assertEqual(set(self.manifest["sources"]), {
            "jira", "confluence", "gitlab", "runner", "github", "jenkins", "sonarqube",
        })
        for record in self.manifest["sources"].values():
            self.assertTrue(record["accounts"])
            self.assertTrue(all("permissions" in account for account in record["accounts"]))
        runtime["runtime"]["craftpulse-product-frontend-1"]["image"] = release["images"]["craftpulse-server"]
        with self.assertRaisesRegex(sources.StandError, "frontend digest"):
            sources.build_manifest(release, runtime)

    def test_version_comparison_passes_and_finds_drift_without_printing_api_values(self):
        expected = self.manifest["sources"]
        stand = sources.Sources(self.manifest)
        clients = {name: Mock() for name in {*expected, "craftpulse"}}
        clients["jira"].get.return_value = {
            "version": expected["jira"]["version"], "deploymentType": "Cloud", "buildNumber": expected["jira"]["build"],
        }
        clients["confluence"].get.return_value = {"commitHash": expected["confluence"]["version"], "edition": "premium"}
        clients["gitlab"].get.side_effect = [
            {"version": expected["gitlab"]["version"] + "-ce", "enterprise": False},
            [{"version": "old", "status": "offline"}, {"version": expected["runner"]["version"], "status": "online"}],
        ] * 2
        clients["github"].request.return_value = (
            b"", {"X-GitHub-Api-Version-Selected": expected["github"]["api_version"]},
        )
        clients["jenkins"].request.return_value = (
            json.dumps({"plugins": [{"shortName": name, "version": version, "active": True}
                                    for name, version in expected["jenkins"]["plugins"].items()]}).encode(),
            {"X-Jenkins": expected["jenkins"]["version"]},
        )
        clients["sonarqube"].request.return_value = (expected["sonarqube"]["version"].encode(), {})
        clients["craftpulse"].get.return_value = {
            "commit_sha": self.manifest["craftpulse"]["commit_sha"],
            "schema_version": self.manifest["craftpulse"]["core_schema_version"],
        }
        with patch.object(stand, "clients", return_value=clients):
            self.assertEqual(stand.compare_versions(), [])
            stand.records["jira"]["version"] = "other version"
            self.assertEqual(stand.compare_versions(), ["jira: differs at version"])
        clients["github"].request.assert_called_with(
            "/meta", extra={"X-GitHub-Api-Version": expected["github"]["api_version"]},
        )

    def test_config_cannot_redirect_credentials(self):
        self.manifest["sources"]["jira"]["url"] = "https://example.org"
        with self.assertRaisesRegex(sources.StandError, "unfamiliar url"):
            sources.Sources(self.manifest)


class FixtureTest(unittest.TestCase):
    def test_atlassian_reset_reverts_edits_and_removes_marked_additions(self):
        initial = {"jira": {"1": {"summary": "before", "description": None, "labels": [],
                                  "status": "100", "security": None, "project": "CPA", "key": "CPA-1"}},
                   "confluence": {"2": {"title": "before", "body": "old", "spaceId": "360451",
                                        "status": "current", "parentId": None}}}
        changed = copy.deepcopy(initial)
        changed["jira"]["1"]["summary"] = "after"
        changed["jira"]["4"] = {**changed["jira"]["1"], "summary": "cp92-reset-extra", "key": "CPA-4"}
        changed["confluence"]["2"]["body"] = "new"
        clients = {"jira": Mock(), "confluence": Mock()}
        clients["confluence"].get.return_value = {"version": {"number": 3}}
        actions = sources.cloud_plan(initial, changed)
        sources.apply_cloud(clients, actions)
        self.assertEqual([action[:3] for action in actions],
                         [("jira", "1", "update"), ("jira", "4", "delete"), ("confluence", "2", "update")])
        self.assertEqual(clients["jira"].request.call_args_list[0].kwargs["data"]["fields"]["summary"], "before")
        self.assertEqual(clients["jira"].request.call_args_list[1].kwargs["method"], "DELETE")
        self.assertEqual(clients["confluence"].request.call_args.kwargs["data"]["version"]["number"], 4)
        self.assertEqual(sources.cloud_plan(initial, initial), [])
        changed["jira"]["4"]["summary"] = "unrelated"
        with self.assertRaisesRegex(sources.StandError, "unmarked"):
            sources.cloud_plan(initial, changed)
        changed["jira"]["4"]["summary"] = "cp92-reset-extra"
        changed["jira"]["1"]["security"] = "unexpected"
        with self.assertRaisesRegex(sources.StandError, "non-editable"):
            sources.cloud_plan(initial, changed)

    def test_confluence_pagination_never_follows_returned_host(self):
        client = Mock()
        client.get.side_effect = [
            {"results": [1], "_links": {"next": "https://elsewhere.test/wiki/api/v2/spaces/42/pages?cursor=abc"}},
            {"results": [2], "_links": {}},
        ]
        self.assertEqual(list(sources.confluence_pages(client, "42")), [1, 2])
        self.assertEqual(client.get.call_args.args[0], "/api/v2/spaces/42/pages?cursor=abc")


class CheckpointTest(unittest.TestCase):
    def test_empty_bucket_object_is_encrypted_and_verified_without_a_ranged_get(self):
        bucket = object.__new__(storage.Bucket)
        bucket.keys = Mock(return_value={"empty": 0})
        bucket.download = Mock(side_effect=AssertionError("invalid ranged GET"))
        archive = Mock()
        self.assertEqual(bucket.backup(archive), 1)
        archive.save.assert_any_call("object-00000000", b"")
        name, body = archive.save.call_args.args
        self.assertEqual(name, "bucket.json")
        records = json.loads(body)
        self.assertEqual(records[0]["sha256"], storage.sha256(b"").hexdigest())
        bucket.verify(records)
        bucket.download.assert_not_called()

    def test_changed_files_are_rejected_before_restore(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            manifest = root / "manifest.json"
            manifest.write_text("{}", encoding="utf-8")
            checkpoint = {"format": 1, "files": {"manifest.json": storage.digest(manifest)}}
            path = root / snapshot.CHECKPOINT
            path.write_bytes(sources.json_bytes(checkpoint))
            checksum = storage.digest(path)
            self.assertEqual(snapshot.open_snapshot(root, checksum), checkpoint)
            manifest.write_text('{"changed": true}', encoding="utf-8")
            with self.assertRaisesRegex(sources.StandError, "file SHA-256 differs"):
                snapshot.open_snapshot(root, checksum)

    def test_bucket_inventory_requires_exact_archive_list_and_unique_keys(self):
        databases = ["craftpulse", "adapter_git"]
        names = ["object-00000000", "object-00000001"]
        entries = [{"file": name, "key": "dir/" + name, "size": 1, "sha256": "f" * 64} for name in names]
        checkpoint = {"object_count": 2, "files": dict.fromkeys(snapshot.required_files(databases, names))}
        snapshot.require_objects(entries, checkpoint, databases)
        entries[1]["key"] = entries[0]["key"]
        with self.assertRaisesRegex(sources.StandError, "invalid bucket object"):
            snapshot.require_objects(entries, checkpoint, databases)


if __name__ == "__main__":
    unittest.main()
