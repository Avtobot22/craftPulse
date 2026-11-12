import assert from 'node:assert/strict';
import {mkdtempSync, writeFileSync, rmSync, copyFileSync, readFileSync} from 'node:fs';
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
  copyFileSync(new URL('docs/technical-specification/redocly.yaml', root), join(temp, 'redocly.yaml'));
  const load = name => YAML.parse(readFileSync(new URL(`docs/technical-specification/${name}`, root), 'utf8'));
  const sources = {'openapi.yaml': contract, 'adapter.yaml': load('adapter.yaml'), 'envelope.yaml': load('envelope.yaml')};
  const rulesets = {'openapi.yaml': 'openapi.spectral.yaml', 'adapter.yaml': 'adapter.spectral.yaml', 'envelope.yaml': 'adapter.spectral.yaml'};
  const cases = [
    ['missing-action', 'spectral', 'openapi.yaml', doc => delete doc.paths['/me'].get['x-craftpulse-action'], 'craftpulse-action-present'],
    ['unsupported-union', 'spectral', 'openapi.yaml', doc => {doc.components.schemas.Problem.properties.title.type = ['string', 'number'];}, 'craftpulse-type-union'],
    ['unbound-probe-example', 'redocly', 'openapi.yaml', doc => {delete doc.paths['/connections/{connection_id}/probes'].post.responses['202'].content['application/json'].example.configuration_version;}, 'no-invalid-media-type-examples'],
    ['indexed-redaction', 'redocly', 'openapi.yaml', doc => {doc.paths['/merge-requests/{merge_request_id}/summary'].get.responses['200'].content['application/json'].example.redactions = ['items/3'];}, 'no-invalid-media-type-examples'],
    ['invalid-example', 'redocly', 'openapi.yaml', doc => {doc.paths['/me'].get.responses['200'].content['application/json'].example.id = 123;}, 'no-invalid-media-type-examples'],
    ['adapter-operation-union', 'spectral', 'adapter.yaml', doc => {doc.components.schemas.FetchInput.properties.limit.type = ['integer', 'string'];}, 'adapter-type-union'],
    ['adapter-missing-example', 'spectral', 'adapter.yaml', doc => {delete doc.paths['/connections/{connection_id}/probe'].post.responses['200'].content['application/json'].example;}, 'adapter-json-example'],
    ['adapter-invalid-example', 'redocly', 'adapter.yaml', doc => {doc.paths['/connections/{connection_id}/probe'].post.responses['200'].content['application/json'].example.contract_versions = '1';}, 'no-invalid-media-type-examples'],
    ['adapter-operation-id-case', 'spectral', 'adapter.yaml', doc => {doc.paths['/connections/{connection_id}/fetch'].post.operationId = 'FetchPage';}, 'adapter-operation-id-camel'],
    ['envelope-invalid-record-example', 'redocly', 'envelope.yaml', doc => {doc.components.schemas.TaskEventV1.example.event_kind = 'renamed';}, 'no-invalid-schema-examples'],
    ['envelope-undeclared-record', 'spectral', 'envelope.yaml', doc => {delete doc.components.schemas.TaskV1['x-craftpulse-record'];}, 'adapter-record-declared'],
  ];
  for (const [name, tool, file, mutate, rule] of cases) {
    const document = structuredClone(sources[file]);
    mutate(document);
    const fixture = join(temp, file);
    writeFileSync(fixture, YAML.stringify(document, {lineWidth: 0}));
    const args = tool === 'spectral'
      ? ['lint', fixture, '--ruleset', fileURLToPath(new URL(`docs/technical-specification/${rulesets[file]}`, root)), '--fail-severity=warn']
      : ['lint', fixture, '--config', join(temp, 'redocly.yaml')];
    const result = spawnSync(bin(tool), args, {encoding: 'utf8'});
    const output = result.stdout + result.stderr;
    assert.equal(result.status, 1, `${name}: linter must reject the fixture. ${output}`);
    assert(output.includes(rule), `${name}: expected rule ${rule}, got ${output}`);
    console.log(`Rejected ${name}: ${rule}.`);
    rmSync(fixture);
  }
} finally {
  rmSync(temp, {recursive: true, force: true});
}
