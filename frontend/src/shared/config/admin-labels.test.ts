import { expect, it } from 'vitest'
import { adminLabels, providerLabel } from './admin-labels'
import { contractLabel } from './contract-labels'

it.each(Object.entries(adminLabels))(
  'has visible labels for every code in %s',
  (_category, labels) => {
    for (const [code, label] of Object.entries(labels)) {
      expect(contractLabel(labels, code)).toBe(label)
      expect(label.trim()).not.toBe('')
      expect(label).not.toBe(code)
    }
  },
)

it.each([
  [
    'probeReason',
    [
      'edition_mismatch',
      'auth_failed',
      'outside_network_scope',
      'certificate_untrusted',
      'timeout',
      'adapter_unavailable',
      'contract_unsupported',
      'configuration_changed',
    ],
  ],
  [
    'capabilityReason',
    [
      'not_checked',
      'permission_missing',
      'no_repository',
      'create_issues_missing',
      'write_permission_missing',
    ],
  ],
  [
    'syncError',
    [
      'unauthorized',
      'forbidden',
      'rate_limited',
      'unavailable',
      'timeout',
      'adapter_unavailable',
      'invalid_response',
    ],
  ],
  [
    'receiptReason',
    [
      'schema_unsupported',
      'envelope_invalid',
      'path_unsafe',
      'limit_exceeded',
      'outside_scope',
    ],
  ],
  [
    'recordReason',
    [
      'missing_source_id',
      'outside_scope',
      'outside_channel',
      'invalid_record',
      'missing_event_time',
      'kind_not_allowed',
      'unsupported_payload_schema',
      'class_not_allowed',
      'not_snapshot',
      'missing_read_id',
      'schema_violation',
      'missing_file',
      'file_mismatch',
    ],
  ],
] as const)('recognizes the known %s codes', (category, codes) => {
  for (const code of codes) expect(adminLabels[category]).toHaveProperty(code)
})

it('labels a new provider as another source', () => {
  expect(providerLabel('gitlab')).toBe('GitLab')
  expect(providerLabel('constructor')).toBe('другой источник')
})
