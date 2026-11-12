#!/usr/bin/env python3
"""Create a repository-wide review after completed work reaches its SP threshold.

Read-only unless --apply. Review issues themselves hold the immutable batch ledger;
neither Actions caches nor a second write to every source issue are required.
"""

import argparse
import json
import os
from pathlib import Path
import re
import sys
from urllib.parse import quote

ROOT = Path(__file__).resolve().parents[1]
# The project's embedded Windows Python does not add the script directory to sys.path.
sys.path.insert(0, str(ROOT / "scripts"))
from github_project_readiness import APIError, GitHub, PAGE_INFO, pages


MARKER = "craftpulse-code-review:v1"
RECORD = re.compile(r"<!-- " + MARKER + r" (\{[^\r\n]*\}) -->")
PREFIX = "[CODE-REVIEW "
SCALE = {1, 2, 3, 5, 8, 13}


def positive_int(value):
    return type(value) is int and value > 0


def load_config(path):
    config = json.loads(path.read_text(encoding="utf-8"))
    if (not re.fullmatch(r"[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+", config["repository"])
            or config["repository"].split("/")[0] != config["owner"]
            or not all(positive_int(config[key]) for key in ("project_number", "threshold", "first_issue"))
            or not isinstance(config["project_title"], str) or not config["project_title"]
            or not isinstance(config["areas"], list) or not config["areas"]
            or any(not isinstance(area, str) or not area.startswith("area:") for area in config["areas"])
            or not isinstance(config["assignees"], list)
            or any(not re.fullmatch(r"[A-Za-z0-9-]+", name) for name in config["assignees"])):
        raise APIError("Invalid periodic review configuration.")
    actual = os.environ.get("GITHUB_REPOSITORY")
    if actual and actual.casefold() != config["repository"].casefold():
        raise APIError("Configured repository differs from GITHUB_REPOSITORY.")
    return config


def project(api, config):
    query = """query ReviewProject($owner: String!, $number: Int!) {
      user(login: $owner) { projectV2(number: $number) {
        id title closed
        fields(first: 100) {
          nodes { ... on ProjectV2Field { name dataType } }
          pageInfo { hasNextPage }
        }
      } }
    }"""
    user = api.graphql(query, owner=config["owner"], number=config["project_number"]).get("user")
    result = user and user.get("projectV2")
    if not result or result["closed"] or result["title"] != config["project_title"]:
        raise APIError("Expected the configured open craftPulse project.")
    fields = result["fields"]
    points = [field for field in fields["nodes"] if field.get("name") == "Story points"]
    if fields["pageInfo"]["hasNextPage"] or len(points) != 1 or points[0].get("dataType") != "NUMBER":
        raise APIError("Expected a unique numeric Story points field.")
    return result["id"]


def project_items(api, project_id):
    query = """query ReviewItems($project: ID!, $cursor: String) {
      node(id: $project) { ... on ProjectV2 { items(first: 100, after: $cursor) {
        nodes {
          points: fieldValueByName(name: "Story points") {
            ... on ProjectV2ItemFieldNumberValue { number }
          }
          content {
            __typename
            ... on Issue {
              id number state stateReason closedAt repository { nameWithOwner }
              labels(first: 100) { nodes { name } pageInfo { hasNextPage } }
            }
          }
        } %s
      } } }
    }""" % PAGE_INFO
    return pages(lambda cursor: api.graphql(query, project=project_id, cursor=cursor)["node"]["items"])


def repository_issues(api, repository):
    page = 1
    while True:
        response = api.request("repos/{}/issues?state=all&sort=created&direction=asc&per_page=100&page={}"
                               .format(repository, page))
        if not isinstance(response, list) or any(not isinstance(issue, dict) for issue in response):
            raise APIError("Invalid repository issue list; review ledger was not read.")
        yield from (issue for issue in response if "pull_request" not in issue)
        if len(response) < 100:
            return
        page += 1


def review_records(issues, repository):
    records = []
    consumed = set()
    for issue in issues:
        body = issue.get("body") or ""
        matches = RECORD.findall(body)
        if not matches and not issue["title"].startswith(PREFIX) and MARKER not in body:
            continue
        if len(matches) != 1:
            raise APIError("Review ledger is missing or malformed; restore the review issue's service block.")
        record = json.loads(matches[0])
        batch = record["issues"]
        if (record["repository"] != repository or record["version"] != 1
                or not re.fullmatch(r"[0-9a-f]{40}", record["head_sha"])
                or not positive_int(record["threshold"]) or not isinstance(batch, list) or not batch):
            raise APIError("Invalid review ledger; refusing to count work twice.")
        for entry in batch:
            if (not positive_int(entry["number"]) or type(entry["points"]) is not int
                    or entry["points"] not in SCALE or not isinstance(entry["pull_requests"], list)
                    or not entry["pull_requests"] or not all(positive_int(n) for n in entry["pull_requests"])
                    or entry["number"] in consumed or entry["number"] == issue["number"]):
                raise APIError("Invalid or overlapping review batches; inspect the review ledger.")
            consumed.add(entry["number"])
        if sum(entry["points"] for entry in batch) < record["threshold"]:
            raise APIError("Review ledger has fewer points than its recorded threshold.")
        records.append((issue, record))
    records.sort(key=lambda pair: pair[0]["number"])
    return records, consumed


def closing_prs(api, issue_id, repository, branch):
    query = """query ReviewPullRequests($issue: ID!, $cursor: String) {
      node(id: $issue) { ... on Issue { closedByPullRequestsReferences(first: 100, after: $cursor) {
        nodes { number mergedAt baseRefName repository { nameWithOwner } } %s
      } } }
    }""" % PAGE_INFO
    pull_requests = pages(lambda cursor: api.graphql(query, issue=issue_id, cursor=cursor)
                          ["node"]["closedByPullRequestsReferences"])
    return sorted({pr["number"] for pr in pull_requests
                   if pr["mergedAt"] and pr["baseRefName"] == branch
                   and pr["repository"]["nameWithOwner"].casefold() == repository.casefold()})


def pending_work(api, items, config, consumed, review_numbers, branch):
    pending = []
    seen = set()
    for item in items:
        issue = item.get("content")
        if (not issue or issue.get("__typename") != "Issue"
                or issue["repository"]["nameWithOwner"].casefold() != config["repository"].casefold()
                or issue["state"] != "CLOSED" or issue["stateReason"] != "COMPLETED"
                or issue["number"] < config["first_issue"]
                or issue["number"] in consumed or issue["number"] in review_numbers):
            continue
        if issue["number"] in seen:
            raise APIError("Duplicate project issue; refusing to double-count Story points.")
        seen.add(issue["number"])
        if issue["labels"]["pageInfo"]["hasNextPage"]:
            raise APIError("Issue has more than 100 labels; cannot safely determine review eligibility.")
        labels = {label["name"] for label in issue["labels"]["nodes"]}
        if "type:epic" in labels or not labels.intersection(config["areas"]):
            continue
        points = (item.get("points") or {}).get("number")
        if points is None:
            print("Skipped #{}: Story points is empty.".format(issue["number"]))
            continue
        if type(points) not in (int, float) or points not in SCALE:
            raise APIError("Completed issue has Story points outside the agreed 1, 2, 3, 5, 8, 13 scale.")
        pull_requests = closing_prs(api, issue["id"], config["repository"], branch)
        if not pull_requests:
            print("Skipped #{}: no closing PR merged into the default branch.".format(issue["number"]))
            continue
        pending.append({"number": issue["number"], "points": int(points), "pull_requests": pull_requests,
                        "closed_at": issue["closedAt"]})
    return sorted(pending, key=lambda entry: (entry["closed_at"], entry["number"]))


def review_body(config, batch, head_sha, previous):
    repository = config["repository"]
    total = sum(entry["points"] for entry in batch)
    url = "https://github.com/" + repository
    previous_sha = previous[1]["head_sha"] if previous else None
    lines = [
        "### Проблема и результат", "",
        "Завершено {} SP при пороге {} SP. Нужно проверить весь код craftPulse на зафиксированном срезе "
        "и оформить найденные дефекты и технический долг.".format(total, config["threshold"]), "",
        "### Границы работы", "",
        "Срез для проверки: [{}]({}/tree/{}).".format(head_sha, url, head_sha), "",
        "Проверить весь репозиторий: Go backend, PostgreSQL и миграции, контракт API, frontend при наличии "
        "реализации, CI, инструменты и конфигурацию развёртывания. Сверить код с ТЗ и правилами проекта.", "",
    ]
    if previous:
        lines.extend(["Предыдущая проверка: #{}. [Изменения после её среза]({}/compare/{}...{})."
                      .format(previous[0]["number"], url, previous_sha, head_sha), ""])
    else:
        lines.extend(["Это первая автоматическая проверка кода; проверить всю существующую реализацию.", ""])
    lines.extend(["Задачи, набравшие порог:", "", "| Задача | SP | Влитые PR |", "| --- | --- | --- |"])
    for entry in batch:
        lines.append("| #{} | {} | {} |".format(entry["number"], entry["points"],
                                                ", ".join("#{}".format(n) for n in entry["pull_requests"])))
    lines.extend([
        "", "### Критерии приёмки", "",
        "- [ ] Проверены архитектура, границы модулей и соответствие реализации контракту и ТЗ.",
        "- [ ] Проверены авторизация, изоляция организаций и RLS, секреты и обработка внешних данных.",
        "- [ ] Проверены транзакции, миграции, конкурентность, фоновые задания и обработка ошибок.",
        "- [ ] Проверены тесты ключевых сценариев и воспроизводимость проверок и сборки.",
        "- [ ] В комментарии приложен отчёт: области проверки, находки с файлами и строками, "
        "важность и выполненные проверки.",
        "- [ ] Подтверждённые дефекты оформлены отдельными задачами; ссылки добавлены в отчёт.",
        "", "### Как проверить", "",
        "Проверять указанный commit SHA. Выполнить команды проверок из README и CONTRIBUTING "
        "для этого среза и изучить соответствующий запуск Repository. Отметить недоступные проверки "
        "и причины. Закрыть эту задачу после публикации результатов ревью.", "",
        "Story points для самой проверки команда согласует отдельно; workflow их не назначает.", "",
        "Служебный блок ниже хранит состав проверки. Не удалять и не редактировать: "
        "он предотвращает повторный учёт задач, в том числе после их повторного открытия.", "",
    ])
    record = {"version": 1, "repository": repository, "threshold": config["threshold"],
              "head_sha": head_sha, "previous_sha": previous_sha, "issues": batch}
    lines.append("<!-- {} {} -->".format(MARKER, json.dumps(record, separators=(",", ":"))))
    return "\n".join(lines) + "\n"


def ensure_project_item(api, project_id, issue_id):
    # Adding an existing item returns that item. This repairs a run interrupted
    # after issue creation, without changing a person's status, SP or assignee.
    mutation = """mutation ReviewItem($input: AddProjectV2ItemByIdInput!) {
      addProjectV2ItemById(input: $input) { item { id project { id } content { ... on Issue { id } } } }
    }"""
    item = api.graphql(mutation, input={"projectId": project_id, "contentId": issue_id})[
        "addProjectV2ItemById"]["item"]
    if not item or item["project"]["id"] != project_id or item["content"]["id"] != issue_id:
        raise APIError("Review project membership was not confirmed; retry to repair it.")


def synchronize(api, writer, config, apply=False):
    repository = config["repository"]
    repo = api.request("repos/" + repository)
    if repo["full_name"].casefold() != repository.casefold():
        raise APIError("GitHub returned a different repository.")
    branch = repo["default_branch"]
    project_id = project(api, config)
    records, consumed = review_records(repository_issues(api, repository), repository)
    review_numbers = {issue["number"] for issue, _ in records}
    batch = pending_work(api, project_items(api, project_id), config, consumed, review_numbers, branch)
    total = sum(entry["points"] for entry in batch)
    print("Pending: {} / {} SP; issues: {}.".format(
        total, config["threshold"], ", ".join("#{}".format(entry["number"]) for entry in batch) or "none"))
    payload = None
    if total >= config["threshold"]:
        head_sha = api.request("repos/{}/commits/{}".format(repository, quote(branch, safe="")))["sha"]
        if not re.fullmatch(r"[0-9a-f]{40}", head_sha):
            raise APIError("Invalid default branch commit SHA.")
        payload = {
            "title": "[CODE-REVIEW {:03d}] Проверить весь код craftPulse после {} SP"
                     .format(len(records) + 1, total),
            "body": review_body(config, batch, head_sha, records[-1] if records else None),
            "labels": ["type:task", "area:infra", "priority:normal"],
            "assignees": config["assignees"],
        }
    if not apply:
        print("Read-only; {}.".format("would create: " + payload["title"] if payload else "threshold not reached"))
        return payload
    if payload and writer is None:
        raise APIError("GITHUB_TOKEN with issues:write is required to create a review issue.")
    # Complete all planning reads before writing. Repair membership even when
    # the threshold is not reached: creation may have succeeded in a failed run.
    for issue, _ in records:
        if issue["state"] == "open":
            ensure_project_item(api, project_id, issue["node_id"])
    if payload:
        created = writer.request("repos/{}/issues".format(repository), payload)
        # A single POST atomically records every consumed issue in the body.
        confirmed = writer.request("repos/{}/issues/{}".format(repository, created["number"]))
        actual, _ = review_records([confirmed], repository)
        if len(actual) != 1 or actual[0][1] != json.loads(RECORD.search(payload["body"]).group(1)):
            raise APIError("Review creation was not confirmed; inspect the issue before retrying.")
        ensure_project_item(api, project_id, confirmed["node_id"])
        print("Created review #{}.".format(confirmed["number"]))
    return payload


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--config", type=Path, default=ROOT / ".github/code-review.json")
    parser.add_argument("--apply", action="store_true", help="Create review issues; default is read-only")
    args = parser.parse_args(argv)
    try:
        config = load_config(args.config)
        api = GitHub(os.environ.get("PROJECT_TOKEN"))
        token = os.environ.get("GITHUB_TOKEN")
        writer = GitHub(token) if args.apply and token else None
        synchronize(api, writer, config, args.apply)
    except (APIError, OSError, LookupError, AttributeError, TypeError, ValueError) as error:
        print(str(error) if isinstance(error, APIError) else "Unexpected review configuration or API response; stopped.",
              file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
