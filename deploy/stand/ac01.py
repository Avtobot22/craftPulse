#!/usr/bin/env python3
"""Run AC-01 against the disposable two-organization craftPulse stand."""

import argparse
from copy import deepcopy
from datetime import datetime, timezone
import json
import os
from pathlib import Path
import re
import subprocess
import sys
from uuid import UUID

from ac01_http import Browser, CheckFailed, GitLabLogin, parse_object, require, tls_context


STRUCTURE = "/organization/structure"
AUDIT = "/audit-events"
SLUGS = ("org-a", "org-b")
ACCOUNTS = ("cp87-admin-a", "cp87-admin-b", "cp87-member", "cp87-no-repo")
STRUCTURE_FIELDS = ("teams", "projects", "services", "components", "team_projects", "project_areas", "team_members")


def now():
    return datetime.now(timezone.utc)


def identity(value):
    require(isinstance(value, str) and str(UUID(value)) == value, "Expected a canonical UUID")
    return value


class Evidence:
    def __init__(self):
        self.case = "release"
        self.secrets = set()
        self.report = {"check": "AC-01", "started_at": now().isoformat(), "commit_sha": None, "requests": []}

    def protect(self, value):
        if value:
            self.secrets.add(value)

    def record(self, operation, method, status):
        # Operation and case are fixed labels. Do not store URLs, headers or raw bodies.
        row = {"case": self.case, "operation": operation, "method": method, "status": status}
        self.report["requests"].append(row)
        return row

    def save(self, stream, outcome):
        self.report.update(finished_at=now().isoformat(), outcome=outcome)
        payload = json.dumps(self.report, ensure_ascii=False, indent=2)
        for secret in self.secrets:
            quoted = json.dumps(secret, ensure_ascii=False)
            require(quoted not in payload and (len(secret) < 8 or quoted[1:-1] not in payload),
                    "Report contains a protected value")
        stream.write(payload + "\n")


class Acceptance:
    def __init__(self, login, evidence):
        self.login, self.evidence = login, evidence

    def request(self, browser, operation, path, *, status=200, code=None, method="GET", data=None,
                etag=None, foreign_origin=False, stale=None):
        headers = {"Accept": "application/json"}
        if stale:
            headers["Cookie"] = "__Host-cp_session=" + stale
        if method != "GET":
            headers["Origin"] = "https://ac01.invalid" if foreign_origin else self.login.site
        if etag:
            headers["If-Match"] = etag
        body = None
        if data is not None:
            headers["Content-Type"] = "application/json"
            body = json.dumps(data).encode()
        url = self.login.site + ("/api/v1" if path != "/version" else "") + path
        record, response_headers, raw = browser.request(operation, method, url, headers=headers, body=body)
        if foreign_origin:
            record["foreign_origin"] = True
        if stale:
            record["replayed_session"] = True
        require(record["status"] == status, f"{operation}: unexpected HTTP status")
        result = parse_object(raw) if raw else {}
        if code is not None:
            require(result.get("code") == code, f"{operation}: unexpected problem code")
            problem_fields = {"type", "title", "status", "code", "correlation_id"}
            require(problem_fields <= result.keys() <= problem_fields | {"detail", "errors"}
                    and result["status"] == status
                    and result["type"] == "urn:craftpulse:problem:" + code,
                    f"{operation}: expected a problem response without resource data")
            record["code"] = code
        return result, response_headers.get("ETag"), record

    def verify_release(self, sha):
        browser = Browser(self.login.tls, self.evidence)
        result, _, _ = self.request(browser, "getVersion", "/version")
        require(result.get("commit_sha") == sha, "Stand SHA does not match the requested release")
        self.evidence.report["commit_sha"] = sha

    def me(self, browser, slug, roles):
        data, _, record = self.request(browser, "getCurrentUser", "/me")
        require(data["organization"]["slug"] == slug and data["organization"]["status"] == "active",
                "getCurrentUser: unexpected organization")
        self.check_roles(data["roles"], roles)
        record.update(organization=slug, roles=[item["role"] for item in roles])
        if any(item["team_id"] is not None for item in roles):
            record["team_scope_matches"] = True
        return identity(data["organization"]["id"]), identity(data["user"]["id"])

    @staticmethod
    def check_roles(actual, expected):
        fields = ("role", "team_id", "project_id", "service_id")

        def assignments(items):
            return [tuple(role[field] for field in fields) for role in items]

        require(assignments(actual) == assignments(expected), "Unexpected role assignments or scope")

    def structure(self, admin):
        data, tag, record = self.request(admin, "getOrganizationStructure", STRUCTURE)
        require(type(data["version"]) is int and tag == f'"{data["version"]}"',
                "Structure version does not match its ETag")
        record["version"] = data["version"]
        return data, tag

    def fixture(self, admin, slug):
        # Full replacement is allowed only on a disposable stand with AC-01 data.
        data, tag = self.structure(admin)
        desired = {key: deepcopy(data[key]) for key in STRUCTURE_FIELDS}
        require(not any(desired[key] for key in ("services", "components", "project_areas", "team_members")),
                "Stand has structure outside the AC-01 fixture")
        create = False
        for key, name in (("teams", f"AC-01 {slug} team"), ("projects", f"AC-01 {slug} project")):
            entries = desired[key]
            require(not entries or (len(entries) == 1 and entries[0]["name"] == name
                                    and entries[0]["status"] == "active"),
                    "Stand has teams or projects outside the AC-01 fixture")
            if not entries:
                entries.append({"id": None, "name": name, "status": "active"})
                create = True
        require(not (create and desired["team_projects"]), "Unexpected link in incomplete AC-01 fixture")
        if create:
            self.request(admin, "replaceOrganizationStructure", STRUCTURE, method="PUT", data=desired, etag=tag)
            data, tag = self.structure(admin)
        team, project = identity(data["teams"][0]["id"]), identity(data["projects"][0]["id"])
        link = {"team_id": team, "project_id": project}
        if not data["team_projects"]:
            desired = {key: deepcopy(data[key]) for key in STRUCTURE_FIELDS}
            desired["team_projects"] = [{**link, "valid_from": None, "valid_to": None}]
            self.request(admin, "replaceOrganizationStructure", STRUCTURE, method="PUT", data=desired, etag=tag)
            data, _ = self.structure(admin)
        require(len(data["team_projects"]) == 1 and all(data["team_projects"][0][k] == v for k, v in link.items()),
                "Unexpected AC-01 team/project link")
        return team, project

    def assign_role(self, admin, user, role):
        path = f"/users/{user}/roles"
        data, tag, _ = self.request(admin, "listUserRoles", path)
        require(tag, "Role assignments are missing an ETag")
        if not data["data"]:
            data, _, _ = self.request(admin, "replaceUserRoles", path, method="PUT", data={"data": [role]}, etag=tag)
        self.check_roles(data["data"], [role])

    def read_page(self, browser, operation, path, expected, *, field="id"):
        data, _, record = self.request(browser, operation, path)
        require(data["next_cursor"] is None and [item[field] for item in data["data"]] == expected,
                f"{operation}: unexpected rows or pagination")
        record["rows"] = len(expected)
        return data["data"]

    def reject_origin(self, admin):
        before, tag = self.structure(admin)
        change = {key: deepcopy(before[key]) for key in STRUCTURE_FIELDS}
        change["teams"][0]["name"] += " foreign origin"
        for link in change["team_projects"]:
            link.update(valid_from=None, valid_to=None)
        self.request(admin, "replaceOrganizationStructure", STRUCTURE, method="PUT",
                     data=change, etag=tag, foreign_origin=True, status=403, code="origin_rejected")
        after, after_tag = self.structure(admin)
        require(before == after and tag == after_tag, "Structure changed after the foreign-Origin request")
        self.evidence.report["structure_unchanged"] = True

    def verify_isolation(self, member, admin_a, admin_b, own, foreign, foreign_admin):
        team, project = foreign
        self.read_page(member, "listTeams", "/teams", [own[0]])
        self.read_page(member, "listProjects", "/projects", [own[1]])
        self.request(member, "listAuditEvents", AUDIT, status=403, code="forbidden")

        # The organization-wide role must also exclude the other tenant's rows.
        self.read_page(admin_a, "listTeamsAsAdministrator", "/teams", [own[0]])
        self.read_page(admin_a, "listProjectsAsAdministrator", "/projects", [own[1]])
        self.read_page(admin_a, "listProjectsByForeignTeam", f"/projects?team_id={team}", [])
        self.read_page(admin_a, "listAuditEventsByForeignActor", f"{AUDIT}?actor_id={foreign_admin}", [])

        catalog = f"/teams/{team}/workflow-catalog"
        baselines = f"/projects/{project}/baselines"
        # Only a process owner can read the catalog. A local administrator gets
        # 403 for a visible team, whereas a foreign team must still look missing.
        self.read_page(admin_b, "listProjectsByOwnTeam", f"/projects?team_id={team}", [project])
        self.request(admin_b, "getOwnWorkflowCatalog", catalog, status=403, code="forbidden")
        self.request(admin_b, "listOwnBaselines", baselines)
        self.request(admin_a, "getForeignWorkflowCatalog", catalog, status=404, code="not_found")
        self.request(admin_a, "listForeignBaselines", baselines, status=404, code="not_found")
        self.request(member, "listOwnBaselinesAsMember", f"/projects/{own[1]}/baselines")
        self.request(member, "listForeignBaselinesAsMember", baselines, status=404, code="not_found")
        roles_path = f"/users/{foreign_admin}/roles"
        data, _, _ = self.request(admin_b, "listOwnUserRoles", roles_path)
        self.check_roles(data["data"], [role("administrator")])
        self.request(admin_a, "listForeignUserRoles", roles_path, status=404, code="not_found")

    def run(self, sha):
        self.verify_release(sha)
        admin_a_name, admin_b_name, member_name, stranger_name = ACCOUNTS
        a, b = SLUGS
        administrator = role("administrator")

        self.evidence.case = "organization-b-before-bootstrap"
        outsider = self.login.run(b, stranger_name, denied=True)
        self.request(outsider, "getCurrentUser", "/me", status=401, code="auth_required")

        self.evidence.case = "administrator-b"
        admin_b = self.login.run(b, admin_b_name)
        org_b, user_b = self.me(admin_b, b, [administrator])
        self.read_page(admin_b, "listUsers", "/users", [user_b])
        login_events = self.read_page(admin_b, "listAuditEventsByOwnActor", f"{AUDIT}?actor_id={user_b}",
                                      ["session.login"], field="operation")
        event_b = identity(login_events[0]["id"])
        foreign = self.fixture(admin_b, b)
        self.read_page(admin_b, "listTeams", "/teams", [foreign[0]])
        self.read_page(admin_b, "listProjects", "/projects", [foreign[1]])

        self.evidence.case = "administrator-a"
        admin_a = self.login.run(a, admin_a_name)
        org_a, user_a = self.me(admin_a, a, [administrator])
        require(org_a != org_b, "Stand must have two distinct organizations")
        own = self.fixture(admin_a, a)

        self.evidence.case = "administrator-b-in-a"
        admin_b_in_a = self.login.run(a, admin_b_name)
        _, user_b_in_a = self.me(admin_b_in_a, a, [])
        require(user_b != user_b_in_a, "Expected distinct organization users for the same GitLab account")

        self.evidence.case = "team-member"
        member = self.login.run(a, member_name)
        # A login maps the OIDC identity to its local user ID. The next request sees a role change.
        mapped, _, _ = self.request(member, "getCurrentUser", "/me")
        require(mapped["organization"]["slug"] == a, "Member was mapped in the wrong organization")
        scoped = role("dev_lead", team=own[0])
        self.assign_role(admin_a, identity(mapped["user"]["id"]), scoped)
        self.me(member, a, [scoped])

        self.evidence.case = "tenant-isolation"
        self.verify_isolation(member, admin_a, admin_b, own, foreign, user_b)
        foreign_audit, _, _ = self.request(admin_b, "listAuditEventsInOrganizationB", AUDIT)
        require(foreign_audit["next_cursor"] is None, "listAuditEvents: expected a complete page in org-b")
        foreign_events = {identity(event["id"]) for event in foreign_audit["data"]}
        require(event_b in foreign_events, "listAuditEvents: expected the login in org-b")
        audit, _, record = self.request(admin_a, "listAuditEventsAsAdministrator", AUDIT)
        require(audit["next_cursor"] is None and any(
            event["actor_id"] == user_a and event["operation"] == "session.login" for event in audit["data"]
        ), "listAuditEvents: expected a complete page with the local login")
        require(all(event["id"] not in foreign_events and event["actor_id"] != user_b for event in audit["data"]),
                "listAuditEvents: returned an event from the other organization")
        record["foreign_event_absent"] = True

        self.evidence.case = "foreign-origin"
        self.reject_origin(admin_a)

        self.evidence.case = "logout"
        stale = member.session()
        require(stale, "Expected a session before logout")
        self.request(member, "logout", "/session/logout", method="POST", status=204)
        require(not member.session(), "Logout did not clear the client cookie")
        replay = Browser(self.login.tls, self.evidence)
        self.request(replay, "getCurrentUser", "/me", stale=stale, status=401, code="auth_required")
        self.request(replay, "listTeams", "/teams", stale=stale, status=401, code="auth_required")
        self.evidence.case = "release"
        self.verify_release(sha)


def role(name, *, team=None):
    return {"role": name, "team_id": team, "project_id": None, "service_id": None}


def load_login(bundle, ca, evidence):
    result = subprocess.run(["sops", "decrypt", "--output-type", "json", str(bundle.resolve())],
                            stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, timeout=30, check=False)
    require(result.returncode == 0, "Could not decrypt the GitLab journal")
    bundle_data = parse_object(result.stdout)
    # The recovery bundle wraps the same journal that manage.py writes on its own.
    state = bundle_data.get("provisioning", bundle_data)
    binding = state["binding"]
    require(binding["organizations"] == list(SLUGS), "GitLab journal must bind org-a followed by org-b")
    site = binding["stand_url"]
    clients = {}
    passwords = {}
    for slug in SLUGS:
        key = f"craftpulse-{slug}"
        clients[slug] = state["objects"][key]["application_id"]
        require(state["objects"][key]["callback_url"] == f"{site}/o/{slug}/callback",
                "GitLab journal callback does not match the stand")
        secret = state["credentials"][key]["secret"]
        require(isinstance(secret, str) and secret, "GitLab journal is missing an OIDC secret")
        evidence.protect(secret)
    require(all(isinstance(value, str) and value for value in clients.values())
            and len(set(clients.values())) == len(SLUGS), "Expected distinct OIDC clients for both organizations")
    for username in ACCOUNTS:
        password = state["credentials"][username]["password"]
        require(isinstance(password, str) and password, "GitLab journal is missing a test password")
        passwords[username] = password
        evidence.protect(password)
    return GitLabLogin(site, binding["gitlab_url"], clients, passwords, tls_context(ca), evidence)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--gitlab-bundle", type=Path, required=True, help="SOPS-encrypted GitLab provisioning journal")
    parser.add_argument("--sha", required=True, help="Full release SHA expected at /version")
    parser.add_argument("--report", type=Path, required=True, help="New evidence JSON file")
    parser.add_argument("--ca", help="CA bundle for HTTPS verification")
    args = parser.parse_args()
    if not re.fullmatch("[0-9a-f]{40}", args.sha):
        parser.error("--sha must be the full lowercase Git SHA")
    evidence = Evidence()
    try:
        with open(args.report, "x", encoding="utf-8",
                  opener=lambda path, flags: os.open(path, flags, 0o600)) as stream:
            outcome = "failed"
            try:
                login = load_login(args.gitlab_bundle, args.ca, evidence)
                Acceptance(login, evidence).run(args.sha)
                outcome = "passed"
            except CheckFailed as exc:
                print(exc, file=sys.stderr)
            except (OSError, ValueError, KeyError, TypeError, IndexError, subprocess.TimeoutExpired):
                print("AC-01 could not read the journal or complete an HTTP exchange", file=sys.stderr)
            finally:
                evidence.save(stream, outcome)
    except (OSError, CheckFailed):
        print("AC-01 could not save the evidence report", file=sys.stderr)
        return 1
    return 0 if outcome == "passed" else 1


if __name__ == "__main__":
    sys.exit(main())
