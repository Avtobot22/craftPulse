// Сверяет adapter.yaml и envelope.yaml с таблицами 10.3 и 12.4 ТЗ и с универсальным импортом openapi.yaml.
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import YAML from 'yaml';
import {contract, root, srs} from './contract.mjs';

const load = name => YAML.parse(readFileSync(new URL(`docs/technical-specification/${name}`, root), 'utf8'));
const adapter = load('adapter.yaml');
const envelope = load('envelope.yaml');

function section(title, next) {
  const start = srs.indexOf(title);
  assert(start >= 0, `SRS section ${title} is missing`);
  return srs.slice(start, srs.indexOf(next, start));
}
const firstCells = text => [...text.matchAll(/^\| ([^|]+?) \|/gm)].map(match => match[1].trim())
  .filter(cell => !/^(-+|Операция|Режим|Источник и вид объекта|Случай)$/.test(cell));

assert.equal(adapter.openapi, '3.1.2');
assert.equal(adapter.info.version, '1');
const operations = Object.values(adapter.paths).flatMap(item => Object.values(item)).filter(op => op?.operationId);
const ids = new Set(operations.map(op => op.operationId));
assert.equal(ids.size, operations.length, 'operationId must be unique');
const srsOperations = firstCells(section('#### 10.3.', '`CheckAccess` работает')).map(cell => cell.replaceAll('`', ''));
assert(srsOperations.length >= 10, 'table 10.3 is not parsed');
for (const name of srsOperations) {
  const id = name[0].toLowerCase() + name.slice(1);
  assert(ids.has(id), `table 10.3 operation ${name} has no ${id} in adapter.yaml`);
}
assert.equal(ids.size, srsOperations.length, 'adapter.yaml has operations outside table 10.3');
for (const op of operations) {
  const names = (op.parameters ?? []).map(p => p.$ref?.split('/').pop());
  for (const required of ['ConnectionId', 'Organization', 'ContractVersion', 'CorrelationId']) {
    assert(names.includes(required), `${op.operationId}: missing ${required}`);
  }
}
const errorCodes = adapter.components.schemas.AdapterError.properties.code.enum;
for (const code of ['rate_limited', 'preparing', 'unavailable', 'timeout', 'auth', 'not_found', 'invalid_response']) {
  assert(errorCodes.includes(code), `AdapterError lacks ${code}`);
}
for (const name of ['RateLimited', 'Unavailable']) {
  assert(adapter.components.responses[name].headers?.['Retry-After'], `${name} lacks Retry-After`);
}

const identityRows = new Set(adapter['x-craftpulse-identity'].map(rule => rule.srs_row));
const srsIdentity = firstCells(section('#### 12.4.', '**Один и тот же')).concat('Учётная запись источника');
for (const row of srsIdentity) assert(identityRows.has(row), `table 12.4 row «${row}» has no identity rule`);
assert.equal(identityRows.size, new Set(srsIdentity).size, 'identity rules outside table 12.4');

const schemas = envelope.components.schemas;
const records = Object.entries(schemas).filter(([, schema]) => schema['x-craftpulse-record']);
const byKind = new Map(records.map(([name, schema]) => [schema['x-craftpulse-record'].kind, name]));
assert.equal(byKind.size, records.length, 'record kinds must be unique');
assert.deepEqual([...byKind.keys()].sort(), [...schemas.Record.properties.record_kind.enum].sort(), 'Record.record_kind differs from record schemas');
const packageSchemas = envelope['x-craftpulse-schemas'];
assert.deepEqual(Object.keys(packageSchemas).sort(), [...schemas.Manifest.properties.schema_id.enum].sort(), 'Manifest.schema_id differs from x-craftpulse-schemas');
for (const [id, spec] of Object.entries(packageSchemas)) {
  for (const kind of spec.kinds) assert(byKind.has(kind), `${id}: unknown record kind ${kind}`);
}
for (const input of ['FetchInput', 'ManifestInput']) {
  assert.deepEqual(adapter.components.schemas[input].properties.schema_id.enum, schemas.Manifest.properties.schema_id.enum, `${input}.schema_id differs from the envelope`);
}
const imports = contract.components.schemas.ImportUpload.properties.schema_id.enum;
for (const id of imports) assert(packageSchemas[id], `ImportUpload.schema_id ${id} has no envelope schema`);
const csvColumns = Object.keys(schemas.TaskCsvRow.properties).filter(column => column !== 'native_id');
for (const column of csvColumns) assert(schemas.TaskV1.properties[column], `TaskCsvRow.${column} is not a TaskV1 field`);
console.log(`Adapter contract: ${ids.size} operations, ${identityRows.size} identity rows, ${byKind.size} record schemas.`);
