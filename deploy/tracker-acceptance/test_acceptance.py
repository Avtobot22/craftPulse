import gzip
import io
import json
import os
from pathlib import Path
import tempfile
from types import SimpleNamespace
import unittest
from unittest.mock import Mock, patch
from urllib.error import HTTPError

from evidence import CheckFailed, Client, Evidence, product_session, read_snapshot, timestamp
from jira_cloud import completed_collection, core_facts, expected_facts, run as run_jira
from jira_fixtures import InventoryChanged, cloud_issues, digest, old_fixture, pages, project_issue, source_client, wait_for_inventory
from observe import daily, denied, recover_import, scan_file, scan_log_export, task_sync
from test_environment import PACKAGE, TESTS, scenario_outcomes
from universal_import import AS_OF, check_journal, expected_snapshot, fixtures, multipart, receipt, receipt_journal, reconcile


class TestEnvironmentTests(unittest.TestCase):
    def test_report_marks_missing_skipped_and_duplicate_scenarios(self):
        completed, skipped, duplicate, missing = TESTS[:4]
        events = [
            {"Action": "pass", "Test": completed},
            {"Action": "skip", "Test": skipped},
            {"Action": "pass", "Test": duplicate},
            {"Action": "fail", "Test": duplicate},
            {"Action": "pass", "Test": missing + "/subtest"},
            {"Action": "output", "Test": missing, "Output": "raw test output"},
            {"Action": "pass"},
        ]
        output = "\n".join(json.dumps({"Package": PACKAGE, **event}) for event in events)
        package, scenarios = scenario_outcomes(output)
        self.assertEqual(package, "pass")
        self.assertEqual(set(scenarios), set(TESTS))
        self.assertEqual([scenarios[name] for name in TESTS[:4]],
                         ["pass", "skip", "duplicate", "missing"])

    def test_report_rejects_a_successful_scenario_with_a_skipped_subtest(self):
        probe = "TestConnections_ProbeFailures"
        events = [
            {"Action": "skip", "Test": probe + "/jira_cloud/forbidden"},
            {"Action": "pass", "Test": probe + "/jira_cloud"},
            {"Action": "pass", "Test": probe},
            {"Action": "pass"},
        ]
        output = "\n".join(json.dumps({"Package": PACKAGE, **event}) for event in events)
        package, scenarios = scenario_outcomes(output)
        self.assertEqual(package, "pass")
        self.assertEqual(scenarios[probe], "skip")


class JiraTests(unittest.TestCase):
    def test_collection_requires_a_fresh_complete_round(self):
        previous = {"watermark": "2026-01-02T09:01:00Z", "last_attempt_at": "2026-01-02T09:00:00Z"}
        task = {
            "data_kind": "tasks", "phase": "incremental", "last_outcome": "success",
            "failed_page_present": False, "watermark": "2026-01-02T09:11:00Z",
            "last_attempt_at": "2026-01-02T09:10:00Z", "last_success_at": "2026-01-02T09:12:00Z",
        }
        reconcile = {
            "outcome": "complete", "started_at": task["watermark"], "finished_at": task["last_success_at"],
        }
        status = {"data_kinds": [task], "last_reconcile": reconcile}
        self.assertEqual(completed_collection(status, previous=previous, require_new_attempt=True), task)
        for bad_reconcile in (None, {**reconcile, "outcome": "running"},
                              {**reconcile, "started_at": previous["watermark"]}):
            self.assertIsNone(completed_collection({**status, "last_reconcile": bad_reconcile}))

        stale_attempt = {**status, "data_kinds": [{**task, "last_attempt_at": previous["last_attempt_at"]}]}
        self.assertIsNone(completed_collection(stale_attempt, previous=previous, require_new_attempt=True))
        self.assertEqual(completed_collection(stale_attempt, previous=previous), stale_attempt["data_kinds"][0])
        self.assertIsNone(completed_collection(status, previous=task))

    def test_replay_mismatch_stops_before_manual_reconciliation(self):
        project, run_id = "FIX", "test"
        connection_id = "00000000-0000-4000-8000-000000000001"
        operation_id = "00000000-0000-4000-8000-000000000002"
        edition = {"edition": "Cloud", "version": "1001.0", "build_number": 123}
        persisted = {
            "id": connection_id, "provider": "jira_cloud", "status": "active",
            "instance": {"id": connection_id, "edition": "cloud", "source_version": "REST v3 / 1001.0 (123)"},
            "capabilities": [{"capability": name, "state": "verified", "reason_code": None}
                             for name in ("read_current", "read_status_history", "read_field_history")],
        }
        fixture_ids = [str(i) for i in range(1, 102)]
        old = {"id": "102", "key": "FIX-102", "minimum_age_days": 30}
        initial = {key: {"key": "FIX-" + key, "created": AS_OF} for key in [*fixture_ids, old["id"]]}
        initial["1"].update(history=[{}] * 101, current={"summary_hash": digest("before")})
        initial["2"]["security_id"] = "42"
        changed = {**initial, "1": {**initial["1"], "current": {"summary_hash": digest("after")}}}
        fixture = {"scenario": "jira-cloud-prepare", "outcome": "passed", "project": project,
                   "source": edition, "fixture_ids": fixture_ids, "old_fixture": old, "issues": initial}
        options = SimpleNamespace(bundle=None, report=None, project=project, sha="a" * 40,
                                  organization=connection_id, history_from=AS_OF, run_id=run_id, wait=0)
        sync_runs = []
        outcomes = {
            "createConnection": {"id": connection_id},
            "startConnectionProbe": {"id": operation_id},
            "getConnectionProbe": {"id": operation_id, "status": "succeeded"},
            "activateConnection": persisted,
            "getBackgroundOperation": {"id": operation_id, "status": "succeeded"},
        }

        def request(operation, _method, _path, _statuses, **kwargs):
            if operation == "createConnection":
                kwargs["response_headers"]["ETag"] = '"fixture"'
            if operation == "requestSyncRun":
                sync_runs.append(json.loads(kwargs["body"])["kind"])
                return {"id": operation_id}
            if operation == "getSyncStatus":
                hour = f"2026-01-02T{12 + len(sync_runs)}"
                task = {"data_kind": "tasks", "phase": "incremental", "last_outcome": "success",
                        "failed_page_present": False, "last_attempt_at": hour + ":00:00Z",
                        "watermark": hour + ":01:00Z", "last_success_at": hour + ":02:00Z"}
                return {"connection_id": connection_id, "data_kinds": [task], "last_reconcile": {
                    "outcome": "complete", "started_at": task["watermark"], "finished_at": task["last_success_at"],
                }}
            return outcomes[operation]

        client = SimpleNamespace(request=request)
        jira = Mock(return_value={"id": "123", "key": project})
        evidence = Evidence(None, "test")
        options.fixtures = Mock()
        options.fixtures.read_bytes.return_value = json.dumps(fixture).encode()
        ids = {"1": connection_id}
        bundle = {"jira_url": "https://jira.example.test", "jira_email": "tester@example.test",
                  "jira_token": "test-marker"}
        with (patch("jira_cloud.load_bundle", return_value=bundle),
              patch("jira_cloud.source_client", return_value=(jira, edition)),
              patch("jira_cloud.product_session", return_value=(client, {})),
              patch("jira_cloud.old_fixture", return_value=old),
              patch("jira_cloud.inventory", side_effect=[initial, changed]),
              patch("jira_cloud.wait_for_inventory", return_value=changed),
              patch("jira_cloud.read_snapshot", side_effect=[initial, changed, initial]),
              patch("jira_cloud.expected_facts", side_effect=lambda issues: issues),
              patch("jira_cloud.core_facts", side_effect=lambda rows, _as_of: (rows, ids))):
            with self.assertRaisesRegex(CheckFailed, "repeated: jira and core facts differ"):
                run_jira(options, evidence)
        self.assertEqual(sync_runs, ["resume", "resume"])
        self.assertEqual([row["matches"] for row in evidence.results["reconciliation"]], [True, True, False])

    def test_source_client_requires_cloud_and_uses_rest_v3(self):
        source = {"jira_url": "https://jira.example.test",
                  "jira_email": "tester@example.com", "jira_token": "test-marker"}
        evidence = Evidence(None, "test")
        with patch.object(Client, "request", return_value={
            "deploymentType": "Cloud", "version": "1001.0", "buildNumber": 123,
        }) as request:
            jira, edition = source_client(source, evidence)
            self.assertEqual(edition, {"edition": "Cloud", "version": "1001.0", "build_number": 123})
            self.assertEqual(request.call_args.args, ("getServerInfo", "GET", "/rest/api/3/serverInfo", [200]))
            self.assertTrue(request.call_args.kwargs["headers"]["Authorization"].startswith("Basic "))
            jira("createFixtureIssue", "/issue", method="POST", data={"fields": {}})
            self.assertEqual(request.call_args.args, ("createFixtureIssue", "POST", "/rest/api/3/issue", [201]))
            request.return_value["deploymentType"] = "Server"
            with self.assertRaisesRegex(CheckFailed, "expected a Jira Cloud deployment"):
                source_client(source, evidence)

    def test_jira_fixture_retry_waits_for_retry_after_and_journals_each_attempt(self):
        class Response(io.BytesIO):
            code = 200
            headers = {}

        opener = SimpleNamespace()
        source = {"jira_url": "https://jira.example.test",
                  "jira_email": "tester@example.com", "jira_token": "test-marker"}
        evidence = Evidence(None, "test")
        client = Client(source["jira_url"], evidence, opener=opener)
        attempt = []

        def open_request(request, timeout):
            attempt.append(request.get_method())
            if len(attempt) == 1:
                raise HTTPError(request.full_url, 429, "rate limited", {"Retry-After": "7"},
                                io.BytesIO(b"plain text"))
            if len(attempt) == 3:
                raise HTTPError(request.full_url, 429, "rate limited",
                                {"Retry-After": "Fri, 02 Jan 2026 11:00:03 GMT"}, io.BytesIO(b""))
            return Response(b'{"deploymentType":"Cloud","version":"1001.0"}')

        opener.open = open_request
        with (patch("jira_fixtures.Client", return_value=client),
              patch("jira_fixtures.now", return_value=timestamp(AS_OF)),
              patch("jira_fixtures.sleep", side_effect=attempt.append)):
            source_client(source, evidence)
        self.assertEqual(attempt, ["GET", 7, "GET", 3, "GET"])
        self.assertEqual([entry["status"] for entry in evidence.requests], [429, 429, 200])

    def test_jira_does_not_retry_auth_failure_or_unusable_retry_after(self):
        source = {"jira_url": "https://jira.example.test",
                  "jira_email": "tester@example.com", "jira_token": "test-marker"}
        for status, headers in ((403, {}), (429, {}), (429, {"Retry-After": "121"})):
            with self.subTest(status=status, headers=headers):
                def open_request(request, timeout):
                    raise HTTPError(request.full_url, status, "unexpected status", headers,
                                    io.BytesIO(b"{}"))

                evidence = Evidence(None, "test")
                opener = SimpleNamespace(open=open_request)
                client = Client(source["jira_url"], evidence, opener=opener)
                with (patch("jira_fixtures.Client", return_value=client),
                      patch("jira_fixtures.sleep") as sleep,
                      self.assertRaisesRegex(CheckFailed, f"unexpected HTTP status {status}")):
                    source_client(source, evidence)
                sleep.assert_not_called()
                self.assertEqual(len(evidence.requests), 1)

    def test_offset_pages_require_every_id_once(self):
        def fetch(start):
            return {"startAt": start, "total": 3,
                    "values": [{"id": str(i)} for i in range(start, min(start + 2, 3))]}

        self.assertEqual([row["id"] for row in pages(fetch, "values")], ["0", "1", "2"])
        with self.assertRaisesRegex(CheckFailed, "stopped"):
            list(pages(lambda start: {"startAt": start, "total": 3, "values": []}, "values"))
        with self.assertRaisesRegex(CheckFailed, "duplicate"):
            list(pages(lambda start: {"startAt": start, "total": 2, "values": [{"id": "0"}]}, "values"))
        with self.assertRaisesRegex(CheckFailed, "last-page"):
            list(pages(lambda start: {"startAt": start, "total": 2, "values": [{"id": "0"}],
                                      "isLast": True}, "values"))

    def test_cloud_search_uses_each_token_once(self):
        cases = [
            {"issues": [{"id": "1"}], "nextPageToken": "page-2"},
            {"issues": [{"id": "2"}], "isLast": True},
        ]
        received = []

        def request(operation, path, *, params):
            self.assertEqual(path, "/search/jql")
            received.append(params.get("nextPageToken"))
            return cases.pop(0)

        self.assertEqual(list(cloud_issues(request, "TEST")), [{"id": "1"}, {"id": "2"}])
        self.assertEqual(received, [None, "page-2"])
        with self.assertRaisesRegex(CheckFailed, "advance"):
            list(cloud_issues(lambda *_, **kwargs: {
                "issues": [{"id": str(kwargs["params"].get("nextPageToken"))}],
                "nextPageToken": "same",
            }, "TEST"))

    def test_prepared_or_changed_issues_wait_for_consistent_search(self):
        cutoff = timestamp(AS_OF)
        observed = {"1": {"key": "TEST-1"}}
        with (patch("jira_fixtures.inventory", side_effect=[InventoryChanged(), {}, observed]) as inventory,
              patch("jira_fixtures.sleep")):
            actual = wait_for_inventory(None, "TEST", cutoff, lambda issues: "1" in issues, 1)
        self.assertEqual(actual, observed)
        self.assertEqual(inventory.call_count, 3)

        with patch("jira_fixtures.inventory", side_effect=CheckFailed("duplicate issue")):
            with self.assertRaisesRegex(CheckFailed, "duplicate issue"):
                wait_for_inventory(None, "TEST", cutoff, lambda issues: True, 1)

    def test_old_fixture_requires_the_expected_identity_and_an_old_open_issue(self):
        issue = {"id": "42", "key": "TEST-1", "fields": {
            "updated": "2025-12-01T00:00:00Z", "status": {"statusCategory": {"key": "new"}},
        }}

        def jira(operation, path, *, params):
            self.assertEqual((operation, path, params),
                             ("getOldIssue", "/issue/TEST-1", {"fields": "updated,status"}))
            return issue

        with patch("jira_fixtures.now", return_value=timestamp(AS_OF)):
            self.assertEqual(old_fixture(jira, "TEST", "TEST-1", 30), {
                "id": "42", "key": "TEST-1", "status_category": "new",
                "updated_at": "2025-12-01T00:00:00+00:00", "minimum_age_days": 30,
            })
            with self.assertRaisesRegex(CheckFailed, "another Jira project"):
                old_fixture(jira, "TEST", "OTHER-1", 30)
            issue["key"] = "TEST-2"
            with self.assertRaisesRegex(CheckFailed, "different old fixture"):
                old_fixture(jira, "TEST", "TEST-1", 30)
            issue["key"] = "TEST-1"
            issue["fields"]["status"]["statusCategory"]["key"] = "done"
            with self.assertRaisesRegex(CheckFailed, "open issue"):
                old_fixture(jira, "TEST", "TEST-1", 30)
            issue["fields"]["status"]["statusCategory"]["key"] = "new"
            issue["fields"]["updated"] = "2026-01-01T00:00:00Z"
            with self.assertRaisesRegex(CheckFailed, "no recent updates"):
                old_fixture(jira, "TEST", "TEST-1", 30)

    def test_inventory_waits_for_changelog_to_catch_up_with_issue_detail(self):
        issue = {"key": "TEST-1", "fields": {
            "created": "2026-01-01T00:00:00Z", "summary": "new",
            "status": {"id": "1"},
        }}
        edits = [
            {"id": "10", "created": "2026-01-02T12:00:00Z", "items": [
                {"field": "summary", "fromString": "initial", "toString": "previous"},
            ]},
            {"id": "11", "created": "2026-01-02T13:00:00Z", "items": [
                {"field": "summary", "fromString": "previous", "toString": "new"},
            ]},
        ]
        batches = iter([edits[:1], edits])

        def read(jira, project, cutoff):
            return {"1": project_issue(issue, next(batches), cutoff)}

        with (patch("jira_fixtures.inventory", side_effect=read),
              patch("jira_fixtures.sleep") as sleep):
            actual = wait_for_inventory(None, "TEST", timestamp(AS_OF), lambda issues: True, 1)
        sleep.assert_called_once()
        self.assertEqual(actual["1"]["as_of"]["summary_hash"], digest("initial"))
        self.assertEqual([event["id"] for event in actual["1"]["history"]], ["10", "11"])

    def test_as_of_and_event_identity_are_compared_across_sources(self):
        issue = {"key": "TEST-1", "fields": {
            "created": "2026-01-01T00:00:00Z", "summary": "after",
            "status": {"id": "2"}, "security": {"id": "13"},
        }}
        changes = [
            {"id": "100", "created": "2026-01-03T00:00:00.000+0000",
             "author": {"accountId": "editor"}, "items": [
                {"field": "summary", "fromString": "before", "toString": "after"},
                {"field": "Status", "fieldId": "status", "from": "1", "to": "2"},
                {"field": "Resolution", "from": "7", "to": "8", "toString": "Fixed"},
                {"field": "Assignee", "from": "alice", "to": "bob", "toString": "Bob"},
                {"field": "Description", "toString": "body omitted from core"},
            ]},
        ]
        cutoff = timestamp(AS_OF)
        reference = project_issue(issue, changes, cutoff)
        actual = {
            "id": "entity-1", "native_id": "10", "key": "TEST-1",
            "summary": "after", "status": "2", "security_id": "13", "created": issue["fields"]["created"],
            "stored_as_of": {"status": "1", "summary": "before"},
            "history": [
                {"id": "10/created", "at": issue["fields"]["created"], "field": None,
                 "kind": "created", "before": None, "after": None, "actor": None},
                {"id": "100/status", "at": "2026-01-03T00:00:00+00:00", "field": "status",
                 "kind": "field_change", "before": "1", "after": "2", "actor": "editor"},
                {"id": "100/summary", "at": "2026-01-03T00:00:00+00:00", "field": "summary",
                 "kind": "field_change", "before": "before", "after": "after", "actor": "editor"},
                {"id": "100/resolution", "at": "2026-01-03T00:00:00+00:00", "field": "resolution",
                 "kind": "field_change", "before": "7", "after": "8", "actor": "editor"},
                {"id": "100/assignee", "at": "2026-01-03T00:00:00+00:00", "field": "assignee",
                 "kind": "field_change", "before_account": "alice", "after_account": "bob", "actor": "editor"},
                {"id": "100/description", "at": "2026-01-03T00:00:00+00:00", "field": "description",
                 "kind": "field_change", "before": None, "after": None, "actor": "editor"},
            ],
        }
        facts, ids = core_facts([actual], cutoff)
        self.assertEqual(reference["as_of"], {"status": "1", "summary_hash": digest("before")})
        self.assertEqual(ids, {"10": "entity-1"})
        self.assertEqual(facts, expected_facts({"10": reference}))
        for index, field, wrong in ((3, "after", "9"), (4, "after_account", "alice"),
                                    (5, "actor", None), (5, "after", "body unexpectedly persisted")):
            with self.subTest(index=index, field=field):
                event = actual["history"][index]
                original = event[field]
                event[field] = wrong
                self.assertNotEqual(core_facts([actual], cutoff)[0], expected_facts({"10": reference}))
                event[field] = original

        stored = actual.pop("stored_as_of")
        actual["stored_as_of"] = None
        without_version, _ = core_facts([actual], cutoff)
        self.assertEqual(without_version["10"]["as_of"], reference["as_of"])
        self.assertNotEqual(without_version, expected_facts({"10": reference}))
        actual["stored_as_of"] = stored

        created = actual["history"].pop(0)
        self.assertNotEqual(core_facts([actual], cutoff)[0], expected_facts({"10": reference}))
        actual["history"].insert(0, created)
        actual["history"].append(actual["history"][0])
        with self.assertRaisesRegex(CheckFailed, "duplicate Jira event"):
            core_facts([actual], cutoff)


class ImportTests(unittest.TestCase):
    def test_expected_records_preserve_history_and_as_of_after_update(self):
        tasks, history, changed, more_history = fixtures("project", "TEST-1")
        result = expected_snapshot(tasks + changed, history + more_history)
        self.assertEqual(result["objects"], ["01", "02"])
        self.assertEqual(len(result["events"]), 2)
        self.assertEqual(result["as_of"][0], {
            "native_id": "01", "key": "TEST-1", "title": "Acceptance fixture",
            "type": "Task", "status": "active",
        })
        current = next(row for row in result["versions"] if row["native_id"] == "01" and row["current"])
        self.assertEqual(current["title"], "Updated acceptance fixture")
        self.assertEqual([v["status"] for v in result["versions"] if v["current"]], ["done", "open"])

    def test_reconciliation_detects_extra_fact(self):
        expected = expected_snapshot([], [])
        evidence = Evidence(None, "test")
        evidence.results["reconciliation"] = []
        with self.assertRaisesRegex(CheckFailed, "differ"):
            reconcile("replay", lambda: {**expected, "objects": ["unexpected"], "entity_ids": {}},
                      expected, evidence, wait=0)
        self.assertFalse(evidence.results["reconciliation"][0]["matches"])

    def test_receipt_journal_rejects_duplicate_record_across_pages(self):
        class Client:
            def request(self, operation, method, path, expected, *, headers, params):
                return {"data": [{"record_no": 1}], "next_cursor": "next" if "cursor" not in params else None}

        with self.assertRaisesRegex(CheckFailed, "repeated"):
            receipt_journal(Client(), {}, "fixture")

    def test_journal_matches_replay_and_new_event_in_order(self):
        _, _, _, events = fixtures("project", "TEST-1")
        expected = ("replay", "accepted")
        journal = [
            {"record_no": 1, "record_kind": "task_event", "native_key": "01/01/status",
             "outcome": "replay", "reason_code": None},
            {"record_no": 2, "record_kind": "task_event", "native_key": "01/02/status",
             "outcome": "accepted", "reason_code": None},
        ]
        entry = {"journal": journal}
        check_journal(entry, events, expected)
        self.assertEqual(entry["expected_journal"], journal)
        wrong_id = [journal[0], {**journal[1], "native_key": "02/02/status"}]
        wrong_reason = [journal[0], {**journal[1], "reason_code": "outside_scope"}]
        for bad in (journal[:1], list(reversed(journal)), wrong_id, wrong_reason):
            with self.assertRaisesRegex(CheckFailed, "differs"):
                check_journal({"journal": bad}, events, expected)

    def test_receipt_rejects_response_for_another_import(self):
        requested_id = "00000000-0000-4000-8000-000000000001"
        wrong_id = "00000000-0000-4000-8000-000000000002"
        for status in ("quarantined", "accepted"):
            with self.subTest(status=status):
                client = SimpleNamespace(request=lambda *args, **kwargs: {
                    "id": wrong_id, "status": status,
                })
                with self.assertRaisesRegex(CheckFailed, "did not return the requested receipt"):
                    receipt(client, {}, requested_id, maximum_wait=0)

    def test_multipart_retains_exact_file_bytes(self):
        content_type, body = multipart({"schema_id": "task_events"}, b"[]")
        boundary = content_type.split("boundary=", 1)[1]
        self.assertIn(b'name="schema_id"\r\n\r\ntask_events\r\n', body)
        self.assertIn(b'name="file"; filename="records.json"', body)
        self.assertTrue(body.endswith(f"\r\n[]\r\n--{boundary}--\r\n".encode()))


class EvidenceTests(unittest.TestCase):
    def test_journal_excludes_headers_url_query_and_payload(self):
        class Response(io.BytesIO):
            code = 200
            headers = {"ETag": '"1"'}

        class Opener:
            def open(self, req, timeout):
                return Response(b'{"status":"active"}')

        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "report.json"
            evidence = Evidence(path, "test")
            evidence.secrets.add("synthetic-secret-value")
            client = Client("https://jira.example.test", evidence, opener=Opener())
            result = client.request("lookup", "POST", "/query", [200],
                                    params={"value": "synthetic-secret-value"},
                                    headers={"Authorization": "Bearer synthetic-secret-value"},
                                    body=b"synthetic-secret-value")
            self.assertEqual(result["status"], "active")
            evidence.save(None)
            report = json.loads(path.read_bytes())
            self.assertEqual(report["requests"][0]["status"], 200)
            self.assertNotIn("synthetic-secret-value", path.read_text())
            self.assertEqual(os.stat(path).st_mode & 0o777, 0o600)
            with self.assertRaises(FileExistsError):
                evidence.save(None)

    def test_proxy_size_rejection_accepts_html_only_when_enabled(self):
        class Response(io.BytesIO):
            code = 413
            headers = {"Content-Type": "text/html; charset=UTF-8"}

        class Opener:
            def open(self, req, timeout):
                return Response(b"<html>Request Entity Too Large</html>")

        evidence = Evidence(None, "test")
        client = Client("https://example.test", evidence, opener=Opener())
        self.assertIsNone(client.request("createImport", "POST", "/imports", [413], allow_html_413=True))
        self.assertEqual(evidence.requests[-1]["status"], 413)
        with self.assertRaisesRegex(CheckFailed, "invalid JSON"):
            client.request("createImport", "POST", "/imports", [413])
        with self.assertRaisesRegex(CheckFailed, "unexpected HTTP status 413"):
            client.request("createImport", "POST", "/imports", [422], allow_html_413=True)

    def test_credential_scan_checks_between_chunks(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "export"
            evidence = Evidence(None, "test")
            evidence.secrets.add("synthetic-secret-value")
            path.write_bytes(b"x" * 65530 + b"synthetic-secret-value" + b"tail")
            with self.assertRaisesRegex(CheckFailed, "protected value"):
                scan_file(path, evidence)
            path.write_bytes(b"safe export")
            self.assertEqual(scan_file(path, evidence), len(b"safe export"))

    def test_export_scan_detects_jira_basic_credentials(self):
        source = {
            "craftpulse_url": "https://craftpulse.example",
            "craftpulse_cookie": "session=stand-session",
            "jira_email": "tester@example.com",
            "jira_token": "test-marker",
        }
        with tempfile.TemporaryDirectory() as directory:
            evidence = Evidence(None, "credentials")
            sha = "a" * 40
            with patch.object(Client, "request", return_value={"commit_sha": sha}):
                product_session(source, evidence, sha)
            path = Path(directory) / "export"
            path.write_bytes(b"Basic dGVzdGVyQGV4YW1wbGUuY29tOnRlc3QtbWFya2Vy")
            with self.assertRaisesRegex(CheckFailed, "protected value"):
                scan_file(path, evidence)

    def test_log_export_detects_json_escaped_credentials(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "export"
            evidence = Evidence(None, "credentials")
            evidence.secrets.add(r"synthetic\marker")
            safe = b'{"_msg":"safe export"}\n'
            for line in (rb'{"_msg":"synthetic\\marker"}', rb'{"_msg":"synthetic\u005cmarker"}'):
                with self.subTest(line=line):
                    path.write_bytes(safe + line + b"\n")
                    with self.assertRaisesRegex(CheckFailed, "protected value"):
                        scan_log_export(path, evidence)
            path.write_bytes(safe)
            self.assertEqual(scan_log_export(path, evidence), len(safe))
            path.write_bytes(b"not a log export\n")
            with self.assertRaisesRegex(CheckFailed, "JSON Lines"):
                scan_log_export(path, evidence)

    def test_export_scan_rejects_compressed_input(self):
        with tempfile.TemporaryDirectory() as directory:
            evidence = Evidence(None, "credentials")
            evidence.secrets.add("test-marker")
            path = Path(directory) / "export"
            path.write_bytes(gzip.compress(b"test-marker"))
            with self.assertRaisesRegex(CheckFailed, "plain UTF-8"):
                scan_file(path, evidence)

    def test_credential_in_response_aborts_before_json_is_returned(self):
        class Response(io.BytesIO):
            headers = {}

        class Opener:
            def open(self, req, timeout):
                response = Response(self.body)
                response.code = self.code
                return response

        evidence = Evidence(None, "test")
        evidence.secrets.add("synthetic-secret-value")
        opener = Opener()
        client = Client("https://example.test", evidence, opener=opener)
        for code in (200, 500):
            for body in (b'{"token":"synthetic-secret-value"}', rb'{"token":"\u0073ynthetic-secret-value"}'):
                with self.subTest(status=code):
                    opener.code, opener.body = code, body
                    with self.assertRaisesRegex(CheckFailed, "protected value"):
                        client.request("getConnection", "GET", "/connections/1", [200])
                    self.assertEqual(evidence.requests[-1]["status"], code)

    def test_snapshot_rejects_json_escaped_credentials(self):
        evidence = Evidence(None, "test")
        evidence.secrets.add("synthetic-secret-value")
        result = SimpleNamespace(stdout=rb'[{"summary":"\u0073ynthetic-secret-value"}]')
        with patch("evidence.subprocess.run", return_value=result):
            with self.assertRaisesRegex(CheckFailed, "protected value"):
                read_snapshot(Path("jira_snapshot.sql"), evidence)


class ObservationTests(unittest.TestCase):
    def test_sync_status_requires_requested_connection(self):
        connection = "123c8e31-70ba-4f8c-adf9-3601cc18b247"
        result = {"connection_id": connection, "data_kinds": [], "last_reconcile": None}
        with patch.object(Client, "request", return_value=result) as request:
            self.assertEqual(task_sync(Client("https://craftpulse.example.test", None), {}, connection), ({}, None))
            self.assertEqual(request.call_args.args[2], f"/api/v1/connections/{connection}/sync-status")

        result["connection_id"] = "f6a12089-ecc6-440f-8aa5-5ec808c74d41"
        with (patch.object(Client, "request", return_value=result),
              self.assertRaisesRegex(CheckFailed, "requested connection")):
            task_sync(Client("https://craftpulse.example.test", None), {}, connection)

    def test_daily_measures_from_saved_baseline_with_a_fresh_session(self):
        before = {"data_kind": "tasks", "phase": "incremental", "last_outcome": "success",
                  "failed_page_present": False,
                  "last_attempt_at": "2026-01-02T09:00:00Z", "last_success_at": "2026-01-02T09:05:00Z",
                  "watermark": "2026-01-02T09:01:00Z"}
        on_time = {**before, "last_attempt_at": "2026-01-03T09:00:00Z",
                   "last_success_at": "2026-01-03T09:02:00Z", "watermark": "2026-01-03T09:01:00Z"}
        finished = {"outcome": "complete", "started_at": "2026-01-03T09:01:00Z",
                    "finished_at": "2026-01-03T09:02:00Z"}
        baseline = {"scenario": "jira-cloud-reconciliation", "outcome": "passed",
                    "release_sha": "a" * 40, "connection": "fixture",
                    "sync_status": {"data_kinds": [before]}}
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "jira.json"
            path.write_text(json.dumps(baseline))
            options = SimpleNamespace(connection="fixture", sha="a" * 40, baseline=path)
            with patch("observe.now", return_value=timestamp("2026-01-03T09:03:00Z")):
                evidence = Evidence(None, "daily")
                with patch("observe.task_sync", return_value=(on_time, finished)) as status:
                    daily(None, {}, options, evidence)
                status.assert_called_once()
                self.assertEqual(evidence.results["interval_seconds"], 86400)
                self.assertEqual(evidence.results["seconds_from_observed_page_to_reconcile"], 120)
                self.assertEqual(evidence.results["reconcile_seconds"], 60)

                later_page = {**on_time, "last_attempt_at": "2026-01-03T09:00:45Z"}
                with (patch("observe.task_sync", side_effect=[(on_time, None), (later_page, finished)]),
                      patch("observe.sleep")):
                    daily(None, {}, options, evidence)
                self.assertEqual(
                    evidence.results["first_observed_attempt_at"],
                    timestamp(on_time["last_attempt_at"]).isoformat(),
                )
                self.assertEqual(evidence.results["latest"], later_page)

                late = {**on_time, "last_attempt_at": "2026-01-03T09:00:01Z"}
                with (patch("observe.task_sync", return_value=(late, None)),
                      self.assertRaisesRegex(CheckFailed, "cannot establish")):
                    daily(None, {}, options, Evidence(None, "daily"))

                with (patch("observe.task_sync", return_value=(before, None)),
                      self.assertRaisesRegex(CheckFailed, "no new collection")):
                    daily(None, {}, options, Evidence(None, "daily"))

                for invalid_status in (
                    {**on_time, "watermark": before["watermark"]},
                    {**on_time, "watermark": "2026-01-03T09:00:30Z"},
                    {**on_time, "failed_page_present": True},
                    {**on_time, "phase": "paused"},
                ):
                    with (patch("observe.task_sync", return_value=(invalid_status, finished)),
                          patch("observe.monotonic", side_effect=[0, 0, 601]),
                          self.assertRaisesRegex(CheckFailed, "reconciled watermark")):
                        daily(None, {}, options, Evidence(None, "daily"))

                options.connection = "other"
                with (patch("observe.task_sync") as status,
                      self.assertRaisesRegex(CheckFailed, "for this connection and release")):
                    daily(None, {}, options, Evidence(None, "daily"))
                status.assert_not_called()

    def test_access_requires_requested_task_or_not_found(self):
        task = "fixture"
        not_found = {"status": 404, "code": "not_found"}

        def client(status, body):
            response = io.BytesIO(json.dumps(body).encode())
            response.code = status
            response.headers = {}
            return Client("https://example.test", Evidence(None, "access"),
                          opener=SimpleNamespace(open=lambda *_args, **_kwargs: response))

        self.assertFalse(denied(client(200, {"id": task}), {}, task))
        self.assertTrue(denied(client(404, not_found), {}, task))
        for status, body, problem in (
            (200, not_found, "did not return the requested task"),
            (200, {"id": "different-task"}, "did not return the requested task"),
            (404, {"id": task}, "invalid not_found response"),
        ):
            with self.subTest(status=status, body=body):
                with self.assertRaisesRegex(CheckFailed, problem):
                    denied(client(status, body), {}, task)

    def test_recovery_requires_an_in_flight_import(self):
        options = SimpleNamespace(started_at=AS_OF, stopped_at=AS_OF)
        with self.assertRaisesRegex(CheckFailed, "started before"):
            recover_import(None, {}, options, Evidence(None, "recover-import"))

    def test_recovery_checks_journal_facts_and_identity(self):
        options = SimpleNamespace(
            started_at="2026-01-03T07:59:59Z", stopped_at="2026-01-03T08:00:00Z",
            control_receipt="control",
            interrupted_receipt="interrupted", organization="org", project="project",
            as_of=AS_OF, control_instance="continuous", interrupted_instance="restarted",
        )
        observed = timestamp("2026-01-03T08:01:00Z")
        result = {"status": "accepted", "channel": "import", "record_count": 2,
                  "accepted_count": 2, "schema_id": "tasks", "schema_version": "1"}
        journal = [{"record_no": i, "native_key": f"project/0{i}", "outcome": "accepted"}
                   for i in (1, 2)]
        tasks, _, _, _ = fixtures("project", "TEST-1")
        empty = {**expected_snapshot([], []), "entity_ids": {}}
        facts = expected_snapshot(tasks, [])
        ids = {"01": "a", "02": "b"}
        recovered_ids = {"01": "c", "02": "d"}
        cases = (
            (journal, {**facts, "entity_ids": recovered_ids}, None),
            (journal[:1], {**facts, "entity_ids": recovered_ids}, "journal differs"),
            (journal, {**facts, "entity_ids": ids}, "distinct identities"),
            (journal, {**facts, "entity_ids": recovered_ids, "versions": []}, "facts differ"),
            (journal, {**facts, "entity_ids": recovered_ids, "as_of": [
                {**facts["as_of"][0], "title": "Wrong historical title"}, facts["as_of"][1],
            ]}, "facts differ"),
        )
        for actual_journal, actual_facts, failure in cases:
            with (self.subTest(failure=failure),
                  patch("observe.now", return_value=observed),
                  patch("universal_import.monotonic", side_effect=[0, 301]),
                  patch("builtins.print"),
                  patch("observe.get_receipt", return_value={"channel": "import", "status": "quarantined"}),
                  patch("observe.receipt", side_effect=[{**result, "id": "control"},
                                                        {**result, "id": "interrupted"}]),
                  patch("observe.receipt_journal", side_effect=[[], journal, actual_journal]),
                  patch("observe.import_facts", side_effect=[{**facts, "entity_ids": ids},
                                                            empty, actual_facts])):
                evidence = Evidence(None, "recover-import")
                if failure:
                    with self.assertRaisesRegex(CheckFailed, failure):
                        recover_import(None, {}, options, evidence)
                else:
                    recover_import(None, {}, options, evidence)
                    self.assertEqual(timestamp(evidence.results["processing_started_at"]),
                                     timestamp(options.started_at))
                    self.assertEqual(evidence.results["receipt_seconds"], 60)
                    self.assertEqual(evidence.results["recovery_seconds"], 60)
                    self.assertEqual(evidence.results["reconciliation"][0]["actual"], facts)
