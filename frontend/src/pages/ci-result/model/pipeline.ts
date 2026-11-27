import { generatePath } from 'react-router'
import { ApiError } from '@/shared/api'
import type {
  components,
  OpenCodes,
  TestExecutionListParams,
} from '@/shared/api'
import { routePaths } from '@/shared/config'

type Schema = components['schemas']
export type Pipeline = OpenCodes<Schema['Pipeline']>
export type Attempt = OpenCodes<Schema['Attempt']>
export type Usage = OpenCodes<Schema['MetricValue']>
export const configParam = 'config_fingerprint'
export const testOutcomeParam = 'test_outcome'

export const testOutcomeLabels = {
  passed: 'Прошли',
  failed: 'Упали',
  error: 'Ошибки',
  skipped: 'Пропущены',
} satisfies Record<NonNullable<TestExecutionListParams['outcome']>, string>

export function testOutcome(value: string | null) {
  switch (value) {
    case 'passed':
    case 'failed':
    case 'error':
    case 'skipped':
      return value
    case null:
    default:
      return undefined
  }
}

export function pipelinePath(pipelineId: string) {
  return generatePath(routePaths.pipeline, {
    pipelineId,
  })
}

export function jobPath(pipelineId: string, jobKey: string) {
  return generatePath(routePaths.pipelineJob, {
    pipelineId,
    jobKey,
  })
}

export function isAccessError(error: Error | null) {
  return (
    error instanceof ApiError &&
    (error.kind === 'not-found' ||
      error.kind === 'forbidden' ||
      error.kind === 'auth-required')
  )
}

export function groupJobs(attempts: readonly Attempt[]) {
  const jobs = new Map<string, Attempt[]>()
  const byId = new Map(attempts.map((attempt) => [attempt.id, attempt]))
  for (const attempt of byId.values()) {
    const key = JSON.stringify([attempt.job_key, attempt.config_fingerprint])
    const job = jobs.get(key)
    if (job) job.push(attempt)
    else jobs.set(key, [attempt])
  }
  return [...jobs.values()].map((job) =>
    job.sort((a, b) => a.attempt_no - b.attempt_no),
  )
}

export function attemptUsage(
  attempts: readonly Attempt[],
  incomplete: boolean,
) {
  const known = attempts.flatMap(({ duration_seconds }) =>
    duration_seconds === null ? [] : [duration_seconds],
  )
  const seconds = known.reduce((sum, value) => sum + value, 0)
  return secondsText(seconds, incomplete || known.length < attempts.length)
}

export function secondsText(seconds: number, lowerBound = false) {
  return `${lowerBound ? 'Не менее ' : ''}${String(seconds)} с`
}
