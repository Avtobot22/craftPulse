"""Exercise batching, durable deduplication and recovery without GitHub writes."""

import contextlib
from copy import deepcopy
import io
from pathlib import Path
import sys
import unittest
from unittest.mock import patch


ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "scripts"))
import github_periodic_review as review


CONFIG = review.load_config(ROOT / ".github/code-review.json")
SHA = "a" * 40


def connection(nodes, cursor=None):
    return {"nodes": nodes, "pageInfo": {"hasNextPage": cursor is not None, "endCursor": cursor}}


def item(number=75, points=8, **issue_changes):
    issue = {
        "__typename": "Issue", "id": "I_" + str(number), "number": number,
        "state": "CLOSED", "stateReason": "COMPLETED", "closedAt": "2026-11-12T01:00:00Z",
        "repository": {"nameWithOwner": CONFIG["repository"]},
        "labels": connection([{"name": "type:task"}, {"name": "area:backend"}]),
    }
    issue.update(issue_changes)
    return {"points": {"number": points}, "content": issue}


class FakeGitHub:
    def __init__(self, points=(13, 13, 13, 13, 13, 5)):
        self.items = [item(75 + n, size) for n, size in enumerate(points)]
        self.issues = []
        self.calls = []
        self.members = set()
        self.fail_add = False
        self.lose_create_response = False
        self.fail_read = False
        self.pr_overrides = {}

    def request(self, path, payload=None):
        self.calls.append((path, payload))
        if payload is not None:
            created = dict(deepcopy(payload), number=301 + len(self.issues), node_id="I_review",
                           state="open")
            self.issues.append(created)
            if self.lose_create_response:
                self.lose_create_response = False
                raise review.APIError("Create response was lost.")
            return created
        if path.endswith(CONFIG["repository"]):
            return {"full_name": CONFIG["repository"], "default_branch": "main"}
        if "/issues?" in path:
            if self.fail_read:
                raise review.APIError("Read failed.")
            return deepcopy(self.issues)
        if "/commits/" in path:
            return {"sha": SHA}
        return deepcopy(next(issue for issue in self.issues if path.endswith("/" + str(issue["number"]))))

    def graphql(self, query, **variables):
        self.calls.append((query, variables))
        if query.startswith("query ReviewProject"):
            return {"user": {"projectV2": {
                "id": "P_4", "title": CONFIG["project_title"], "closed": False,
                "fields": connection([{"name": "Story points", "dataType": "NUMBER"}]),
            }}}
        if query.startswith("query ReviewItems"):
            # More than one GraphQL page, including an empty first page, is valid.
            if variables["cursor"] is None:
                return {"node": {"items": connection([], "next")}}
            return {"node": {"items": connection(deepcopy(self.items))}}
        if query.startswith("query ReviewPullRequests"):
            number = int(variables["issue"].split("_")[1])
            prs = self.pr_overrides.get(number, [{
                "number": number + 200, "mergedAt": "2026-11-12T00:59:00Z", "baseRefName": "main",
                "repository": {"nameWithOwner": CONFIG["repository"]},
            }])
            return {"node": {"closedByPullRequestsReferences": connection(prs)}}
        if query.startswith("mutation ReviewItem"):
            if self.fail_add:
                raise review.APIError("Project write failed.")
            issue_id = variables["input"]["contentId"]
            self.members.add(issue_id)
            return {"addProjectV2ItemById": {"item": {
                "id": "PVTI_1", "project": {"id": "P_4"}, "content": {"id": issue_id},
            }}}
        raise AssertionError("Unexpected GraphQL operation")

    def writes(self):
        return [(path, payload) for path, payload in self.calls
                if path.startswith("mutation ") or (path.startswith("repos/") and payload is not None)]


class PeriodicReviewTest(unittest.TestCase):
    def sync(self, api, apply=False, writer=True):
        with contextlib.redirect_stdout(io.StringIO()):
            return review.synchronize(api, api if writer else None, CONFIG, apply)

    def test_read_only_above_threshold_does_not_write(self):
        api = FakeGitHub()
        payload = self.sync(api)
        self.assertIn("70 SP", payload["title"])
        self.assertEqual([], api.writes())
        self.assertEqual(CONFIG["assignees"], payload["assignees"])

    def test_below_threshold_does_not_create_a_review(self):
        api = FakeGitHub((13, 13, 13, 13, 13, 3, 1))  # 69 SP
        self.assertIsNone(self.sync(api, apply=True))
        self.assertEqual([], api.writes())

    def test_exact_threshold_creates_one_review_and_adds_it_to_project(self):
        api = FakeGitHub()
        self.sync(api, apply=True)
        self.assertEqual(1, len(api.issues))
        self.assertEqual({"I_review"}, api.members)
        records, consumed = review.review_records(api.issues, CONFIG["repository"])
        self.assertEqual(set(range(75, 81)), consumed)
        self.assertEqual(SHA, records[0][1]["head_sha"])

    def test_overshoot_consumes_whole_tasks_without_carrying_reviewed_points(self):
        api = FakeGitHub((13, 13, 13, 13, 13, 8))  # 73 SP
        self.sync(api, apply=True)
        self.assertIn("73 SP", api.issues[0]["title"])
        self.assertIsNone(self.sync(api, apply=True))
        self.assertEqual(1, len(api.issues))

    def test_reopened_and_reclosed_work_is_not_counted_again(self):
        api = FakeGitHub()
        self.sync(api, apply=True)
        api.items[0]["content"]["state"] = "OPEN"
        self.assertIsNone(self.sync(api, apply=True))
        api.items[0]["content"]["state"] = "CLOSED"
        api.items[0]["points"]["number"] = 13
        api.issues[0]["state"] = "closed"
        api.issues[0]["state_reason"] = "not_planned"
        self.assertIsNone(self.sync(api, apply=True))
        self.assertEqual(1, len(api.issues))

    def test_next_batch_links_to_previous_snapshot_even_if_review_still_open(self):
        api = FakeGitHub()
        self.sync(api, apply=True)
        api.items.extend(item(100 + n, size) for n, size in enumerate((13, 13, 13, 13, 13, 5)))
        self.sync(api, apply=True)
        self.assertEqual(2, len(api.issues))
        self.assertIn("compare/" + SHA + "..." + SHA, api.issues[1]["body"])
        self.assertIn("[CODE-REVIEW 002]", api.issues[1]["title"])

    def test_interrupted_project_addition_repairs_membership_without_duplicate(self):
        api = FakeGitHub()
        api.fail_add = True
        with self.assertRaisesRegex(review.APIError, "Project write failed"):
            self.sync(api, apply=True)
        self.assertEqual(1, len(api.issues))
        api.fail_add = False
        self.sync(api, apply=True)
        self.assertEqual(1, len(api.issues))
        self.assertEqual({"I_review"}, api.members)

    def test_lost_create_response_recovers_from_server_ledger(self):
        api = FakeGitHub()
        api.lose_create_response = True
        with self.assertRaisesRegex(review.APIError, "response was lost"):
            self.sync(api, apply=True)
        self.sync(api, apply=True)
        self.assertEqual(1, len(api.issues))
        self.assertEqual({"I_review"}, api.members)

    def test_review_itself_is_excluded_even_with_points_and_merged_pr(self):
        api = FakeGitHub()
        self.sync(api, apply=True)
        api.items.append(item(api.issues[0]["number"], 13))
        self.assertIsNone(self.sync(api, apply=True))

    def test_read_failure_stops_before_any_writes(self):
        api = FakeGitHub()
        api.fail_read = True
        with self.assertRaises(review.APIError):
            self.sync(api, apply=True)
        self.assertEqual([], api.writes())

    def test_missing_issue_token_stops_before_project_mutations(self):
        api = FakeGitHub()
        with self.assertRaisesRegex(review.APIError, "GITHUB_TOKEN"):
            self.sync(api, apply=True, writer=False)
        self.assertEqual([], api.writes())

    def test_missing_or_corrupt_ledger_stops_before_writes(self):
        for body in ("", "<!-- " + review.MARKER + " {} -->", "<!-- " + review.MARKER + " bad -->"):
            with self.subTest(body=body):
                api = FakeGitHub()
                api.issues = [{"number": 301, "title": "[CODE-REVIEW 001] Review", "body": body}]
                with self.assertRaises((review.APIError, KeyError)):
                    self.sync(api, apply=True)
                self.assertEqual([], api.writes())

    def test_overlapping_ledger_batches_are_rejected(self):
        api = FakeGitHub()
        self.sync(api, apply=True)
        api.issues.append(dict(api.issues[0], number=302))
        with self.assertRaisesRegex(review.APIError, "overlapping"):
            self.sync(api, apply=True)

    def test_unknown_points_are_skipped_and_can_be_counted_after_estimation(self):
        api = FakeGitHub((13, 13, 13, 13, 13, None))
        self.assertIsNone(self.sync(api, apply=True))
        api.items[-1]["points"]["number"] = 5
        self.sync(api, apply=True)
        self.assertEqual(1, len(api.issues))

    def test_invalid_points_stop_before_writes(self):
        for size in (0, -1, 7, True, "8", 8.5):
            with self.subTest(size=size):
                api = FakeGitHub((13, 13, 13, 13, 13, size))
                with self.assertRaises(review.APIError):
                    self.sync(api, apply=True)
                self.assertEqual([], api.writes())

    def test_non_code_and_uncompleted_issues_do_not_contribute(self):
        variants = [
            item(74), item(state="OPEN"), item(stateReason="NOT_PLANNED"),
            item(repository={"nameWithOwner": "another/repo"}),
            item(labels=connection([{"name": "area:docs"}])),
            item(labels=connection([{"name": "area:design"}])),
            item(labels=connection([{"name": "area:backend"}, {"name": "type:epic"}])),
            {"content": {"__typename": "PullRequest"}}, {"content": None},
        ]
        for variant in variants:
            with self.subTest(variant=variant):
                api = FakeGitHub()
                api.items = [variant]
                self.assertIsNone(self.sync(api, apply=True))
                self.assertEqual([], api.writes())

    def test_unmerged_wrong_branch_and_foreign_prs_do_not_contribute(self):
        for changes in ({"mergedAt": None}, {"baseRefName": "feature"},
                        {"repository": {"nameWithOwner": "another/repo"}}):
            with self.subTest(changes=changes):
                api = FakeGitHub()
                api.pr_overrides[75] = [{"number": 275, "mergedAt": "now", "baseRefName": "main",
                                         "repository": {"nameWithOwner": CONFIG["repository"]}}]
                api.pr_overrides[75][0].update(changes)
                self.assertIsNone(self.sync(api, apply=True))

    def test_closed_issue_without_closing_pr_is_not_counted(self):
        api = FakeGitHub()
        api.pr_overrides[75] = []
        self.assertIsNone(self.sync(api, apply=True))

    def test_rest_pagination_does_not_treat_prs_as_review_records(self):
        api = FakeGitHub()
        issue = {"number": 1, "title": "PR", "pull_request": {}, "body": review.MARKER}
        with patch.object(api, "request", side_effect=[[issue] * 100, [{"number": 2, "title": "Task"}]]):
            self.assertEqual([{"number": 2, "title": "Task"}],
                             list(review.repository_issues(api, CONFIG["repository"])))

    def test_config_rejects_another_repository_in_actions(self):
        with patch.dict(review.os.environ, {"GITHUB_REPOSITORY": "another/repo"}):
            with self.assertRaisesRegex(review.APIError, "differs"):
                review.load_config(ROOT / ".github/code-review.json")


if __name__ == "__main__":
    unittest.main()
