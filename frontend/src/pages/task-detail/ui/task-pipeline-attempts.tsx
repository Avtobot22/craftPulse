import { Button } from '@gravity-ui/uikit'
import { useInfiniteQuery } from '@tanstack/react-query'
import { SourceLink } from '@/entities/activity'
import { MetricValue } from '@/entities/metric'
import { pipelineAttemptPages } from '@/shared/api'
import type { ApiClient, components, OpenCodes } from '@/shared/api'
import { contractLabel, contractLabels } from '@/shared/config'
import { ErrorState, LoadingState } from '@/shared/ui'

type Attempt = OpenCodes<components['schemas']['Attempt']>

function groupAttempts(attempts: Attempt[]) {
  const jobs = new Map<string, Map<string, Attempt>>()
  for (const attempt of attempts) {
    const key = JSON.stringify([attempt.job_key, attempt.config_fingerprint])
    const job = jobs.get(key) ?? new Map<string, Attempt>()
    job.set(attempt.id, attempt)
    jobs.set(key, job)
  }
  return [...jobs].map(([key, attempts]) => ({
    key,
    attempts: [...attempts.values()].sort(
      (a, b) => a.attempt_no - b.attempt_no,
    ),
  }))
}

function JobAttempts({ attempts }: { attempts: Attempt[] }) {
  const job = attempts[0]
  if (!job) return null
  const failedAt = attempts.findIndex(({ outcome }) => outcome === 'failed')
  const passedAfterFailure =
    failedAt >= 0 &&
    attempts.slice(failedAt + 1).some(({ outcome }) => outcome === 'success')
  return (
    <li>
      <h5>
        <code>{job.job_key}</code>
      </h5>
      <p>
        Отпечаток конфигурации <code>{job.config_fingerprint}</code>
      </p>
      {passedAfterFailure && <p>Сбой → успех после повтора</p>}
      <ul>
        {attempts.map((attempt) => (
          <li key={attempt.id}>
            <SourceLink url={attempt.source_url}>
              Попытка {attempt.attempt_no} · {job.job_key}
            </SourceLink>
            {' · '}
            {contractLabel(contractLabels.ciOutcome, attempt.outcome)}
            {' · '}
            {attempt.duration_seconds === null
              ? 'Длительность неизвестна'
              : `${String(attempt.duration_seconds)} с`}
            {attempt.outcome === 'failed' && (
              <>
                {' · '}
                {attempt.failure_kind
                  ? contractLabel(
                      contractLabels.failureKind,
                      attempt.failure_kind,
                    )
                  : 'Причина неизвестна'}
                {attempt.allow_failure && ' · Разрешённое падение'}
              </>
            )}
          </li>
        ))}
      </ul>
    </li>
  )
}

export function TaskPipelineAttempts({
  api,
  pipelineId,
}: {
  api: ApiClient
  pipelineId: string
}) {
  const attempts = useInfiniteQuery(pipelineAttemptPages(api, pipelineId))
  const jobs = groupAttempts(
    attempts.data?.pages.flatMap(({ data }) => data) ?? [],
  )
  const usage = attempts.data?.pages[0]?.usage_seconds
  return (
    <>
      <h4>Попытки CI</h4>
      {attempts.isPending && <LoadingState />}
      {attempts.isError && (
        <ErrorState
          reason="Не удалось загрузить попытки CI"
          onRetry={() => {
            if (attempts.isFetchNextPageError)
              void attempts.fetchNextPage({ cancelRefetch: false })
            else void attempts.refetch()
          }}
          retrying={attempts.isFetching}
        />
      )}
      {usage && (
        <div>
          <p>Время CI с учётом повторов</p>
          <MetricValue value={usage} />
        </div>
      )}
      {jobs.length > 0 && (
        <ul className="cp-task-detail__jobs">
          {jobs.map(({ key, attempts }) => (
            <JobAttempts key={key} attempts={attempts} />
          ))}
        </ul>
      )}
      {attempts.isSuccess && jobs.length === 0 && !attempts.hasNextPage && (
        <p>Нет данных о попытках.</p>
      )}
      {attempts.hasNextPage && !attempts.isFetchNextPageError && (
        <Button
          view="outlined"
          disabled={attempts.isFetching}
          onClick={() => void attempts.fetchNextPage({ cancelRefetch: false })}
        >
          Загрузить ещё попытки
        </Button>
      )}
    </>
  )
}
