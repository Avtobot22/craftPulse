#!/usr/bin/env python3
"""Provision and inspect the isolated #87 test instance using the GitLab REST API."""

import argparse
from datetime import date, timedelta
import json
import os
from pathlib import Path, PurePosixPath
import re
import secrets
import ssl
import sys
import tempfile
from urllib.error import HTTPError, URLError
from urllib.parse import urlencode, urlsplit
from urllib.request import HTTPRedirectHandler, HTTPSHandler, Request, build_opener


VERSION = "19.5.5"
RUNNER_VERSION = "19.5.0"
ROOT = Path(__file__).resolve().parents[2]
CURL_IMAGE = ("curlimages/curl:8.17.0@sha256:"
              "935d9100e9ba842cdb060de42472c7ca90cfe9a7c96e4dacb55e79e560b3ff40")


def origin(value):
    parsed = urlsplit(value)
    if (parsed.scheme != "https" or not parsed.hostname or parsed.username
            or parsed.password or parsed.query or parsed.fragment
            or parsed.path not in ("", "/")):
        raise ValueError("Expected an HTTPS origin without credentials, path or query")
    return value.rstrip("/")


def slug(value):
    if not re.fullmatch(r"[a-z][a-z0-9]*(?:-[a-z0-9]+)*", value):
        raise ValueError("Invalid organization slug")
    return value


class NoRedirect(HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        # Never forward an administrator or integration token through redirects.
        return None


class APIError(RuntimeError):
    def __init__(self, method, code):
        super().__init__(f"GitLab {method} failed (HTTP {code}); response omitted")
        self.code = code


class API:
    def __init__(self, url, token, ca=None):
        self.url = origin(url)
        self.token = token
        self.opener = build_opener(
            NoRedirect(), HTTPSHandler(context=ssl.create_default_context(cafile=ca)))

    def request(self, method, path, data=None):
        body = None if data is None else json.dumps(data).encode()
        headers = {"Content-Type": "application/json", "PRIVATE-TOKEN": self.token}
        request = Request(self.url + "/api/v4/" + path, body, headers, method=method)
        try:
            with self.opener.open(request, timeout=30) as response:
                content = response.read()
                return json.loads(content) if content else None
        except HTTPError as error:
            raise APIError(method, error.code) from None
        except URLError:
            raise RuntimeError("GitLab connection or TLS verification failed") from None

    def pages(self, path):
        for page in range(1, 1001):
            separator = "&" if "?" in path else "?"
            items = self.request("GET", f"{path}{separator}per_page=100&page={page}")
            yield from items
            if len(items) < 100:
                return
        raise RuntimeError("Pagination limit reached")


def save_private(path, value):
    path = Path(path).resolve()
    if path.is_relative_to(ROOT):
        raise ValueError("Secret output must be outside the source checkout")
    path.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
    if os.name != "nt" and path.parent.stat().st_mode & 0o077:
        raise ValueError("Secret directory must have mode 0700")
    fd, temporary = tempfile.mkstemp(dir=path.parent)
    try:
        with os.fdopen(fd, "w", encoding="utf-8", newline="\n") as stream:
            stream.write(value)
            stream.flush()
            os.fsync(stream.fileno())
        os.replace(temporary, path)
    finally:
        if os.path.exists(temporary):
            os.unlink(temporary)


class Provision:
    def __init__(self, api, state_file, stand, orgs, email_domain):
        self.api, self.path = api, Path(state_file)
        binding = {"gitlab_url": api.url, "stand_url": origin(stand),
                   "organizations": [slug(org) for org in orgs], "email_domain": email_domain}
        if orgs[0] == orgs[1]:
            raise ValueError("Organizations must be different")
        self.state = json.loads(self.path.read_text()) if self.path.exists() else {
            "binding": binding, "objects": {}, "credentials": {}}
        if self.state["binding"] != binding:
            raise ValueError("State belongs to another instance or organization layout")
        self.save()  # Check output permissions before creating any remote object.

    def save(self):
        save_private(self.path, json.dumps(self.state, indent=2) + "\n")

    def ensure(self, key, lookup, matches, endpoint, payload, secret_fields=()):
        objects = self.state["objects"]
        if key in objects:
            return objects[key]
        if any(matches(item) for item in self.api.pages(lookup)):
            raise RuntimeError(f"{key} exists without this journal; reconcile manually")
        result = self.api.request("POST", endpoint, payload)
        objects[key] = {k: v for k, v in result.items()
                        if k in ("id", "username", "path", "application_id", "callback_url")}
        if secret_fields:
            self.state["credentials"][key] = {k: result[k] for k in secret_fields}
        self.save()
        return objects[key]

    def member(self, endpoint, user_id, level):
        try:
            member = self.api.request("GET", f"{endpoint}/{user_id}")
        except APIError as error:
            if error.code != 404:
                raise
            self.api.request("POST", endpoint, {"user_id": user_id, "access_level": level})
        else:
            if member["access_level"] != level:
                raise RuntimeError("Unexpected existing membership; reconcile manually")

    def run(self):
        metadata = self.api.request("GET", "metadata")
        if metadata["version"] != VERSION or metadata["enterprise"]:
            raise RuntimeError("Expected the pinned GitLab Community Edition version")
        self.api.request("PUT", "application/settings", {
            "signup_enabled": False, "shared_runners_enabled": False,
            "auto_devops_enabled": False})
        users = {}
        for role in ("collector", "clone", "mr-bot", "admin-a", "admin-b",
                     "member", "no-repo", "inactive"):
            name = "cp87-" + role
            password = self.state["credentials"].setdefault(name, {}).setdefault(
                "password", secrets.token_urlsafe(32))
            self.save()
            user = self.ensure(name, "users?" + urlencode({"username": name}),
                               lambda u: u["username"] == name, "users", {
                                   "username": name, "name": name,
                                   "email": f"{name}@{self.state['binding']['email_domain']}",
                                   "password": password, "skip_confirmation": True,
                                   "external": True, "admin": False,
                                   "can_create_group": False, "projects_limit": 0})
            users[role] = user["id"]
        inactive = self.api.request("GET", f"users/{users['inactive']}")
        if inactive["state"] != "blocked":
            self.api.request("POST", f"users/{users['inactive']}/block")
        projects = []
        for index, org in enumerate(self.state["binding"]["organizations"]):
            group_path = "cp87-" + org
            group = self.ensure(group_path, "groups?" + urlencode({"search": group_path}),
                                lambda g: g["full_path"] == group_path, "groups", {
                                    "name": group_path, "path": group_path,
                                    "visibility": "internal"})
            self.member(f"groups/{group['id']}/members", users["admin-a" if index == 0 else "admin-b"], 50)
            for visibility in ("private", "internal"):
                key = f"{group_path}/{visibility}"
                project = self.ensure(key, f"groups/{group['id']}/projects",
                                      lambda p: p["path"] == visibility, "projects", {
                                          "name": visibility, "path": visibility,
                                          "namespace_id": group["id"], "visibility": visibility,
                                          "initialize_with_readme": False,
                                          "builds_access_level": "enabled",
                                          "issues_access_level": "disabled",
                                          "wiki_access_level": "disabled",
                                          "shared_runners_enabled": False})
                projects.append(project["id"])
                for role in ("collector", "clone", "mr-bot"):
                    self.member(f"projects/{project['id']}/members", users[role], 20)
                if index == 0:
                    self.member(f"projects/{project['id']}/members", users["member"], 20)
            app_name = "craftpulse-" + org
            self.ensure(app_name, "applications", lambda a: a["application_name"] == app_name,
                        "applications", {"name": app_name,
                                         "redirect_uri": f"{self.state['binding']['stand_url']}/o/{org}/callback",
                                         "scopes": "openid profile", "confidential": True}, ("secret",))
        for role, scopes in (("collector", ["read_api"]), ("clone", ["read_repository"]),
                             ("mr-bot", ["api"])):
            key = role + "-token"
            if key not in self.state["credentials"]:
                token = self.api.request("POST", f"users/{users[role]}/personal_access_tokens", {
                    "name": "cp87-" + role, "scopes": scopes,
                    "expires_at": (date.today() + timedelta(days=30)).isoformat()})
                self.state["credentials"][key] = {
                    k: token[k] for k in ("id", "token", "scopes", "expires_at")}
                self.save()
        if "runner" not in self.state["objects"]:
            runner = self.api.request("POST", "user/runners", {
                "runner_type": "project_type", "project_id": projects[0],
                "description": "cp87-test", "tag_list": "cp87-test",
                "run_untagged": False, "locked": False, "access_level": "not_protected"})
            self.state["objects"]["runner"] = {"id": runner["id"]}
            self.state["credentials"]["runner"] = {"token": runner["token"]}
            self.save()
        runner_id = self.state["objects"]["runner"]["id"]
        attached = self.api.request("GET", f"runners/{runner_id}")
        current = {p["id"] for p in attached["projects"]}
        if current - set(projects):
            raise RuntimeError("Runner is attached to a non-test project")
        for project_id in projects:
            if project_id not in current:
                self.api.request("POST", f"projects/{project_id}/runners", {"runner_id": runner_id})
        self.api.request("PUT", f"runners/{runner_id}", {"locked": True, "run_untagged": False})
        self.state["test_project_ids"] = projects
        self.save()


def runner_config(state, ca_host_dir):
    if not PurePosixPath(ca_host_dir).is_absolute() or ":" in ca_host_dir:
        raise ValueError("CA directory must be an absolute Linux path")
    token = state["credentials"]["runner"]["token"]
    if not token.startswith("glrt-"):
        raise ValueError("Expected a runner authentication token")
    # JSON string escaping is compatible with these TOML basic strings.
    return f'''concurrent = 1
check_interval = 3

[[runners]]
  name = "cp87-test"
  url = {json.dumps(origin(state["binding"]["gitlab_url"]))}
  token = {json.dumps(token)}
  executor = "docker"
  tls-ca-file = "/etc/gitlab-runner/certs/ca-bundle.crt"
  [runners.docker]
    image = {json.dumps(CURL_IMAGE)}
    privileged = false
    cpus = "0.5"
    memory = "512m"
    memory_swap = "768m"
    volumes = [{json.dumps(ca_host_dir + ":/etc/craftpulse-ca:ro")}, "/cache"]
'''


def inspect(api, state):
    metadata = api.request("GET", "metadata")
    if metadata["version"] != VERSION or metadata["enterprise"]:
        raise RuntimeError("GitLab version or edition differs")
    for role in ("collector", "clone", "mr-bot", "no-repo"):
        user = api.request("GET", f"users/{state['objects']['cp87-' + role]['id']}")
        if user["is_admin"] or not user["external"] or user["state"] != "active":
            raise RuntimeError("Account isolation differs")
    inactive = api.request("GET", f"users/{state['objects']['cp87-inactive']['id']}")
    if inactive["state"] != "blocked":
        raise RuntimeError("Inactive account is not blocked")
    for project_id in state["test_project_ids"]:
        project = api.request("GET", f"projects/{project_id}")
        if project["shared_runners_enabled"] or project["issues_enabled"]:
            raise RuntimeError("Shared runners or issue tracker unexpectedly enabled")
        for role in ("collector", "clone", "mr-bot"):
            member = api.request("GET", f"projects/{project_id}/members/all/"
                                 f"{state['objects']['cp87-' + role]['id']}")
            if member["access_level"] != 20:
                raise RuntimeError("Integration account is not Reporter")
    for role, scopes in (("collector", {"read_api"}), ("clone", {"read_repository"}),
                         ("mr-bot", {"api"})):
        token = api.request("GET", "personal_access_tokens/" +
                            str(state["credentials"][role + "-token"]["id"]))
        if set(token["scopes"]) != scopes or token["revoked"] or not token["active"]:
            raise RuntimeError("Integration token scopes or validity differ")
    apps = {app["id"]: app for app in api.pages("applications")}
    for org in state["binding"]["organizations"]:
        app = apps[state["objects"]["craftpulse-" + org]["id"]]
        expected = f"{state['binding']['stand_url']}/o/{org}/callback"
        if app["callback_url"] != expected or set(app["scopes"]) != {"openid", "profile"}:
            raise RuntimeError("OIDC redirect or scopes differ")
    runner_id = state["objects"]["runner"]["id"]
    runner = api.request("GET", f"runners/{runner_id}")
    if (not runner["locked"] or runner["run_untagged"] or not runner["online"]
            or runner["runner_type"] != "project_type" or runner["tag_list"] != ["cp87-test"]
            or {p["id"] for p in runner["projects"]} != set(state["test_project_ids"])):
        raise RuntimeError("Runner is offline or its project isolation differs")
    managers = list(api.pages(f"runners/{runner_id}/managers"))
    if not managers or any(m["version"] != RUNNER_VERSION for m in managers):
        raise RuntimeError("Runner version differs")
    return {"gitlab_url": api.url, "version": VERSION, "edition": "CE",
            "runner_id": runner_id, "runner_version": RUNNER_VERSION,
            "projects": state["test_project_ids"], "configuration_check": "passed",
            "live_acceptance": "pending: HTTPS from stand, pipeline, negative access and MR checks"}


def acceptance(api, state, ca, control_projects, test_mr, control_mr, pipeline_id):
    report = inspect(api, state)
    test_ids = state["test_project_ids"]
    if len(set(control_projects)) != 2 or set(control_projects) & set(test_ids):
        raise ValueError("Two distinct non-test control projects are required")
    for project_id, visibility in zip(control_projects, ("private", "internal")):
        project = api.request("GET", f"projects/{project_id}")
        if project["visibility"] != visibility:
            raise ValueError("Control projects must be private and internal, respectively")
    reader = API(api.url, state["credentials"]["collector-token"]["token"], ca)
    for project_id in test_ids:
        reader.request("GET", f"projects/{project_id}")
    bot = API(api.url, state["credentials"]["mr-bot-token"]["token"], ca)
    for project_id in control_projects:
        try:
            reader.request("GET", f"projects/{project_id}")
        except APIError as error:
            if error.code != 404:
                raise
        else:
            raise RuntimeError("Collector can read a control project")
        try:
            note = bot.request("POST", f"projects/{project_id}/merge_requests/{control_mr}/notes",
                               {"body": "craftPulse #87 access acceptance probe"})
        except APIError as error:
            if error.code != 404:
                raise
        else:
            bot.request("DELETE", f"projects/{project_id}/merge_requests/{control_mr}/notes/{note['id']}")
            raise RuntimeError("Bot can comment in a control project")
    for project_id in control_projects:
        # Prove each negative probe addressed a real MR, not a missing object.
        api.request("GET", f"projects/{project_id}/merge_requests/{control_mr}")
    bot.request("POST", f"projects/{test_ids[0]}/merge_requests/{test_mr}/notes",
                {"body": "craftPulse #87: test-project MR comment accepted"})
    opener = build_opener(NoRedirect(), HTTPSHandler(context=ssl.create_default_context(cafile=ca)))
    with opener.open(api.url + "/.well-known/openid-configuration", timeout=30) as response:
        discovery = json.load(response)
    if discovery["issuer"] != api.url:
        raise RuntimeError("OIDC issuer differs from GitLab origin")
    for field in ("authorization_endpoint", "token_endpoint", "jwks_uri"):
        endpoint = urlsplit(discovery[field])
        if endpoint.scheme != "https" or endpoint.netloc != urlsplit(api.url).netloc:
            raise RuntimeError("OIDC endpoint differs from GitLab origin")
    pipeline = api.request("GET", f"projects/{test_ids[0]}/pipelines/{pipeline_id}")
    jobs = list(api.pages(f"projects/{test_ids[0]}/pipelines/{pipeline_id}/jobs"))
    smoke_jobs = [job for job in jobs if job["name"] == "stand-connectivity"]
    if (pipeline["status"] != "success" or len(smoke_jobs) != 1
            or smoke_jobs[0]["status"] != "success"
            or smoke_jobs[0]["runner"]["id"] != state["objects"]["runner"]["id"]):
        raise RuntimeError("Smoke pipeline did not pass on the separate runner")
    report.update({"issuer": discovery["issuer"], "pipeline_id": pipeline_id,
                   "job_id": smoke_jobs[0]["id"], "access_checks": "passed",
                   "live_acceptance": "pending: clone-read and browser login for both organizations"})
    return report


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("command", choices=("bootstrap", "runner-config", "inspect", "acceptance"))
    parser.add_argument("--state", required=True, type=Path)
    parser.add_argument("--url")
    parser.add_argument("--admin-token-file", type=Path)
    parser.add_argument("--ca", help="PEM CA bundle; otherwise use system trust")
    parser.add_argument("--stand-url")
    parser.add_argument("--org-a", default="org-a")
    parser.add_argument("--org-b", default="org-b")
    parser.add_argument("--email-domain", default="example.test")
    parser.add_argument("--output", type=Path)
    parser.add_argument("--ca-host-dir", default="/etc/craftpulse-gitlab-runner/certs")
    parser.add_argument("--control-private", type=int)
    parser.add_argument("--control-internal", type=int)
    parser.add_argument("--test-mr", type=int)
    parser.add_argument("--control-mr", type=int)
    parser.add_argument("--pipeline", type=int)
    args = parser.parse_args()
    try:
        if args.command == "runner-config":
            if not args.output:
                parser.error("--output is required")
            save_private(args.output, runner_config(json.loads(args.state.read_text()), args.ca_host_dir))
        else:
            if not args.url or not args.admin_token_file:
                parser.error("--url and --admin-token-file are required")
            api = API(args.url, args.admin_token_file.read_text().strip(), args.ca)
            if args.command == "bootstrap":
                if not args.stand_url:
                    parser.error("--stand-url is required")
                Provision(api, args.state, args.stand_url,
                          [args.org_a, args.org_b], args.email_domain).run()
            else:
                state = json.loads(args.state.read_text())
                if api.url != state["binding"]["gitlab_url"]:
                    raise ValueError("State belongs to another GitLab instance")
                if args.command == "acceptance":
                    if not all((args.control_private, args.control_internal,
                                args.test_mr, args.control_mr, args.pipeline)):
                        parser.error("Acceptance requires control project IDs, MR IIDs and pipeline ID")
                    report = acceptance(api, state, args.ca,
                                        [args.control_private, args.control_internal],
                                        args.test_mr, args.control_mr, args.pipeline)
                else:
                    report = inspect(api, state)
                print(json.dumps(report, indent=2))
        print("Completed; secrets were not printed.", file=sys.stderr)
        return 0
    except RuntimeError as error:
        print(str(error), file=sys.stderr)
        return 1
    except (ValueError, OSError, KeyError):
        print("Operation failed; check inputs, TLS, permissions and journal. Secrets were not printed.",
              file=sys.stderr)
        return 1


if __name__ == "__main__":
    sys.exit(main())
