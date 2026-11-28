"""HTTP and evidence helpers for the tracker acceptance scripts."""

from base64 import b64encode
import json
import os
import subprocess
from datetime import datetime, timezone
from http.client import HTTPException
from time import monotonic
from urllib.error import HTTPError
from urllib.parse import urlencode, urlsplit
from urllib.request import HTTPRedirectHandler, Request, build_opener


class CheckFailed(Exception):
    pass


class UnexpectedStatus(CheckFailed):
    def __init__(self, operation, status, retry_after):
        super().__init__(f"{operation}: unexpected HTTP status {status}")
        self.status = status
        self.retry_after = retry_after


def check(condition, message):
    if not condition:
        raise CheckFailed(message)


def now():
    return datetime.now(timezone.utc)


def timestamp(value):
    result = datetime.fromisoformat(value.replace("Z", "+00:00"))
    check(result.tzinfo is not None, "timestamp must include a timezone")
    return result.astimezone(timezone.utc)


def load_bundle(path):
    try:
        result = subprocess.run(
            ["sops", "decrypt", "--output-type", "json", str(path)],
            stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, check=True, timeout=30,
        )
        return json.loads(result.stdout)
    except (subprocess.SubprocessError, OSError, ValueError):
        raise CheckFailed("could not decrypt the SOPS bundle") from None


class NoRedirect(HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


class Evidence:
    def __init__(self, path, scenario):
        self.path = path
        self.started = now()
        self.requests = []
        self.secrets = set()
        self.results = {"scenario": scenario}

    def protected_bytes(self):
        return {variant.encode() for secret in self.secrets
                for variant in (secret, json.dumps(secret, ensure_ascii=False)[1:-1])}

    def guard(self, data):
        check(
            not any(secret in data for secret in self.protected_bytes()),
            "response or evidence contains a protected value",
        )

    def decode_json(self, raw):
        result = json.loads(raw)
        self.guard(json.dumps(result, ensure_ascii=False).encode())
        return result

    def save(self, error):
        report = {
            **self.results,
            "started_at": self.started.isoformat(),
            "finished_at": now().isoformat(),
            "outcome": "failed" if error else "passed",
            "problem": error,
            "requests": self.requests,
        }
        encoded = json.dumps(report, ensure_ascii=False, indent=2).encode() + b"\n"
        self.guard(encoded)
        fd = os.open(self.path, os.O_CREAT | os.O_EXCL | os.O_WRONLY, 0o600)
        with os.fdopen(fd, "wb") as output:
            output.write(encoded)


def run_report(parser, options, scenario, run):
    evidence = Evidence(options.report, scenario)
    error = None
    try:
        check(not options.report.exists(), "evidence path already exists")
        run(options, evidence)
    except CheckFailed as exc:
        error = str(exc)
    except (KeyError, ValueError, TypeError, AttributeError, OSError):
        error = "invalid SOPS configuration or response"
    try:
        evidence.save(error)
    except (OSError, CheckFailed):
        parser.exit(1, "could not write safe evidence\n")
    if error:
        parser.exit(1, error + "\n")


class Client:
    def __init__(self, origin, evidence, timeout=20, opener=None):
        origin = origin.rstrip("/")
        parts = urlsplit(origin)
        check(
            parts.scheme == "https" and parts.hostname and parts.username is None
            and parts.password is None and not parts.query and not parts.fragment,
            "expected an HTTPS base URL without credentials, query or fragment",
        )
        self.origin = origin
        self.evidence = evidence
        self.timeout = timeout
        self.opener = opener if opener is not None else build_opener(NoRedirect())

    def request(self, operation, method, path, expected, *, params=None, headers=None, body=None,
                response_headers=None, allow_html_413=False, include_status=False):
        query = "?" + urlencode(params) if params else ""
        req = Request(self.origin + path + query, method=method, data=body, headers={
            "Accept": "application/json", **(headers or {}),
        })
        entry = {"operation": operation, "method": method, "at": now().isoformat(), "status": None}
        self.evidence.requests.append(entry)
        started = monotonic()
        try:
            try:
                response = self.opener.open(req, timeout=self.timeout)
            except HTTPError as err:
                response = err
            with response:
                entry["status"] = response.code
                raw = response.read()
                self.evidence.guard(raw)
                self.evidence.guard(str(response.headers).encode())
                if response.code not in expected:
                    # Error responses can echo values too, including JSON escapes.
                    if raw:
                        try:
                            self.evidence.decode_json(raw)
                        except ValueError:
                            pass
                    raise UnexpectedStatus(operation, response.code, response.headers.get("Retry-After"))
                if response_headers is not None:
                    response_headers["ETag"] = response.headers.get("ETag")
                html_size_error = allow_html_413 and response.code == 413 and (
                    response.headers.get("Content-Type", "").split(";", 1)[0].strip().lower() == "text/html"
                )
                result = self.evidence.decode_json(raw) if raw and not html_size_error else None
                return (response.code, result) if include_status else result
        except (HTTPException, OSError, ValueError):
            raise CheckFailed(f"{operation}: transport error or invalid JSON") from None
        finally:
            entry["duration_ms"] = round((monotonic() - started) * 1000)


def read_snapshot(sql, evidence, **variables):
    args = ["psql", "-XqAtw", "-v", "ON_ERROR_STOP=1"]
    for key, value in variables.items():
        args.extend(("-v", f"{key}={value}"))
    args.extend(("-f", str(sql)))
    try:
        result = subprocess.run(args, check=True, timeout=15, stdout=subprocess.PIPE,
                                stderr=subprocess.DEVNULL)
    except (subprocess.SubprocessError, OSError):
        raise CheckFailed("could not read the core snapshot") from None
    evidence.guard(result.stdout)
    return evidence.decode_json(result.stdout)


def protect_jira_credentials(source, evidence):
    token = source["jira_token"]
    check(isinstance(token, str) and token and token.isascii() and token.isprintable(),
          "invalid Jira token in SOPS bundle")
    evidence.secrets.add(token)
    basic_value = b64encode(f'{source["jira_email"]}:{token}'.encode()).decode()
    evidence.secrets.add(basic_value)
    return "Basic " + basic_value


def product_session(source, evidence, sha):
    cookie = source["craftpulse_cookie"]
    check(isinstance(cookie, str) and cookie and cookie.isascii() and cookie.isprintable(),
          "invalid craftPulse session cookie in SOPS bundle")
    evidence.secrets.add(cookie)
    for pair in cookie.split(";"):
        name, sep, value = pair.strip().partition("=")
        check(name and sep and value, "expected semicolon-separated name=value cookies")
        evidence.secrets.add(value)
    if "jira_token" in source:
        protect_jira_credentials(source, evidence)
    client = Client(source["craftpulse_url"], evidence)
    version = client.request("version", "GET", "/version", [200])
    check(version["commit_sha"] == sha, "release SHA differs from the requested one")
    evidence.results["release_sha"] = sha
    return client, {"Cookie": cookie, "Origin": client.origin}
