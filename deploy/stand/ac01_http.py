"""HTTPS and GitLab form handling for the AC-01 stand tool."""

from html.parser import HTMLParser
from http.client import HTTPException
from http.cookiejar import CookieJar
import json
import re
import ssl
from urllib.error import HTTPError
from urllib.parse import parse_qs, urlencode, urljoin, urlsplit
from urllib.request import (
    HTTPCookieProcessor, HTTPRedirectHandler, HTTPSHandler, Request, build_opener,
)


class CheckFailed(RuntimeError):
    """An assertion with a message safe for the operator and the report."""


def require(condition, message):
    if not condition:
        raise CheckFailed(message)


def origin(url):
    parsed = urlsplit(url)
    require(
        parsed.scheme == "https" and parsed.hostname and parsed.username is None
        and parsed.path in ("", "/") and not (parsed.query or parsed.fragment),
        "Expected an HTTPS origin without credentials, path or query",
    )
    return f"https://{parsed.netloc}"


def parse_object(raw):
    data = json.loads(raw)
    require(isinstance(data, dict), "Expected a JSON object")
    return data


class NoRedirect(HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


class Forms(HTMLParser):
    def __init__(self, raw):
        super().__init__()
        self.forms = []
        self.current = None
        self.feed(raw.decode("utf-8"))

    def handle_starttag(self, tag, attrs):
        attrs = dict(attrs)
        if tag == "form":
            self.current = attrs, {}
            self.forms.append(self.current)
        if tag == "input" and self.current is not None and attrs.get("name") and "disabled" not in attrs:
            self.current[1][attrs["name"]] = attrs

    def handle_endtag(self, tag):
        if tag == "form":
            self.current = None


class Browser:
    def __init__(self, tls, evidence):
        self.jar = CookieJar()
        self.evidence = evidence
        self.opener = build_opener(NoRedirect(), HTTPSHandler(context=tls), HTTPCookieProcessor(self.jar))

    def request(self, step, method, url, *, headers=None, body=None):
        request = Request(url, data=body, headers=headers or {}, method=method)
        try:
            try:
                response = self.opener.open(request, timeout=30)
            except HTTPError as exc:
                response = exc
            with response:
                record = self.evidence.record(step, method, response.code)
                for cookie in self.jar:
                    self.evidence.protect(cookie.value)
                raw = response.read(4 * 1024 * 1024 + 1)
                require(len(raw) <= 4 * 1024 * 1024, f"{step}: response too large")
                return record, response.headers, raw
        except (OSError, HTTPException):
            raise CheckFailed(f"{step}: HTTPS request failed") from None

    def session(self):
        values = [item.value for item in self.jar if item.name == "__Host-cp_session"]
        require(len(values) <= 1, "Ambiguous craftPulse session")
        return values[0] if values else None


def tls_context(ca):
    return ssl.create_default_context(cafile=ca)


class GitLabLogin:
    def __init__(self, site, issuer, clients, passwords, tls, evidence):
        self.site, self.issuer = origin(site), origin(issuer)
        require(self.site != self.issuer, "craftPulse and GitLab must have different origins")
        self.clients, self.passwords = clients, passwords
        self.tls, self.evidence = tls, evidence

    def target(self, location, base):
        require(location and not re.search(r"[\\\s\x00-\x1f\x7f]", location), "Invalid OIDC redirect")
        url = urljoin(base, location)
        parsed = urlsplit(url)
        require(parsed.username is None and not parsed.fragment, "Invalid OIDC redirect")
        for key in ("code", "state", "nonce", "code_challenge", "authenticity_token", "access_token", "id_token"):
            for value in parse_qs(parsed.query).get(key, []):
                self.evidence.protect(value)
        return url

    def run(self, slug, username, *, denied=False):
        site = Browser(self.tls, self.evidence)
        provider = Browser(self.tls, self.evidence)
        login = f"{self.site}/o/{slug}/login?return_to=%2F"
        callback = f"{self.site}/o/{slug}/callback"
        record, headers, _ = site.request("startLogin", "GET", login)
        require(record["status"] == 302, "startLogin: expected HTTP 302")
        url = self.target(headers.get("Location"), login)
        query = parse_qs(urlsplit(url).query)
        require(
            url.split("?", 1)[0] == self.issuer + "/oauth/authorize"
            and query.get("client_id") == [self.clients[slug]]
            and query.get("redirect_uri") == [callback]
            and query.get("response_type") == ["code"] and query.get("code_challenge_method") == ["S256"]
            and set(query.get("scope", [""])[0].split()) == {"openid", "profile"}
            and "state" in query and "nonce" in query and "code_challenge" in query,
            "startLogin: unexpected OIDC application, callback or authorization parameters",
        )
        record["application_matches"] = True
        url = self.authorize(provider, url, callback, query["state"], username)
        record, headers, _ = site.request("completeLogin", "GET", url)
        expected = f"{self.site}/o/{slug}?login_error=organization_not_configured" if denied else self.site + "/"
        require(record["status"] == 302 and self.target(headers.get("Location"), url) == expected,
                "completeLogin: unexpected login result")
        require(bool(site.session()) != denied, "completeLogin: unexpected session result")
        record["session_issued"] = not denied
        if denied:
            record["code"] = "organization_not_configured"
        return site

    def authorize(self, provider, url, callback, state, username):
        method, body = "GET", None
        signed_in, consent = False, False
        for _ in range(12):
            stem = url.split("?", 1)[0]
            require(stem in (self.issuer + "/oauth/authorize", self.issuer + "/users/sign_in"),
                    "GitLab: unexpected redirect outside sign-in or consent")
            headers = {"Content-Type": "application/x-www-form-urlencoded", "Origin": self.issuer} if body else {}
            step = "gitlabSignIn" if stem.endswith("/users/sign_in") else "gitlabAuthorize"
            record, response_headers, raw = provider.request(step, method, url, headers=headers, body=body)
            if record["status"] in (301, 302, 303):
                url = self.target(response_headers.get("Location"), url)
                if url.split("?", 1)[0] == callback:
                    query = parse_qs(urlsplit(url).query)
                    require(signed_in and query.get("state") == state and len(query.get("code", [])) == 1,
                            "GitLab: callback lacks a matching authorization code")
                    return url
                method, body = "GET", None
                continue
            require(record["status"] == 200, f"{step}: unexpected HTTP status")
            if stem.endswith("/users/sign_in"):
                require(not signed_in, "GitLab: sign-in did not complete")
                signed_in = True
                url, body = self.form(raw, url, stem, {"user[login]": username, "user[password]": self.passwords[username]})
            else:
                require(signed_in and not consent, "GitLab: consent did not complete")
                consent = True
                url, body = self.form(raw, url, stem, {})
            method = "POST"
        raise CheckFailed("GitLab: too many redirects")

    def form(self, raw, url, endpoint, fields):
        for form, inputs in Forms(raw).forms:
            if (form.get("method") or "").lower() != "post" or not form.get("action") or not fields.keys() <= inputs.keys():
                continue
            action = self.target(form["action"], url)
            hidden = {name: attrs.get("value", "") for name, attrs in inputs.items() if attrs.get("type") == "hidden"}
            if action != endpoint or hidden.get("_method") not in (None, "post", "POST") or not hidden.get("authenticity_token"):
                continue
            self.evidence.protect(hidden["authenticity_token"])
            return action, urlencode({**hidden, **fields}).encode()
        raise CheckFailed("GitLab: expected sign-in or consent form was not found")
