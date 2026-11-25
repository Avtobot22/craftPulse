#!/usr/bin/env python3
"""Replay the MVP-09 plan with synthetic HTTP responses and no network."""

from email.message import Message
from io import BytesIO
import json
from pathlib import Path
import sys
from urllib.error import HTTPError
from urllib.parse import parse_qs, urlencode, urlsplit
from urllib.response import addinfourl

from source_checks import Source, list_pages, run


SHA = "a" * 40
OLD = "2026-11-01T00:00:00Z"
NEW = "2026-11-25T00:00:00Z"
HISTORIES = {
    "github_pr_timeline", "github_pr_reviews", "github_pr_review_comments",
    "github_pr_issue_comments",
}
REPOSITORY = {"private": True, "permissions": {"pull": True}}
ISSUE_PAGE = {"paging": {"pageIndex": 1, "pageSize": 1, "total": 5}, "issues": [{"key": "A"}]}


def reply(status=200, body=None, **headers):
    return status, body, headers


def permission(login, level):
    return reply(body={"permission": level, "role_name": level, "user": {"login": login}})


def single_responses():
    return {
        "github_permission_direct": permission("direct-fixture", "read"),
        "github_permission_team": permission("team-fixture", "read"),
        "github_permission_no_access": permission("no-access-fixture", "none"),
        "github_permission_second_org": permission("second-org-fixture", "none"),
        "github_permission_control": reply(404),
        "github_private_user": reply(body=REPOSITORY),
        "github_user_identity": reply(body={"login": "team-fixture", "id": 1}),
        "github_public": reply(body={"private": False}),
        "github_public_permission": permission("no-access-fixture", "none"),
        "github_run": reply(body={"run_attempt": 2, "updated_at": NEW}),
        "github_attempt_1": reply(body={"run_attempt": 1, "head_sha": SHA, "head_branch": "main",
                                        "status": "completed", "conclusion": "failure"}),
        "github_attempt_2": reply(body={"run_attempt": 2, "head_sha": SHA, "head_branch": "main",
                                        "status": "completed", "conclusion": "success"}),
        "github_artifact_live": reply(302, Location="https://download.invalid/artifact"),
        "github_artifact_expired": reply(410),
        "github_rate_baseline": reply(body={"resources": {"core": {"limit": 5000,
                                     "remaining": 4999, "reset": 1795600000}}}),
        "github_rate_primary": reply(403, {"message": "API rate limit exceeded"},
                                     **{"X-RateLimit-Limit": "5000", "X-RateLimit-Remaining": "0",
                                        "X-RateLimit-Used": "5000", "X-RateLimit-Reset": "1795600000",
                                        "X-RateLimit-Resource": "core"}),
        "github_rate_secondary": reply(429, {"message": "Secondary rate limit exceeded"},
                                       **{"Retry-After": "60", "X-RateLimit-Remaining": "4321"}),
        "jenkins_user_identity": reply(body={"name": "viewer-fixture", "authenticated": True}),
        "jenkins_allowed": reply(body={"name": "cp90-org-a"}),
        "jenkins_second_org": reply(body={"name": "cp90-org-b"}),
        "jenkins_control": reply(404),
        "jenkins_user_allowed": reply(body={"name": "cp90-org-a"}),
        "jenkins_user_second_org": reply(403),
        "jenkins_user_control": reply(404),
        "jenkins_other_user_record": reply(body={"id": "viewer-fixture"}),
        "jenkins_git_build": reply(body={"number": 123, "actions": [
            {"lastBuiltRevision": {"SHA1": SHA, "branch": [{"name": "origin/main", "SHA1": SHA}]}}
        ]}),
        "sonar_version": reply(body={"version": "synthetic", "status": "UP"}),
        "sonar_user_identity": reply(body={"login": "viewer-fixture", "isLoggedIn": True}),
        "sonar_allowed": reply(body=ISSUE_PAGE),
        "sonar_second_org": reply(body=ISSUE_PAGE),
        "sonar_control": reply(404),
        "sonar_user_allowed": reply(body=ISSUE_PAGE),
        "sonar_user_second_org": reply(403),
        "sonar_user_control": reply(404),
        "sonar_other_user_permissions": reply(403),
        "sonar_issues_depth_20": reply(body={
            "paging": {"pageIndex": 20, "pageSize": 500, "total": 10001},
            "issues": [{"key": f"seed-{i}"} for i in range(9501, 10001)],
        }),
        "sonar_issues_depth_21": reply(400, {"errors": [{"msg": "Synthetic search window exceeded"}]}),
        "sonar_branches": reply(body={"branches": [{"name": "main", "isMain": True}]}),
        "sonar_pull_requests": reply(body={"pullRequests": []}),
        "sonar_branch_issues": reply(404),
        "sonar_pr_issues": reply(404),
    }


def initial_rows(name):
    if name == "github_pr_timeline":
        return [{"id": i} for i in range(1, 102)] + [{"node_id": "fixture-event"}, {"sha": SHA}]
    if name in HISTORIES:
        return [{"id": i} for i in range(1, 104)]
    if name.startswith("github_jobs_attempt_"):
        attempt = int(name[-1])
        return [{"id": attempt * 100 + i, "run_attempt": attempt, "status": "completed",
                 "conclusion": "failure" if attempt == 1 else "success"} for i in range(1, 4)]
    if name == "github_artifacts":
        return [{"id": 1234567, "expired": False, "expires_at": NEW},
                {"id": 1234568, "expired": True, "expires_at": OLD}]
    if name.startswith("github_pulls"):
        return [{"id": i, "updated_at": OLD} for i in range(1, 6)]
    if name.startswith("github_runs"):
        return [{"id": i, "created_at": OLD, "updated_at": OLD, "run_attempt": 1,
                 "head_sha": SHA, "head_branch": "main"} for i in range(1, 6)]
    if name.startswith("jenkins_builds"):
        return [{"number": i, "timestamp": i * 1000, "building": i == 5,
                 "result": None if i == 5 else "SUCCESS"} for i in range(5, 0, -1)]
    if name.startswith("sonar_issues"):
        return [{"key": key, "updateDate": OLD, "status": "OPEN"} for key in "ABCDE"]
    if name == "sonar_analyses":
        return [{"key": key, "revision": SHA, "date": OLD} for key in "ABC"]
    raise ValueError(f"Missing collection fixture for {name}")


class SyntheticHTTP:
    def __init__(self, check, api_version):
        self.check = check
        self.api_version = api_version
        self.rows = initial_rows(check["name"]) if "mode" in check else []
        self.responses = single_responses() if "mode" not in check else {}
        self.revoked = False

    def open(self, request, timeout):
        url = urlsplit(request.full_url)
        assert url.hostname == "mvp09.invalid" and url.path == self.check["path"]
        assert request.get_method() == "GET" and timeout == 20
        query = {key: values[0] for key, values in parse_qs(url.query).items()}
        if self.api_version:
            assert request.get_header("X-github-api-version") == self.api_version
        if "mode" in self.check:
            status, payload, headers = self.page(url, query)
        else:
            assert query == {key: str(value) for key, value in self.check.get("query", {}).items()}
            name = self.check["name"]
            status, payload, headers = self.responses[name]
            if self.revoked and name == "github_permission_team":
                status, payload, headers = permission("team-fixture", "none")
            if self.revoked and name == "github_private_user":
                status, payload, headers = reply(404)
        metadata = Message()
        for key, value in headers.items():
            metadata[key] = value
        if self.api_version:
            metadata["X-GitHub-Api-Version-Selected"] = self.api_version
        if self.check["source"].startswith("jenkins"):
            metadata["X-Jenkins"] = "synthetic"
        body = BytesIO(json.dumps(payload).encode())
        if status != 200:
            raise HTTPError(request.full_url, status, "Synthetic HTTP response", metadata, body)
        return addinfourl(body, metadata, request.full_url, status)

    def page(self, url, query):
        mode = self.check["mode"]
        items = self.check.get("items")
        rows = self.rows
        if mode == "github":
            if "created" in query:
                since, until = query["created"].split("..")
                rows = [row for row in rows if since <= row["created_at"][:10] <= until]
            page, size = int(query.get("page", 1)), int(query["per_page"])
            end = page * size
            batch = rows[end - size:end]
            headers = {}
            if end < len(rows):
                following = urlencode({**query, "page": page + 1})
                headers["Link"] = f'<https://mvp09.invalid{url.path}?{following}>; rel="next"'
            body = {items: batch, "total_count": len(rows)} if items else batch
            return reply(body=body, **headers)
        if mode == "jenkins":
            tree, _, bounds = query["tree"].rpartition("{")
            assert tree == self.check["query"]["tree"].rpartition("{")[0]
            start, end = map(int, bounds.rstrip("}").split(","))
            return reply(body={items: rows[start:end]})
        assert mode == "sonar"
        page, size = int(query["p"]), int(query["ps"])
        return reply(body={
            items: rows[(page - 1) * size:page * size],
            "paging": {"pageIndex": page, "pageSize": size, "total": len(rows)},
        })

    def mutate(self):
        name = self.check["name"]
        if name == "github_pulls_drift":
            self.rows.insert(0, {**self.rows.pop(), "updated_at": NEW})
            self.rows.insert(0, {"id": 6, "updated_at": NEW})
        elif name == "github_runs_drift":
            self.rows[0] = {**self.rows[0], "run_attempt": 2, "updated_at": NEW}
            self.rows.insert(0, {**self.rows[0], "id": 6, "created_at": NEW, "run_attempt": 1})
        elif name == "jenkins_builds_drift":
            self.rows[0] = {**self.rows[0], "building": False, "result": "SUCCESS"}
            self.rows.insert(0, {"number": 6, "timestamp": 6000, "building": True, "result": None})
        elif name == "sonar_issues_drift":
            for _ in range(2):
                self.rows.insert(0, {**self.rows.pop(), "updateDate": NEW, "status": "CONFIRMED"})
        else:
            raise ValueError(f"Missing mutation fixture for {name}")


def verify(results):
    for name, permission_level in {
        "github_permission_direct": "read", "github_permission_team": "read",
        "github_permission_no_access": "none", "github_permission_second_org": "none",
        "github_public_permission": "none",
    }.items():
        assert results[name]["status"] == 200 and results[name]["observed"]["permission"] == permission_level
    assert results["github_permission_control"]["status"] == 404
    assert results["github_public"]["fields_match"] == {"private": True}
    for source in ("jenkins", "sonar"):
        assert [results[f"{source}_{name}"]["status"] for name in (
            "allowed", "second_org", "control", "user_allowed", "user_second_org", "user_control"
        )] == [200, 200, 404, 200, 403, 404]
    assert results["jenkins_other_user_record"]["observed"] == {"id": "viewer-fixture"}
    assert results["sonar_other_user_permissions"]["status"] == 403
    for name in HISTORIES:
        scan = results[name]["scan"]
        expected = list(map(str, range(1, 104)))
        if name == "github_pr_timeline":
            expected[-2:] = ["node_id:fixture-event", f"sha:{SHA}"]
        assert len(scan["pages"]) == 52 and scan["ids"] == expected, name
        assert len(results[name]["reference"]["pages"]) == 2, name
    expected_drift = {
        "github_pulls_drift": (["5", "6"], ["1", "2"], ["5"], []),
        "github_runs_drift": (["6"], ["2"], ["1"], ["1"]),
        "jenkins_builds_drift": (["6"], ["4"], ["5"], ["5"]),
        "sonar_issues_drift": (["D", "E"], ["A", "B"], ["D", "E"], []),
    }
    for name, expected in expected_drift.items():
        row = results[name]
        assert (row["missing"], row["scan"]["duplicates"], row["changed_rows"],
                row["stale_rows"]) == expected, name
        assert row["extra"] == []
    for attempt in (1, 2):
        jobs = results[f"github_jobs_attempt_{attempt}"]["scan"]
        assert jobs["ids"] == [str(100 * attempt + i) for i in range(1, 4)]
        assert all(row["run_attempt"] == attempt for page in jobs["pages"] for row in page["rows"])
    for name in ("jenkins_builds", "sonar_issues"):
        assert len(results[name]["scan"]["pages"]) == 3 and len(results[name]["scan"]["ids"]) == 5
    assert results["github_artifact_live"]["status"] == 302
    assert results["github_artifact_expired"]["status"] == 410
    assert results["github_rate_primary"]["headers"]["X-RateLimit-Remaining"] == "0"
    assert results["github_rate_secondary"]["headers"]["Retry-After"] == "60"
    assert results["sonar_issues_depth_20"]["count"] == 500
    assert results["sonar_issues_depth_21"]["status"] == 400
    analyses = results["sonar_analyses"]["scan"]
    assert all(row["revision"] == SHA for page in analyses["pages"] for row in page["rows"])
    revision = results["jenkins_git_build"]["observed"]["actions"][0]["lastBuiltRevision"]
    assert revision == {"SHA1": SHA, "branch": [{"name": "origin/main", "SHA1": SHA}]}


def main():
    if not __debug__:
        sys.exit("Run without Python optimization to enable the fixture assertions.")
    plan_file = Path(__file__).with_name("mvp-09.example.json")
    plan = json.loads(plan_file.read_text(encoding="utf-8"))
    results = {}
    with_mock = {}
    for check in plan["checks"]:
        source_name, name = check["source"], check["name"]
        version = plan["sources"][source_name].get("github_api_version")
        source = Source("https://mvp09.invalid", {"X-GitHub-Api-Version": version} if version else {})
        fixture = SyntheticHTTP(check, version)
        source.opener = fixture
        with_mock[name] = (source, fixture, check)
        local = run({"checks": [check]}, {source_name: source}, lambda _name, _ids: fixture.mutate())
        assert local["matches"], name
        results.update(local["checks"])

    for name in ("github_permission_team", "github_private_user"):
        source, fixture, check = with_mock[name]
        before = results[name]
        fixture.revoked = True
        after = run({"checks": [check]}, {check["source"]: source})["checks"][name]
        assert before["status"] == 200 and after["within_budget"]
        if name == "github_permission_team":
            assert before["observed"]["permission"] == "read" and after["observed"]["permission"] == "none"
        else:
            assert before["observed"]["permissions.pull"] is True and after["status"] == 404
        results[name + "_after_revoke"] = after

    source, _, check = with_mock["github_runs_drift"]
    creation_window = {**check, "query": {**check["query"], "created": "2026-11-25..2026-11-25"}}
    filtered = list_pages(source, creation_window)
    assert filtered["complete"] and filtered["ids"] == ["6"]
    results["github_runs_filtered_on_update_day"] = filtered

    verify(results)
    print(json.dumps({"simulation": True, "matches": True, "checks": results}, indent=2))


if __name__ == "__main__":
    main()
