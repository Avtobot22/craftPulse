import assert from 'node:assert/strict';
import {contract, operations, dereference, srs} from './contract.mjs';

assert.equal(contract.openapi, '3.1.2');
const byId = Object.fromEntries(operations.map(operation => [operation.operationId, operation]));
assert.equal(Object.keys(byId).length, operations.length, 'operationId must be unique');
const trace = new Set(operations.flatMap(operation => operation['x-craftpulse-trace'] ?? []));
for (const [prefix, count, width, excluded] of [['UC', 48, 2, []], ['FR', 64, 3, ['FR-011']]]) {
  for (let i = 1; i <= count; i++) {
    const id = `${prefix}-${String(i).padStart(width, '0')}`;
    if (!excluded.includes(id)) assert(trace.has(id), `${id} has no operation`);
  }
}
// FR-011: installation runs outside HTTP; getLatestInstallationProtocol exposes its result.
assert(srs.includes('FR-011') && byId.getLatestInstallationProtocol);
const apiKeyActions = new Set(['org.read', 'report.read', 'export.create']);
for (const operation of operations) {
  assert(operation['x-craftpulse-action'], `${operation.operationId}: missing action`);
  assert(['M0', 'M1', 'M2'].includes(operation['x-craftpulse-delivery']), `${operation.operationId}: missing delivery`);
  assert(operation['x-craftpulse-trace']?.length, `${operation.operationId}: missing trace`);
  const security = operation.security ?? contract.security;
  if (security.some(item => 'apiKey' in item)) {
    assert(apiKeyActions.has(operation['x-craftpulse-action']), `${operation.operationId}: key exceeds BI permissions`);
  }
  if (!['get', 'head', 'options'].includes(operation.method) && security.some(item => 'cookieSession' in item)) {
    assert(operation.responses['403'], `${operation.operationId}: missing Origin rejection`);
  }
  const parameters = operation.parameters?.map(dereference) ?? [];
  if (parameters.some(parameter => parameter.name === 'If-Match')) {
    assert(operation.responses['412'] && operation.responses['428'], `${operation.operationId}: missing precondition errors`);
  }
  for (const [status, response] of Object.entries(operation.responses)) {
    if (status === '202') {
      const accepted = dereference(response);
      assert(accepted.headers?.Location, `${operation.operationId}: missing Location`);
      for (const media of Object.values(accepted.content ?? {})) {
        const schema = dereference(media.schema);
        assert(schema?.required?.includes('retry_after_seconds'), `${operation.operationId}: missing polling delay`);
      }
    }
  }
}
for (const id of ['requestSyncRun', 'refreshSeries', 'requestDocumentationCheck']) {
  const accepted = byId[id].responses['202'];
  assert.equal(accepted.content['application/json'].schema.$ref, '#/components/schemas/BackgroundOperation');
  assert.match(accepted.headers.Location.example, /^\/api\/v1\/operations\//);
}
assert(byId.getBackgroundOperation.responses['404']);
const targetParameters = byId.listTargetVersions.parameters.map(dereference);
assert(targetParameters.some(parameter => parameter.name === 'size_label'));
assert(byId.listTargetVersions.responses['200'].headers.ETag);
const targetExample = byId.listTargetVersions.responses['200'].content['application/json'].example;
assert.equal(byId.listTargetVersions.responses['200'].headers.ETag.example,
  `"${Math.max(...targetExample.data.map(version => version.version_no))}"`, 'Target ETag example must match the latest version');
assert.deepEqual(contract.components.schemas.ExceptionDecision.properties.decision.enum, ['approve', 'reject']);
assert.equal(byId.decideException['x-craftpulse-action'], 'exceptions.review');
assert(contract.info.description.includes('required') && contract.info.description.includes('redactions'));

for (const id of ['createCalendarVersion', 'createTargetVersion', 'createCheckProfileVersion', 'createDodVersion', 'createCheckPolicyVersion']) {
  assert(byId[id].responses['200'] && byId[id].responses['201'], `${id}: no unchanged-content response`);
}
const returnTo = byId.startLogin.parameters.find(parameter => parameter.name === 'return_to');
const returnToPattern = new RegExp(returnTo.schema.pattern);
for (const path of ['/', '/projects?view=team', '/tasks/123#history']) assert(returnToPattern.test(path));
for (const path of ['//evil.example', '/\\evil.example', '/x\\y', '/x\ny', 'https://evil.example']) {
  assert(!returnToPattern.test(path), `Unsafe return_to accepted: ${JSON.stringify(path)}`);
}
console.log(`Contract invariants passed: ${operations.length} operations, UC-01..48 and FR-001..064 mapped.`);

// Examples must obey access and metric semantics in addition to JSON Schema types.
const dataStates = new Set(contract.components.schemas.DataState.enum);
function checkExample(value) {
  if (!value || typeof value !== 'object') return;
  if (Array.isArray(value.redactions)) {
    for (const field of value.redactions) {
      assert(/^[a-z][a-z0-9_]*$/.test(field), 'Redactions must not expose array indices');
      assert(!(field in value), `Redacted field ${field} is still present`);
    }
  }
  if (dataStates.has(value.state) && 'value' in value && 'unit' in value) {
    if (['UNKNOWN', 'NOT_EVALUATED', 'NOT_APPLICABLE', 'BOUNDED'].includes(value.state)) {
      assert.equal(value.value, null, `${value.state} must not contain an exact value`);
    }
    if (value.state === 'BOUNDED') assert(value.bound, 'BOUNDED needs a bound');
    if (['ASSESSED', 'PROVISIONAL'].includes(value.state) && value.unit === 'ratio') {
      assert(value.denominator > 0, 'An assessed ratio needs a positive denominator');
    }
  }
  if ('retry_after_seconds' in value && ['pending', 'quarantined', 'queued', 'running'].includes(value.status)) {
    assert(Number.isInteger(value.retry_after_seconds) && value.retry_after_seconds >= 1, 'Active operation needs a polling delay');
  }
  for (const child of Object.values(value)) checkExample(child);
}
for (const operation of operations) for (const response of Object.values(operation.responses)) {
  for (const media of Object.values(dereference(response).content ?? {})) {
    if (media.example) checkExample(media.example);
    for (const example of Object.values(media.examples ?? {})) if ('value' in example) checkExample(example.value);
  }
}
const summarySchema = contract.components.schemas.MergeRequestSummary;
assert(summarySchema.required.includes('access_limited') && summarySchema.required.includes('access_notes'));
assert.equal(contract.components.schemas.Redactions.items.pattern, '^[a-z][a-z0-9_]*$');
console.log('Access, polling and metric examples passed.');
