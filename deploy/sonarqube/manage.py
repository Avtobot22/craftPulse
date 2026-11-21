#!/usr/bin/env python3
"""Provision and check the isolated #90 SonarQube source; never print credentials."""

import argparse
import base64
import json
import os
from pathlib import Path
import secrets
import ssl
from urllib.error import HTTPError, URLError
from urllib.parse import urlencode, urlsplit
from urllib.request import HTTPRedirectHandler, HTTPSHandler, Request, build_opener

VERSION = '26.11.0.134175'
PROJECTS = ('cp90-org-a', 'cp90-org-b')
CONTROL = 'cp90-control'
READER = 'cp90-reader'


class NoRedirect(HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


class API:
    def __init__(self, url, user=None, password=None, token=None):
        parsed = urlsplit(url)
        if (parsed.scheme != 'https' or not parsed.hostname or parsed.username
                or parsed.password or parsed.path not in ('', '/')
                or parsed.query or parsed.fragment):
            raise ValueError('Expected an HTTPS origin without credentials')
        self.url = url.rstrip('/')
        self.authorization = ('Bearer ' + token if token else 'Basic ' +
                              base64.b64encode(f'{user}:{password}'.encode()).decode())
        self.opener = build_opener(NoRedirect(), HTTPSHandler(context=ssl.create_default_context()))

    def request(self, method, path, data=None, expected=(200, 204)):
        body = None if data is None else urlencode(data).encode()
        req = Request(self.url + path, body, method=method, headers={
            'Authorization': self.authorization,
            'Content-Type': 'application/x-www-form-urlencoded',
        })
        try:
            with self.opener.open(req, timeout=45) as response:
                code, raw = response.status, response.read()
        except HTTPError as error:
            code, raw = error.code, error.read()
        except URLError:
            raise RuntimeError('SonarQube connection or TLS verification failed') from None
        if code not in expected:
            raise RuntimeError(f'SonarQube {method} {path.split("?")[0]}: HTTP {code}')
        try:
            value = json.loads(raw) if raw else None
        except ValueError:
            value = raw.decode()
        return code, value


def private_json(path, value):
    path = Path(path).resolve()
    if path.is_relative_to(Path(__file__).resolve().parents[2]):
        raise ValueError('Credentials must be stored outside the source checkout')
    if os.name != 'nt' and path.parent.stat().st_mode & 0o077:
        raise ValueError('Secret directory must have mode 0700')
    fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
    with os.fdopen(fd, 'w', encoding='utf-8') as stream:
        json.dump(value, stream)
        stream.write('\n')


def bootstrap(url, secret_dir):
    root = Path(secret_dir)
    state_path = root / 'sonar-credentials.json'
    state = json.loads(state_path.read_text()) if state_path.exists() else {'url': url}
    if state.get('url', url) != url:
        raise ValueError('Provisioning state belongs to another origin')
    state['url'] = url
    password = (root / 'sonar-admin-password').read_text().strip()
    admin = API(url, 'admin', password)
    _, auth = admin.request('GET', '/api/authentication/validate')
    initial = None if auth['valid'] else API(url, 'admin', 'admin')
    _, version = (initial or admin).request('GET', '/api/server/version')
    if version != VERSION:
        raise RuntimeError('Unexpected SonarQube version; provisioning stopped')
    # Crash recovery after changing the default password must use the saved password.
    if initial is not None:
        initial.request('POST', '/api/users/change_password', {
            'login': 'admin', 'previousPassword': 'admin', 'password': password})
    admin.request('POST', '/api/settings/set', {'key': 'sonar.forceAuthentication', 'value': 'true'})
    admin.request('POST', '/api/settings/set', {'key': 'sonar.core.serverBaseURL', 'value': url})
    admin.request('POST', '/api/settings/set', {'key': 'sonar.defaultProjectVisibility', 'value': 'private'})
    for permission in ('admin', 'gateadmin', 'profileadmin', 'scan', 'provisioning'):
        admin.request('POST', '/api/permissions/remove_group', {
            'groupName': 'sonar-users', 'permission': permission})
    _, users = admin.request('GET', '/api/users/search?' + urlencode({'q': READER}))
    if not any(user['login'] == READER for user in users['users']):
        admin.request('POST', '/api/users/create', {
            'login': READER, 'name': 'craftPulse read-only collector',
            'password': (root / 'sonar-reader-password').read_text().strip(), 'local': 'true'})
    for project in (*PROJECTS, CONTROL):
        _, existing = admin.request('GET', '/api/projects/search?' + urlencode({'projects': project}))
        if not any(item['key'] == project for item in existing['components']):
            admin.request('POST', '/api/projects/create', {
                'project': project, 'name': project, 'visibility': 'private'})
        admin.request('POST', '/api/projects/update_visibility', {
            'project': project, 'visibility': 'private'})
        for group in ('sonar-users',):
            for permission in ('user', 'codeviewer', 'scan', 'admin', 'issueadmin', 'securityhotspotadmin'):
                admin.request('POST', '/api/permissions/remove_group', {
                    'groupName': group, 'projectKey': project, 'permission': permission})
        for permission in ('user', 'codeviewer', 'scan', 'admin', 'issueadmin', 'securityhotspotadmin'):
            admin.request('POST', '/api/permissions/remove_user', {
                'login': READER, 'projectKey': project, 'permission': permission})
        for permission in ('admin', 'user', 'codeviewer'):
            admin.request('POST', '/api/permissions/add_user', {
                'login': 'admin', 'projectKey': project, 'permission': permission})
        if project in PROJECTS:
            for permission in ('user', 'codeviewer'):
                admin.request('POST', '/api/permissions/add_user', {
                    'login': READER, 'projectKey': project, 'permission': permission})
    if 'reader_token' not in state:
        token_name = 'craftpulse90-' + secrets.token_hex(4)
        _, result = admin.request('POST', '/api/user_tokens/generate', {
            'login': READER, 'name': token_name, 'type': 'USER_TOKEN'})
        state.update(reader_token=result['token'], token_name=token_name)
    private_json(state_path, state)
    print('SonarQube projects and read-only token provisioned; credentials omitted')


def acceptance(url, secret_dir):
    root = Path(secret_dir)
    state = json.loads((root / 'sonar-credentials.json').read_text())
    reader = API(url, token=state['reader_token'])
    admin = API(url, 'admin', (root / 'sonar-admin-password').read_text().strip())
    _, version = reader.request('GET', '/api/server/version')
    if version != VERSION:
        raise RuntimeError('Unexpected SonarQube version')
    checks = []
    for project in (*PROJECTS, CONTROL):
        query = '?' + urlencode({'component': project})
        admin.request('GET', '/api/components/show' + query)
        code, _ = reader.request('GET', '/api/components/show' + query,
                                 expected=(200,) if project in PROJECTS else (403, 404))
        checks.append({'project': project, 'reader_status': code, 'admin_status': 200})
    _, visible = reader.request('GET', '/api/components/search?qualifiers=TRK&ps=500')
    keys = {project['key'] for project in visible['components']}
    if keys != set(PROJECTS):
        raise RuntimeError('Read token project listing violates isolation')
    code, _ = reader.request('POST', '/api/projects/create', {
        'project': 'cp90-reader-must-not-create', 'name': 'must-not-create', 'visibility': 'private'},
        expected=(403,))
    return {'version': version, 'edition': 'Community Build', 'projects': checks,
            'visible_projects': sorted(keys), 'project_creation_status': code, 'tls_verified': True}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('command', choices=('bootstrap', 'acceptance'))
    parser.add_argument('--url', default='https://sonar.defects.team')
    parser.add_argument('--secret-dir', type=Path, default=Path('/etc/craftpulse-ci90'))
    parser.add_argument('--output', type=Path)
    args = parser.parse_args()
    if args.command == 'bootstrap':
        bootstrap(args.url, args.secret_dir)
    else:
        result = acceptance(args.url, args.secret_dir)
        content = json.dumps(result, indent=2) + '\n'
        if args.output:
            args.output.write_text(content, encoding='utf-8')
        else:
            print(content, end='')


if __name__ == '__main__':
    main()
