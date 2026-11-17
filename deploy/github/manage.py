#!/usr/bin/env python3
"""Provision and reset the disposable GitHub fixtures for craftPulse issue #89."""

import argparse
import json
import os
from pathlib import Path
import re
import subprocess
import sys
from urllib.error import HTTPError
from urllib.parse import quote
from urllib.request import HTTPRedirectHandler, Request, build_opener

API_VERSION = "2022-11-28"
ORGANIZATION = "craftpulse-mvp"
ORGANIZATION_ID = 357841588
MARKER = "craftPulse MVP-01-T15 disposable fixture (#89)"
REPOSITORIES = {"cp89-org-a": "org-a", "cp89-org-a-private": "org-a", "cp89-org-b": "org-b"}
CONTROL = "cp89-control"


class APIError(RuntimeError):
    def __init__(self, method, path, code):
        self.code = code
        super().__init__(f"GitHub {method} {path}: HTTP {code}")


class NoRedirect(HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


class API:
    def __init__(self, token):
        if not token or any(c.isspace() for c in token):
            raise ValueError("A nonempty GitHub token without whitespace is required")
        self.token = token
        self.opener = build_opener(NoRedirect())
        self.writes = 0

    def request(self, method, path, data=None):
        if not re.fullmatch(r"[A-Za-z0-9_./?=&%+-]+", path) or path.startswith("/") or ".." in path:
            raise ValueError("Expected a relative GitHub API path")
        headers = {"Accept": "application/vnd.github+json", "X-GitHub-Api-Version": API_VERSION,
                   "Authorization": "Bearer " + self.token, "User-Agent": "craftpulse-cp89"}
        body = None if data is None else json.dumps(data).encode()
        if body is not None:
            headers["Content-Type"] = "application/json"
        request = Request("https://api.github.com/" + path, data=body, headers=headers, method=method)
        try:
            with self.opener.open(request, timeout=45) as response:
                selected = response.headers.get("X-GitHub-Api-Version-Selected")
                if selected and selected != API_VERSION:
                    raise RuntimeError("GitHub selected an unexpected API version")
                raw = response.read()
        except HTTPError as error:
            raise APIError(method, path, error.code) from None
        if method != "GET":
            self.writes += 1
        return json.loads(raw) if raw else None

    def pages(self, path):
        for page in range(1, 1001):
            separator = "&" if "?" in path else "?"
            items = self.request("GET", f"{path}{separator}per_page=100&page={page}")
            if not isinstance(items, list):
                raise RuntimeError("Expected a paginated array")
            yield from items
            if len(items) < 100:
                return
        raise RuntimeError("Pagination exceeded 1000 pages; refusing partial snapshot")


def operator_api():
    token = os.environ.get("GITHUB_RESET_TOKEN")
    if not token:
        result = subprocess.run(["gh", "auth", "token", "--hostname", "github.com"],
                                capture_output=True, text=True, check=False)
        if result.returncode:
            raise RuntimeError("Set GITHUB_RESET_TOKEN or authenticate gh on github.com")
        token = result.stdout.strip()
    return API(token)


def save_new(path, value):
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("x", encoding="utf-8") as output:
        json.dump(value, output, indent=2, ensure_ascii=False)
        output.write("\n")


def binding(api):
    org = api.request("GET", "orgs/" + ORGANIZATION)
    if org["id"] != ORGANIZATION_ID or org["type"] != "Organization":
        raise RuntimeError("Unexpected organization identity")


def validate(baseline):
    if (baseline.get("schema") != 1 or baseline.get("organization") != ORGANIZATION
            or baseline.get("organization_id") != ORGANIZATION_ID
            or baseline.get("api_version") != API_VERSION
            or set(baseline.get("repositories", {})) != set(REPOSITORIES)):
        raise ValueError("Unexpected baseline binding/schema/repository allowlist")
    for name, repo in baseline["repositories"].items():
        if (repo["name"] != name or not isinstance(repo["id"], int)
                or repo["tenant"] != REPOSITORIES[name] or repo["private"] is not True
                or repo["default_branch"] != "main" or not repo["pulls"]):
            raise ValueError("Invalid repository baseline")
        if "main" not in repo["branches"]:
            raise ValueError("Missing main branch")
        for ref, sha in repo["branches"].items():
            if (not re.fullmatch(r"[A-Za-z0-9_-]+(?:/[A-Za-z0-9_-]+)*", ref)
                    or not re.fullmatch(r"[a-f0-9]{40}", sha)):
                raise ValueError("Invalid ref/SHA")
        numbers = [p["number"] for p in repo["pulls"]]
        if len(numbers) != len(set(numbers)):
            raise ValueError("Duplicate PR number")
        for pull in repo["pulls"]:
            if (pull["state"] != "open" or pull["merged"] or pull["draft"]
                    or pull["reviews"] or pull["review_comments"]
                    or pull["head"]["ref"] == pull["base"]["ref"]
                    or any(pull[end]["sha"] != repo["branches"].get(pull[end]["ref"])
                           for end in ("base", "head"))):
                raise ValueError("Baseline requires open, unmerged, ready PRs without submitted reviews")


def snapshot_repo(api, name):
    base = f"repos/{ORGANIZATION}/{name}"
    repo = api.request("GET", base)
    if (repo["owner"]["id"] != ORGANIZATION_ID or repo["name"] != name
            or repo["description"] != MARKER or not repo["private"]):
        raise RuntimeError(f"Refusing non-fixture repository: {name}")
    branches = {branch["name"]: branch["commit"]["sha"] for branch in api.pages(base + "/branches")}
    pulls = []
    for summary in api.pages(base + "/pulls?state=all"):
        number = summary["number"]
        pull = api.request("GET", f"{base}/pulls/{number}")
        if any(pull[end]["repo"] is None or pull[end]["repo"]["id"] != repo["id"]
               for end in ("base", "head")):
            raise RuntimeError("Fork PRs cannot be reset by this fixture tool")
        reviewers = api.request("GET", f"{base}/pulls/{number}/requested_reviewers")
        comments = [{"id": c["id"], "created_at": c["created_at"],
                     "author": c["user"]["login"], "body": c["body"]}
                    for c in api.pages(f"{base}/issues/{number}/comments")]
        reviews = [{"id": r["id"], "state": r["state"], "body": r["body"],
                    "commit_id": r["commit_id"], "author": r["user"]["login"]}
                   for r in api.pages(f"{base}/pulls/{number}/reviews")]
        review_comments = [{"id": c["id"], "body": c["body"], "author": c["user"]["login"]}
                           for c in api.pages(f"{base}/pulls/{number}/comments")]
        pulls.append({"id": pull["id"], "number": number, "created_at": pull["created_at"],
                      "state": pull["state"], "merged": pull["merged"], "draft": pull["draft"],
                      "base": {"ref": pull["base"]["ref"], "sha": pull["base"]["sha"]},
                      "head": {"ref": pull["head"]["ref"], "sha": pull["head"]["sha"]},
                      "reviewers": sorted(u["login"] for u in reviewers["users"]),
                      "teams": sorted(t["slug"] for t in reviewers["teams"]),
                      "comments": sorted(comments, key=lambda c: c["id"]),
                      "reviews": sorted(reviews, key=lambda r: r["id"]),
                      "review_comments": sorted(review_comments, key=lambda c: c["id"])})
    return {"name": name, "id": repo["id"], "tenant": REPOSITORIES[name],
            "private": repo["private"], "default_branch": repo["default_branch"],
            "branches": branches, "pulls": sorted(pulls, key=lambda p: p["number"])}


def snapshot(api):
    binding(api)
    return {"schema": 1, "organization": ORGANIZATION, "organization_id": ORGANIZATION_ID,
            "api_version": API_VERSION,
            "repositories": {name: snapshot_repo(api, name) for name in REPOSITORIES}}


def changes(expected, actual, path=""):
    if isinstance(expected, dict) and isinstance(actual, dict):
        return [change for key in sorted(set(expected) | set(actual))
                for change in changes(expected.get(key), actual.get(key), path + "/" + str(key))]
    if isinstance(expected, list) and isinstance(actual, list) and len(expected) == len(actual):
        return [change for i, (left, right) in enumerate(zip(expected, actual))
                for change in changes(left, right, path + "/" + str(i))]
    # Comment contents never appear in diagnostic logs.
    return [path] if expected != actual else []


def reset_plan(expected, actual, comment_authors):
    """Preflight the entire organization before issuing any write."""
    validate(expected)
    plan = []
    for name, wanted in expected["repositories"].items():
        current = actual["repositories"][name]
        if any(current[k] != wanted[k] for k in ("id", "private", "default_branch")):
            raise RuntimeError("Repository binding/settings drift; no writes planned")
        pulls = {p["number"]: p for p in current["pulls"]}
        if set(pulls) != {p["number"] for p in wanted["pulls"]}:
            raise RuntimeError("PR set changed; refusing to close or recreate PRs")
        base = f"repos/{ORGANIZATION}/{name}"
        for pull in wanted["pulls"]:
            observed = pulls[pull["number"]]
            if (any(observed[k] != pull[k] for k in ("id", "created_at", "state", "draft", "merged"))
                    or observed["head"]["ref"] != pull["head"]["ref"]
                    or observed["base"]["ref"] != pull["base"]["ref"]
                    or observed["reviews"] != pull["reviews"]
                    or observed["review_comments"] != pull["review_comments"]):
                raise RuntimeError("PR identity/lifecycle/reviews changed; manual reconciliation required")
            comments = {c["id"]: c for c in observed["comments"]}
            original = {c["id"]: c for c in pull["comments"]}
            if original.keys() - comments.keys():
                raise RuntimeError("An original comment was deleted; its identity cannot be restored")
            for cid, comment in comments.items():
                if cid not in original:
                    if comment["author"] not in comment_authors:
                        raise RuntimeError("Unexpected comment author; refusing deletion")
                    plan.append(("DELETE", f"{base}/issues/comments/{cid}", None))
                elif comment != original[cid]:
                    if (comment["author"] not in comment_authors
                            or any(comment[k] != original[cid][k] for k in ("created_at", "author"))):
                        raise RuntimeError("Original comment identity/author drift")
                    plan.append(("PATCH", f"{base}/issues/comments/{cid}", {"body": original[cid]["body"]}))
            endpoint = f"{base}/pulls/{pull['number']}/requested_reviewers"
            for method, left, right in (("DELETE", observed, pull), ("POST", pull, observed)):
                users = sorted(set(left["reviewers"]) - set(right["reviewers"]))
                teams = sorted(set(left["teams"]) - set(right["teams"]))
                if users or teams:
                    plan.append((method, endpoint, {"reviewers": users, "team_reviewers": teams}))
        for branch, sha in wanted["branches"].items():
            if branch not in current["branches"]:
                plan.append(("POST", base + "/git/refs", {"ref": "refs/heads/" + branch, "sha": sha}))
            elif current["branches"][branch] != sha:
                plan.append(("PATCH", base + "/git/refs/heads/" + quote(branch, safe="/"),
                             {"sha": sha, "force": True}))
        for branch in sorted(current["branches"].keys() - wanted["branches"].keys()):
            plan.append(("DELETE", base + "/git/refs/heads/" + quote(branch, safe="/"), None))
    return plan


def reset(api, baseline, apply=False, comment_authors=()):
    current = snapshot(api)
    differences = changes(baseline, current)
    print(json.dumps({"differences": differences}, ensure_ascii=False))
    plan = reset_plan(baseline, current, set(comment_authors))
    print(json.dumps({"planned_writes": len(plan), "apply": apply}))
    if apply:
        for method, path, data in plan:
            api.request(method, path, data)
        # Re-fetch even for an empty plan. A concurrent edit must not look successful.
        remaining = changes(baseline, snapshot(api))
        print(json.dumps({"differences_after": remaining, "writes": len(plan)}))
        if remaining:
            raise RuntimeError("Reset incomplete/concurrent modification; compare again before retrying")
    return bool(differences)


def commit(api, name, parent_sha, files, message):
    base = f"repos/{ORGANIZATION}/{name}"
    parent = api.request("GET", f"{base}/git/commits/{parent_sha}")
    tree = api.request("POST", base + "/git/trees", {
        "base_tree": parent["tree"]["sha"],
        "tree": [{"path": path, "mode": "100644", "type": "blob", "content": content}
                 for path, content in files.items()]})
    return api.request("POST", base + "/git/commits", {
        "message": message, "tree": tree["sha"], "parents": [parent_sha]})["sha"]


def bootstrap(api, journal_path):
    binding(api)
    journal = json.loads(journal_path.read_text(encoding="utf-8")) if journal_path.exists() else {}
    if journal and (journal.get("organization_id") != ORGANIZATION_ID
                    or journal.get("organization") != ORGANIZATION):
        raise RuntimeError("Wrong bootstrap journal")
    journal.setdefault("organization", ORGANIZATION)
    journal.setdefault("organization_id", ORGANIZATION_ID)
    journal.setdefault("repositories", {})

    def checkpoint():
        journal_path.parent.mkdir(parents=True, exist_ok=True)
        temporary = journal_path.with_suffix(".tmp")
        temporary.write_text(json.dumps(journal, indent=2) + "\n", encoding="utf-8")
        temporary.replace(journal_path)

    workflow = Path(__file__).with_name("connectivity.yml").read_text(encoding="utf-8")
    for name in (*REPOSITORIES, CONTROL):
        base = f"repos/{ORGANIZATION}/{name}"
        entry = journal["repositories"].get(name)
        if entry is None:
            try:
                api.request("GET", base)
            except APIError as error:
                if error.code != 404:
                    raise
            else:
                raise RuntimeError(f"Existing unjournalled repository: {name}; reconcile before retrying")
            repo = api.request("POST", f"orgs/{ORGANIZATION}/repos", {
                "name": name, "description": MARKER, "private": True, "auto_init": True,
                "has_issues": False, "has_wiki": False, "has_projects": False})
            entry = journal["repositories"][name] = {"id": repo["id"]}
            checkpoint()
        repo = api.request("GET", base)
        if repo["id"] != entry["id"] or repo["description"] != MARKER or not repo["private"]:
            raise RuntimeError("Bootstrap repository identity changed")
        if "main_sha" not in entry:
            sha = api.request("GET", base + "/git/ref/heads/main")["object"]["sha"]
            main_sha = commit(api, name, sha, {".github/workflows/connectivity.yml": workflow},
                              "ci: проверить доступ к тестовому стенду craftPulse")
            api.request("PATCH", base + "/git/refs/heads/main", {"sha": main_sha, "force": False})
            entry["main_sha"] = main_sha
            checkpoint()
        if "head_sha" not in entry:
            entry["head_sha"] = commit(api, name, entry["main_sha"], {
                "cp89-fixture.txt": f"Issue #89 infrastructure fixture: {name}\n"},
                "test: добавить fixture для проверки PR craftPulse")
            checkpoint()
        if not entry.get("branch_created"):
            api.request("POST", base + "/git/refs", {"ref": "refs/heads/cp89-demo", "sha": entry["head_sha"]})
            entry["branch_created"] = True
            checkpoint()
        if "pull_number" not in entry:
            existing = list(api.pages(base + "/pulls?state=all&head=" + ORGANIZATION + "%3Acp89-demo"))
            if len(existing) > 1:
                raise RuntimeError("Ambiguous fixture PR")
            pull = existing[0] if existing else api.request("POST", base + "/pulls", {
                "title": "test: проверить интеграции craftPulse (#89)",
                "body": MARKER, "head": "cp89-demo", "base": "main", "draft": False})
            entry["pull_number"] = pull["number"]
            entry["pull_created_at"] = pull["created_at"]
            checkpoint()
        print(json.dumps({"repository": repo["html_url"], "pull_number": entry["pull_number"]}), flush=True)
    return journal


def check_read_access(operator, reader, baseline, control):
    """Positive control uses the operator; reader must not access the existing private repo."""
    validate(baseline)
    if control != f"{ORGANIZATION}/{CONTROL}":
        raise ValueError("Only the dedicated private control repository may be probed")
    control_repo = operator.request("GET", "repos/" + control)
    if (not control_repo["private"] or control_repo["description"] != MARKER
            or control_repo["owner"]["id"] != ORGANIZATION_ID):
        raise RuntimeError("Invalid negative control")
    for name, wanted in baseline["repositories"].items():
        actual = reader.request("GET", f"repos/{ORGANIZATION}/{name}")
        if actual["id"] != wanted["id"]:
            raise RuntimeError("Reader repository identity mismatch")
        list(reader.pages(f"repos/{ORGANIZATION}/{name}/pulls?state=all"))
        # Actions runs wraps the page in an object, unlike ordinary paginated arrays.
        reader.request("GET", f"repos/{ORGANIZATION}/{name}/actions/runs?per_page=1")
        print(json.dumps({"repository": name, "read": "allowed"}))
    try:
        reader.request("GET", "repos/" + control)
    except APIError as error:
        if error.code not in (403, 404):
            raise
        print(json.dumps({"repository": control, "read": "denied", "status": error.code}))
    else:
        raise RuntimeError("Reader can access the private control repository")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("command", choices=("bootstrap", "capture", "compare", "reset", "check-read"))
    parser.add_argument("--baseline", type=Path)
    parser.add_argument("--journal", type=Path)
    parser.add_argument("--apply", action="store_true")
    parser.add_argument("--comment-author", action="append", default=[])
    args = parser.parse_args()
    api = operator_api()
    if args.command == "bootstrap":
        if not args.apply or not args.journal:
            parser.error("bootstrap requires --apply and --journal")
        bootstrap(api, args.journal)
        return 0
    if not args.baseline:
        parser.error("--baseline is required")
    if args.command == "capture":
        if args.baseline.exists():
            raise RuntimeError("Refusing to overwrite baseline")
        baseline = snapshot(api)
        validate(baseline)
        save_new(args.baseline, baseline)
        print("Baseline captured; PR numbers and creation times are fixed")
        return 0
    baseline = json.loads(args.baseline.read_text(encoding="utf-8"))
    validate(baseline)
    if args.command == "compare":
        differences = changes(baseline, snapshot(api))
        print(json.dumps({"differences": differences}, ensure_ascii=False))
        return int(bool(differences))
    if args.command == "check-read":
        reader = API(os.environ.get("GITHUB_READ_TOKEN", ""))
        check_read_access(api, reader, baseline, f"{ORGANIZATION}/{CONTROL}")
        return 0
    changed = reset(api, baseline, args.apply, args.comment_author)
    return int(changed and not args.apply)


if __name__ == "__main__":
    try:
        sys.exit(main())
    except (RuntimeError, ValueError, OSError, KeyError) as error:
        print(str(error), file=sys.stderr)
        sys.exit(1)
