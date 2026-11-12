import importlib.util
import io
import json
import os
from pathlib import Path
import tempfile
import tomllib
import unittest
from unittest.mock import Mock, patch
from urllib.error import HTTPError


ROOT = Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location("gitlab_manage", ROOT / "deploy/gitlab/manage.py")
manage = importlib.util.module_from_spec(spec)
spec.loader.exec_module(manage)


class FakeGitLab:
    """REST response shapes follow GitLab's documented API, including OIDC arrays."""

    url = "https://gitlab.example.test"

    def __init__(self):
        self.tables = {key: {} for key in ("users", "groups", "projects", "applications",
                                          "runners", "personal_access_tokens")}
        self.members = {}
        self.calls = []
        self.counter = 0

    def pages(self, path):
        resource = path.split("?")[0]
        if resource.startswith("runners/") and resource.endswith("/managers"):
            return iter([{"version": manage.RUNNER_VERSION}])
        if resource.startswith("groups/"):
            group_id = int(resource.split("/")[1])
            return iter([p for p in self.tables["projects"].values() if p["namespace_id"] == group_id])
        return iter(self.tables[resource].values())

    def request(self, method, path, data=None):
        self.calls.append((method, path, data))
        if path == "metadata":
            return {"version": manage.VERSION, "enterprise": False}
        if path == "application/settings":
            return data
        pieces = path.split("/")
        table = pieces[0]
        if "members" in pieces:
            key = "/".join(pieces[:3])
            if method == "POST":
                self.members[key, data["user_id"]] = data["access_level"]
                return data
            lookup = (key, int(pieces[-1]))
            if lookup not in self.members:
                raise manage.APIError(method, 404)
            return {"access_level": self.members[lookup]}
        if path == "user/runners":
            table = "runners"
        if method == "POST" and (len(pieces) == 1 or path == "user/runners"):
            self.counter += 1
            result = dict(data, id=self.counter)
            if table == "users":
                result.update(state="active", is_admin=False)
            elif table == "groups":
                result["full_path"] = data["path"]
            elif table == "applications":
                result.update(application_id="public-client-id-" + str(self.counter),
                              application_name=data["name"], callback_url=data["redirect_uri"],
                              scopes=data["scopes"].split(), secret="test-client-secret")
            elif table == "runners":
                result.update(token="glrt-test-only", projects=[{"id": data["project_id"]}],
                              online=True, tag_list=data['tag_list'].split(','))
            elif table == "projects":
                result['issues_enabled'] = False
            self.tables[table][result["id"]] = result
            return result
        if len(pieces) >= 2:
            object_id = int(pieces[1])
            obj = self.tables[table][object_id]
            if method == "GET":
                return obj
            if method == "PUT":
                obj.update(data)
                return obj
            if pieces[-1] == "block":
                obj["state"] = "blocked"
                return None
            if pieces[-1] == "personal_access_tokens":
                result = dict(data, id=object_id + 1000, token="test-token-" + str(object_id),
                              active=True, revoked=False)
                self.tables['personal_access_tokens'][result['id']] = result
                return result
            if table == "projects" and pieces[-1] == "runners":
                self.tables["runners"][data["runner_id"]]["projects"].append({"id": object_id})
                return None
        raise AssertionError((method, path, data))


class GitLabDeployTests(unittest.TestCase):
    def provision(self, directory, api=None):
        api = api or FakeGitLab()
        provision = manage.Provision(api, Path(directory) / "state.json", "https://app.example.test",
                                    ["org-a", "org-b"], "example.test")
        provision.run()
        return api, provision

    def test_bootstrap_limits_accounts_projects_oidc_and_runner(self):
        with tempfile.TemporaryDirectory() as directory:
            api, provision = self.provision(directory)
            self.assertEqual(8, len(api.tables["users"]))
            for user in api.tables["users"].values():
                self.assertTrue(user["external"])
                self.assertFalse(user["admin"])
            self.assertEqual(4, len(api.tables["projects"]))
            for project in api.tables["projects"].values():
                self.assertFalse(project["shared_runners_enabled"])
                self.assertEqual("disabled", project["issues_access_level"])
            self.assertEqual({"https://app.example.test/o/org-a/callback",
                              "https://app.example.test/o/org-b/callback"},
                             {app["callback_url"] for app in api.tables["applications"].values()})
            for app in api.tables["applications"].values():
                self.assertEqual(["openid", "profile"], app["scopes"])
            runner = next(iter(api.tables["runners"].values()))
            self.assertEqual("project_type", runner["runner_type"])
            self.assertTrue(runner["locked"])
            self.assertFalse(runner["run_untagged"])
            self.assertEqual(set(provision.state["test_project_ids"]), {p["id"] for p in runner["projects"]})
            for role, scopes in (("collector", ["read_api"]), ("clone", ["read_repository"]),
                                 ("mr-bot", ["api"])):
                self.assertEqual(scopes, provision.state["credentials"][role + "-token"]["scopes"])

    def test_resume_does_not_reissue_credentials_or_create_duplicates(self):
        with tempfile.TemporaryDirectory() as directory:
            api, provision = self.provision(directory)
            original_state = json.dumps(provision.state)
            api.calls.clear()
            self.provision(directory, api)
            self.assertFalse(any(method == "POST" for method, _, _ in api.calls))
            self.assertEqual(original_state, json.dumps(json.loads(provision.path.read_text())))

    def test_inspect_matches_real_oidc_response_and_does_not_claim_live_acceptance(self):
        with tempfile.TemporaryDirectory() as directory:
            api, provision = self.provision(directory)
            result = manage.inspect(api, provision.state)
            self.assertEqual('passed', result['configuration_check'])
            self.assertIn('pending', result['live_acceptance'])
            self.assertNotIn('test-token', json.dumps(result))

    def test_inspect_rejects_widened_collector_scope(self):
        with tempfile.TemporaryDirectory() as directory:
            api, provision = self.provision(directory)
            token_id = provision.state['credentials']['collector-token']['id']
            api.tables['personal_access_tokens'][token_id]['scopes'] = ['api']
            with self.assertRaisesRegex(RuntimeError, 'scopes or validity'):
                manage.inspect(api, provision.state)

    def test_inspect_rejects_runner_attached_to_another_project(self):
        with tempfile.TemporaryDirectory() as directory:
            api, provision = self.provision(directory)
            runner = next(iter(api.tables['runners'].values()))
            runner['projects'].append({'id': 99999})
            with self.assertRaisesRegex(RuntimeError, 'isolation differs'):
                manage.inspect(api, provision.state)

    def test_state_cannot_be_reused_for_another_origin(self):
        with tempfile.TemporaryDirectory() as directory:
            api, _ = self.provision(directory)
            with self.assertRaisesRegex(ValueError, "another instance"):
                manage.Provision(api, Path(directory) / "state.json", "https://other.example.test",
                                 ["org-a", "org-b"], "example.test")

    def test_unmanaged_existing_user_is_not_adopted(self):
        with tempfile.TemporaryDirectory() as directory:
            api = FakeGitLab()
            api.tables["users"][999] = {"username": "cp87-collector"}
            with self.assertRaisesRegex(RuntimeError, "without this journal"):
                self.provision(directory, api)
            self.assertFalse(any(method == "POST" for method, _, _ in api.calls))

    def test_wrong_version_prevents_remote_mutations(self):
        with tempfile.TemporaryDirectory() as directory:
            api = FakeGitLab()
            api.request = Mock(return_value={"version": "18.0.0", "enterprise": False})
            with self.assertRaisesRegex(RuntimeError, "pinned GitLab"):
                self.provision(directory, api)
            api.request.assert_called_once_with("GET", "metadata")

    def test_generated_runner_config_is_valid_and_separates_host_socket(self):
        with tempfile.TemporaryDirectory() as directory:
            _, provision = self.provision(directory)
            config = tomllib.loads(manage.runner_config(provision.state, "/etc/cp-ca"))
            self.assertEqual(1, config["concurrent"])
            runner = config["runners"][0]
            self.assertEqual("glrt-test-only", runner["token"])
            self.assertFalse(runner["docker"]["privileged"])
            self.assertEqual("512m", runner["docker"]["memory"])
            self.assertNotIn("docker.sock", str(runner["docker"]["volumes"]))
            self.assertIn("/etc/cp-ca:/etc/craftpulse-ca:ro", runner["docker"]["volumes"])

    def test_secret_output_inside_checkout_is_rejected(self):
        with self.assertRaisesRegex(ValueError, "outside"):
            manage.save_private(ROOT / "deploy/gitlab/test-secret.json", "never write this")

    @unittest.skipIf(os.name == "nt", "POSIX file modes are checked on Linux")
    def test_secret_files_are_owner_only_and_public_directory_is_rejected(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "state.json"
            manage.save_private(path, "private")
            self.assertEqual(0o600, path.stat().st_mode & 0o777)
            os.chmod(directory, 0o755)
            with self.assertRaisesRegex(ValueError, "0700"):
                manage.save_private(path, "private")

    def test_tokens_are_not_forwarded_on_redirect(self):
        request = Mock()
        self.assertIsNone(manage.NoRedirect().redirect_request(
            request, None, 302, "Found", {}, "https://attacker.example.test"))

    def test_remote_error_body_does_not_disclose_secrets(self):
        api = manage.API("https://gitlab.example.test", "secret-admin-token")
        api.opener = Mock()
        api.opener.open.side_effect = HTTPError(api.url, 400, "Bad request", {},
                                               io.BytesIO(b'{"token":"secret-response"}'))
        with self.assertRaises(manage.APIError) as error:
            api.request("POST", "applications", {"secret": "secret-input"})
        self.assertNotIn("secret", str(error.exception))

    def test_only_plain_https_origins_are_accepted(self):
        for value in ("http://host", "https://user:pass@host", "https://host/path",
                      "https://host?token=secret", "https://host/#fragment"):
            with self.subTest(value=value), self.assertRaises(ValueError):
                manage.origin(value)
        self.assertEqual("https://gitlab.example.test", manage.origin("https://gitlab.example.test/"))


if __name__ == "__main__":
    unittest.main()
