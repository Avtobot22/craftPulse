#!/usr/bin/env python3
"""Verify Jenkins #90 TLS, plugin lock and read-only build API with positive controls."""

import argparse
import base64
import json
from pathlib import Path
import ssl
import time
from urllib.error import HTTPError, URLError
from urllib.parse import urlsplit
from urllib.request import HTTPRedirectHandler, HTTPSHandler, Request, build_opener

VERSION = '2.580.2'
JOBS = ('cp90-org-a', 'cp90-org-b')


class NoRedirect(HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


class API:
    def __init__(self, url, user, token):
        parsed = urlsplit(url)
        if (parsed.scheme != 'https' or not parsed.hostname or parsed.username
                or parsed.password or parsed.path not in ('', '/')
                or parsed.query or parsed.fragment):
            raise ValueError('Expected an HTTPS origin without credentials')
        self.url = url.rstrip('/')
        self.authorization = 'Basic ' + base64.b64encode(f'{user}:{token}'.encode()).decode()
        self.opener = build_opener(NoRedirect(), HTTPSHandler(context=ssl.create_default_context()))

    def request(self, method, path, body=None, expected=(200,), content_type='application/xml'):
        req = Request(self.url + path, body, method=method, headers={
            'Authorization': self.authorization, 'Content-Type': content_type})
        try:
            with self.opener.open(req, timeout=45) as response:
                code, raw, headers = response.status, response.read(), response.headers
        except HTTPError as error:
            code, raw, headers = error.code, error.read(), error.headers
        except URLError:
            raise RuntimeError('Jenkins connection or TLS verification failed') from None
        if code not in expected:
            raise RuntimeError(f'Jenkins {method} {path.split("?")[0]}: HTTP {code}')
        return code, raw, headers

    def get(self, path):
        _, raw, _ = self.request('GET', path)
        return json.loads(raw)


def acceptance(url, credentials_path, lock_path):
    credentials = json.loads(Path(credentials_path).read_text())
    admin = API(url, 'cp90-admin', credentials['cp90-admin']['token'])
    reader = API(url, 'cp90-reader', credentials['cp90-reader']['token'])
    _, _, headers = reader.request('GET', '/api/json')
    if headers.get('X-Jenkins') != VERSION:
        raise RuntimeError('Unexpected Jenkins version')
    actual = {p['shortName']: p['version'] for p in
              admin.get('/pluginManager/api/json?depth=1')['plugins'] if p['active']}
    locked = dict(line.strip().split(':', 1) for line in Path(lock_path).read_text().splitlines()
                  if line.strip() and not line.startswith('#'))
    if actual != locked:
        raise RuntimeError('Installed plugins differ from plugins.txt')
    visible = {job['name'] for job in reader.get('/api/json?tree=jobs[name]')['jobs']}
    if visible != set(JOBS):
        raise RuntimeError('Read API exposes jobs outside the two test organizations')
    checks = []
    for job in JOBS:
        path = '/job/' + job
        current = admin.get(path + '/api/json?tree=lastBuild[number],nextBuildNumber')
        # Only the operator token creates one infrastructure smoke build on an empty job.
        if current['lastBuild'] is None:
            admin.request('POST', path + '/build', b'', expected=(201,))
        for _ in range(60):
            status = admin.get(path + '/api/json?tree=lastBuild[number,building,result],nextBuildNumber')
            if status['lastBuild'] and not status['lastBuild']['building']:
                break
            time.sleep(2)
        else:
            raise RuntimeError('Infrastructure smoke build did not finish')
        if status['lastBuild']['result'] != 'SUCCESS':
            raise RuntimeError('Infrastructure smoke build failed')
        number = status['lastBuild']['number']
        reader.get(path + f'/{number}/api/json')
        _, config, _ = admin.request('GET', path + '/config.xml')
        # A valid config succeeds for the operator, so a reader rejection is an ACL check.
        admin.request('POST', path + '/config.xml', config)
        build_code, _, _ = reader.request('POST', path + '/build', b'', expected=(403,))
        config_code, _, _ = reader.request('POST', path + '/config.xml', config, expected=(403,))
        after = admin.get(path + '/api/json?tree=nextBuildNumber')
        if after['nextBuildNumber'] != status['nextBuildNumber']:
            raise RuntimeError('Reader unexpectedly queued a build')
        checks.append({'job': job, 'build': number, 'read_status': 200,
                       'build_status': build_code, 'configure_status': config_code})
    admin.get('/job/cp90-control/api/json')
    code, _, _ = reader.request('GET', '/job/cp90-control/api/json', expected=(403, 404))
    return {'version': VERSION, 'plugins': dict(sorted(actual.items())), 'jobs': checks,
            'control_reader_status': code, 'control_admin_status': 200, 'tls_verified': True}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--url', default='https://jenkins.defects.team')
    parser.add_argument('--credentials', type=Path,
                        default=Path('/etc/craftpulse-ci90/jenkins/credentials.json'))
    parser.add_argument('--plugins', type=Path, default=Path(__file__).with_name('plugins.txt'))
    parser.add_argument('--output', type=Path)
    args = parser.parse_args()
    content = json.dumps(acceptance(args.url, args.credentials, args.plugins), indent=2) + '\n'
    if args.output:
        args.output.write_text(content, encoding='utf-8')
    else:
        print(content, end='')


if __name__ == '__main__':
    main()
