#!/usr/bin/env python3
"""Mint a repository-limited installation token for the fixture comment bot."""

import base64
import json
from pathlib import Path
import shutil
import subprocess
import time

from manage import API, ORGANIZATION, ORGANIZATION_ID, validate


def b64(data):
    return base64.urlsafe_b64encode(data).rstrip(b"=")


def app_jwt(client_id, key_file, now=None, openssl=None):
    now = int(time.time()) if now is None else int(now)
    executable = openssl or shutil.which("openssl")
    if executable is None:
        candidate = Path("C:/Program Files/Git/usr/bin/openssl.exe")
        if candidate.is_file():
            executable = str(candidate)
        else:
            raise RuntimeError("OpenSSL is required to sign the GitHub App JWT")
    header = b64(json.dumps({"alg": "RS256", "typ": "JWT"}, separators=(",", ":")).encode())
    payload = b64(json.dumps({"iat": now - 60, "exp": now + 540, "iss": client_id},
                             separators=(",", ":")).encode())
    signing_input = header + b"." + payload
    result = subprocess.run([executable, "dgst", "-sha256", "-sign", str(key_file)],
                            input=signing_input, stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=False)
    if result.returncode or not result.stdout:
        raise RuntimeError("GitHub App JWT signing failed")
    return (signing_input + b"." + b64(result.stdout)).decode()


def installation_api(config, baseline, contents_read=False):
    validate(baseline)
    if (config["organization_id"] != ORGANIZATION_ID or config["organization"] != ORGANIZATION
            or not isinstance(config["app_id"], int)):
        raise ValueError("Wrong App organization binding")
    issuer = API(app_jwt(config["client_id"], Path(config["key_file"])))
    app = issuer.request("GET", "app")
    if (app["id"] != config["app_id"] or app["slug"] != config["slug"]
            or app["owner"]["id"] != ORGANIZATION_ID
            or app["permissions"] not in ({"metadata": "read", "pull_requests": "write"},
                                           {"metadata": "read", "pull_requests": "write", "contents": "read"})):
        raise RuntimeError("Unexpected GitHub App identity/permissions")
    installation = issuer.request("GET", f"orgs/{ORGANIZATION}/installation")
    if (installation["account"]["id"] != ORGANIZATION_ID or installation.get("suspended_at")
            or installation["repository_selection"] != "selected"):
        raise RuntimeError("App must be installed on selected test repositories only")
    ids = sorted(repo["id"] for repo in baseline["repositories"].values())
    permissions = {"pull_requests": "write"}
    if contents_read:
        permissions["contents"] = "read"
    result = issuer.request("POST", f"app/installations/{installation['id']}/access_tokens", {
        "repository_ids": ids, "permissions": permissions})
    if result["permissions"] != dict(permissions, metadata="read"):
        raise RuntimeError("Unexpected installation token permissions")
    api = API(result["token"])
    repositories = api.request("GET", "installation/repositories?per_page=100")
    if (repositories["total_count"] != len(ids)
            or sorted(repo["id"] for repo in repositories["repositories"]) != ids):
        raise RuntimeError("Installation token repository allowlist mismatch")
    return api
