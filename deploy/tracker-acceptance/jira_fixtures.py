"""Prepare and independently enumerate a dedicated Jira Cloud project over REST v3."""

import argparse
import hashlib
import json
from email.utils import parsedate_to_datetime
from pathlib import Path
import re
from time import monotonic, sleep

from evidence import (
    CheckFailed, Client, UnexpectedStatus, check, load_bundle, now,
    protect_jira_credentials, run_report, timestamp,
)


FIELDS = ("summary", "status", "created", "updated", "security")
ACCOUNT_FIELDS = {"assignee", "reporter", "creator"}
IDENTITY_ONLY_FIELDS = {"description", "attachment", "comment"}


class InventoryChanged(CheckFailed):
    pass


def digest(value):
    return hashlib.sha256(value.encode()).hexdigest() if value is not None else None


def change_value(item, side):
    value = item.get(side)
    if value is None:
        value = item.get(side + "String")
    return (value.encode()[:1024].decode(errors="ignore") or None) if value is not None else None


def pages(fetch, name):
    offset = 0
    total = None
    seen = set()
    while True:
        page = fetch(offset)
        batch = page[name]
        if total is None:
            total = page["total"]
        check(page["startAt"] == offset and page["total"] == total,
              "Jira changed the page offset or total")
        check(offset + len(batch) <= total, "Jira returned more rows than total")
        if "isLast" in page:
            check(page["isLast"] is (offset + len(batch) == total), "Jira returned inconsistent last-page metadata")
        for row in batch:
            item_id = row["id"]
            check(item_id not in seen, "Jira returned a duplicate row")
            seen.add(item_id)
            yield row
        offset += len(batch)
        if offset == total:
            return
        check(batch, "Jira stopped before the last page")


def cloud_issues(jira, project):
    tokens = set()
    seen = set()
    params = {"jql": f'project = "{project}" ORDER BY id ASC',
              "maxResults": 50, "fields": ",".join(FIELDS)}
    while True:
        page = jira("searchIssues", "/search/jql", params=params)
        for issue in page["issues"]:
            check(issue["id"] not in seen, "Jira returned a duplicate issue")
            seen.add(issue["id"])
            yield issue
        if page.get("isLast") is True:
            check(not page.get("nextPageToken"), "Jira returned a continuation on the last page")
            return
        token = page["nextPageToken"]
        check(token and token not in tokens and page["issues"], "Jira search did not advance")
        tokens.add(token)
        params["nextPageToken"] = token


def inventory(jira, project, as_of):
    result = {}
    for issue in cloud_issues(jira, project):
        issue_id = issue["id"]
        check(re.fullmatch(r"[0-9]+", issue_id), "Jira returned a malformed issue ID")

        def fetch(start):
            return jira("getChangelogPage", f"/issue/{issue_id}/changelog",
                        params={"startAt": start, "maxResults": 50})

        changes = list(pages(fetch, "values"))
        detail = jira("getIssue", f"/issue/{issue_id}", params={"fields": ",".join(FIELDS)})
        check(detail["id"] == issue_id, "Jira returned a different issue ID")
        if timestamp(detail["fields"]["updated"]) != timestamp(issue["fields"]["updated"]):
            raise InventoryChanged("Jira search and issue detail report different update times")
        result[issue_id] = project_issue(detail, changes, as_of)
    return result


def wait_for_inventory(jira, project, as_of, ready, wait):
    deadline = monotonic() + wait
    while True:
        try:
            issues = inventory(jira, project, as_of)
        except InventoryChanged:
            issues = None
        if issues is not None and ready(issues):
            return issues
        check(monotonic() < deadline, "Jira did not return the expected stable inventory")
        sleep(1)


def project_issue(issue, changes, as_of):
    fields = issue["fields"]
    current = {
        "status": fields["status"]["id"],
        "summary_hash": digest(fields["summary"]),
    }
    events = []
    seen = set()
    for change in changes:
        check(change["id"] not in seen, "Jira history contains a duplicate")
        seen.add(change["id"])
        items = []
        for item in change["items"]:
            field = item.get("fieldId")
            if not field:
                field = item["field"]
                alias = field.lower()
                if alias in {"issue link", "link"}:
                    field = "issuelinks"
                elif alias in {"status", "resolution", "assignee", "reporter", "creator",
                               "project", "security", "description", "attachment", "comment"}:
                    field = alias
            row = {"field": field}
            for label, side in (("before", "from"), ("after", "to")):
                if field in ACCOUNT_FIELDS:
                    row[label] = digest(item.get(side) or None)
                elif field in IDENTITY_ONLY_FIELDS:
                    row[label] = None
                else:
                    value = change_value(item, side)
                    row[label] = value if field == "status" else digest(value)
            items.append(row)
        actor = (change.get("author") or {}).get("accountId") or None
        events.append({"id": change["id"], "at": timestamp(change["created"]).isoformat(),
                       "actor": digest(actor), "items": items})
    events.sort(key=lambda event: (event["at"], int(event["id"])))
    state = current.copy()
    created = timestamp(fields["created"])
    if created > as_of:
        state = None
    else:
        for event in reversed(events):
            if timestamp(event["at"]) <= as_of:
                break
            for item in reversed(event["items"]):
                if item["field"] in ("status", "summary"):
                    key = "status" if item["field"] == "status" else "summary_hash"
                    if state[key] != item["after"]:
                        raise InventoryChanged("Jira history cannot reconstruct as-of state")
                    state[key] = item["before"]
    return {"key": issue["key"], "created": created.isoformat(),
            "current": current, "as_of": state, "history": events,
            "security_id": fields["security"]["id"] if fields.get("security") else None}


def old_fixture(jira, project, key, minimum_age_days):
    check(re.fullmatch(re.escape(project) + r"-[0-9]+", key),
          "old fixture belongs to another Jira project")
    check(type(minimum_age_days) is int and minimum_age_days >= 1, "invalid old fixture minimum age")
    issue = jira("getOldIssue", f"/issue/{key}", params={"fields": "updated,status"})
    check(issue["key"] == key and re.fullmatch(r"[0-9]+", issue["id"]),
          "Jira returned a different old fixture")
    category = issue["fields"]["status"]["statusCategory"]["key"]
    updated = timestamp(issue["fields"]["updated"])
    check(category and category != "done" and (now() - updated).days >= minimum_age_days,
          "expected an open issue with no recent updates")
    return {"id": issue["id"], "key": key, "status_category": category,
            "updated_at": updated.isoformat(), "minimum_age_days": minimum_age_days}


def prepare(jira, project, options, evidence):
    evidence.results["old_fixture"] = old_fixture(jira, project, options.old_key, options.old_days)
    ids = []
    evidence.results["fixture_ids"] = ids
    for index in range(options.count):
        fields = {"project": {"key": project}, "issuetype": {"id": options.issue_type},
                  "summary": f"craftPulse acceptance {options.run_id} {index}"}
        if index == 1:
            fields["security"] = {"id": options.security_level}
        created = jira("createFixtureIssue", "/issue", method="POST", data={"fields": fields})
        ids.append(created["id"])
    for index in range(options.history):
        jira("extendFixtureHistory", f"/issue/{ids[0]}", method="PUT",
             data={"fields": {"summary": f"craftPulse acceptance {options.run_id} edit {index}"}})
    return ids


def source_client(source, evidence):
    auth = protect_jira_credentials(source, evidence)
    client = Client(source["jira_url"], evidence)
    headers = {"Authorization": auth, "Content-Type": "application/json"}

    def jira(operation, path, *, method="GET", data=None, params=None):
        expected = {"GET": [200], "POST": [201], "PUT": [204]}[method]
        body = json.dumps(data).encode() if data is not None else None
        wait_budget = 120
        for attempt in range(4):
            try:
                return client.request(operation, method, "/rest/api/3" + path, expected,
                                      params=params, headers=headers, body=body)
            except UnexpectedStatus as error:
                if error.status != 429 or attempt == 3:
                    raise
                delay = retry_delay(error.retry_after)
                if delay is None or delay > wait_budget:
                    raise
                sleep(delay)
                wait_budget -= delay

    server = jira("getServerInfo", "/serverInfo")
    check(server["deploymentType"] == "Cloud" and server["version"],
          "expected a Jira Cloud deployment with a version")
    return jira, {"edition": server["deploymentType"], "version": server["version"],
                  "build_number": server.get("buildNumber")}


def retry_delay(value):
    if not value:
        return None
    if re.fullmatch(r"[0-9]{1,9}", value):
        return int(value)
    try:
        retry_at = parsedate_to_datetime(value)
        if retry_at.tzinfo is not None:
            return max(0, (retry_at - now()).total_seconds())
    except (ValueError, OverflowError):
        pass
    return None


def run(options, evidence):
    source = load_bundle(options.bundle)
    jira, edition = source_client(source, evidence)
    evidence.results["source"] = edition
    evidence.results["project"] = options.project
    ids = prepare(jira, options.project, options, evidence) if options.command == "prepare" else []
    as_of = timestamp(options.as_of) if options.as_of else now()
    check(as_of <= now(), "as-of time cannot be in the future")
    evidence.results["as_of"] = as_of.isoformat()
    if ids:
        actual = wait_for_inventory(
            jira, options.project, as_of,
            lambda issues: all(issue_id in issues for issue_id in ids)
            and len(issues[ids[0]]["history"]) >= options.history,
            options.wait,
        )
    else:
        actual = inventory(jira, options.project, as_of)
    evidence.results["issues"] = actual
    if ids:
        check(actual[ids[1]]["security_id"] == options.security_level,
              "source inventory missed the fixture security level")
        old = evidence.results["old_fixture"]
        check(actual.get(old["id"], {}).get("key") == old["key"],
              "source inventory missed the old open issue")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--bundle", type=Path, required=True)
    parser.add_argument("--project", required=True)
    parser.add_argument("--as-of")
    parser.add_argument("--report", type=Path, required=True)
    commands = parser.add_subparsers(dest="command", required=True)
    prep = commands.add_parser("prepare", help="create fixtures in a dedicated Jira test project")
    prep.add_argument("--issue-type", required=True)
    prep.add_argument("--security-level", required=True)
    prep.add_argument("--old-key", required=True)
    prep.add_argument("--old-days", type=int, default=30)
    prep.add_argument("--count", type=int, default=101)
    prep.add_argument("--history", type=int, default=101)
    prep.add_argument("--wait", type=int, default=300)
    prep.add_argument("--run-id", required=True)
    commands.add_parser("inventory", help="capture inventory without modifying Jira")
    options = parser.parse_args()
    if not re.fullmatch(r"[A-Z][A-Z0-9_]*", options.project):
        parser.error("invalid Jira project key")
    if options.command == "prepare" and (
        not 101 <= options.count <= 1000 or not 101 <= options.history <= 1000
        or not 1 <= options.wait <= 1800 or options.old_days < 1 or not re.fullmatch(r"[A-Za-z0-9-]{1,36}", options.run_id)
        or not re.fullmatch(r"[A-Z][A-Z0-9_]*-[0-9]+", options.old_key)
        or not all(re.fullmatch(r"[0-9]+", value) for value in (options.issue_type, options.security_level))
    ):
        parser.error("invalid fixture sizes or identifiers")
    run_report(parser, options, f"jira-cloud-{options.command}", run)


if __name__ == "__main__":
    main()
