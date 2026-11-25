"""Source versions and fixture state for the craftPulse demo stand."""

from contextlib import redirect_stdout
from hashlib import sha256
from http.client import HTTPSConnection
import importlib.util
import io
import json
import os
from pathlib import Path
import re
import socket
import sys
from urllib.error import URLError
from urllib.parse import urlencode, urlsplit
from urllib.request import HTTPRedirectHandler, HTTPSHandler, ProxyHandler, Request, build_opener


HERE = Path(__file__).resolve().parent
DEPLOY = HERE.parents[1]
sys.path.insert(0, str(DEPLOY / "profile-d"))
from rollout import SHA, validate_manifest

spec = importlib.util.spec_from_file_location("github_fixture", DEPLOY / "github/manage.py")
github = importlib.util.module_from_spec(spec)
spec.loader.exec_module(github)

IMAGE = re.compile(r"[a-zA-Z0-9._/:+-]+@sha256:[a-f0-9]{64}")
DEMO_MARKER = "cp92-reset-"


class StandError(RuntimeError):
    pass


def require(condition, message):
    if not condition:
        raise StandError(message)


def read_json(path):
    return json.loads(path.read_bytes())


def json_bytes(value):
    return (json.dumps(value, indent=2, sort_keys=True, ensure_ascii=False) + "\n").encode("utf-8")


def fingerprint(value):
    return sha256(json_bytes(value)).hexdigest()


def build_manifest(release, runtime):
    sha = release["commit_sha"]
    require(isinstance(sha, str) and SHA.fullmatch(sha), "invalid release SHA")
    release = validate_manifest(release, sha)
    require(runtime["release_sha"] == sha and runtime["schema_version"] == release["core_schema_version"],
            "release manifest and stand record disagree on SHA or schema")
    images = {name: item["image"] for name, item in runtime["runtime"].items()}
    require(all(IMAGE.fullmatch(ref) for ref in images.values()), "stand record contains an unpinned image")
    for service in ("server", "worker", "frontend"):
        image = "craftpulse-frontend" if service == "frontend" else "craftpulse-server"
        require(images[f"craftpulse-product-{service}-1"] == release["images"][image],
                f"stand record and release manifest disagree on {service} digest")

    sources = read_json(HERE / "sources.json")
    for name, edition in (("jenkins", "Jenkins LTS"), ("sonarqube", "Community Build")):
        record = read_json(DEPLOY / name / "instance.json")
        sources[name] = {key: record[key] for key in ("url", "version", "accounts")}
        sources[name]["edition"] = record.get("edition", edition)
    sources["jenkins"]["plugins"] = dict(
        line.split(":", 1) for line in (DEPLOY / "jenkins/plugins.txt").read_text().splitlines()
        if line and not line.startswith("#")
    )
    sources["jenkins"]["jobs"] = list(read_json(DEPLOY / "jenkins/instance.json")["jobs"].values())
    sonar_projects = read_json(DEPLOY / "sonarqube/instance.json")["projects"]
    sources["sonarqube"]["projects"] = [sonar_projects[key] for key in ("org-a", "org-b", "negative_control")]
    for source, suffix, major in (
        ("gitlab", "gitlab-gitlab-1", r":([0-9.]+)-ce\."),
        ("runner", "gitlab-runner", r":v([0-9.]+)@"),
    ):
        image = images["craftpulse-" + suffix]
        match = re.search(major, image)
        require(match, f"cannot determine {source} version from stand image")
        sources[source].update(image=image, version=match[1])
    return {"format": 1, "sources": sources, "runtime_images": images,
            "craftpulse": {"url": "https://craftpulse.defects.team", **release}}


def validate_stand(manifest):
    """Bind credentials and mutating fixture operations to the recorded test scopes."""
    require(manifest["format"] == 1, "unsupported stand manifest format")
    configured = read_json(HERE / "sources.json")
    configured.update({name: read_json(DEPLOY / name / "instance.json") for name in ("jenkins", "sonarqube")})
    require(set(manifest["sources"]) == configured.keys(), "missing or unexpected stand source")
    fixture_keys = ("url", "projects", "spaces", "id")
    for name, source in manifest["sources"].items():
        for key in fixture_keys:
            if key in configured[name] and (name, key) != ("sonarqube", "projects"):
                require(source[key] == configured[name][key], f"{name}: unfamiliar {key}")
    require(manifest["craftpulse"]["url"] == "https://craftpulse.defects.team", "unfamiliar craftPulse URL")
    release = manifest["craftpulse"]
    require(SHA.fullmatch(release["commit_sha"]), "invalid release SHA")
    validate_manifest(release, release["commit_sha"])


class NoRedirect(HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


class LoopbackHTTPSHandler(HTTPSHandler):
    def https_open(self, request):
        def connection(host, **options):
            client = HTTPSConnection(host, **options)
            # Keep the original host for SNI and certificate verification.
            client._create_connection = lambda address, *args: socket.create_connection(("127.0.0.1", address[1]), *args)
            return client

        return self.do_open(connection, request, context=self._context)


class Client:
    def __init__(self, name, base_url, credential=None, header="Authorization", prefix="", local=False):
        self.name = name
        self.url = base_url
        self.headers = {"Accept": "application/json", "User-Agent": "craftpulse-stand-snapshot"}
        if credential:
            value = os.environ.get(credential, "")
            require(bool(value) and not any(c in value for c in "\r\n"), f"configure {credential}")
            self.headers[header] = prefix + value
        local_handlers = (ProxyHandler({}), LoopbackHTTPSHandler()) if local else ()
        self.opener = build_opener(NoRedirect(), *local_handlers)

    def request(self, path, *, query=None, method="GET", data=None, extra=None):
        require(path.startswith("/") and not path.startswith("//"), f"{self.name}: invalid API path")
        url = self.url + path + ("?" + urlencode(query) if query else "")
        headers = dict(self.headers, **(extra or {}))
        try:
            request = Request(url, headers=headers, data=json_bytes(data) if data is not None else None, method=method)
            if data is not None:
                request.add_header("Content-Type", "application/json")
            with self.opener.open(request, timeout=25) as response:
                return response.read(), response.headers
        except URLError as error:
            code = getattr(error, "code", None)
            reason = f"HTTP {code}" if code else "connection or TLS error"
            raise StandError(f"{self.name}: {method} {reason}") from None
        except (TimeoutError, OSError, ValueError):
            raise StandError(f"{self.name}: {method} request failed") from None

    def get(self, path, **options):
        return json.loads(self.request(path, **options)[0])


def differences(expected, actual, path=""):
    if isinstance(expected, dict) and isinstance(actual, dict):
        return [item for key in sorted(expected.keys() | actual.keys())
                for item in ([f"{path}.{key}"] if key not in expected or key not in actual
                             else differences(expected[key], actual[key], f"{path}.{key}"))]
    return [] if expected == actual else [path]


class Sources:
    def __init__(self, manifest, local=False):
        validate_stand(manifest)
        self.manifest = manifest
        self.records = manifest["sources"]
        self.local = local

    def clients(self):
        urls = {name: source["url"] for name, source in self.records.items()}
        return {
            "jira": Client("jira", urls["jira"], "CP92_ATLASSIAN_BASIC", prefix="Basic "),
            "confluence": Client("confluence", urls["confluence"], "CP92_ATLASSIAN_BASIC", prefix="Basic "),
            "gitlab": Client("gitlab", urls["gitlab"], "CP92_GITLAB_TOKEN", header="PRIVATE-TOKEN", local=self.local),
            "jenkins": Client("jenkins", urls["jenkins"], "CP92_JENKINS_BASIC", prefix="Basic ", local=self.local),
            "sonarqube": Client("sonarqube", urls["sonarqube"], "CP92_SONAR_TOKEN", prefix="Bearer ", local=self.local),
            "github": Client("github", "https://api.github.com"),
            "craftpulse": Client("craftpulse", self.manifest["craftpulse"]["url"], local=self.local),
        }

    def compare_versions(self, only=None):
        clients = self.clients()
        checks = {
            "jira": lambda: jira_version(clients["jira"]),
            "confluence": lambda: confluence_version(clients["confluence"]),
            "gitlab": lambda: gitlab_version(clients["gitlab"]),
            "runner": lambda: runner_version(clients["gitlab"], self.records["runner"]["id"]),
            "github": lambda: github_version(clients["github"], self.records["github"]["api_version"]),
            "jenkins": lambda: jenkins_version(clients["jenkins"]),
            "sonarqube": lambda: {"version": clients["sonarqube"].request("/api/server/version")[0].decode().strip()},
            "craftpulse": lambda: craftpulse_version(clients["craftpulse"]),
        }
        errors = []
        for name, check in checks.items():
            if only and name not in only:
                continue
            try:
                actual = check()
                expected = self.records.get(name, self.manifest["craftpulse"])
                if name == "craftpulse":
                    expected = {**expected, "schema_version": expected["core_schema_version"]}
                errors.extend(f"{name}: differs at {key}"
                              for key, value in actual.items() if expected[key] != value)
            except (StandError, KeyError, TypeError, ValueError) as error:
                detail = str(error) if isinstance(error, StandError) else "invalid version response"
                errors.append(f"{name}: {detail}")
        return errors

    def cloud_state(self, clients):
        jira = clients["jira"]
        confluence = clients["confluence"]
        issues = {}
        for key, identifier in self.records["jira"]["projects"].items():
            project = jira.get("/rest/api/3/project/" + key)
            require(project["id"] == identifier, "jira: unexpected fixture project ID")
            for issue in jira_issues(jira, key):
                fields = issue["fields"]
                issues[issue["id"]] = {
                    "key": issue["key"], "project": key,
                    "summary": fields["summary"], "description": fields["description"],
                    "status": fields["status"]["id"], "labels": sorted(fields["labels"]),
                    "security": (fields.get("security") or {}).get("id"),
                }

        pages = {}
        for key, identifier in self.records["confluence"]["spaces"].items():
            space = confluence.get("/api/v2/spaces/" + identifier)
            require(space["key"] == key, "confluence: unexpected fixture space ID")
            for page in confluence_pages(confluence, identifier):
                require(page["spaceId"] == identifier, "confluence: page outside fixture space")
                pages[page["id"]] = {
                    "spaceId": identifier, "parentId": page.get("parentId"), "title": page["title"],
                    "body": page["body"]["storage"]["value"], "status": page["status"],
                }
        return {"jira": issues, "confluence": pages}

    def local_state(self, clients):
        gitlab = clients["gitlab"]
        projects = {}
        for identifier in self.records["gitlab"]["projects"]:
            root = f"/api/v4/projects/{identifier}"
            projects[str(identifier)] = {
                "branches": selected(gitlab, root + "/repository/branches",
                                     lambda row: [row["name"], row["commit"]["id"]]),
                "merge_requests": selected(gitlab, root + "/merge_requests",
                                           lambda row: [row["iid"], row["sha"], row["state"], row["title"]]),
                "pipelines": selected(gitlab, root + "/pipelines",
                                      lambda row: [row["id"], row["sha"], row["status"]]),
            }
        jenkins = clients["jenkins"]
        builds = {}
        for job in self.records["jenkins"]["jobs"]:
            require(re.fullmatch(r"[a-z0-9-]+", job), "jenkins: invalid fixture job")
            data = jenkins.get(f"/job/{job}/api/json", query={"tree": "allBuilds[number,result,building]"})
            require(not any(build["building"] for build in data["allBuilds"]), "jenkins: fixture build is running")
            builds[job] = data["allBuilds"]

        sonar = clients["sonarqube"]
        analyses = {}
        for project in self.records["sonarqube"]["projects"]:
            analyses[project] = sonar_analyses(sonar, project)
        return {"gitlab": fingerprint(projects), "jenkins": fingerprint(builds), "sonarqube": fingerprint(analyses)}


def jira_version(client):
    data = client.get("/rest/api/3/serverInfo")
    return {"version": data["version"], "edition": data["deploymentType"], "build": data["buildNumber"]}


def confluence_version(client):
    data = client.get("/rest/api/settings/systemInfo")
    build = data["commitHash"]
    require(isinstance(build, str) and re.fullmatch(r"[0-9a-f]{40}", build),
            "confluence: systemInfo does not expose a commit build")
    return {"version": build, "edition": "Cloud " + data["edition"].capitalize()}


def gitlab_version(client):
    data = client.get("/api/v4/metadata")
    require(type(data["enterprise"]) is bool, "gitlab: metadata does not identify edition")
    return {"version": data["version"].removesuffix("-ce").removesuffix("-ee"),
            "edition": "Enterprise Edition" if data["enterprise"] else "Community Edition"}


def runner_version(client, identifier):
    managers = client.get(f"/api/v4/runners/{identifier}/managers")
    versions = {manager["version"] for manager in managers if manager["status"] == "online"}
    require(len(versions) == 1, "runner: expected a single version among online managers")
    return {"version": versions.pop()}


def github_version(client, version):
    _, headers = client.request("/meta", extra={"X-GitHub-Api-Version": version})
    selected_version = headers.get("X-GitHub-Api-Version-Selected")
    require(bool(selected_version), "github: response omitted the selected API version")
    return {"version": selected_version, "api_version": selected_version}


def jenkins_version(client):
    raw, headers = client.request("/pluginManager/api/json",
                                  query={"tree": "plugins[shortName,version,active]"})
    return {"version": headers["X-Jenkins"],
            "plugins": {p["shortName"]: p["version"] for p in json.loads(raw)["plugins"] if p["active"]}}


def craftpulse_version(client):
    data = client.get("/version")
    return {key: data[key] for key in ("commit_sha", "schema_version")}


def jira_issues(client, key):
    query = {"jql": f"project = {key} ORDER BY key", "maxResults": 100,
             "fields": "summary,description,status,labels,security"}
    for _ in range(1000):
        data = client.get("/rest/api/3/search/jql", query=query)
        yield from data["issues"]
        if data["isLast"]:
            return
        query["nextPageToken"] = data["nextPageToken"]
    raise StandError("jira: refusing partial fixture inventory")


def confluence_pages(client, space):
    path = f"/api/v2/spaces/{space}/pages"
    query = {"status": "current", "body-format": "storage", "limit": 100}
    request_path = path
    for _ in range(1000):
        data = client.get(request_path, query=query)
        yield from data["results"]
        next_url = data["_links"].get("next")
        if not next_url:
            return
        next_path = urlsplit(next_url)
        require(next_path.path.removeprefix("/wiki") == path and next_path.query,
                "confluence: unexpected pagination path")
        request_path, query = path + "?" + next_path.query, None
    raise StandError("confluence: refusing partial fixture inventory")


def selected(client, path, pick):
    items = []
    for page in range(1, 1001):
        rows = client.get(path, query={"per_page": 100, "page": page})
        items.extend(pick(row) for row in rows)
        if len(rows) < 100:
            return sorted(items)
    raise StandError("gitlab: refusing partial fixture inventory")


def sonar_analyses(client, project):
    analyses = []
    for page in range(1, 1001):
        data = client.get("/api/project_analyses/search", query={"project": project, "ps": 100, "p": page})
        analyses.extend([item["key"], item["date"]] for item in data["analyses"])
        if len(analyses) >= data["paging"]["total"]:
            return sorted(analyses)
    raise StandError("sonarqube: refusing partial fixture inventory")


def cloud_plan(expected, current):
    actions = []
    for source, title in (("jira", "summary"), ("confluence", "title")):
        before, after = expected[source], current[source]
        require(before.keys() <= after.keys(), f"{source}: an original fixture is missing")
        for identifier, item in sorted(after.items()):
            require(identifier.isdecimal(), f"{source}: invalid fixture ID")
            original = before.get(identifier)
            if original is None:
                require(item[title].startswith(DEMO_MARKER), f"{source}: unmarked new fixture; restore stopped")
                actions.append((source, identifier, "delete", item))
                continue
            editable = ("summary", "description", "labels") if source == "jira" else ("title", "body")
            require(all(item[key] == original[key] for key in item.keys() - set(editable)),
                    f"{source}: non-editable fixture attributes differ")
            if item != original:
                actions.append((source, identifier, "update", {key: original[key] for key in editable}))
    return actions


def apply_cloud(clients, actions):
    for source, identifier, operation, item in actions:
        client = clients[source]
        if source == "jira":
            path = "/rest/api/3/issue/" + identifier
            data = {"fields": item} if operation == "update" else None
        else:
            path = "/api/v2/pages/" + identifier
            data = None
            if operation == "update":
                current = client.get(path, query={"body-format": "storage"})
                data = {"id": identifier, "status": "current", "title": item["title"],
                        "body": {"representation": "storage", "value": item["body"]},
                        "version": {"number": current["version"]["number"] + 1}}
        client.request(path, method="PUT" if operation == "update" else "DELETE", data=data)


def github_call(operation):
    # The baseline and GitHub diff may contain fixture text or comments.
    try:
        with redirect_stdout(io.StringIO()):
            return operation(github.operator_api())
    except (RuntimeError, ValueError, OSError, KeyError):
        raise StandError("github: fixture operation failed") from None
