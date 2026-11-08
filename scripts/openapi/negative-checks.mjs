import assert from 'node:assert/strict';
import {mkdtempSync, writeFileSync, rmSync, copyFileSync} from 'node:fs';
import {tmpdir} from 'node:os';
import {join} from 'node:path';
import {fileURLToPath} from 'node:url';
import {spawnSync} from 'node:child_process';
import YAML from 'yaml';
import {contract, root} from './contract.mjs';

const temp = mkdtempSync(join(tmpdir(), 'craftpulse-openapi-negative-'));
const bin = name => fileURLToPath(new URL(`node_modules/.bin/${name}`, import.meta.url));
try {
  copyFileSync(new URL('docs/technical-specification/.redocly.lint-ignore.yaml', root), join(temp, '.redocly.lint-ignore.yaml'));
  const cases = [
    ['missing-action', 'spectral', doc => delete doc.paths['/me'].get['x-craftpulse-action'], 'craftpulse-action-present'],
    ['unsupported-union', 'spectral', doc => {doc.components.schemas.Problem.properties.title.type = ['string', 'number'];}, 'craftpulse-type-union'],
    ['invalid-example', 'redocly', doc => {doc.paths['/me'].get.responses['200'].content['application/json'].example.id = 123;}, 'no-invalid-media-type-examples'],
  ];
  for (const [name, tool, mutate, rule] of cases) {
    const document = structuredClone(contract);
    mutate(document);
    const fixture = join(temp, 'openapi.yaml');
    writeFileSync(fixture, YAML.stringify(document, {lineWidth: 0}));
    const args = tool === 'spectral'
      ? ['lint', fixture, '--ruleset', fileURLToPath(new URL('docs/technical-specification/openapi.spectral.yaml', root)), '--fail-severity=warn']
      : ['lint', fixture, '--config', fileURLToPath(new URL('docs/technical-specification/redocly.yaml', root))];
    const result = spawnSync(bin(tool), args, {encoding: 'utf8'});
    const output = result.stdout + result.stderr;
    assert.equal(result.status, 1, `${name}: linter must reject the fixture. ${output}`);
    assert(output.includes(rule), `${name}: expected rule ${rule}, got ${output}`);
    console.log(`Rejected ${name}: ${rule}.`);
  }
} finally {
  rmSync(temp, {recursive: true, force: true});
}
