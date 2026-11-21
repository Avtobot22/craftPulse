#!/usr/bin/env python3
"""Read-only checks of source APIs on the test stand (Python 3.10+, stdlib)."""

import argparse
from collections import Counter
from datetime import datetime, timezone
import json
import os
from pathlib import Path
import sys
import time
from urllib.error import HTTPError
from urllib.parse import parse_qsl, urlencode, urlsplit
from urllib.request import HTTPRedirectHandler, Request, build_opener


class NoRedirect(HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


class Source:
    def __init__(self, base_url, headers):
        url = urlsplit(base_url)
        if url.scheme != "https" or not url.hostname or url.username or url.query or url.fragment:
            raise ValueError("base URL must be HTTPS without credentials, query or fragment")
        self.base_url = base_url.rstrip("/")
        self.headers = {"Accept": "application/json", **headers}
        self.opener = build_opener(NoRedirect())

    def get(self, path, query):
        endpoint = urlsplit(path)
        if endpoint.scheme or endpoint.netloc or endpoint.query or endpoint.fragment or not path.startswith("/"):
            raise ValueError("each path must be relative to the configured API base")
        target = self.base_url + path + ("?" + urlencode(query) if query else "")
        start = time.monotonic()
        try:
            response = self.opener.open(Request(target, headers=self.headers), timeout=20)
        except HTTPError as error:
            response = error
        with response:
            meta = {
                "status": response.code,
                "ms": round((time.monotonic() - start) * 1000),
            }
            if response.code != 200:
                return meta, response.headers, None
            payload = json.load(response)
            meta["ms"] = round((time.monotonic() - start) * 1000)
            return meta, response.headers, payload


def field(value, path):
    for name in path.split("."):
        if not isinstance(value, dict):
            return None
        value = value.get(name)
    return value


def request_check(source, check):
    meta, _, body = source.get(check["path"], check.get("query", {}))
    expected = check.get("status", [200])
    meta["matches"] = meta["status"] in expected
    if "budget_ms" in check:
        meta["within_budget"] = meta["ms"] <= check["budget_ms"]
        meta["matches"] &= meta["within_budget"]
    if "contains" in check:
        match = check["contains"]
        meta["contains_match"] = isinstance(body, list) and (
            any(all(field(row, key) == value for key, value in match.items()) for row in body)
            == check.get("contains_expected", True)
        )
        meta["matches"] &= meta["contains_match"]
    if "expect_fields" in check:
        meta["fields_match"] = {
            key: field(body, key) == value for key, value in check["expect_fields"].items()
        }
        meta["matches"] &= all(meta["fields_match"].values())
    meta["observed"] = {key: field(body, key) for key in check.get("observe", [])}
    return meta


def continuation(mode, body, headers, query, count):
    next_query = query.copy()
    if mode == "jira-search":
        token = body.get("nextPageToken")
        if body.get("isLast") is True:
            return None
        if not token:
            raise ValueError("Jira search did not supply the next page token")
        next_query["nextPageToken"] = token
    elif mode == "jira-changelog":
        if body["isLast"]:
            return None
        if count == 0:
            raise ValueError("empty nonfinal changelog page")
        next_query["startAt"] = body["startAt"] + count
    elif mode == "confluence":
        link = body.get("_links", {}).get("next")
        if not link:
            link = link_next(headers)
        if not link:
            return None
        next_query.update(parse_qsl(urlsplit(link).query, keep_blank_values=True))
    elif mode == "gitlab":
        page = headers.get("X-Next-Page") or dict(parse_qsl(urlsplit(link_next(headers)).query)).get("page")
        if not page:
            return None
        next_query["page"] = page
    else:
        raise ValueError("unknown pagination mode")
    return next_query


def link_next(headers):
    for item in headers.get("Link", "").split(","):
        url, _, parameters = item.strip().partition(";")
        if 'rel="next"' in parameters:
            return url.strip("<>")
    return ""


def list_pages(source, check, after_first=None):
    path, mode = check["path"], check["mode"]
    query = check.get("query", {}).copy()
    pages, ids, seen_queries = [], [], set()
    complete = False
    for number in range(1, check.get("max_pages", 300) + 1):
        signature = urlencode(sorted(query.items()))
        if signature in seen_queries:
            raise ValueError("pagination repeated a request")
        seen_queries.add(signature)
        meta, headers, body = source.get(path, query)
        pages.append(meta)
        if meta["status"] != 200:
            break
        rows = field(body, check["items"]) if "items" in check else body
        if not isinstance(rows, list):
            raise ValueError("expected list of rows")
        batch = [field(row, check.get("id", "id")) for row in rows]
        if any(value is None for value in batch):
            raise ValueError("row is missing its identifier")
        ids.extend(str(value) for value in batch)
        meta["count"] = len(batch)
        query = continuation(mode, body, headers, query, len(batch))
        if number == 1 and after_first is not None:
            if query is None:
                raise ValueError("the write must be between at least two pages")
            after_first()
        if query is None:
            complete = True
            break
    counts = Counter(ids)
    return {
        "pages": pages,
        "ids": ids,
        "complete": complete,
        "duplicates": sorted(key for key, count in counts.items() if count > 1),
    }


def compare_pages(check, source, after_first=None):
    reference = {**check, "query": {**check.get("query", {}), **check["reference_query"]}}
    baseline = list_pages(source, reference) if after_first else None
    scanned = list_pages(source, check, (lambda: after_first(baseline["ids"])) if baseline else None)
    settled = list_pages(source, reference)
    reference_ids = set(settled["ids"])
    scanned_ids = set(scanned["ids"])
    result = {
        "scan": scanned,
        "reference": settled,
        "missing": sorted(reference_ids - scanned_ids),
        "extra": sorted(scanned_ids - reference_ids),
    }
    if baseline is not None:
        result["before_write"] = baseline
        result["reference_removed"] = sorted(set(baseline["ids"]) - reference_ids)
        result["reference_added"] = sorted(reference_ids - set(baseline["ids"]))
        result["order_changed"] = baseline["ids"] != settled["ids"]
    if "expect_ids" in check:
        result["missing_from_reference"] = sorted(set(map(str, check["expect_ids"])) - reference_ids)
    if "expected_count" in check:
        result["reference_count_matches"] = len(reference_ids) == check["expected_count"]
    stable_only = not after_first or not check.get("allow_drift", False)
    result["matches"] = (
        scanned["complete"] and settled["complete"] and not settled["duplicates"]
        and (baseline is None or baseline["complete"] and not baseline["duplicates"] and result["order_changed"])
        and not result.get("missing_from_reference", []) and result.get("reference_count_matches", True)
        and len(scanned["pages"]) >= check.get("min_pages", 1)
        and (not stable_only or not (result["missing"] or result["extra"] or scanned["duplicates"]))
    )
    return result


def run(plan, sources, pause=None):
    results = {}
    for check in plan["checks"]:
        source = sources[check["source"]]
        name = check["name"]
        if name in results:
            raise ValueError("check names must be unique")
        if "mode" not in check:
            results[name] = request_check(source, check)
            continue
        if check.get("pause_after_first") and pause is None:
            raise ValueError("this plan requires an operator between pages")
        callback = (lambda baseline: pause(name, baseline)) if check.get("pause_after_first") else None
        results[name] = compare_pages(check, source, callback)
    return {
        "checked_at": datetime.now(timezone.utc).isoformat(timespec="seconds"),
        "checks": results,
        "matches": all(result["matches"] for result in results.values()),
    }


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("plan", type=Path)
    parser.add_argument("--source", action="append", help="run only the named source (repeatable)")
    parser.add_argument("--interactive", action="store_true", help="pause for fixture edits after page 1")
    args = parser.parse_args()
    try:
        plan = json.loads(args.plan.read_text(encoding="utf-8"))
        names = set(args.source or plan["sources"])
        if names - plan["sources"].keys():
            raise ValueError("unknown source in --source")
        plan["checks"] = [check for check in plan["checks"] if check["source"] in names]
        if not plan["checks"]:
            raise ValueError("no selected checks")
        sources = {}
        for name in sorted(names):
            config = plan["sources"][name]
            header = config.get("auth_header", "Authorization")
            prefix = config.get("auth_prefix", "Bearer ")
            if header not in ("Authorization", "PRIVATE-TOKEN"):
                raise ValueError("unsupported authentication header")
            auth = {header: prefix + os.environ[config["credential_env"]]} if "credential_env" in config else {}
            sources[name] = Source(config["base_url"], auth)

        def pause(name, baseline):
            print(f"{name}: before the write: {baseline}", file=sys.stderr)
            print("Edit two objects from the end, wait for them to become visible at the start, then press Enter.", file=sys.stderr)
            if not sys.stdin.readline():
                raise ValueError("fixture editing was interrupted")

        output = run(plan, sources, pause if args.interactive else None)
    except (OSError, ValueError, TypeError, KeyError) as error:
        print(f"Source check failed ({type(error).__name__}). See the plan, credentials and connectivity.", file=sys.stderr)
        return 2
    print(json.dumps(output, indent=2))
    return 0 if output["matches"] else 1


if __name__ == "__main__":
    sys.exit(main())
