import { Card, Text } from '@gravity-ui/uikit'
import { useSearchParams } from 'react-router'
import { contractLabel, contractLabels } from '@/shared/config'
import { InspectionLink, useInspectionReturn } from '@/shared/ui'
import {
  attemptUsage,
  configParam,
  groupJobs,
  isAccessError,
  jobPath,
} from '../model/pipeline'
import type { Attempt, Pipeline } from '../model/pipeline'
import type { usePipelineAttempts } from '../model/use-pipeline-attempts'
import { PipelineAttemptList } from './pipeline-attempt-list'
import { AttemptTotals } from './pipeline-overview'
import { PipelineTable } from './pipeline-table'

export function PipelineJobs({
  pipeline,
  query,
}: {
  pipeline: Pipeline
  query: ReturnType<typeof usePipelineAttempts>
}) {
  const pages = isAccessError(query.error) ? undefined : query.data?.pages
  const jobs = groupJobs(pages?.flatMap(({ data }) => data) ?? [])
  const attempts = jobs.flat()
  const retries = attempts.filter(({ attempt_no }) => attempt_no > 1)
  const partial =
    query.hasNextPage ||
    pipeline.attempt_history !== 'complete' ||
    attempts.length < pipeline.attempts_total
  useInspectionReturn(pages !== undefined)

  return (
    <Card className="cp-ci-panel">
      <Text as="h2" variant="subheader-2">
        Задания и повторы
      </Text>
      <AttemptTotals
        pipeline={pipeline}
        usage={pages?.[0]?.usage_seconds}
        {...(pages ? { retryUsage: attemptUsage(retries, partial) } : {})}
      />
      <PipelineAttemptList query={query}>
        {({ hasNextPage }) =>
          jobs.length > 0 ? (
            <PipelineJobTable
              pipelineId={pipeline.id}
              jobs={jobs}
              partial={partial}
            />
          ) : (
            <p>
              {hasNextPage
                ? 'Среди загруженных попыток пока нет заданий.'
                : 'Нет данных о заданиях CI.'}
            </p>
          )
        }
      </PipelineAttemptList>
    </Card>
  )
}

function PipelineJobTable({
  pipelineId,
  jobs,
  partial,
}: {
  pipelineId: string
  jobs: Attempt[][]
  partial: boolean
}) {
  const [params] = useSearchParams()
  const rows = jobs.flatMap((job) => {
    const first = job[0]
    const last = job.at(-1)
    if (!first || !last) return []
    const search = new URLSearchParams(params)
    search.set(configParam, first.config_fingerprint)
    const firstFailure = job.find(({ outcome }) => outcome === 'failed')
    const passedAfterFailure =
      firstFailure &&
      job.some(
        ({ outcome, attempt_no }) =>
          outcome === 'success' && attempt_no > firstFailure.attempt_no,
      )
    const allowedFailure = last.outcome === 'failed' && last.allow_failure
    return [
      {
        key: first.id,
        cells: [
          <InspectionLink
            key="job"
            id={`pipeline-job-${first.id}`}
            to={{
              pathname: jobPath(pipelineId, first.job_key),
              search: search.toString(),
            }}
          >
            {first.job_key}
          </InspectionLink>,
          <code key="config">{first.config_fingerprint}</code>,
          first.attempt_no === 1
            ? contractLabel(contractLabels.ciOutcome, first.outcome)
            : 'Неизвестен',
          contractLabel(contractLabels.ciOutcome, last.outcome),
          <>
            {passedAfterFailure && <div>Сбой → успех после повтора</div>}
            {allowedFailure && <div>Разрешённое падение</div>}
            {!passedAfterFailure && !allowedFailure && 'Нет'}
          </>,
        ],
      },
    ]
  })
  return (
    <PipelineTable
      label="Задания CI"
      columns={[
        'Задание',
        'Конфигурация',
        'Первый исход',
        partial ? 'Последний известный исход' : 'Итоговый исход',
        'Оговорки',
      ]}
      rows={rows}
    />
  )
}
