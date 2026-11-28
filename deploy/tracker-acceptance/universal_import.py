"""Reconcile tasks and history from createImport with a scoped read-only core snapshot."""

import argparse
import json
from pathlib import Path
import re
from time import monotonic, sleep
from uuid import UUID, uuid4

from evidence import check, load_bundle, product_session, read_snapshot, run_report


FIRST = "2026-01-02T09:00:00Z"
FIRST_EVENT = "2026-01-02T10:00:00Z"
AS_OF = "2026-01-02T11:00:00Z"
SECOND = "2026-01-02T12:00:00Z"
API = "/api/v1"
SQL = Path(__file__).with_name("import_snapshot.sql")


def record(kind, native_id, revision, payload, *, namespace="", at=None):
    return {
        "record_class": "event" if at else "object", "record_kind": kind,
        "native_ns": namespace, "native_id": native_id, "payload_schema": kind + "/1",
        "source_revision": revision, "event_time": at, "observed_time": None,
        "file": None, "payload": payload,
    }


def fixtures(project, task_key):
    def task(native_id, key, revision, title, status, at):
        return record("task", native_id, revision, {
            "project_native_id": project, "task_key": key, "title": title,
            "type_native_id": "Task", "status_native_id": status, "created_at": FIRST,
            "updated_at": at,
        })

    def transition(event_id, before, after, at):
        return record("task_event", event_id, event_id, {
            "task_native_id": "01", "event_kind": "field_change",
            "field_native_id": "status", "before_value": before, "after_value": after,
        }, namespace="01", at=at)

    first_event = transition("01/status", "open", "active", FIRST_EVENT)
    return (
        [task("01", task_key, "1", "Acceptance fixture", "active", FIRST_EVENT),
         task("02", "IMPORTFIXTURE-2" if task_key != "IMPORTFIXTURE-2" else "IMPORTFIXTURE-3",
              "1", "Second acceptance fixture", "open", FIRST)],
        [first_event],
        [task("01", task_key, "2", "Updated acceptance fixture", "done", SECOND)],
        [first_event, transition("02/status", "active", "done", SECOND)],
    )


def expected_snapshot(task_records, event_records):
    versions = []
    for task in task_records:
        payload = task["payload"]
        versions.append({"native_id": task["native_id"], "key": payload["task_key"],
                         "title": payload["title"], "type": payload["type_native_id"],
                         "status": payload["status_native_id"], "at": payload["updated_at"]})
    versions.sort(key=lambda row: (row["native_id"], row["at"]))
    current = {}
    as_of = {}
    for row in versions:
        current[row["native_id"]] = row
        if row["at"] <= AS_OF:
            as_of[row["native_id"]] = {key: row[key] for key in ("native_id", "key", "title", "type", "status")}
    for row in versions:
        row["current"] = row is current[row["native_id"]]

    events = {}
    for event in event_records:
        payload = event["payload"]
        key = (payload["task_native_id"], event["native_id"])
        row = {"native_id": key[0], "event_key": key[1], "kind": payload["event_kind"],
               "field": payload["field_native_id"], "at": event["event_time"],
               "before": payload["before_value"], "after": payload["after_value"]}
        check(key not in events or events[key] == row, "fixture reuses an event ID with different data")
        events[key] = row
    return {
        "objects": sorted(current),
        "versions": versions,
        "events": [events[key] for key in sorted(events)],
        "as_of": [as_of[key] for key in sorted(as_of)],
    }


def multipart(fields, raw):
    boundary = "craftpulse-" + uuid4().hex
    parts = [
        f'--{boundary}\r\nContent-Disposition: form-data; name="{name}"\r\n\r\n{value}\r\n'.encode()
        for name, value in fields.items()
    ]
    parts += [
        f'--{boundary}\r\nContent-Disposition: form-data; name="file"; filename="records.json"\r\n'
        'Content-Type: application/json\r\n\r\n'.encode(),
        raw, f"\r\n--{boundary}--\r\n".encode(),
    ]
    return "multipart/form-data; boundary=" + boundary, b"".join(parts)


def snapshot(options, evidence):
    return read_snapshot(SQL, evidence, organization=options.organization,
                         instance=options.instance, project=options.project, as_of=AS_OF)


def reconcile(name, read, expected, evidence, wait):
    deadline = monotonic() + wait
    entry = {"phase": name, "expected": expected}
    evidence.results["reconciliation"].append(entry)
    while True:
        actual = read()
        entry["entity_ids"] = actual.pop("entity_ids")
        entry["actual"] = actual
        entry["matches"] = actual == expected
        if entry["matches"]:
            return entry["entity_ids"]
        check(monotonic() < deadline, f"{name}: normalized facts differ from the input")
        sleep(1)


def get_receipt(client, headers, receipt_id):
    receipt_id = str(UUID(receipt_id))
    result = client.request("getReceipt", "GET", API + "/receipts/" + receipt_id, [200], headers=headers)
    check(result.get("id") == receipt_id, "getReceipt did not return the requested receipt")
    return result


def receipt(client, headers, receipt_id, maximum_wait):
    deadline = monotonic() + maximum_wait
    while True:
        result = get_receipt(client, headers, receipt_id)
        if result["status"] in {"accepted", "rejected", "partial", "duplicate"}:
            return {field: result.get(field) for field in (
                "id", "status", "channel", "schema_id", "schema_version", "record_count",
                "accepted_count", "rejected_count", "replay_count", "conflict_count", "duplicate_of",
                "reason_code", "affected_metrics",
            )}
        check(monotonic() < deadline, "receipt did not reach a terminal state")
        sleep(1)


def receipt_journal(client, headers, receipt_id):
    path = API + "/receipts/" + receipt_id + "/records"
    records = []
    seen = set()
    params = {"limit": 100}
    while True:
        page = client.request("listReceiptRecords", "GET", path, [200],
                              headers=headers, params=params)
        for row in page["data"]:
            check(not records or row["record_no"] > records[-1]["record_no"],
                  "receipt journal returned a repeated or unordered record")
            records.append(row)
        cursor = page["next_cursor"]
        if cursor is None:
            return records
        check(cursor and cursor not in seen and page["data"], "receipt journal did not advance")
        seen.add(cursor)
        params["cursor"] = cursor


def check_journal(entry, records, outcomes):
    expected = []
    for number, (record, outcome) in enumerate(zip(records, outcomes, strict=True), 1):
        native_key = "/".join(filter(None, (record["native_ns"], record["native_id"])))
        expected.append({
            "record_no": number, "record_kind": record["record_kind"], "native_key": native_key,
            "outcome": outcome, "reason_code": None,
        })
    entry["expected_journal"] = expected
    check(entry["journal"] == expected, "receipt journal differs from the expected records")


def run(options, evidence):
    source = load_bundle(options.bundle)
    client, headers = product_session(source, evidence, options.sha)
    report_bytes = options.jira_report.read_bytes()
    evidence.guard(report_bytes)
    jira = json.loads(report_bytes)
    check(jira["scenario"] == "jira-cloud-reconciliation" and jira["outcome"] == "passed"
          and jira["release_sha"] == options.sha and jira["organization"] == options.organization,
          "expected a successful Jira reconciliation for this organization and release")
    fixture = jira["reconciliation"][-1]
    jira_ids = [fixture["entity_ids"][native] for native, issue in fixture["actual"].items()
                if issue["key"] == options.task_key]
    check(len(jira_ids) == 1, "expected exactly one Jira task with the supplied key")

    def read():
        return snapshot(options, evidence)

    evidence.results.update(
        source_version="tasks/1, task_events/1",
        project=options.project, instance=options.instance, as_of=AS_OF,
        receipts=[], reconciliation=[],
    )
    check(read() == {"entity_ids": {}, **expected_snapshot([], [])}, "use a fresh universal instance key for each run")

    def upload_file(schema, raw, status, version="1"):
        content_type, body = multipart({
            "schema_id": schema, "schema_version": version, "source_provider": "universal",
            "source_instance_key": options.instance, "project_id": options.project,
        }, raw)
        return client.request("createImport", "POST", API + "/imports", [status],
                              headers={**headers, "Content-Type": content_type}, body=body,
                              allow_html_413=status == 413)

    def submit(schema, records, *, version="1"):
        upload = upload_file(schema, json.dumps(records).encode(), 202, version)
        result = receipt(client, headers, upload["receipt_id"], options.wait)
        entry = {"schema": schema, "version": version, "records": records, "result": result}
        evidence.results["receipts"].append(entry)
        entry["journal"] = receipt_journal(client, headers, result["id"])
        check(result["channel"] == "import", "receipt belongs to a different intake channel")
        if version == "1":
            check(result["schema_id"] == schema and result["schema_version"] == version,
                  "receipt did not preserve the submitted schema and version")
        return entry

    tasks, history, changed, more_history = fixtures(options.project, options.task_key)
    first_tasks = submit("tasks", tasks)
    first_history = submit("task_events", history)
    check(first_tasks["result"]["status"] == first_history["result"]["status"] == "accepted",
          "initial tasks or history were not accepted")
    check_journal(first_tasks, tasks, ("accepted", "accepted"))
    check_journal(first_history, history, ("accepted",))
    initial_ids = reconcile("initial", read, expected_snapshot(tasks, history), evidence, options.wait)
    check(jira_ids[0] != initial_ids["01"], "same-key identity check failed")
    evidence.results["same_key"] = {"key": options.task_key, "jira": jira_ids[0],
                                    "universal": initial_ids["01"]}
    change = submit("tasks", changed)
    new_events = submit("task_events", more_history)
    check(change["result"]["status"] == new_events["result"]["status"] == "accepted"
          and new_events["result"]["accepted_count"] == 1 and new_events["result"]["replay_count"] == 1,
          "expected one new event and one replay in the update")
    check_journal(change, changed, ("accepted",))
    check_journal(new_events, more_history, ("replay", "accepted"))
    expected = expected_snapshot(tasks + changed, history + more_history)
    check(reconcile("changed", read, expected, evidence, options.wait) == initial_ids,
          "updated import changed task IDs")

    for _ in range(3):
        for schema, records, original in (("tasks", tasks, first_tasks),
                                          ("task_events", history, first_history)):
            repeated = submit(schema, records)
            check(repeated["result"]["status"] == "duplicate"
                  and repeated["result"]["duplicate_of"] == original["result"]["id"]
                  and not repeated["journal"], "repeat package added records or lost its original receipt")
    check(reconcile("repeated", read, expected, evidence, wait=0) == initial_ids,
          "repeated import changed task IDs")

    rejected = submit("tasks", changed, version="unsupported")
    bad = rejected["result"]
    check(bad["status"] == "rejected" and bad["reason_code"] == "schema_unsupported"
          and bad["accepted_count"] == 0 and bad["affected_metrics"] and not rejected["journal"],
          "unsupported schema was not rejected with a reason and an empty journal")
    check(reconcile("rejected", read, expected, evidence, wait=0) == initial_ids,
          "rejected import changed task IDs")
    evidence.results["rejected_inputs"] = []
    for phase, raw, status in (
        ("malformed-json", b"[", 422),
        ("upload-size-limit", b" " * (25 * 1024 * 1024 + 1), 413),
    ):
        problem = upload_file("tasks", raw, status)
        if problem is None and status == 413:
            reason = "http_content_too_large"
        else:
            check(problem and problem["status"] == status and problem["code"],
                  "rejection had no API problem code")
            reason = problem["code"]
        evidence.results["rejected_inputs"].append({"phase": phase, "bytes": len(raw),
                                                    "status": status, "code": reason})
        check(reconcile(phase, read, expected, evidence, wait=0) == initial_ids,
              "rejected upload changed task IDs")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ("bundle", "report"):
        parser.add_argument("--" + name, required=True, type=Path)
    for name in ("sha", "organization", "project", "instance"):
        parser.add_argument("--" + name, required=True)
    parser.add_argument("--jira-report", type=Path, required=True)
    parser.add_argument("--task-key", required=True)
    parser.add_argument("--wait", type=int, default=60)
    options = parser.parse_args()
    for name in ("organization", "project"):
        try:
            setattr(options, name, str(UUID(getattr(options, name))))
        except ValueError:
            parser.error("invalid organization or project UUID")
    if not (re.fullmatch(r"[0-9a-f]{40}", options.sha)
            and re.fullmatch(r"[A-Za-z0-9.:-]{1,128}", options.instance)
            and re.fullmatch(r"[A-Z][A-Z0-9_]*-[0-9]+", options.task_key)
            and 1 <= options.wait <= 300):
        parser.error("invalid release SHA, fixture identifiers or wait")
    run_report(parser, options, "universal-import", run)


if __name__ == "__main__":
    main()
