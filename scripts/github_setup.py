#!/usr/bin/env python3
"""Read/restore the repository settings stored in .github, via authenticated gh."""

import argparse
import json
from pathlib import Path
import subprocess
import sys
from urllib.parse import quote


ROOT = Path(__file__).resolve().parents[1]
REPOSITORY = "Avtobot22/craftPulse"


def gh_api(endpoint, method="GET", payload=None):
    command = ["gh", "api", "--hostname", "github.com", endpoint, "--method", method]
    if payload is not None:
        command.extend(["--input", "-"])
    result = subprocess.run(
        command, input=json.dumps(payload) if payload is not None else None,
        text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE, cwd=ROOT,
    )
    if result.returncode:
        raise RuntimeError(result.stderr.strip())
    return json.loads(result.stdout) if result.stdout.strip() else None


def load(name):
    return json.loads((ROOT / ".github" / name).read_text(encoding="utf-8"))


def matches_expected(actual, expected):
    """GitHub adds response-only fields and can reorder rules; permissions stay exact."""
    if isinstance(expected, dict):
        return isinstance(actual, dict) and all(
            key in actual and matches_expected(actual[key], value)
            for key, value in expected.items()
        )
    if isinstance(expected, list):
        return (isinstance(actual, list) and len(actual) == len(expected)
                and all(any(matches_expected(item, wanted) for item in actual)
                        for wanted in expected))
    return actual == expected


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("section", choices=["repository", "labels", "protection"])
    parser.add_argument("--apply", action="store_true", help="Apply changes; default is read-only")
    args = parser.parse_args()
    repo = gh_api("repos/" + REPOSITORY)
    if repo["full_name"].lower() != REPOSITORY.lower() or repo["default_branch"] != "main":
        raise RuntimeError("Unexpected repository or default branch; no changes applied")
    if args.apply and not repo.get("permissions", {}).get("admin"):
        raise RuntimeError("Repository admin permission is required")
    base = "repos/" + REPOSITORY
    if args.section == "repository":
        desired = load("repository-settings.json")
        changes = {key: value for key, value in desired.items() if repo.get(key) != value}
        print(json.dumps(changes, ensure_ascii=False, indent=2))
        if args.apply and changes:
            gh_api(base, "PATCH", changes)
        actual = gh_api(base) if args.apply else repo
        if args.apply and any(actual.get(key) != value for key, value in desired.items()):
            raise RuntimeError("Repository settings did not converge")
    elif args.section == "labels":
        # Paginate to preserve compatibility with repositories containing many labels.
        existing = {}
        page = 1
        while True:
            labels = gh_api(base + "/labels?per_page=100&page=" + str(page))
            existing.update({label["name"]: label for label in labels})
            if len(labels) < 100:
                break
            page += 1
        for label in load("labels.json"):
            current = existing.get(label["name"])
            if current and all(current.get(k) == v for k, v in label.items()):
                continue
            print(("update " if current else "create ") + label["name"])
            if args.apply:
                endpoint = base + "/labels"
                if current:
                    endpoint += "/" + quote(label["name"], safe="")
                gh_api(endpoint, "PATCH" if current else "POST", label)
        if args.apply:
            # Re-read managed labels; leave unrelated labels untouched.
            for label in load("labels.json"):
                actual = gh_api(base + "/labels/" + quote(label["name"], safe=""))
                if any(actual.get(k) != v for k, v in label.items()):
                    raise RuntimeError("Label did not converge: " + label["name"])
    else:
        desired = load("main-ruleset.json")
        rulesets = gh_api(base + "/rulesets?per_page=100&includes_parents=false")
        matching = [rule for rule in rulesets if rule["name"] == desired["name"]]
        if len(matching) > 1:
            raise RuntimeError("Duplicate rulesets; inspect before updating")
        print(json.dumps(desired, ensure_ascii=False, indent=2))
        if args.apply:
            # Bootstrap CI in the setup PR before requiring its successful check.
            runs = gh_api(base + "/actions/runs?per_page=100")
            successful = [run for run in runs["workflow_runs"]
                          if run["path"] == ".github/workflows/repository.yml"
                          and run["status"] == "completed" and run["conclusion"] == "success"]
            if not successful:
                raise RuntimeError("Run Repository CI successfully in a PR before enabling protection")
            jobs = gh_api(base + "/actions/runs/" + str(successful[0]["id"]) + "/jobs")
            if not any(job["name"] == "repository-check" and job["conclusion"] == "success"
                       for job in jobs["jobs"]):
                raise RuntimeError("Successful repository-check job is required before protection")
            endpoint = base + "/rulesets"
            if matching:
                endpoint += "/" + str(matching[0]["id"])
            actual = gh_api(endpoint, "PUT" if matching else "POST", desired)
            actual = gh_api(base + "/rulesets/" + str(actual["id"]))
            if not matches_expected(actual, desired):
                raise RuntimeError("Ruleset differs after apply; inspect GitHub response")
    print("Applied and verified." if args.apply else "Read-only; pass --apply to change GitHub.")


if __name__ == "__main__":
    try:
        main()
    except (RuntimeError, subprocess.SubprocessError, OSError, ValueError) as error:
        print(str(error), file=sys.stderr)
        sys.exit(1)
