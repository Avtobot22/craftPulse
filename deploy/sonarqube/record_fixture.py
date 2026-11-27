#!/usr/bin/env python3
"""Record the current fixture analysis directly from SonarQube Web API."""

import argparse
import json
import os
from pathlib import Path, PurePosixPath
import re
import tempfile
from urllib.parse import urlencode

from manage import API

PAGE_SIZE = 100
ISSUE_WINDOW = 10000


def get(api, endpoint, **query):
    return api.request('GET', endpoint + '?' + urlencode(query))[1]


def latest(api, project):
    result = get(api, '/api/project_analyses/search', project=project, p=1, ps=1)
    if len(result['analyses']) != 1:
        raise RuntimeError('The fixture project has no current analysis')
    row = result['analyses'][0]
    return dict(key=row['key'], revision=row.get('revision'), date=row['date'])


def inventory(api, project):
    output = {}
    total = None
    page = 1
    while True:
        result = get(api, '/api/issues/search', components=project, p=page, ps=PAGE_SIZE)
        paging = result['paging']
        if (paging['pageIndex'] != page or paging['pageSize'] != PAGE_SIZE
                or total is not None and total != paging['total']):
            raise RuntimeError('Issue inventory changed while being read')
        total = paging['total']
        if not 0 <= total < ISSUE_WINDOW:
            raise RuntimeError('Issue inventory cannot be verified within the search window')
        rows = result['issues']
        if len(rows) != min(PAGE_SIZE, total - (page - 1) * PAGE_SIZE):
            raise RuntimeError('Expected a full SonarQube issues page')
        paths = {entry['key']: entry.get('path') for entry in result['components']}
        for row in rows:
            if row['project'] != project or row['key'] in output:
                raise RuntimeError('Issue inventory contains another project or a duplicate')
            file = paths.get(row['component'])
            if row['component'] != project and (
                    not file or PurePosixPath(file).is_absolute()
                    or '..' in PurePosixPath(file).parts):
                raise RuntimeError('Issue has no project-relative location')
            span = row.get('textRange') or {}
            output[row['key']] = {
                'key': row['key'], 'rule': row['rule'], 'path': file,
                'line_start': span.get('startLine', row.get('line')),
                'line_end': span.get('endLine', row.get('line')),
                'type': row.get('type'), 'severity': row.get('severity'),
                'status': row.get('status') or row.get('issueStatus'),
                'resolution': row.get('resolution'),
            }
        if len(output) == total:
            return [output[key] for key in sorted(output)]
        page += 1


def record(api, project, revision):
    analysis = latest(api, project)
    if analysis['revision'] != revision:
        raise RuntimeError('Latest analysis revision does not match the fixture checkout')
    issues = inventory(api, project)
    if (latest(api, project) != analysis or inventory(api, project) != issues
            or latest(api, project) != analysis):
        raise RuntimeError('SonarQube analysis or issue inventory changed during capture')
    return dict(analysis, issues=issues)


def save(path, api, project, revision):
    status = get(api, '/api/system/status')
    if status['status'] != 'UP':
        raise RuntimeError('SonarQube is not ready')
    origin = dict(url=api.url, project=project, server_id=status.get('id'),
                  server_version=status['version'])
    evidence = dict(origin, analyses=[])
    if path.exists():
        evidence = json.loads(path.read_text(encoding='utf-8'))
        if any(evidence[key] != expected for key, expected in origin.items()):
            raise RuntimeError('Existing inventory belongs to another SonarQube source')
    analysis = record(api, project, revision)
    entries = evidence['analyses']
    if analysis in entries:
        return
    if len(entries) >= 2 or any(
            row['key'] == analysis['key'] or row['revision'] == revision for row in entries):
        raise RuntimeError('Expected two distinct fixture revisions and analysis keys')
    entries.append(analysis)
    fd, temporary = tempfile.mkstemp(dir=path.parent)
    try:
        with os.fdopen(fd, 'w', encoding='utf-8') as stream:
            json.dump(evidence, stream, indent=2, ensure_ascii=False)
            stream.write('\n')
            stream.flush()
            os.fsync(stream.fileno())
        os.replace(temporary, path)
    finally:
        Path(temporary).unlink(missing_ok=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--url', required=True)
    parser.add_argument('--project', required=True)
    parser.add_argument('--revision', required=True)
    parser.add_argument('--out', type=Path, required=True)
    args = parser.parse_args()
    if not re.fullmatch(r'[a-f0-9]{40}|[a-f0-9]{64}', args.revision):
        parser.error('--revision must be a full lowercase Git SHA')
    token = os.environ.get('SONAR_READ_TOKEN', '').strip()
    if not token:
        parser.error('SONAR_READ_TOKEN is required')
    try:
        save(args.out, API(args.url, token=token), args.project, args.revision)
    except (RuntimeError, ValueError, KeyError, OSError) as error:
        parser.exit(1, f'SonarQube fixture capture failed ({type(error).__name__}).\n')


if __name__ == '__main__':
    main()
