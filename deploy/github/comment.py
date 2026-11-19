#!/usr/bin/env python3
"""Local socket worker: upsert one App bot comment on an allowlisted fixture PR."""

import argparse
import json
import os
from pathlib import Path
import sys

from manage import ORGANIZATION, validate
from app_auth import installation_api

MARKER = "<!-- craftpulse:cp89:summary -->"
MAX_INPUT = 32768


class Denied(ValueError):
    pass


def upsert(api, baseline, actor, payload):
    validate(baseline)
    if not isinstance(payload, dict) or set(payload) != {"repository", "pull_number", "body"}:
        raise Denied("Only repository, pull_number and body are accepted")
    repo = baseline["repositories"].get(payload["repository"])
    if repo is None or type(payload["pull_number"]) is not int:
        raise Denied("Repository/PR is not allowlisted")
    candidates = [p for p in repo["pulls"] if p["number"] == payload["pull_number"]]
    if len(candidates) != 1:
        raise Denied("PR is not allowlisted")
    original = candidates[0]
    body = payload["body"]
    if not isinstance(body, str) or not body.strip() or len(body.encode()) > 24000 or MARKER in body:
        raise Denied("Invalid comment body")
    base = f"repos/{ORGANIZATION}/{repo['name']}"
    actual_repo = api.request("GET", base)
    if actual_repo["id"] != repo["id"] or actual_repo["owner"]["id"] != baseline["organization_id"]:
        raise Denied("Repository identity changed")
    pull = api.request("GET", f"{base}/pulls/{original['number']}")
    if (pull["id"] != original["id"] or pull["created_at"] != original["created_at"]
            or pull["state"] != "open" or pull["merged"]):
        raise Denied("PR identity/state changed")
    matches = [c for c in api.pages(f"{base}/issues/{original['number']}/comments")
               if c["user"]["login"] == actor and (c["body"] or "").startswith(MARKER + "\n")]
    if len(matches) > 1:
        raise Denied("Multiple bot summary comments; reconcile before updating")
    desired = MARKER + "\n" + body
    if matches:
        comment = matches[0]
        if comment["body"] == desired:
            return {"id": comment["id"], "changed": False}
        updated = api.request("PATCH", f"{base}/issues/comments/{comment['id']}", {"body": desired})
    else:
        updated = api.request("POST", f"{base}/issues/{original['number']}/comments", {"body": desired})
    return {"id": updated["id"], "changed": True}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--config", type=Path, required=True)
    args = parser.parse_args()
    # SSH_ORIGINAL_COMMAND is set by sshd, never interpreted by a shell here.
    if os.environ.get("SSH_ORIGINAL_COMMAND", ""):
        raise Denied("Remote commands are forbidden; send only a JSON request on stdin")
    config = json.loads(args.config.read_text(encoding="utf-8"))
    baseline = json.loads(Path(config["baseline"]).read_text(encoding="utf-8"))
    raw = sys.stdin.buffer.read(MAX_INPUT + 1)
    if len(raw) > MAX_INPUT:
        raise Denied("Request exceeds 32 KiB")
    payload = json.loads(raw)
    app_config = json.loads(Path(config["app_config"]).read_text(encoding="utf-8"))
    if config["actor"] != app_config["slug"] + "[bot]":
        raise Denied("Unexpected bot identity")
    api = installation_api(app_config, baseline)
    result = upsert(api, baseline, config["actor"], payload)
    print(json.dumps(result))


if __name__ == "__main__":
    try:
        main()
    except (ValueError, RuntimeError, OSError, KeyError, TypeError):
        # Never echo the untrusted body, config, credentials or GitHub response.
        print('{"error":"comment request denied or upstream unavailable"}', file=sys.stderr)
        sys.exit(1)
