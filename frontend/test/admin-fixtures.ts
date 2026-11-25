import type { components } from '@/shared/api'

// Examples of successful reads from docs/technical-specification/openapi.yaml.
export const connectionExample = {
  id: '0193a5b0-5555-7000-8000-000000000001',
  name: 'Jira основной',
  provider: 'jira_cloud',
  instance: {
    id: '0193a5b0-aaaa-7000-8000-000000000001',
    base_url: 'https://example.atlassian.net',
    edition: 'cloud',
    source_version: '1001.0.0-SNAPSHOT',
  },
  status: 'active',
  adapter_kind: 'builtin',
  access_mode: 'service_account',
  credentials_set_at: '2026-10-01T09:00:00Z',
  sync_paused: false,
  paused_until: null,
  last_success_at: '2026-11-08T03:12:00Z',
  scopes: [
    {
      id: '0193a5b0-5656-7000-8000-000000000001',
      kind: 'tracker_project',
      native_scope_id: '10200',
      display_name: 'CORE',
      area_id: '0193a5b0-8888-7000-8000-000000000002',
      history_from: '2026-01-01T00:00:00Z',
    },
  ],
  capabilities: [
    {
      capability: 'read_current',
      state: 'verified',
      reason_code: null,
      checked_at: '2026-10-01T09:05:00Z',
    },
    {
      capability: 'write_tracker_issue',
      state: 'missing',
      reason_code: 'permission_missing',
      checked_at: '2026-10-01T09:05:00Z',
    },
  ],
} satisfies components['schemas']['Connection']

export const probeExample = {
  id: '0193a5b0-5757-7000-8000-000000000001',
  status: 'succeeded',
  started_at: '2026-11-08T10:01:00Z',
  finished_at: '2026-11-08T10:01:14Z',
  edition: 'cloud',
  source_version: '1001.0.0-SNAPSHOT',
  contract_version: '1',
  capabilities: [
    {
      capability: 'read_current',
      state: 'verified',
      reason_code: null,
      checked_at: '2026-11-08T10:01:14Z',
    },
    {
      capability: 'read_status_history',
      state: 'verified',
      reason_code: null,
      checked_at: '2026-11-08T10:01:14Z',
    },
    {
      capability: 'write_tracker_issue',
      state: 'missing',
      reason_code: 'permission_missing',
      checked_at: '2026-11-08T10:01:14Z',
    },
  ],
  reason_code: null,
  retry_after_seconds: null,
  configuration_version: 1,
} satisfies components['schemas']['ConnectionProbe']

export const syncStatusExample = {
  connection_id: '0193a5b0-5555-7000-8000-000000000001',
  connection_status: 'active',
  paused_until: null,
  data_kinds: [
    {
      data_kind: 'tasks',
      phase: 'incremental',
      last_attempt_at: '2026-11-08T03:00:00Z',
      last_outcome: 'error',
      last_error_code: 'timeout',
      failed_page_present: true,
      last_success_at: '2026-11-07T03:12:00Z',
      watermark: '2026-11-07T03:00:00Z',
      lag_seconds: 111600,
      history_from: '2026-01-01T00:00:00Z',
      completeness: 'partial',
      unknown_fields: ['customfield_10031'],
      affected_metrics: ['T08', 'T12'],
    },
  ],
  last_reconcile: {
    started_at: '2026-11-07T02:00:00Z',
    finished_at: '2026-11-07T02:04:00Z',
    outcome: 'complete',
    expected_count: 1240,
    observed_count: 1240,
    missing_count: 0,
    changed_count: 3,
  },
} satisfies components['schemas']['SyncStatus']

export const receiptExample = {
  id: '0193a5b0-5858-7000-8000-000000000002',
  channel: 'import',
  status: 'partial',
  received_at: '2026-11-08T10:10:00Z',
  schema_id: 'tasks',
  schema_version: '1',
  record_count: 2,
  accepted_count: 1,
  rejected_count: 1,
  replay_count: 0,
  conflict_count: 0,
  reason_code: null,
  duplicate_of: null,
  affected_metrics: ['T04', 'T05'],
  retry_after_seconds: null,
  result_url: '/teams/0193a5b0-3333-7000-8000-000000000001/report',
} satisfies components['schemas']['Receipt']
