"""Save a contract report from the tracker integration tests without raw test logs."""

import argparse
import json
import os
from pathlib import Path
import subprocess

from evidence import check, run_report

BACKEND = Path(__file__).resolve().parents[2] / "backend"
PACKAGE = "github.com/Avtobot22/craftPulse/backend/internal/app"
TESTS = (
    "TestJiraCloudAdapterPagesAndReplay",
    "TestJiraCloudAdapterLongHistory",
    "TestJiraCloudAdapterProbeAndFailures",
    "TestJiraCloudCheckAccess",
    "TestConnections_ConnectProbeAndActivate",
    "TestConnections_ProbeFailures",
    "TestConnections_SyncStatusAndLastSuccess",
    "TestConnections_SyncRunsAreBackgroundOperations",
    "TestSync_RecoveryValidatesEveryHistoryPage",
    "TestSync_InterruptedPageIsReadAgainWithoutDuplicates",
    "TestSync_RateLimitHoldsEveryRequest",
    "TestSync_NextPageFailureKeepsTheCursorAndJournal",
    "TestSync_AuthStopsCollection",
    "TestSync_OneRunPerConnection",
    "TestSync_ScheduleStartsTheDailyIncrement",
    "TestSync_V06_StatusChangesOnlyAfterReconcile",
    "TestSync_UnavailableSourceIsRepeatedWithBackoff",
    "TestSync_HistoryRepeatsAfterRateLimit",
    "TestImports_UniversalImportOfTasks",
    "TestIngest_RefusesUnsafePackagesAtOnce",
    "TestIngest_JournalOfImports",
    "TestIngest_PagesOfSync",
    "TestIngest_InterruptedJobGivesTheSameJournal",
    "TestAccess_ProjectFollowsTheRightInTheSource",
    "TestAccess_AreaThatCannotBeCheckedIsClosed",
    "TestAccess_TaskIsHiddenAfterAdapterStops",
    "TestNormalize_SameKeyInJiraAndUniversalImportGivesTwoObjects",
    "TestNormalize_RepeatsAddNothing",
)


def scenario_outcomes(output):
    scenarios = dict.fromkeys(TESTS, "missing")
    package = "missing"
    skipped_subtests = set()
    for line in output.splitlines():
        event = json.loads(line)
        outcome = event.get("Action")
        if event.get("Package") != PACKAGE or outcome not in {"pass", "fail", "skip"}:
            continue
        test = event.get("Test")
        if test is None:
            package = outcome if package == "missing" else "duplicate"
        elif test in scenarios:
            scenarios[test] = outcome if scenarios[test] == "missing" else "duplicate"
        elif outcome == "skip":
            scenario, _, _ = test.partition("/")
            if scenario in scenarios:
                skipped_subtests.add(scenario)

    for scenario in skipped_subtests:
        if scenarios[scenario] == "pass":
            scenarios[scenario] = "skip"
    return package, scenarios


def run(options, evidence):
    result = subprocess.run(
        ["git", "rev-parse", "HEAD"], cwd=BACKEND, check=False,
        stdout=subprocess.PIPE, stderr=subprocess.DEVNULL,
    )
    revision = result.stdout.decode().strip()
    check(result.returncode == 0 and len(revision) == 40, "could not identify the checkout")
    evidence.results.update(checkout_sha=revision, environment="local integration tests",
                            expected="all selected top-level scenarios pass")
    env = os.environ.copy()
    env.setdefault("GOMAXPROCS", "2")
    result = subprocess.run(
        ["timeout", "-k", "2s", "90s", "go", "test", "-p=2", "./internal/app", "-parallel=2",
         "-count=1", "-json", "-run", "^(" + "|".join(TESTS) + ")$"],
        cwd=BACKEND, env=env, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL,
    )
    evidence.results["test_exit_code"] = result.returncode
    package, scenarios = scenario_outcomes(result.stdout)
    evidence.results.update(package_outcome=package, scenarios=scenarios)
    check(result.returncode not in {124, 137}, "integration suite exceeded its local time limit or was stopped")
    check(result.returncode == 0 and package == "pass" and all(outcome == "pass" for outcome in scenarios.values()),
          "integration tests failed, were skipped or did not run")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--report", required=True, type=Path)
    run_report(parser, parser.parse_args(), "tracker-test-environment", run)


if __name__ == "__main__":
    main()
