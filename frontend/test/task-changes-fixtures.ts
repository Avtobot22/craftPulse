import type { components } from '../src/shared/api'
import { currentUser } from './fixtures'
import {
  mrReference,
  pipelineReference,
  readyAt,
  taskDetail,
  taskId,
} from './task-fixtures'

type Schema = components['schemas']

export const manager = {
  ...currentUser,
  actions: [...currentUser.actions, 'links.decide'],
  roles: [
    {
      role: 'project_manager',
      project_id: taskDetail.project_id,
      team_id: null,
      service_id: null,
      assigned_at: '2026-10-01T09:00:00Z',
    },
  ],
} satisfies Schema['CurrentUser']

export const candidate: Schema['Link'] = {
  id: '0193a5b0-c0c0-7000-8000-000000000002',
  link_type: 'task_mr',
  method: 'branch_key',
  status: 'candidate',
  from_object_id: taskId,
  to: mrReference,
  decided_at: null,
}

export const confirmed: Schema['Link'] = {
  ...candidate,
  id: '0193a5b0-c0c0-7000-8000-000000000003',
  method: 'user_confirmed',
  status: 'confirmed',
  decided_at: '2026-11-08T11:40:00Z',
}

export const mergeRequest = {
  id: mrReference.id,
  iid: 42,
  title: mrReference.title,
  state: 'open',
  draft: false,
  area: {
    id: '0193a5b0-8888-7000-8000-000000000001',
    kind: 'repository',
    display_name: 'platform/core',
    connection_id: '0193a5b0-5555-7000-8000-000000000002',
  },
  author: { account_id: null, kind: 'bot', display_name: 'Build Bot' },
  head_sha: '9f2c1e0d8b7a6f5e4d3c2b1a0f9e8d7c6b5a4f3e',
  opened_at: '2026-11-05T10:00:00Z',
  ready_at: readyAt,
  review_wait: {
    kind: 'unknown',
    since: readyAt,
    wait_seconds: 165600,
    fallback_open: false,
  },
  source_url: mrReference.source_url,
} satisfies Schema['MergeRequest']

export const runLink: Schema['Link'] = {
  ...confirmed,
  id: '0193a5b0-c0c0-7000-8000-000000000004',
  link_type: 'mr_run',
  from_object_id: mrReference.id,
  method: 'source_explicit',
  to: { ...pipelineReference, source_url: null },
  decided_at: null,
}

export const pipeline = {
  id: pipelineReference.id,
  ref_name: 'feature/CORE-15',
  sha: mergeRequest.head_sha,
  merge_request_id: mrReference.id,
  outcome: 'success',
  native_status: 'success',
  started_at: '2026-11-08T10:50:00Z',
  finished_at: '2026-11-08T11:01:00Z',
  attempts_total: 3,
  retried_jobs: 1,
  allowed_failures: 1,
  attempt_history: 'truncated',
  reports: [{ analysis_kind: 'coverage', state: 'missing' }],
  tests: {
    state: 'NOT_EVALUATED',
    passed: null,
    failed: null,
    error: null,
    skipped: null,
  },
  source_url: pipelineReference.source_url,
} satisfies Schema['Pipeline']

export const failedAttempt = {
  id: '0193a5b0-d2d2-7000-8000-000000000001',
  job_key: 'test-linux',
  config_fingerprint: 'a'.repeat(64),
  attempt_no: 1,
  outcome: 'failed',
  native_status: 'failed',
  allow_failure: false,
  failure_kind: null,
  runner_class: 'linux-medium',
  started_at: '2026-11-08T10:51:00Z',
  finished_at: '2026-11-08T10:52:00Z',
  duration_seconds: 60,
  source_url: 'https://gitlab.example/core/-/jobs/99101',
} satisfies Schema['Attempt']

export const successfulRetry = {
  ...failedAttempt,
  id: '0193a5b0-d2d2-7000-8000-000000000002',
  attempt_no: 2,
  outcome: 'success',
  native_status: 'success',
  duration_seconds: 30,
  source_url: 'https://gitlab.example/core/-/jobs/99107',
} satisfies Schema['Attempt']

export const otherConfiguration = {
  ...successfulRetry,
  id: '0193a5b0-d2d2-7000-8000-000000000003',
  config_fingerprint: 'b'.repeat(64),
  attempt_no: 1,
  source_url: 'https://gitlab.example/core/-/jobs/99108',
} satisfies Schema['Attempt']

export const usageSeconds = {
  state: 'BOUNDED',
  value: null,
  unit: 'seconds',
  numerator: null,
  denominator: null,
  n: 3,
  bound: { kind: 'lower', value: 130 },
  low_sample: true,
  excluded_count: 0,
  missing_count: 0,
  reason_code: 'history_incomplete',
} satisfies Schema['MetricValue']
