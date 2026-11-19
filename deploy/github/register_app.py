#!/usr/bin/env python3
"""One-hour loopback-only GitHub App manifest registration helper for the fixture bot."""

import argparse
from html import escape
from http.server import BaseHTTPRequestHandler, HTTPServer
import hmac
import json
import os
from pathlib import Path
import secrets
import sys
import subprocess
import threading
from urllib.parse import parse_qs, urlsplit
from urllib.request import Request, build_opener

sys.path.insert(0, str(Path(__file__).resolve().parent))
from manage import API_VERSION, NoRedirect, ORGANIZATION, ORGANIZATION_ID


def private_output(path):
    root = Path(__file__).resolve().parents[2]
    resolved = Path(path).resolve()
    if resolved.is_relative_to((root / "shared").resolve()):
        raise ValueError("App credentials must not be written to shared")
    if resolved.is_relative_to(root):
        ignored = subprocess.run(["git", "check-ignore", "--quiet", "--", str(resolved)],
                                 cwd=root, stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=False)
        if ignored.returncode:
            raise ValueError("App credentials inside the checkout must be ignored by Git")
    if resolved.exists():
        raise ValueError("App credentials already exist; refusing overwrite")
    resolved.parent.mkdir(parents=True, mode=0o700, exist_ok=True)
    if os.name != "nt" and resolved.parent.stat().st_mode & 0o077:
        raise ValueError("App credential directory must have mode 0700")
    return resolved


def save_config(path, config):
    path = private_output(path)
    descriptor = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    with os.fdopen(descriptor, "w", encoding="utf-8") as target:
        json.dump(config, target)


def serve(output, url_file, port=8789):
    output = private_output(output)
    state = secrets.token_urlsafe(32)
    origin = f"http://127.0.0.1:{port}"
    manifest = {"name": "craftpulse-mvp-bot", "url": "https://github.com/" + ORGANIZATION,
                "description": "Comment identity for craftPulse MVP test pull requests (#89)",
                "public": False, "redirect_url": origin + "/callback",
                "hook_attributes": {"url": "https://craftpulse.defects.team/github/webhooks", "active": False},
                "default_permissions": {"pull_requests": "write", "contents": "read"}, "default_events": []}

    class Handler(BaseHTTPRequestHandler):
        def log_message(self, format, *args):
            pass  # Never log callback codes or state values.

        def send_page(self, status, body):
            content = body.encode("utf-8")
            self.send_response(status)
            self.send_header("Content-Type", "text/html; charset=utf-8")
            self.send_header("Content-Length", str(len(content)))
            self.send_header("Cache-Control", "no-store")
            self.send_header("Referrer-Policy", "no-referrer")
            self.send_header("X-Content-Type-Options", "nosniff")
            self.send_header("Content-Security-Policy", "default-src 'none'; form-action https://github.com; frame-ancestors 'none'")
            self.end_headers()
            self.wfile.write(content)

        def do_GET(self):
            if self.headers.get("Host") != f"127.0.0.1:{port}":
                self.send_page(403, "Forbidden")
                return
            parsed = urlsplit(self.path)
            if parsed.path == "/start/" + state:
                if output.exists():
                    self.send_page(409, "App already registered")
                    return
                action = f"https://github.com/organizations/{ORGANIZATION}/settings/apps/new?state={state}"
                self.send_page(200, '<!doctype html><html lang="ru"><meta charset="utf-8">'
                               '<title>Регистрация бота craftPulse</title><h1>craftpulse-mvp-bot</h1>'
                               '<p>Владелец: craftpulse-mvp. Права: Pull requests - write, Contents - read. Webhook выключен.</p>'
                               '<form method="post" action="' + escape(action, quote=True) + '">'
                               '<input type="hidden" name="manifest" value="' + escape(json.dumps(manifest), quote=True) + '">'
                               '<button type="submit">Создать GitHub App</button></form></html>')
                return
            if parsed.path != "/callback" or output.exists():
                self.send_page(404, "Not found")
                return
            query = parse_qs(parsed.query)
            if (len(query.get("state", [])) != 1 or not hmac.compare_digest(query["state"][0], state)
                    or len(query.get("code", [])) != 1):
                self.send_page(403, "Invalid registration state")
                return
            code = query["code"][0]
            if not code.isalnum() or len(code) > 256:
                self.send_page(400, "Invalid code")
                return
            try:
                request = Request("https://api.github.com/app-manifests/" + code + "/conversions", method="POST",
                                  headers={"Accept": "application/vnd.github+json", "User-Agent": "craftpulse-cp89",
                                           "X-GitHub-Api-Version": API_VERSION})
                with build_opener(NoRedirect()).open(request, timeout=30) as response:
                    config = json.load(response)
                # The conversion code is single-use: persist its response before validation.
                # Unexpected permissions/owner still prevent installation and must be reconciled.
                save_config(output, config)
                if (config["owner"]["id"] != ORGANIZATION_ID or not config.get("pem")
                        or config["permissions"] != {"metadata": "read", "pull_requests": "write", "contents": "read"}):
                    raise ValueError("Unexpected app owner/permissions")
                public = {k: config[k] for k in ("id", "slug", "html_url", "owner", "permissions", "client_id")}
                output.with_name("app-public.json").write_text(json.dumps(public, indent=2), encoding="utf-8")
                self.send_response(302)
                self.send_header("Location", "https://github.com/apps/" + config["slug"] + "/installations/new")
                self.send_header("Cache-Control", "no-store")
                self.send_header("Referrer-Policy", "no-referrer")
                self.end_headers()
            except (OSError, ValueError, KeyError):
                self.send_page(502, "Registration could not be completed; ask the operator to inspect local state")

    server = HTTPServer(("127.0.0.1", port), Handler)
    timer = threading.Timer(3600, server.shutdown)
    timer.daemon = True
    timer.start()
    url_file.write_text(origin + "/start/" + state, encoding="utf-8")
    print("Loopback registration helper ready; expires after one hour", flush=True)
    try:
        server.serve_forever()
    finally:
        timer.cancel()
        server.server_close()


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--url-file", type=Path, required=True)
    args = parser.parse_args()
    serve(args.output, args.url_file)
