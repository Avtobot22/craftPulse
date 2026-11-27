import { infiniteQueryOptions, queryOptions } from '@tanstack/react-query'
import type { ApiClient } from './client'
import type { components, operations } from './generated/schema'
import type { OpenCodes } from './read-fields'
import { readMetricValue, readNullableNumber, readNumber } from './read-fields'
import {
  readArray,
  readBoolean,
  readNullableString,
  readRecord,
  readString,
} from './response'

type Schema = components['schemas']
export type PipelineAttemptsParams = NonNullable<
  operations['listPipelineAttempts']['parameters']['query']
>

function readPipeline(value: unknown): OpenCodes<Schema['Pipeline']> {
  const raw = readRecord(value)
  const tests = readRecord(raw.tests)
  const coverage =
    raw.coverage === undefined ? undefined : readRecord(raw.coverage)
  return {
    id: readString(raw.id),
    ref_name: readString(raw.ref_name),
    sha: readString(raw.sha),
    merge_request_id: readNullableString(raw.merge_request_id),
    outcome: readString(raw.outcome),
    native_status: readString(raw.native_status),
    started_at: readNullableString(raw.started_at),
    finished_at: readNullableString(raw.finished_at),
    attempts_total: readNumber(raw.attempts_total),
    retried_jobs: readNumber(raw.retried_jobs),
    allowed_failures: readNumber(raw.allowed_failures),
    attempt_history: readString(raw.attempt_history),
    reports: readArray(raw.reports, (value) => {
      const report = readRecord(value)
      return {
        analysis_kind: readString(report.analysis_kind),
        state: readString(report.state),
      }
    }),
    tests: {
      state: readString(tests.state),
      passed: readNullableNumber(tests.passed),
      failed: readNullableNumber(tests.failed),
      error: readNullableNumber(tests.error),
      skipped: readNullableNumber(tests.skipped),
      ...(tests.distinct_cases === undefined
        ? {}
        : { distinct_cases: readNullableNumber(tests.distinct_cases) }),
      ...(tests.executions === undefined
        ? {}
        : { executions: readNullableNumber(tests.executions) }),
    },
    ...(coverage === undefined
      ? {}
      : {
          coverage: {
            line: readMetricValue(coverage.line),
            branch: readMetricValue(coverage.branch),
          },
        }),
    source_url: readNullableString(raw.source_url),
  }
}

function readAttempt(value: unknown): OpenCodes<Schema['Attempt']> {
  const raw = readRecord(value)
  return {
    id: readString(raw.id),
    job_key: readString(raw.job_key),
    config_fingerprint: readString(raw.config_fingerprint),
    attempt_no: readNumber(raw.attempt_no),
    outcome: readString(raw.outcome),
    native_status: readString(raw.native_status),
    allow_failure: readBoolean(raw.allow_failure),
    failure_kind: readNullableString(raw.failure_kind),
    runner_class: readNullableString(raw.runner_class),
    started_at: readNullableString(raw.started_at),
    finished_at: readNullableString(raw.finished_at),
    duration_seconds: readNullableNumber(raw.duration_seconds),
    source_url: readNullableString(raw.source_url),
  }
}

function readAttemptPage(value: unknown): OpenCodes<Schema['AttemptPage']> {
  const raw = readRecord(value)
  return {
    data: readArray(raw.data, readAttempt),
    next_cursor: readNullableString(raw.next_cursor),
    usage_seconds: readMetricValue(raw.usage_seconds),
  }
}

export function getPipeline(
  api: ApiClient,
  pipelineId: operations['getPipeline']['parameters']['path']['pipeline_id'],
) {
  return queryOptions({
    queryKey: ['pipelines', 'detail', pipelineId],
    queryFn: async ({ signal }) =>
      (
        await api.read('/pipelines/{pipeline_id}', readPipeline, {
          pathParams: { pipeline_id: pipelineId },
          signal,
        })
      ).data,
  })
}

export function listPipelineAttempts(
  api: ApiClient,
  pipelineId: operations['listPipelineAttempts']['parameters']['path']['pipeline_id'],
  params: PipelineAttemptsParams = {},
) {
  return queryOptions({
    queryKey: ['pipelines', 'attempts', pipelineId, params],
    queryFn: ({ signal }) =>
      readPipelineAttempts(api, pipelineId, params, signal),
  })
}

async function readPipelineAttempts(
  api: ApiClient,
  pipelineId: string,
  params: PipelineAttemptsParams,
  signal: AbortSignal,
) {
  const { data } = await api.read(
    '/pipelines/{pipeline_id}/attempts',
    readAttemptPage,
    {
      pathParams: { pipeline_id: pipelineId },
      query: params,
      signal,
    },
  )
  return data
}

export function pipelineAttemptPages(api: ApiClient, pipelineId: string) {
  return infiniteQueryOptions({
    queryKey: ['pipelines', 'attempt-pages', pipelineId],
    initialPageParam: null as string | null,
    queryFn: ({ signal, pageParam }) =>
      readPipelineAttempts(
        api,
        pipelineId,
        {
          limit: 100,
          ...(pageParam === null ? {} : { cursor: pageParam }),
        },
        signal,
      ),
    getNextPageParam: (lastPage) => lastPage.next_cursor ?? undefined,
    retry: false,
  })
}

export type TestExecutionListParams = NonNullable<
  operations['listTestExecutions']['parameters']['query']
>

function readTestExecutionPage(
  value: unknown,
): OpenCodes<Schema['TestExecutionPage']> {
  const raw = readRecord(value)
  return {
    data: readArray(raw.data, (value) => {
      const row = readRecord(value)
      const testCase = readRecord(row.test_case)
      return {
        test_case: {
          id: readString(testCase.id),
          key: readString(testCase.key),
        },
        attempt_id: readNullableString(row.attempt_id),
        config_fingerprint: readString(row.config_fingerprint),
        outcome: readString(row.outcome),
        duration_ms: readNullableNumber(row.duration_ms),
      }
    }),
    next_cursor: readNullableString(raw.next_cursor),
  }
}

export function listTestExecutions(
  api: ApiClient,
  pipelineId: operations['listTestExecutions']['parameters']['path']['pipeline_id'],
  params: TestExecutionListParams = {},
) {
  return queryOptions({
    queryKey: ['pipelines', 'test-executions', pipelineId, params],
    queryFn: ({ signal }) =>
      readTestExecutions(api, pipelineId, params, signal),
  })
}

async function readTestExecutions(
  api: ApiClient,
  pipelineId: string,
  params: TestExecutionListParams,
  signal: AbortSignal,
) {
  const { data } = await api.read(
    '/pipelines/{pipeline_id}/test-executions',
    readTestExecutionPage,
    { pathParams: { pipeline_id: pipelineId }, query: params, signal },
  )
  return data
}

export function testExecutionPages(
  api: ApiClient,
  pipelineId: string,
  outcome: TestExecutionListParams['outcome'],
) {
  return infiniteQueryOptions({
    queryKey: ['pipelines', 'test-execution-pages', pipelineId, outcome],
    initialPageParam: '',
    queryFn: ({ signal, pageParam }) =>
      readTestExecutions(
        api,
        pipelineId,
        {
          limit: 100,
          ...(outcome ? { outcome } : {}),
          ...(pageParam ? { cursor: pageParam } : {}),
        },
        signal,
      ),
    getNextPageParam: (last, _, __, cursors) =>
      last.next_cursor && !cursors.includes(last.next_cursor)
        ? last.next_cursor
        : undefined,
    retry: false,
  })
}
