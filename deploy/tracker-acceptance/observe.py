"""Measure access closure or the daily collection on an isolated acceptance stand."""

import argparse
from codecs import getincrementaldecoder
import json
from pathlib import Path
import re
from time import monotonic, sleep
from uuid import UUID, uuid4

from evidence import CheckFailed, check, load_bundle, now, product_session, read_snapshot, run_report, timestamp
from jira_cloud import get_sync_status
from universal_import import SQL as IMPORT_SQL
from universal_import import get_receipt, receipt, receipt_journal, reconcile

API = "/api/v1"


def denied(client, headers, task):
    status, result = client.request("getTask", "GET", API + "/tasks/" + task, [200, 404],
                                    headers=headers, include_status=True)
    check(isinstance(result, dict), "getTask returned no JSON object")
    if status == 404:
        check(result.get("status") == 404 and result.get("code") == "not_found",
              "getTask returned an invalid not_found response")
        return True
    check(result.get("id") == task, "getTask did not return the requested task")
    return False


def access(client, headers, source, options, evidence):
    payload = {"items": [{
        "source_account_id": options.source_account, "person_id": options.person,
        "actor_kind": "human", "base_assignment_id": options.base_assignment,
    }]}
    result = client.request("createAccountAssignments", "POST", API + "/account-assignments", [200],
                            headers={**headers, "Content-Type": "application/json",
                                     "Idempotency-Key": str(uuid4())},
                            body=json.dumps(payload).encode())
    check(len(result["items"]) == 1 and result["items"][0]["outcome"] == "saved",
          "account assignment was not saved")
    evidence.results["assignment"] = result["items"][0]["assignment_id"]
    _, user_headers = product_session({**source, "craftpulse_cookie": source["viewer_cookie"]},
                                      evidence, options.sha)
    check(not denied(client, user_headers, options.task), "fixture task is already hidden")
    control = options.control_task
    check(not denied(client, user_headers, control), "control task is already hidden")
    evidence.results["task"] = options.task
    evidence.results["control_task"] = control
    started = monotonic()
    evidence.results["started_poll_at"] = now().isoformat()
    print("Apply the requested change now to the dedicated Jira fixture or adapter.", flush=True)
    while monotonic() - started < 60:
        if denied(client, user_headers, options.task):
            elapsed = monotonic() - started
            check(not denied(client, user_headers, control), "control task became hidden too")
            evidence.results["milliseconds_until_denied"] = round(elapsed * 1000)
            check(elapsed <= 60, "access closure exceeded 60 seconds")
            return
        sleep(1)
    check(False, "fixture task was still visible after 60 seconds")


def task_sync(client, headers, connection):
    result = get_sync_status(client, headers, connection)
    return next((row for row in result["data_kinds"] if row["data_kind"] == "tasks"), {}), result["last_reconcile"]


def daily(client, headers, options, evidence):
    raw = options.baseline.read_bytes()
    evidence.guard(raw)
    baseline = json.loads(raw)
    check(baseline["scenario"] == "jira-cloud-reconciliation" and baseline["outcome"] == "passed"
          and baseline["release_sha"] == options.sha and baseline["connection"] == options.connection,
          "expected a successful Jira report for this connection and release")
    sync = next((row for row in baseline["sync_status"]["data_kinds"] if row["data_kind"] == "tasks"), {})
    check(sync.get("last_outcome") == "success" and sync["last_success_at"]
          and sync["last_attempt_at"] and sync["watermark"],
          "expected a successful baseline collection with a watermark")
    evidence.results.update(connection=options.connection, before=sync)
    previous = timestamp(sync["last_attempt_at"])
    previous_watermark = timestamp(sync["watermark"])
    age = (now() - previous).total_seconds()
    check(age >= 0, "baseline attempt time is in the future")
    # Read once even after the deadline. A new session can resume from the saved baseline.
    deadline = monotonic() + 86400 - age
    while True:
        current, reconcile = task_sync(client, headers, options.connection)
        evidence.results["latest"] = current
        if current["last_attempt_at"] and timestamp(current["last_attempt_at"]) > previous:
            detected = timestamp(current["last_attempt_at"])
            evidence.results["first_observed_attempt_at"] = detected.isoformat()
            break
        remaining = deadline - monotonic()
        check(remaining > 0, "no new collection attempt was observed within 24 hours")
        sleep(min(15, remaining))
    interval = (detected - previous).total_seconds()
    evidence.results["interval_seconds"] = interval
    check(interval <= 86400, "getSyncStatus cannot establish a new attempt within 24 hours")

    finish = monotonic() + 600
    while True:
        evidence.results.update(latest=current, last_reconcile=reconcile)
        if (
            current["phase"] == "incremental" and not current["failed_page_present"]
            and current["last_outcome"] == "success" and current["watermark"]
            and current["last_success_at"] and reconcile and reconcile["outcome"] == "complete"
            and reconcile["finished_at"]
        ):
            watermark = timestamp(current["watermark"])
            started = timestamp(reconcile["started_at"])
            finished = timestamp(reconcile["finished_at"])
            last_attempt = timestamp(current["last_attempt_at"])
            last_success = timestamp(current["last_success_at"])
            matches_reconcile = watermark == started and last_success == finished
            if matches_reconcile and watermark > previous_watermark and detected <= last_attempt <= started <= finished:
                evidence.results["seconds_from_observed_page_to_reconcile"] = (finished - detected).total_seconds()
                evidence.results["reconcile_seconds"] = (finished - started).total_seconds()
                return
        check(monotonic() < finish, "daily collection did not finish with reconciled watermark")
        sleep(2)
        current, reconcile = task_sync(client, headers, options.connection)


def import_facts(options, evidence, instance):
    return read_snapshot(IMPORT_SQL, evidence, organization=options.organization,
                         project=options.project, instance=instance, as_of=options.as_of)


def recover_import(client, headers, options, evidence):
    started = timestamp(options.started_at)
    stop = timestamp(options.stopped_at)
    check(started < stop, "the import must have started before the worker stopped")
    elapsed = (now() - stop).total_seconds()
    check(0 <= elapsed < 300, "stopped-at must be within the previous five minutes")
    check(options.control_receipt != options.interrupted_receipt, "use two distinct receipts")
    check(options.control_instance != options.interrupted_instance, "use two distinct import instances")
    evidence.results.update(project=options.project, as_of=options.as_of,
                            source_version="tasks/1", control_instance=options.control_instance,
                            interrupted_instance=options.interrupted_instance, reconciliation=[])
    before = get_receipt(client, headers, options.interrupted_receipt)
    check(before["channel"] == "import" and before["status"] == "quarantined",
          "interrupted import must still be quarantined")
    before_journal = receipt_journal(client, headers, options.interrupted_receipt)
    evidence.results.update(processing_started_at=started.isoformat(), stopped_at=stop.isoformat(),
                            before_status=before["status"],
                            before_journal=before_journal)
    check(not before_journal, "interrupted import already committed journal records")
    control = receipt(client, headers, options.control_receipt, maximum_wait=0)
    check(control["channel"] == "import" and control["status"] in {"accepted", "partial"}
          and control["accepted_count"] > 0,
          "continuous control run must contain accepted records")
    check(control["schema_id"] == "tasks" and control["schema_version"] == "1",
          "use the tasks/1 schema for the recovery runs")
    expected_facts = import_facts(options, evidence, options.control_instance)
    control_ids = expected_facts.pop("entity_ids")
    check(expected_facts["objects"] and expected_facts["versions"],
          "control run must have normalized tasks")
    before_facts = import_facts(options, evidence, options.interrupted_instance)
    evidence.results["before_facts"] = before_facts
    check(not any(before_facts.values()), "interrupted run already has normalized facts")
    print("Start the dedicated worker to resume the quarantined import.", flush=True)
    after = receipt(client, headers, options.interrupted_receipt,
                    maximum_wait=max(0, 300 - (now() - stop).total_seconds()))
    receipt_seconds = (now() - stop).total_seconds()
    evidence.results.update(receipt_seconds=receipt_seconds, expected_receipt=control, actual_receipt=after)
    check(receipt_seconds <= 300, "receipt did not reach a terminal state within five minutes")
    check({k: v for k, v in control.items() if k != "id"} ==
          {k: v for k, v in after.items() if k != "id"}, "recovered receipt differs from the control")
    expected = receipt_journal(client, headers, options.control_receipt)
    actual = receipt_journal(client, headers, options.interrupted_receipt)
    evidence.results.update(expected_journal=expected, actual_journal=actual)
    check(len(expected) == control["record_count"] and bool(expected) and expected == actual,
          "recovered import journal differs from the control")

    recovered_ids = reconcile(
        "worker-recovery",
        lambda: import_facts(options, evidence, options.interrupted_instance),
        expected_facts, evidence, wait=max(0, 300 - (now() - stop).total_seconds()),
    )
    seconds = (now() - stop).total_seconds()
    evidence.results.update(recovery_seconds=seconds, control_entity_ids=control_ids)
    check(set(control_ids) == set(recovered_ids) and
          set(control_ids.values()).isdisjoint(recovered_ids.values()),
          "recovered tasks do not have distinct identities under the new instance")
    check(seconds <= 300, "import recovery exceeded five minutes")


def scan_file(path, evidence):
    longest = max(map(len, evidence.protected_bytes()))
    size = 0
    tail = b""
    decode = getincrementaldecoder("utf-8")()
    try:
        with path.open("rb") as stream:
            while chunk := stream.read(65536):
                decode.decode(chunk)
                check(b"\0" not in chunk, "expected a plain UTF-8 export")
                size += len(chunk)
                window = tail + chunk
                evidence.guard(window)
                tail = window[-longest:]
        decode.decode(b"", final=True)
    except UnicodeDecodeError:
        raise CheckFailed("expected a plain UTF-8 export") from None
    check(size > 0, "empty export")
    return size


def scan_log_export(path, evidence):
    size = 0
    with path.open("rb") as stream:
        for line in stream:
            if not line.strip():
                continue
            evidence.guard(line)
            try:
                row = evidence.decode_json(line.decode("utf-8"))
            except ValueError:
                raise CheckFailed("expected a VictoriaLogs JSON Lines export") from None
            check(isinstance(row, dict), "expected a VictoriaLogs JSON Lines export")
            size += len(line)
    check(size > 0, "empty export")
    return size


def credentials(client, headers, source, options, evidence):
    check("jira_token" in source, "expected a dedicated test token")
    client.request("getConnection", "GET", API + "/connections/" + options.connection, [200], headers=headers)
    task_sync(client, headers, options.connection)
    exports = evidence.results["exports_checked"] = {}
    exports["victorialogs"] = scan_log_export(options.victorialogs, evidence)
    exports["core_dump"] = scan_file(options.core_dump, evidence)


def run(options, evidence):
    source = load_bundle(options.bundle)
    client, headers = product_session(source, evidence, options.sha)
    if options.command == "daily":
        daily(client, headers, options, evidence)
    elif options.command == "recover-import":
        recover_import(client, headers, options, evidence)
    elif options.command == "credentials":
        credentials(client, headers, source, options, evidence)
    else:
        access(client, headers, source, options, evidence)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ("bundle", "report"):
        parser.add_argument("--" + name, type=Path, required=True)
    parser.add_argument("--sha", required=True)
    commands = parser.add_subparsers(dest="command", required=True)
    for command in ("revocation", "adapter-unavailable"):
        probe = commands.add_parser(command)
        for name in ("task", "control-task", "source-account", "person", "base-assignment"):
            probe.add_argument("--" + name, type=lambda value: str(UUID(value)),
                               required=name != "base-assignment")
    recovery = commands.add_parser("recover-import")
    recovery.add_argument("--started-at", required=True)
    recovery.add_argument("--stopped-at", required=True)
    recovery.add_argument("--as-of", required=True)
    for name in ("control-receipt", "interrupted-receipt", "organization", "project"):
        recovery.add_argument("--" + name, required=True, type=lambda value: str(UUID(value)))
    for name in ("control-instance", "interrupted-instance"):
        recovery.add_argument("--" + name, required=True)
    schedule = commands.add_parser("daily")
    schedule.add_argument("--baseline", type=Path, required=True)
    leaks = commands.add_parser("credentials")
    for command in (schedule, leaks):
        command.add_argument("--connection", required=True, type=lambda value: str(UUID(value)))
    for name in ("victorialogs", "core-dump"):
        leaks.add_argument("--" + name, type=Path, required=True)
    options = parser.parse_args()
    if not re.fullmatch(r"[0-9a-f]{40}", options.sha):
        parser.error("invalid release SHA")
    if options.command == "recover-import":
        try:
            options.as_of = timestamp(options.as_of).isoformat()
        except (ValueError, CheckFailed):
            parser.error("invalid as-of timestamp")
        if not all(re.fullmatch(r"[A-Za-z0-9.:-]{1,128}", value)
                   for value in (options.control_instance, options.interrupted_instance)):
            parser.error("invalid import instance key")
    run_report(parser, options, options.command, run)


if __name__ == "__main__":
    main()
