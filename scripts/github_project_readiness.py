#!/usr/bin/env python3
"""Synchronize Backlog/Ready from issue dependencies; read-only unless --apply."""

import argparse
import json
import os
import re
import sys
from urllib.error import HTTPError, URLError
from urllib.request import Request, urlopen


PAGE_INFO = "pageInfo { hasNextPage endCursor }"
ITEM = """
id isArchived project { id }
status: fieldValueByName(name: "Status") {
  ... on ProjectV2ItemFieldSingleSelectValue { name optionId }
}
points: fieldValueByName(name: "Story points") {
  ... on ProjectV2ItemFieldNumberValue { number }
}
content {
  __typename
  ... on Issue {
    id number state repository { nameWithOwner }
    labels(first: 100) { nodes { name } pageInfo { hasNextPage endCursor } }
  }
}
"""


class APIError(RuntimeError):
    """Safe-to-log error: never includes response bodies or request credentials."""


class GitHub:
    def __init__(self, token):
        if not token or not token.strip():
            raise APIError("PROJECT_TOKEN is required; no GitHub changes were made.")
        self.token = token

    def request(self, path, payload=None):
        request = Request(
            "https://api.github.com/" + path,
            data=json.dumps(payload).encode("utf-8") if payload is not None else None,
            headers={
                "Authorization": "Bearer " + self.token,
                "Accept": "application/vnd.github+json",
                "X-GitHub-Api-Version": "2022-11-28",
                "Content-Type": "application/json",
                "User-Agent": "craftPulse-project-readiness",
            },
        )
        try:
            with urlopen(request, timeout=30) as response:
                return json.load(response)
        except HTTPError as error:
            raise APIError("GitHub API returned HTTP {}; check PROJECT_TOKEN permissions and API availability."
                           .format(error.code)) from None
        except (URLError, OSError, ValueError):
            raise APIError("GitHub API request failed; check connectivity and API availability.") from None

    def graphql(self, query, **variables):
        response = self.request("graphql", {"query": query, "variables": variables})
        if not isinstance(response, dict) or response.get("errors") or not response.get("data"):
            raise APIError("GitHub GraphQL request failed; check PROJECT_TOKEN permissions and project configuration.")
        return response["data"]

    def blocked_by(self, repository, number):
        blockers = []
        page = 1
        while True:
            response = self.request("repos/{}/issues/{}/dependencies/blocked_by?per_page=100&page={}"
                                    .format(repository, number, page))
            if not isinstance(response, list) or any(
                    not isinstance(issue, dict) or issue.get("state") not in {"open", "closed"}
                    for issue in response):
                raise APIError("Invalid blocked_by response; refusing to infer readiness.")
            blockers.extend(response)
            if len(response) < 100:
                return blockers
            page += 1


def pages(fetch):
    cursor = None
    seen = set()
    while True:
        connection = fetch(cursor)
        if not isinstance(connection, dict) or "nodes" not in connection or "pageInfo" not in connection:
            raise APIError("Incomplete GraphQL connection; refusing to infer readiness.")
        yield from connection["nodes"]
        info = connection["pageInfo"]
        if not info["hasNextPage"]:
            return
        cursor = info.get("endCursor")
        if not cursor or cursor in seen:
            raise APIError("Invalid GraphQL pagination cursor.")
        seen.add(cursor)


def discover(api, owner, project_name):
    query = """query Projects($owner: String!, $cursor: String) {
      user(login: $owner) { projectsV2(first: 100, after: $cursor) {
        nodes { id title closed } %s
      } }
    }""" % PAGE_INFO

    def projects(cursor):
        user = api.graphql(query, owner=owner, cursor=cursor).get("user")
        if not user:
            raise APIError("Project owner not found or inaccessible.")
        return user["projectsV2"]

    matches = [project for project in pages(projects)
               if project and project["title"] == project_name and not project["closed"]]
    if len(matches) != 1:
        raise APIError("Expected exactly one accessible open project with the configured name.")
    project_id = matches[0]["id"]
    query = """query Fields($project: ID!, $cursor: String) {
      node(id: $project) { ... on ProjectV2 { fields(first: 100, after: $cursor) {
        nodes {
          ... on ProjectV2Field { id name dataType }
          ... on ProjectV2SingleSelectField { id name options { id name } }
        } %s
      } } }
    }""" % PAGE_INFO
    fields = list(pages(lambda cursor: api.graphql(query, project=project_id, cursor=cursor)
                        ["node"]["fields"]))
    statuses = [field for field in fields if field.get("name") == "Status"]
    points = [field for field in fields if field.get("name") == "Story points"]
    if len(statuses) != 1 or len(points) != 1 or points[0].get("dataType") != "NUMBER":
        raise APIError("Expected Status and numeric Story points fields.")
    options = {}
    for name in ("Backlog", "Ready"):
        matches = [option for option in statuses[0].get("options", []) if option["name"] == name]
        if len(matches) != 1:
            raise APIError("Expected unique Backlog and Ready status options.")
        options[name] = matches[0]["id"]
    return project_id, statuses[0]["id"], options


def items(api, project_id):
    query = """query Items($project: ID!, $cursor: String) {
      node(id: $project) { ... on ProjectV2 { items(first: 100, after: $cursor) {
        nodes { %s } %s
      } } }
    }""" % (ITEM, PAGE_INFO)
    return pages(lambda cursor: api.graphql(query, project=project_id, cursor=cursor)
                 ["node"]["items"])


def read_item(api, item_id):
    query = "query Item($item: ID!) { node(id: $item) { ... on ProjectV2Item { %s } } }" % ITEM
    return api.graphql(query, item=item_id)["node"]


def issue_labels(api, issue):
    connection = issue["labels"]
    query = """query Labels($issue: ID!, $cursor: String) {
      node(id: $issue) { ... on Issue { labels(first: 100, after: $cursor) {
        nodes { name } %s
      } } }
    }""" % PAGE_INFO

    def fetch(cursor):
        if cursor is None:
            return connection
        return api.graphql(query, issue=issue["id"], cursor=cursor)["node"]["labels"]

    return {label["name"] for label in pages(fetch)}


def target_status(api, item, repository, project_id):
    if not item or item.get("isArchived") or item.get("project", {}).get("id") != project_id:
        return None
    issue = item.get("content")
    status = (item.get("status") or {}).get("name")
    if (not issue or issue.get("__typename") != "Issue"
            or issue["repository"]["nameWithOwner"].casefold() != repository.casefold()
            or issue["state"] != "OPEN" or status not in {"Backlog", "Ready"}):
        return None
    if status == "Backlog":
        if (item.get("points") or {}).get("number") is None or "type:epic" in issue_labels(api, issue):
            return None
    open_blocker = any(blocker["state"] == "open"
                       for blocker in api.blocked_by(repository, issue["number"]))
    if status == "Ready" and open_blocker:
        return "Backlog"
    if status == "Backlog" and not open_blocker:
        return "Ready"
    return None


def synchronize(api, owner, project_name, repository, apply=False):
    project_id, field_id, options = discover(api, owner, project_name)
    # Finish the initial reads before any mutation: an inaccessible dependency is not a closed one.
    plan = []
    for item in items(api, project_id):
        target = target_status(api, item, repository, project_id)
        if target:
            plan.append((item["id"], item["content"]["number"], target))
    mutation = """mutation Status($input: UpdateProjectV2ItemFieldValueInput!) {
      updateProjectV2ItemFieldValue(input: $input) { projectV2Item { id } }
    }"""
    changed = 0
    for item_id, number, target in plan:
        if apply:
            # A person may have started the issue since the scan. Re-read all readiness inputs.
            current = read_item(api, item_id)
            if target_status(api, current, repository, project_id) != target:
                continue
            api.graphql(mutation, input={
                "projectId": project_id, "itemId": item_id, "fieldId": field_id,
                "value": {"singleSelectOptionId": options[target]},
            })
            actual = read_item(api, item_id)
            if not actual or (actual.get("status") or {}).get("name") != target:
                raise APIError("Status differs after mutation; inspect the project before retrying.")
        print("{} issue #{} -> {}".format("Updated" if apply else "Would update", number, target))
        changed += 1
    print("{} {} item(s).".format("Updated" if apply else "Read-only; would update", changed))
    return changed


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--owner", default="Avtobot22")
    parser.add_argument("--project", default="craftPulse", help="Exact user project title")
    parser.add_argument("--repository", default="Avtobot22/craftPulse")
    parser.add_argument("--apply", action="store_true", help="Write Status changes; default is read-only")
    args = parser.parse_args(argv)
    if not re.fullmatch(r"[A-Za-z0-9-]+", args.owner) or not re.fullmatch(
            r"[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+", args.repository):
        parser.error("Invalid owner or repository")
    try:
        synchronize(GitHub(os.environ.get("PROJECT_TOKEN")), args.owner, args.project,
                    args.repository, args.apply)
    except (APIError, LookupError, AttributeError, TypeError, ValueError) as error:
        # APIError texts are fixed. Never print arbitrary response data or exception text.
        print(str(error) if isinstance(error, APIError) else "Unexpected GitHub API response; synchronization stopped.",
              file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
