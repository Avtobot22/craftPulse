import { useInfiniteQuery } from '@tanstack/react-query'
import { generatePath } from 'react-router'
import { pipelineAttemptPages } from '@/shared/api'
import type { ApiClient, components, OpenCodes } from '@/shared/api'
import { contractLabel, contractLabels, routePaths } from '@/shared/config'
import { ErrorState, InspectionLink, LoadingState } from '@/shared/ui'

type CI = NonNullable<
  OpenCodes<components['schemas']['MergeRequestSummary']['ci']>
>
type Attempt = OpenCodes<components['schemas']['Attempt']>

function findSuccessfulRetries(attempts: Attempt[]) {
  const jobs = new Map<string, Attempt[]>()
  for (const attempt of attempts) {
    const key = JSON.stringify([attempt.job_key, attempt.config_fingerprint])
    const job = jobs.get(key)
    if (job) job.push(attempt)
    else jobs.set(key, [attempt])
  }

  return [...jobs.entries()].flatMap(([key, job]) => {
    const firstFailure = job
      .filter(({ outcome }) => outcome === 'failed')
      .sort((a, b) => a.attempt_no - b.attempt_no)[0]
    if (!firstFailure) return []
    const nextSuccess = job
      .filter(
        ({ outcome, attempt_no }) =>
          outcome === 'success' && attempt_no > firstFailure.attempt_no,
      )
      .sort((a, b) => a.attempt_no - b.attempt_no)[0]
    return nextSuccess ? [{ key, firstFailure, nextSuccess }] : []
  })
}

function RetryOutcomes({
  api,
  pipelineId,
}: {
  api: ApiClient
  pipelineId: string
}) {
  const query = useInfiniteQuery(pipelineAttemptPages(api, pipelineId))
  const retries = findSuccessfulRetries(
    query.data?.pages.flatMap(({ data }) => data) ?? [],
  )

  return (
    <div>
      <p>Повторные попытки по данным pipeline</p>
      {query.isPending && <LoadingState />}
      {query.isError && (
        <ErrorState
          reason="Не удалось получить исходы попыток"
          retrying={query.isFetching}
          onRetry={() => {
            if (query.isFetchNextPageError) void query.fetchNextPage()
            else void query.refetch()
          }}
        />
      )}
      {retries.length > 0 && (
        <ul>
          {retries.map(({ key, firstFailure, nextSuccess }) => (
            <li key={key}>
              <code>{firstFailure.job_key}</code> · Попытка{' '}
              {firstFailure.attempt_no} завершилась ошибкой → попытка{' '}
              {nextSuccess.attempt_no} прошла
            </li>
          ))}
        </ul>
      )}
      {query.isSuccess && !query.hasNextPage && retries.length === 0 && (
        <p>
          Смена исхода с ошибки на успех не подтверждена доступными попытками.
        </p>
      )}
      {query.hasNextPage && !query.isFetchNextPageError && (
        <button
          type="button"
          disabled={query.isFetching}
          onClick={() => void query.fetchNextPage()}
        >
          Загрузить ещё попытки
        </button>
      )}
    </div>
  )
}

export function SummaryAttempts({
  api,
  ci,
  canReadCI,
}: {
  api: ApiClient
  ci: CI | null
  canReadCI: boolean
}) {
  if (!ci)
    return (
      <section>
        <h3>Попытки CI</h3>
        <p>Нет данных о запуске CI.</p>
      </section>
    )

  return (
    <section>
      <h3>Попытки CI</h3>
      <p>
        Исход CI {contractLabel(contractLabels.ciOutcome, ci.outcome)}. Попыток{' '}
        {ci.attempts}, заданий с повтором {ci.retried_jobs}
      </p>
      {canReadCI && (
        <>
          <InspectionLink
            id="summary-pipeline"
            to={generatePath(routePaths.pipeline, {
              pipelineId: ci.pipeline_id,
            })}
          >
            Открыть pipeline
          </InspectionLink>
          {ci.retried_jobs > 0 && (
            <RetryOutcomes api={api} pipelineId={ci.pipeline_id} />
          )}
        </>
      )}
    </section>
  )
}
