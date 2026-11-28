"""Reconcile a dedicated Jira Cloud project through the craftPulse API."""

import argparse
import json
from collections import Counter
from pathlib import Path
import re
from time import monotonic, sleep
from urllib.parse import quote
from uuid import UUID

from evidence import CheckFailed, check, load_bundle, now, product_session, read_snapshot, run_report, timestamp
from jira_fixtures import ACCOUNT_FIELDS, digest, inventory, old_fixture, source_client, wait_for_inventory

API = "/api/v1"
SQL = Path(__file__).with_name("jira_snapshot.sql")


def wait_for(read, predicate, wait, failure):
    deadline = monotonic() + wait
    while True:
        result = read()
        if predicate(result):
            return result
        check(monotonic() < deadline, failure)
        sleep(1)


def get_sync_status(client, headers, connection):
    result = client.request("getSyncStatus", "GET", API + "/connections/" + connection + "/sync-status",
                            [200], headers=headers)
    check(result.get("connection_id") == connection, "getSyncStatus did not return the requested connection")
    return result


def completed_collection(status, *, previous=None, require_new_attempt=False):
    tasks = next((row for row in status["data_kinds"] if row["data_kind"] == "tasks"), {})
    reconcile = status["last_reconcile"]
    if not (
        tasks.get("phase") == "incremental" and tasks.get("last_outcome") == "success"
        and not tasks["failed_page_present"] and tasks["watermark"] and tasks["last_success_at"]
        and tasks["last_attempt_at"] and reconcile and reconcile["outcome"] == "complete"
        and reconcile["finished_at"]
    ):
        return None

    watermark = timestamp(tasks["watermark"])
    attempted = timestamp(tasks["last_attempt_at"])
    succeeded = timestamp(tasks["last_success_at"])
    if (watermark != timestamp(reconcile["started_at"])
            or succeeded != timestamp(reconcile["finished_at"])
            or not attempted <= watermark <= succeeded):
        return None
    if previous is not None:
        if watermark <= timestamp(previous["watermark"]):
            return None
        if require_new_attempt and attempted <= timestamp(previous["last_attempt_at"]):
            return None
    return tasks


def order_history(history):
    def key(row):
        change_id, _, field = row["id"].partition("/")
        field, _, occurrence = field.partition("/")
        return timestamp(row["at"]), int(change_id), field, int(occurrence or "1")

    return sorted(history, key=key)


def expected_facts(issues):
    kinds = {"issuelinks": "link_change", "Link": "link_change", "project": "moved"}
    facts = {}
    for issue_id, issue in issues.items():
        history = [{"id": issue_id + "/created", "at": issue["created"], "field": None,
                    "kind": "created", "before": None, "after": None, "actor": None}]
        for change in issue["history"]:
            occurrences = Counter()
            for item in change["items"]:
                field = item["field"]
                occurrences[field] += 1
                event_id = change["id"] + "/" + quote(field, safe="$&+:=@")
                if occurrences[field] > 1:
                    event_id += "/" + str(occurrences[field])
                history.append({"id": event_id, "at": change["at"], "field": field,
                                "kind": kinds.get(field, "field_change"),
                                "before": item["before"], "after": item["after"], "actor": change["actor"]})
        facts[issue_id] = {
            "key": issue["key"], "current": issue["current"], "as_of": issue["as_of"],
            "stored_as_of": issue["as_of"], "security_id": issue["security_id"], "history": order_history(history),
        }
    return facts


def core_facts(rows, as_of):
    facts = {}
    ids = {}
    for row in rows:
        native = row["native_id"]
        check(native not in facts, "duplicate native Jira task")
        ids[native] = row["id"]
        current = {"status": row["status"], "summary_hash": digest(row["summary"])}
        history = []
        seen = set()
        for event in row["history"]:
            check(event["id"] not in seen, "duplicate Jira event")
            seen.add(event["id"])
            field = event["field"]
            item = {"id": event["id"], "at": timestamp(event["at"]).isoformat(),
                    "field": field, "kind": event["kind"], "actor": digest(event["actor"])}
            for side in ("before", "after"):
                value = event[side + "_account"] if field in ACCOUNT_FIELDS else event[side]
                item[side] = value if field == "status" else digest(value)
            history.append(item)
        history = order_history(history)
        state = current.copy() if timestamp(row["created"]) <= as_of else None
        if state is not None:
            for event in reversed(history):
                if timestamp(event["at"]) <= as_of:
                    break
                if event["field"] in {"status", "summary"}:
                    key = "status" if event["field"] == "status" else "summary_hash"
                    state[key] = event["before"]
        version = row["stored_as_of"]
        stored_as_of = {"status": version["status"], "summary_hash": digest(version["summary"])} if version else None
        facts[native] = {"key": row["key"], "current": current, "as_of": state, "stored_as_of": stored_as_of,
                         "security_id": row["security_id"], "history": history}
    return facts, ids


def run(options, evidence):
    source = load_bundle(options.bundle)
    jira, edition = source_client(source, evidence)
    client, headers = product_session(source, evidence, options.sha)
    headers["Content-Type"] = "application/json"

    def api(operation, method, path, status, data=None, **kwargs):
        return client.request(operation, method, API + path, [status],
                              headers={**headers, **kwargs.pop("headers", {})}, **kwargs,
                              body=json.dumps(data).encode() if data is not None else None)

    fixture_bytes = options.fixtures.read_bytes()
    evidence.guard(fixture_bytes)
    fixture = json.loads(fixture_bytes)
    check(fixture["outcome"] == "passed" and fixture["scenario"] == "jira-cloud-prepare"
          and fixture["project"] == options.project and fixture["source"] == edition,
          "expected a successful preparation report for the same Jira project")
    fixtures = fixture["fixture_ids"]
    check(len(fixtures) >= 101 and len(set(fixtures)) == len(fixtures)
          and all(re.fullmatch(r"[0-9]+", issue_id) for issue_id in fixtures),
          "expected at least 101 distinct fixture issue IDs")
    edited, restricted = fixtures[:2]
    prepared_old = fixture["old_fixture"]
    old = old_fixture(jira, options.project, prepared_old["key"], prepared_old["minimum_age_days"])
    evidence.results.update(expected_old_fixture=prepared_old, actual_old_fixture=old)
    check(old == prepared_old, "old Jira fixture changed since preparation")
    as_of = now()
    initial = inventory(jira, options.project, as_of)
    check(set(fixtures) <= initial.keys(), "Jira no longer contains the prepared fixtures")
    check(len(initial[edited]["history"]) >= 101, "Jira no longer contains the prepared long history")
    security_level = fixture["issues"][restricted]["security_id"]
    check(security_level and initial[restricted]["security_id"] == security_level,
          "Jira fixture security level differs from preparation")
    check(initial.get(old["id"], {}).get("key") == old["key"],
          "old Jira fixture is missing from the fresh inventory")
    check(timestamp(options.history_from) <= min(timestamp(issue["created"]) for issue in initial.values()),
          "history-from must cover the oldest issue in the Jira project")
    project = jira("getProject", "/project/" + options.project)
    scope = project["id"]
    check(project["key"] == options.project and re.fullmatch(r"[0-9]+", scope),
          "unexpected Jira project identity")
    evidence.results.update(source=edition, project=options.project, as_of=as_of.isoformat(),
                            organization=options.organization,
                            reconciliation=[])

    response_headers = {}
    connection = api("createConnection", "POST", "/connections", 201, {
        "name": "Jira acceptance " + options.run_id, "provider": "jira_cloud",
        "base_url": source["jira_url"], "adapter_kind": "builtin", "adapter_url": None, "adapter_key": None,
        "credentials": {"email": source["jira_email"], "token": source["jira_token"]},
        "scopes": [{"kind": "tracker_project", "native_scope_id": scope,
                    "history_from": options.history_from}],
    }, response_headers=response_headers)
    connection_id = str(UUID(connection["id"]))
    path = "/connections/" + connection_id
    evidence.results["connection"] = connection_id
    check(response_headers["ETag"], "createConnection returned no ETag")
    started = api("startConnectionProbe", "POST", path + "/probes", 202)
    probe_id = str(UUID(started["id"]))
    probe_path = path + "/probes/" + probe_id
    probe = wait_for(lambda: api("getConnectionProbe", "GET", probe_path, 200),
                     lambda row: row["status"] in {"succeeded", "failed"},
                     options.wait, "probe did not finish")
    check(probe.get("id") == probe_id, "getConnectionProbe did not return the requested probe")
    evidence.results["probe"] = probe
    check(probe["status"] == "succeeded", "probe failed")
    active = api("activateConnection", "POST", path + "/activation", 200,
                 {"probe_id": probe_id}, headers={"If-Match": response_headers["ETag"]})
    check(active.get("id") == connection_id and active.get("provider") == "jira_cloud",
          "activation did not return the requested Jira Cloud connection")
    instance = active["instance"]
    evidence.results.update(instance=instance, capabilities=active["capabilities"])
    version = "REST v3 / " + edition["version"]
    if edition["build_number"]:
        version += f' ({edition["build_number"]})'
    check(active["status"] == "active" and instance["edition"] == "cloud"
          and instance["source_version"] == version,
          "activation did not preserve Jira edition or version")
    verified = {item["capability"] for item in active["capabilities"] if item["state"] == "verified"}
    check({"read_current", "read_status_history", "read_field_history"} <= verified, "expected verified task capabilities")
    instance_id = str(UUID(instance["id"]))

    def read_status():
        status = get_sync_status(client, headers, connection_id)
        evidence.results["sync_status"] = status
        return status

    def wait_for_collection(phase, previous=None, require_new_attempt=False):
        def completed():
            status = read_status()
            return completed_collection(status, previous=previous, require_new_attempt=require_new_attempt)

        tasks = wait_for(completed, bool, options.wait, phase + ": no new completed task reconciliation")
        evidence.results.setdefault("collection_rounds", []).append({"phase": phase, "status": evidence.results["sync_status"]})
        return tasks

    def reconcile(phase, issues):
        entry = {"phase": phase, "expected": expected_facts(issues)}
        evidence.results["reconciliation"].append(entry)

        def snapshot():
            rows = read_snapshot(SQL, evidence, organization=options.organization,
                                 instance=instance_id, project=scope, as_of=as_of.isoformat())
            entry["actual"], entry["entity_ids"] = core_facts(rows, as_of)
            entry["matches"] = entry["actual"] == entry["expected"]
            return entry["matches"]

        wait_for(snapshot, bool, options.wait,
                 phase + ": jira and core facts differ")
        return entry["entity_ids"]

    def sync(kind):
        pending = api("requestSyncRun", "POST", path + "/sync-runs", 202,
                      {"kind": kind, "data_kind": "tasks"})
        operation_id = str(UUID(pending["id"]))
        operation = "/operations/" + operation_id
        result = wait_for(lambda: api("getBackgroundOperation", "GET", operation, 200),
                          lambda row: row["status"] in {"succeeded", "failed"},
                          options.wait, "sync operation did not finish")
        check(result.get("id") == operation_id, "getBackgroundOperation did not return the requested operation")
        evidence.results.setdefault("runs", []).append({"kind": kind, "result": result})
        check(result["status"] == "succeeded", "sync operation failed")

    baseline = wait_for_collection("initial")
    original_ids = reconcile("initial", initial)
    title = f"craftPulse acceptance increment {options.run_id}"
    check(digest(title) != initial[edited]["current"]["summary_hash"], "use a fresh run ID")
    jira("editFixtureIssue", "/issue/" + edited, method="PUT", data={"fields": {"summary": title}})
    changed = wait_for_inventory(
        jira, options.project, as_of,
        lambda issues: edited in issues
        and issues[edited]["current"]["summary_hash"] == digest(title)
        and issues[edited]["as_of"] == initial[edited]["current"],
        options.wait,
    )
    check(set(changed) == set(initial), "Jira issue set changed during the run")
    sync("resume")
    increment = wait_for_collection("changed", baseline, require_new_attempt=True)
    check(reconcile("changed", changed) == original_ids, "increment changed internal task IDs")
    sync("resume")
    replay = wait_for_collection("repeated", increment, require_new_attempt=True)
    repeated = inventory(jira, options.project, as_of)
    check(repeated == changed, "Jira changed during replay")
    check(reconcile("repeated", repeated) == original_ids, "replay changed internal task IDs")
    sync("reconcile")
    wait_for_collection("reconciled", replay)
    check(reconcile("reconciled", repeated) == original_ids, "reconciliation changed internal task IDs")
    persisted = api("getConnection", "GET", path, 200)
    check(persisted.get("id") == connection_id and persisted.get("provider") == "jira_cloud",
          "getConnection did not return the requested Jira Cloud connection")
    evidence.results["persisted_connection"] = {
        "status": persisted["status"], "instance": persisted["instance"], "capabilities": persisted["capabilities"],
    }
    check(persisted["status"] == "active" and persisted["instance"] == instance,
          "stored connection or Jira version differs from activation")
    before = {cap["capability"]: (cap["state"], cap["reason_code"]) for cap in active["capabilities"]}
    after = {cap["capability"]: (cap["state"], cap["reason_code"]) for cap in persisted["capabilities"]}
    check(before == after, "stored capability states or reasons differ from activation")
    read_status()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ("bundle", "report", "fixtures"):
        parser.add_argument("--" + name, required=True, type=Path)
    for name in ("organization", "project", "sha", "run-id", "history-from"):
        parser.add_argument("--" + name, required=True)
    parser.add_argument("--wait", type=int, default=300)
    options = parser.parse_args()
    try:
        options.organization = str(UUID(options.organization))
        options.history_from = timestamp(options.history_from).isoformat()
    except (ValueError, CheckFailed):
        parser.error("invalid organization UUID or history-from timestamp")
    if not (re.fullmatch(r"[A-Z][A-Z0-9_]*", options.project) and
            re.fullmatch(r"[0-9a-f]{40}", options.sha) and
            re.fullmatch(r"[A-Za-z0-9-]{1,36}", options.run_id) and 1 <= options.wait <= 1800):
        parser.error("invalid project key, release SHA, run ID or wait")
    run_report(parser, options, "jira-cloud-reconciliation", run)


if __name__ == "__main__":
    main()
