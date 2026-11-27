import type { components } from '../src/shared/api'

type Schema = components['schemas']
export const pipelineId = '0193a5b0-d1d1-7000-8000-000000000001'
export const jobKey = 'test: matrix/build'
export const linux = 'a'.repeat(64)
export const windows = 'b'.repeat(64)

export const noReports: Schema['Pipeline'] = {
  id: pipelineId,
  ref_name: 'feature/report-move',
  sha: '9f2c1e0d8b7a6f5e4d3c2b1a0f9e8d7c6b5a4f3e',
  merge_request_id: '0193a5b0-b0b0-7000-8000-000000000001',
  outcome: 'success',
  native_status: 'passed',
  started_at: '2026-11-08T10:50:00Z',
  finished_at: '2026-11-08T11:01:00Z',
  attempts_total: 0,
  retried_jobs: 0,
  allowed_failures: 0,
  attempt_history: 'complete',
  reports: [],
  tests: {
    state: 'NOT_EVALUATED',
    passed: null,
    failed: null,
    error: null,
    skipped: null,
  },
  source_url: 'https://gitlab.example/core/-/pipelines/7781',
}

const coverage: Schema['MetricValue'] = {
  state: 'ASSESSED',
  value: 0.5,
  unit: 'ratio',
  numerator: 2,
  denominator: 4,
  n: 4,
  bound: null,
  low_sample: false,
  excluded_count: 0,
  missing_count: 0,
  reason_code: null,
}

export const v28: Schema['Pipeline'] = {
  ...noReports,
  attempts_total: 3,
  retried_jobs: 1,
  reports: [
    { analysis_kind: 'junit', state: 'received' },
    { analysis_kind: 'coverage', state: 'received' },
  ],
  tests: {
    state: 'ASSESSED',
    passed: 2,
    failed: 1,
    error: 1,
    skipped: 1,
    distinct_cases: 4,
    executions: 5,
  },
  coverage: {
    line: coverage,
    branch: { ...coverage, value: 0.25, numerator: 1 },
  },
}

const first: Schema['Attempt'] = {
  id: '0193a5b0-d2d2-7000-8000-000000000001',
  job_key: jobKey,
  config_fingerprint: linux,
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
}

export const v28Attempts: Schema['AttemptPage'] = {
  // The API cursor sorts by ID. The view orders each job by attempt_no.
  data: [
    {
      ...first,
      id: '0193a5b0-d2d2-7000-8000-000000000000',
      attempt_no: 2,
      outcome: 'success',
      native_status: 'passed',
      duration_seconds: 30,
      started_at: '2026-11-08T10:53:00Z',
      finished_at: '2026-11-08T10:53:30Z',
      source_url: 'https://gitlab.example/core/-/jobs/99107',
    },
    first,
    {
      ...first,
      id: '0193a5b0-d2d2-7000-8000-000000000003',
      config_fingerprint: windows,
      outcome: 'success',
      native_status: 'passed',
      runner_class: 'windows-medium',
      duration_seconds: 40,
      finished_at: '2026-11-08T10:51:40Z',
      source_url: 'https://gitlab.example/core/-/jobs/99109',
    },
  ],
  next_cursor: null,
  usage_seconds: {
    ...coverage,
    unit: 'seconds',
    value: 130,
    numerator: null,
    denominator: null,
    n: 3,
    low_sample: true,
  },
}

export const emptyAttempts: Schema['AttemptPage'] = {
  ...v28Attempts,
  data: [],
  usage_seconds: { ...v28Attempts.usage_seconds, value: 0, n: 0 },
}

export const executionRows: Schema['TestExecution'][] = v28Attempts.data
  .filter(({ attempt_no }) => attempt_no === 1)
  .map(({ id, config_fingerprint, outcome }) => ({
    test_case: {
      id: '0193a5b0-d3d3-7000-8000-000000000001',
      key: 'billing.TestExport/csv',
    },
    config_fingerprint,
    attempt_id: id,
    outcome: outcome === 'success' ? 'passed' : 'failed',
    duration_ms: 1840,
  }))
