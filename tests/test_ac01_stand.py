from email.message import Message
from io import BytesIO, StringIO
import json
from pathlib import Path
import sys
from tempfile import TemporaryDirectory
from types import SimpleNamespace
import unittest
from unittest.mock import patch
from urllib.parse import parse_qs, urlencode, urlsplit
from urllib.request import HTTPSHandler
from urllib.response import addinfourl
from uuid import uuid4

ROOT = Path(__file__).resolve().parents[1]
with patch.object(sys, "path", [str(ROOT / "deploy/stand"), *sys.path]):
    import ac01

SITE = "https://app.example.test"
GITLAB = "https://gitlab.example.test"
SHA = "1" * 40


class SimulatedStand(HTTPSHandler):
    """Keeps urllib redirects, form submissions and cookie behavior in the test."""

    def __init__(self, *, fault=None, consent_required=True):
        super().__init__()
        self.fault = fault
        self.consent_required = consent_required
        self.state = {
            "binding": {"stand_url": SITE, "gitlab_url": GITLAB, "organizations": list(ac01.SLUGS)},
            "objects": {},
            "credentials": {},
        }
        self.structures, self.org_ids = {}, {}
        for slug in ac01.SLUGS:
            app = "craftpulse-" + slug
            self.state["objects"][app] = {"application_id": app, "callback_url": SITE + f"/o/{slug}/callback"}
            self.state["credentials"][app] = {"secret": uuid4().hex}
            self.structures[slug] = {**{key: [] for key in ac01.STRUCTURE_FIELDS}, "version": 0}
            self.org_ids[slug] = str(uuid4())
        for account in ac01.ACCOUNTS:
            self.state["credentials"][account] = {"password": uuid4().hex}
        self.active = set()
        self.pending, self.codes, self.sessions, self.users, self.roles = {}, {}, {}, {}, {}
        self.login_events = {slug: [] for slug in ac01.SLUGS}
        self.revoked, self.sensitive = set(), set()
        self.replays = 0
        self.payload_marker = uuid4().hex

    def https_open(self, req):
        parts = urlsplit(req.full_url)
        query = parse_qs(parts.query)
        host = parts.hostname
        path = parts.path
        method = req.get_method()
        cookies = req.get_header("Cookie", "")
        if host == "gitlab.example.test":
            # No craftPulse cookies cross the provider boundary.
            assert "__Host-cp_" not in cookies
            if path == "/users/sign_in" and method == "GET":
                return self.html(req, path, '<input name="user[login]"><input name="user[password]" type="password">')
            if path == "/users/sign_in" and method == "POST":
                form = parse_qs(req.data.decode())
                name, = form["user[login]"]
                assert form["user[password]"] == [self.state["credentials"][name]["password"]]
                state, = parse_qs(req.get_header("Cookie"))["gitlab_flow"]
                return self.reply(req, 302, Location=self.pending[state], **{"Set-Cookie": f"gitlab_user={name}; Secure; Path=/"})
            if path == "/oauth/authorize" and method == "GET":
                state, = query["state"]
                if "gitlab_user=" not in cookies:
                    self.pending[state] = req.full_url
                    return self.reply(req, 302, Location="/users/sign_in",
                                      **{"Set-Cookie": f"gitlab_flow={state}; Secure; Path=/"})
                if self.consent_required:
                    return self.html(req, path, f'<input type="hidden" name="state" value="{state}">')
                return self.grant(req, state, cookies)
            if path == "/oauth/authorize" and method == "POST":
                state, = parse_qs(req.data.decode())["state"]
                return self.grant(req, state, cookies)
            raise AssertionError("Unexpected GitLab request")
        assert host == "app.example.test" and "gitlab_user=" not in cookies
        if path == "/version":
            return self.reply(req, 200, {"commit_sha": SHA})
        if path.endswith("/login"):
            slug = path.split("/")[2]
            issuer = "https://other.example.test" if self.fault == "wrong_provider" else GITLAB
            client_slug = "org-a" if self.fault == "org_b_uses_org_a_client" else slug
            oidc = dict(client_id="craftpulse-" + client_slug, redirect_uri=SITE + f"/o/{slug}/callback",
                        state=uuid4().hex, nonce=uuid4().hex, code_challenge=uuid4().hex,
                        code_challenge_method="S256", scope="openid profile", response_type="code")
            self.sensitive.update(oidc[key] for key in ("state", "nonce", "code_challenge"))
            return self.reply(req, 302, Location=issuer + "/oauth/authorize?" + urlencode(oidc),
                              **{"Set-Cookie": f"__Host-cp_login={oidc['state']}; Secure; Path=/"})
        if path.endswith("/callback"):
            slug = path.split("/")[2]
            state, = query["state"]
            assert cookies == f"__Host-cp_login={state}"
            user = self.codes.pop(query["code"][0])
            if slug not in self.active:
                if user != f"cp87-admin-{slug[-1]}" and self.fault != "outsider_bootstraps_org_b":
                    self.login_events[slug].append({
                        "id": str(uuid4()), "operation": "session.login", "actor_id": None,
                    })
                    return self.reply(req, 302, Location=f"/o/{slug}?login_error=organization_not_configured")
                self.active.add(slug)
                self.roles[slug, user] = [ac01.role("administrator")]
            if slug == "org-a" and user == "cp87-admin-b" and self.fault == "admin_role_leaks_to_org_a":
                self.roles[slug, user] = [ac01.role("administrator")]
            self.users.setdefault((slug, user), str(uuid4()))
            self.login_events[slug].append({
                "id": str(uuid4()), "operation": "session.login", "actor_id": self.users[slug, user],
            })
            token = uuid4().hex
            self.sessions[token] = (slug, user)
            self.sensitive.add(token)
            return self.reply(req, 302, Location="/",
                              **{"Set-Cookie": f"__Host-cp_session={token}; Secure; Path=/; HttpOnly"})
        token = dict(cookie.split("=", 1) for cookie in cookies.split("; ") if cookie).get("__Host-cp_session")
        if token in self.revoked:
            self.replays += 1
        if token not in self.sessions:
            if token in self.revoked and self.fault == "stale_session_returns_data":
                return self.reply(req, 401, {"code": "auth_required", "data": [self.payload_marker]})
            return self.reply(req, 401, {"code": "auth_required"})
        slug, user = self.sessions[token]
        return self.api(req, path.removeprefix("/api/v1"), query, slug, user, token)

    def grant(self, req, state, cookies):
        callback, = parse_qs(urlsplit(self.pending[state]).query)["redirect_uri"]
        name = dict(cookie.split("=", 1) for cookie in cookies.split("; "))["gitlab_user"]
        code = uuid4().hex
        self.sensitive.add(code)
        self.codes[code] = name
        return self.reply(req, 302, Location=callback + "?" + urlencode({"state": state, "code": code}))

    def api(self, req, path, query, slug, user, token):
        structure = self.structures[slug]
        user_id = self.users[slug, user]
        roles = self.roles.get((slug, user), [])
        method = req.get_method()
        if path == "/me":
            return self.reply(req, 200, {"organization": {"slug": slug, "id": self.org_ids[slug], "status": "active"},
                                         "user": {"id": user_id}, "roles": roles, "extra": self.payload_marker})
        if path == ac01.STRUCTURE:
            if method == "PUT":
                assert req.get_header("If-match") == f'"{structure["version"]}"'
                if req.get_header("Origin") != SITE:
                    if self.fault == "origin_changes_state":
                        structure["version"] += 1
                    return self.reply(req, 403, {"code": "origin_rejected"})
                structure.update(json.loads(req.data))
                for key in ("teams", "projects"):
                    for item in structure[key]:
                        item["id"] = item["id"] or str(uuid4())
                structure["version"] += 1
            return self.reply(req, 200, structure, ETag=f'"{structure["version"]}"')
        if path in ("/teams", "/projects"):
            entries = structure[path[1:]]
            if path == "/projects" and "team_id" in query:
                entries = [entry for entry in entries
                           if any(link["project_id"] == entry["id"] and link["team_id"] in query["team_id"]
                                  for link in structure["team_projects"])]
            if path == "/projects" and self.fault == "foreign_project_leaked" and user == "cp87-member":
                entries = [*entries, *self.structures["org-b"]["projects"]]
            if path == "/teams" and self.fault == "foreign_team_leaked_to_admin" and user == "cp87-admin-a":
                entries = [*entries, *self.structures["org-b"]["teams"]]
            return self.page(req, entries)
        if path == "/users":
            return self.page(req, [{"id": value} for (org, _), value in self.users.items() if org == slug])
        if path == ac01.AUDIT:
            if not any(role["role"] == "administrator" for role in roles):
                return self.reply(req, 403, {"code": "forbidden"})
            events = self.login_events[slug]
            fault = "foreign_audit_leaked" if "actor_id" in query else "foreign_audit_leaked_on_unfiltered_list"
            if slug == "org-a" and self.fault == fault:
                events = events + self.login_events["org-b"]
            if slug == "org-a" and self.fault == "foreign_denied_login_leaked":
                events = events + [event for event in self.login_events["org-b"] if event["actor_id"] is None]
            if "actor_id" in query:
                events = [event for event in events if event["actor_id"] in query["actor_id"]]
            return self.page(req, events)
        if path.endswith("/roles"):
            target = path.split("/")[2]
            users = {value: name for (org, name), value in self.users.items() if org == slug}
            if target not in users:
                return self.reply(req, 404, {"code": "not_found"})
            key = slug, users[target]
            if method == "PUT":
                assert req.get_header("If-match") == '"0"' and req.get_header("Origin") == SITE
                self.roles[key] = json.loads(req.data)["data"]
            return self.reply(req, 200, {"data": self.roles.get(key, [])}, ETag='"0"')
        if path.endswith(("/workflow-catalog", "/baselines")):
            _, field, target, endpoint = path.split("/")
            if any(item["id"] == target for item in structure[field]):
                if endpoint == "workflow-catalog":
                    assert "process_owner" not in {role["role"] for role in roles}
                    return self.reply(req, 403, {"code": "forbidden"})
                if self.fault == "broken_own_read":
                    return self.reply(req, 404, {"code": "not_found"})
                return self.reply(req, 200, {"data": []})
            if self.fault == "foreign_team_disclosed" and endpoint == "workflow-catalog":
                return self.reply(req, 403, {"code": "forbidden"})
            if self.fault == "foreign_baselines_visible" and endpoint == "baselines":
                return self.reply(req, 200, {"data": []})
            return self.reply(req, 404, {"code": "not_found"})
        if path == "/session/logout":
            assert req.get_header("Origin") == SITE
            if self.fault != "stale_session_alive":
                self.sessions.pop(token)
            self.revoked.add(token)
            return self.reply(req, 204, **{"Set-Cookie": "__Host-cp_session=; Secure; Path=/; Max-Age=0"})
        raise AssertionError("Unexpected craftPulse API request")

    def page(self, req, data):
        return self.reply(req, 200, {"data": data, "next_cursor": None})

    def html(self, req, path, fields):
        csrf = uuid4().hex
        self.sensitive.add(csrf)
        if path == "/users/sign_in" and self.fault == "offsite_password_form":
            path = "https://other.example.test/users/sign_in"
        return self.reply(req, 200, f'<form method="post" action="{path}">'
                          f'<input type="hidden" name="authenticity_token" value="{csrf}">{fields}</form>')

    @staticmethod
    def reply(req, status, data=None, **headers):
        if isinstance(data, dict):
            if "code" in data:
                data = {
                    "type": "urn:craftpulse:problem:" + data["code"],
                    "title": "Test problem",
                    "status": status,
                    "correlation_id": str(uuid4()),
                    **data,
                }
            data = json.dumps(data)
        message = Message()
        for name, value in headers.items():
            message[name] = value
        response = addinfourl(BytesIO((data or "").encode()), message, req.full_url, status)
        response.msg = "test response"
        return response


class StandAcceptanceTests(unittest.TestCase):
    def simulate(self, fault=None, failure=None, *, consent_required=True, recovery_bundle=False):
        stand = SimulatedStand(fault=fault, consent_required=consent_required)
        evidence = ac01.Evidence()
        bundle = {"provisioning": stand.state} if recovery_bundle else stand.state
        result = SimpleNamespace(returncode=0, stdout=json.dumps(bundle))
        with patch("ac01.subprocess.run", return_value=result), patch("ac01_http.HTTPSHandler", return_value=stand):
            login = ac01.load_login(Path("stand.sops.json"), None, evidence)
            check = ac01.Acceptance(login, evidence)
            if failure:
                with self.assertRaisesRegex(ac01.CheckFailed, failure):
                    check.run(SHA)
            else:
                check.run(SHA)
                self.assertEqual(stand.replays, 2)
        stream = StringIO()
        evidence.save(stream, "failed" if failure else "passed")
        output = stream.getvalue()
        for secret in evidence.secrets | stand.sensitive | {stand.payload_marker}:
            self.assertNotIn(secret, output)
        return json.loads(output)

    def test_two_runs_have_comparable_secret_free_evidence(self):
        reports = [self.simulate(), self.simulate(consent_required=False, recovery_bundle=True)]
        self.assertNotEqual(len(reports[0]["requests"]), len(reports[1]["requests"]))
        for report in reports:
            self.assertEqual(report["outcome"], "passed")
            self.assertEqual(report["commit_sha"], SHA)
            self.assertTrue(report["structure_unchanged"])
            self.assertLessEqual(report.pop("started_at"), report.pop("finished_at"))
            report["requests"] = [row for row in report["requests"] if not row["operation"].startswith("gitlab")]
        self.assertEqual(*reports)

    def test_fails_closed_on_regressions(self):
        for fault, failure in (
            ("wrong_provider", "unexpected OIDC application"),
            ("org_b_uses_org_a_client", "unexpected OIDC application"),
            ("offsite_password_form", "expected sign-in or consent form"),
            ("outsider_bootstraps_org_b", "completeLogin: unexpected login result"),
            ("admin_role_leaks_to_org_a", "Unexpected role assignments or scope"),
            ("foreign_project_leaked", "listProjects: unexpected rows"),
            ("foreign_team_leaked_to_admin", "listTeamsAsAdministrator: unexpected rows"),
            ("foreign_audit_leaked", "listAuditEventsByForeignActor: unexpected rows"),
            ("foreign_audit_leaked_on_unfiltered_list", "listAuditEvents: returned an event"),
            ("foreign_denied_login_leaked", "listAuditEvents: returned an event"),
            ("broken_own_read", "listOwnBaselines: unexpected HTTP status"),
            ("foreign_team_disclosed", "getForeignWorkflowCatalog: unexpected HTTP status"),
            ("foreign_baselines_visible", "listForeignBaselines: unexpected HTTP status"),
            ("origin_changes_state", "Structure changed"),
            ("stale_session_alive", "getCurrentUser: unexpected HTTP status"),
            ("stale_session_returns_data", "getCurrentUser: expected a problem response without resource data"),
        ):
            with self.subTest(fault=fault):
                self.assertEqual(self.simulate(fault, failure)["outcome"], "failed")

    def test_evidence_refuses_protected_values(self):
        evidence = ac01.Evidence()
        evidence.protect("secret-test-value")
        evidence.report["mock_bad_field"] = "unexpected secret-test-value"
        stream = StringIO()
        with self.assertRaisesRegex(ac01.CheckFailed, "protected value"):
            evidence.save(stream, "failed")
        self.assertEqual(stream.getvalue(), "")

    def test_cli_saves_private_report_and_preserves_existing_evidence(self):
        stand = SimulatedStand()
        result = SimpleNamespace(returncode=0, stdout=json.dumps({"provisioning": stand.state}))
        with TemporaryDirectory() as directory:
            path = Path(directory) / "ac01.json"
            argv = ["ac01.py", "--gitlab-bundle", "gitlab-secrets.sops.json", "--sha", SHA, "--report", str(path)]
            with patch.object(sys, "argv", argv), patch("ac01.subprocess.run", return_value=result) as decrypt, \
                    patch("ac01_http.HTTPSHandler", return_value=stand):
                self.assertEqual(ac01.main(), 0)
                payload = path.read_text(encoding="utf-8")
                self.assertEqual(json.loads(payload)["outcome"], "passed")
                self.assertEqual(path.stat().st_mode & 0o777, 0o600)
                for value in stand.sensitive | {stand.payload_marker}:
                    self.assertNotIn(value, payload)
                with patch("sys.stderr", new_callable=StringIO):
                    self.assertEqual(ac01.main(), 1)
                self.assertEqual(path.read_text(encoding="utf-8"), payload)
                decrypt.assert_called_once()


if __name__ == "__main__":
    unittest.main()
