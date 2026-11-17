import copy
import importlib.util
import io
import json
from pathlib import Path
import unittest
from unittest.mock import MagicMock, Mock, patch

ROOT = Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location("github_manage", ROOT / "deploy/github/manage.py")
manage = importlib.util.module_from_spec(spec)
spec.loader.exec_module(manage)


def fixture():
    repos = {}
    for i, (name, tenant) in enumerate(manage.REPOSITORIES.items(), 1):
        repos[name] = {"name": name, "id": i, "tenant": tenant, "private": True,
                       "default_branch": "main", "branches": {"main": "a" * 40, "cp89-demo": "b" * 40},
                       "pulls": [{"id": i * 10, "number": 1, "created_at": "2026-11-17T10:00:00Z",
                                  "state": "open", "merged": False, "draft": False,
                                  "base": {"ref": "main", "sha": "a" * 40},
                                  "head": {"ref": "cp89-demo", "sha": "b" * 40},
                                  "reviewers": ["reviewer-a"], "teams": ["reviewers"],
                                  "comments": [{"id": i * 100, "created_at": "2026-11-17T10:01:00Z",
                                                "author": "bot", "body": "original"}],
                                  "reviews": [], "review_comments": []}]}
    return {"schema": 1, "organization": manage.ORGANIZATION,
            "organization_id": manage.ORGANIZATION_ID, "api_version": manage.API_VERSION,
            "repositories": repos}


class GitHubResetTests(unittest.TestCase):
    def setUp(self):
        self.expected = fixture()
        self.actual = copy.deepcopy(self.expected)
        self.repo = self.actual["repositories"]["cp89-org-a"]
        self.pull = self.repo["pulls"][0]

    def plan(self, authors=("bot",)):
        return manage.reset_plan(self.expected, self.actual, set(authors))

    def test_second_reset_does_not_write(self):
        self.assertEqual([], self.plan())
        api = Mock()
        with patch.object(manage, "snapshot", return_value=self.expected), patch("sys.stdout", io.StringIO()):
            self.assertFalse(manage.reset(api, self.expected, apply=True))
        api.request.assert_not_called()

    def test_three_changes_restore_sha_reviewers_and_comments_without_recreating_pr(self):
        self.repo["branches"]["cp89-demo"] = "c" * 40
        self.pull["head"]["sha"] = "c" * 40
        self.pull["reviewers"] = ["reviewer-b"]
        self.pull["comments"].append({"id": 999, "created_at": "later", "author": "bot", "body": "probe"})
        plan = self.plan()
        self.assertIn(("PATCH", "repos/craftpulse-mvp/cp89-org-a/git/refs/heads/cp89-demo",
                       {"sha": "b" * 40, "force": True}), plan)
        self.assertIn(("DELETE", "repos/craftpulse-mvp/cp89-org-a/issues/comments/999", None), plan)
        self.assertEqual(4, len(plan))
        self.assertFalse(any(path.endswith("/pulls") or (data and "state" in data)
                             for _, path, data in plan))
        self.assertEqual(self.expected["repositories"]["cp89-org-a"]["pulls"][0]["created_at"],
                         self.pull["created_at"])

    def test_existing_comment_is_updated_in_place(self):
        self.pull["comments"][0]["body"] = "changed"
        self.assertEqual([("PATCH", "repos/craftpulse-mvp/cp89-org-a/issues/comments/100",
                           {"body": "original"})], self.plan())

    def test_missing_baseline_comment_is_irrecoverable(self):
        self.pull["comments"] = []
        with self.assertRaisesRegex(RuntimeError, "identity cannot be restored"):
            self.plan()

    def test_unexpected_human_comments_are_preserved(self):
        self.pull["comments"].append({"id": 999, "created_at": "later", "author": "human", "body": "keep"})
        with self.assertRaisesRegex(RuntimeError, "author"):
            self.plan()

    def test_all_repositories_preflight_before_any_write(self):
        self.repo["branches"]["cp89-demo"] = "c" * 40
        self.actual["repositories"]["cp89-org-b"]["pulls"][0]["merged"] = True
        api = Mock()
        with patch.object(manage, "snapshot", return_value=self.actual), patch("sys.stdout", io.StringIO()):
            with self.assertRaises(RuntimeError):
                manage.reset(api, self.expected, apply=True)
        api.request.assert_not_called()

    def test_replaced_repository_and_pr_are_rejected(self):
        for target, key in ((self.repo, "id"), (self.pull, "id"), (self.pull, "created_at")):
            original = target[key]
            target[key] = 999
            with self.assertRaises(RuntimeError):
                self.plan()
            target[key] = original

    def test_extra_pr_closed_pr_and_submitted_review_are_rejected(self):
        for change in (lambda: self.repo["pulls"].append(dict(self.pull, number=2)),
                       lambda: self.pull.update(state="closed"),
                       lambda: self.pull.update(reviews=[{"id": 15}]),
                       lambda: self.pull.update(review_comments=[{"id": 16}])):
            self.setUp()
            change()
            with self.assertRaises(RuntimeError):
                self.plan()

    def test_extra_branch_is_deleted_missing_branch_is_recreated(self):
        del self.repo["branches"]["cp89-demo"]
        self.repo["branches"]["temporary"] = "c" * 40
        plan = self.plan()
        self.assertIn(("POST", "repos/craftpulse-mvp/cp89-org-a/git/refs",
                       {"ref": "refs/heads/cp89-demo", "sha": "b" * 40}), plan)
        self.assertIn(("DELETE", "repos/craftpulse-mvp/cp89-org-a/git/refs/heads/temporary", None), plan)

    def test_tampered_baseline_and_wrong_sha_are_rejected(self):
        for alter in (lambda b: b.update(organization="Avtobot22"),
                      lambda b: b.update(api_version="latest"),
                      lambda b: b["repositories"]["cp89-org-a"]["branches"].update(main="oops"),
                      lambda b: b["repositories"]["cp89-org-a"].update(private=False)):
            baseline = copy.deepcopy(self.expected)
            alter(baseline)
            with self.assertRaises(ValueError):
                manage.validate(baseline)

    def test_comparison_never_prints_comment_contents(self):
        self.pull["comments"][0]["body"] = "do-not-log-this"
        differences = manage.changes(self.expected, self.actual)
        self.assertEqual(["/repositories/cp89-org-a/pulls/0/comments/0/body"], differences)
        self.assertNotIn("do-not-log-this", json.dumps(differences))

    def test_failed_final_comparison_is_an_error(self):
        api = Mock()
        with patch.object(manage, "snapshot", side_effect=[self.expected, self.actual]), patch("sys.stdout", io.StringIO()):
            self.pull["reviewers"] = []
            with self.assertRaisesRegex(RuntimeError, "concurrent"):
                manage.reset(api, self.expected, apply=True)

    def test_pagination_includes_second_page_and_rejects_wrong_shape(self):
        api = manage.API("fixture-token")
        api.request = Mock(side_effect=[[{}] * 100, [{"id": 101}]])
        self.assertEqual(101, len(list(api.pages("repos/craftpulse-mvp/cp89-org-a/pulls?state=all"))))
        self.assertTrue(api.request.call_args.args[1].endswith("&per_page=100&page=2"))
        api.request = Mock(return_value={"incomplete": True})
        with self.assertRaises(RuntimeError):
            list(api.pages("repos/craftpulse-mvp/cp89-org-a/branches"))

    def test_every_api_request_pins_version_and_does_not_log_token(self):
        api = manage.API("fixture-token")
        response = Mock()
        response.headers = {"X-GitHub-Api-Version-Selected": manage.API_VERSION}
        response.read.return_value = b'{}'
        api.opener = MagicMock()
        api.opener.open.return_value.__enter__.return_value = response
        self.assertEqual({}, api.request("GET", "meta"))
        request = api.opener.open.call_args.args[0]
        self.assertEqual(manage.API_VERSION, request.get_header("X-github-api-version"))
        for path in ("https://evil.example/", "//evil.example/", "repos/../secrets"):
            with self.assertRaises(ValueError):
                api.request("GET", path)

    def test_read_probe_requires_existing_private_negative_control(self):
        operator = Mock()
        reader = Mock()
        operator.request.return_value = {"private": True, "description": manage.MARKER,
                                         "owner": {"id": manage.ORGANIZATION_ID}}
        def get(method, path):
            if path == "repos/craftpulse-mvp/cp89-control":
                raise manage.APIError(method, path, 404)
            if path.endswith("/actions/runs?per_page=1"):
                return {"workflow_runs": []}
            return {"id": self.expected["repositories"][path.rsplit("/", 1)[1]]["id"]}
        reader.request.side_effect = get
        reader.pages.return_value = []
        with patch("sys.stdout", io.StringIO()):
            manage.check_read_access(operator, reader, self.expected, "craftpulse-mvp/cp89-control")
        operator.request.return_value["private"] = False
        with self.assertRaisesRegex(RuntimeError, "negative control"):
            manage.check_read_access(operator, reader, self.expected, "craftpulse-mvp/cp89-control")

class GitHubCommentTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        import sys
        spec = importlib.util.spec_from_file_location("github_comment", ROOT / "deploy/github/comment.py")
        cls.comment = importlib.util.module_from_spec(spec)
        with patch.dict(sys.modules, {"manage": manage, "app_auth": Mock()}):
            spec.loader.exec_module(cls.comment)

    def setUp(self):
        self.baseline = fixture()
        self.api = Mock()
        self.api.pages.return_value = []
        self.api.request.side_effect = self.request
        self.payload = {"repository": "cp89-org-a", "pull_number": 1, "body": "summary"}

    def request(self, method, path, data=None):
        if path == "repos/craftpulse-mvp/cp89-org-a":
            return {"id": 1, "owner": {"id": manage.ORGANIZATION_ID}}
        if path == "repos/craftpulse-mvp/cp89-org-a/pulls/1":
            return dict(self.baseline["repositories"]["cp89-org-a"]["pulls"][0])
        if method in ("POST", "PATCH") and "/comments" in path:
            return {"id": 125}
        raise AssertionError((method, path, data))

    def upsert(self):
        return self.comment.upsert(self.api, self.baseline, "bot", self.payload)

    def test_only_allowlisted_pr_comments_are_accepted_before_network_access(self):
        for payload in ({"repository": "cp89-control", "pull_number": 1, "body": "x"},
                        {"repository": "cp89-org-a", "pull_number": 2, "body": "x"},
                        {"repository": "cp89-org-a", "pull_number": True, "body": "x"},
                        dict(self.payload, path="git/refs/heads/main"),
                        dict(self.payload, body=""), dict(self.payload, body="x" * 24001)):
            self.payload = payload
            with self.assertRaises(ValueError):
                self.upsert()
        self.api.request.assert_not_called()

    def test_upsert_creates_one_marker_comment_then_updates_own_id(self):
        self.assertEqual({"id": 125, "changed": True}, self.upsert())
        self.api.request.assert_called_with("POST", "repos/craftpulse-mvp/cp89-org-a/issues/1/comments",
                                          {"body": self.comment.MARKER + "\nsummary"})
        self.api.pages.return_value = [{"id": 125, "user": {"login": "bot"},
                                       "body": self.comment.MARKER + "\nold"}]
        self.upsert()
        self.api.request.assert_called_with("PATCH", "repos/craftpulse-mvp/cp89-org-a/issues/comments/125",
                                          {"body": self.comment.MARKER + "\nsummary"})

    def test_unchanged_comment_does_not_write(self):
        self.api.pages.return_value = [{"id": 125, "user": {"login": "bot"},
                                       "body": self.comment.MARKER + "\nsummary"}]
        self.assertEqual({"id": 125, "changed": False}, self.upsert())
        self.assertTrue(all(call.args[0] == "GET" for call in self.api.request.call_args_list))

    def test_other_users_comments_cannot_be_overwritten(self):
        self.api.pages.return_value = [{"id": 99, "user": {"login": "human"},
                                       "body": self.comment.MARKER + "\nkeep"}]
        self.upsert()
        self.api.request.assert_called_with("POST", "repos/craftpulse-mvp/cp89-org-a/issues/1/comments",
                                          {"body": self.comment.MARKER + "\nsummary"})

    def test_replaced_pr_and_closed_pr_are_rejected(self):
        for change in ({"id": 999}, {"state": "closed"}, {"merged": True}):
            def request(method, path, data=None):
                result = self.request(method, path, data)
                if path.endswith("/pulls/1"):
                    result.update(change)
                return result
            self.api.request.side_effect = request
            with self.assertRaises(ValueError):
                self.upsert()
        self.assertTrue(all(call.args[0] == "GET" for call in self.api.request.call_args_list))

class GitHubAppAuthTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        import sys
        spec = importlib.util.spec_from_file_location("github_app_auth", ROOT / "deploy/github/app_auth.py")
        cls.auth = importlib.util.module_from_spec(spec)
        with patch.dict(sys.modules, {"manage": manage}):
            spec.loader.exec_module(cls.auth)

    def test_jwt_uses_rs256_client_id_and_ten_minute_window(self):
        import base64
        with patch.object(self.auth.subprocess, "run", return_value=Mock(returncode=0, stdout=b"signature")) as run:
            jwt = self.auth.app_jwt("test-client-id", Path("fixture.pem"), now=10000, openssl="openssl")
        header, payload, signature = jwt.split(".")
        decode = lambda part: json.loads(base64.urlsafe_b64decode(part + "=" * (-len(part) % 4)))
        self.assertEqual({"alg": "RS256", "typ": "JWT"}, decode(header))
        self.assertEqual({"iat": 9940, "exp": 10540, "iss": "test-client-id"}, decode(payload))
        self.assertEqual(["openssl", "dgst", "-sha256", "-sign", "fixture.pem"], run.call_args.args[0])
        self.assertNotIn("signature", jwt)

    def test_token_is_downscoped_to_three_ids_and_only_pr_write(self):
        expected = fixture()
        config = {"organization": manage.ORGANIZATION, "organization_id": manage.ORGANIZATION_ID,
                  "app_id": 123, "client_id": "client", "slug": "fixture-bot", "key_file": "fixture.pem"}
        issuer, bot = Mock(), Mock()
        issuer.request.side_effect = [
            {"id": 123, "slug": "fixture-bot", "owner": {"id": manage.ORGANIZATION_ID},
             "permissions": {"metadata": "read", "pull_requests": "write", "contents": "read"}},
            {"id": 456, "account": {"id": manage.ORGANIZATION_ID}, "repository_selection": "selected",
             "suspended_at": None},
            {"token": "installation-token", "permissions": {"metadata": "read", "pull_requests": "write"}}]
        bot.request.return_value = {"total_count": 3, "repositories": [{"id": i} for i in (1, 2, 3)]}
        with patch.object(self.auth, "app_jwt", return_value="jwt"), patch.object(self.auth, "API", side_effect=[issuer, bot]):
            self.assertIs(bot, self.auth.installation_api(config, expected))
        issuer.request.assert_called_with("POST", "app/installations/456/access_tokens", {
            "repository_ids": [1, 2, 3], "permissions": {"pull_requests": "write"}})

    def test_app_cannot_expand_permissions_or_install_on_all_repos(self):
        config = {"organization": manage.ORGANIZATION, "organization_id": manage.ORGANIZATION_ID,
                  "app_id": 123, "client_id": "client", "slug": "fixture-bot", "key_file": "fixture.pem"}
        for permissions, selection in (({"metadata": "read", "pull_requests": "write", "contents": "write"}, "selected"),
                                       ({"metadata": "read", "pull_requests": "write"}, "all")):
            issuer = Mock()
            issuer.request.side_effect = [
                {"id": 123, "slug": "fixture-bot", "owner": {"id": manage.ORGANIZATION_ID}, "permissions": permissions},
                {"id": 456, "account": {"id": manage.ORGANIZATION_ID}, "repository_selection": selection}]
            with patch.object(self.auth, "app_jwt", return_value="jwt"), patch.object(self.auth, "API", return_value=issuer):
                with self.assertRaises(RuntimeError):
                    self.auth.installation_api(config, fixture())
            self.assertTrue(all(call.args[0] == "GET" for call in issuer.request.call_args_list))

class GitHubAppRegistrationTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        spec = importlib.util.spec_from_file_location("github_register", ROOT / "deploy/github/register_app.py")
        cls.registration = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(cls.registration)

    def test_credentials_cannot_be_written_to_shared_or_tracked_checkout(self):
        for path in (ROOT / "shared/89-github-test-organization/credentials.json",
                     ROOT / "deploy/github/app-private.json"):
            with self.assertRaises(ValueError):
                self.registration.private_output(path)

    def test_secret_file_is_exclusive_and_not_overwritten(self):
        import tempfile
        import os
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "app-private.json"
            self.registration.save_config(path, {"pem": "fixture-private-key"})
            self.assertEqual({"pem": "fixture-private-key"}, json.loads(path.read_text()))
            with self.assertRaises(ValueError):
                self.registration.save_config(path, {"pem": "replacement"})
            self.assertEqual("fixture-private-key", json.loads(path.read_text())["pem"])
            if os.name != "nt":
                self.assertEqual(0o600, path.stat().st_mode & 0o777)


class GitHubTransportTests(unittest.TestCase):
    def test_transient_get_is_retried_without_writes(self):
        from urllib.error import URLError
        api = manage.API("fixture-token")
        api.opener = MagicMock()
        response = api.opener.open.return_value.__enter__.return_value
        response.headers = {}
        response.read.return_value = b'{}'
        context = api.opener.open.return_value
        api.opener.open.side_effect = [URLError(TimeoutError()), context]
        with patch.object(manage.time, "sleep"):
            self.assertEqual({}, api.request("GET", "meta"))
        self.assertEqual(2, api.opener.open.call_count)
        self.assertEqual(0, api.writes)

    def test_uncertain_write_is_never_retried(self):
        from urllib.error import URLError
        api = manage.API("fixture-token")
        api.opener = MagicMock()
        api.opener.open.side_effect = URLError(TimeoutError())
        with patch.object(manage.time, "sleep") as sleep:
            with self.assertRaisesRegex(RuntimeError, "inspect remote state"):
                api.request("POST", "repos/craftpulse-mvp/cp89-org-a/pulls", {})
        self.assertEqual(1, api.opener.open.call_count)
        sleep.assert_not_called()

    def test_certificate_validation_failure_is_not_retried(self):
        import ssl
        from urllib.error import URLError
        api = manage.API("fixture-token")
        api.opener = MagicMock()
        api.opener.open.side_effect = URLError(ssl.SSLCertVerificationError("fixture"))
        with patch.object(manage.time, "sleep") as sleep:
            with self.assertRaisesRegex(RuntimeError, "certificate verification"):
                api.request("GET", "meta")
        self.assertEqual(1, api.opener.open.call_count)
        sleep.assert_not_called()


if __name__ == "__main__":
    unittest.main()
